package glivo

import (
	"net/textproto"
	"fmt"
	"strings"
)

//Representa un evento de Freeswitch
//Las claves estan Camelised, aun no se porque
//creo que debe ser el lector de textproto
type Event struct {
	call *Call
	Content map[string]string
}

//Permite llamar procedimiento cuando se cumpla el filro
//o bien cuando el evento cumpla las condiciones esperadas.
type HandlerEvent interface {
	//Delimita el handler a que eventos trabajar
	//retorna +true+ si es evento esperadon
	Filter(Event) bool

	//Recibe evento y reacciona a este
	Handle(Event)

}

//Crea evento apartir de el MIME leido 
func EventFromMIME(call *Call, mime textproto.MIMEHeader) Event{
	return Event{ call, mimeToMap(mime)}
}

func eventDispatch(call *Call, event Event) {

	if event.Content["Unique-Id"] != call.uuid {
		return
	}

	for _, handler := range call.handlers {
		if handler == nil {
			continue
		}
		if handler.Filter(event) {
			go handler.Handle(event)
		}
	}
	
}


//Este permite esperar un evento determinado
//y bloquear la gorutina atraves del chan *wait*
//@todo como manejar un timeout por demora??
type WaitEventHandle struct {
	wait chan Event
	filter map[string]string
}

func NewWaitEventHandle(wait chan Event, filter map[string]string) WaitEventHandle {
	return WaitEventHandle{wait, filter}
}

func (we WaitEventHandle) Filter(ev Event) bool {
	for fk,fv := range we.filter {
		if ev.Content[fk] != fv {
			return false
		}
	}
	return true
}

func (we WaitEventHandle) Handle(ev Event) {
	we.wait <- ev
}


//Recolecta marcacion
type CollectDTMFEventHandle struct {
	wait chan string

	//Caracter con que se termina deteccion
	terminator uint8 

	//los digitos que se esperan recibir
	validDigits string

	//la cantidad que se espera recibir
	numDigits int

	//donde se recopilan lo que van escribiendo
	collection *string
}

func NewCollectDTMFEventHandle(wait chan string, numDigits int, validDigits string, terminator uint8) CollectDTMFEventHandle {
	return CollectDTMFEventHandle{wait,terminator, validDigits, numDigits, new(string)}
}

func (cl CollectDTMFEventHandle) Filter(ev Event) bool {
	if ev.Content["Event-Name"] == "DTMF" {
		return true
	}

	return false
}

func (cl CollectDTMFEventHandle) Handle(ev Event) {
	switch(ev.Content["Dtmf-Digit"]) {
	case fmt.Sprintf("%c",cl.terminator):
		cl.wait <- *cl.collection
		*cl.collection = ""
	default:
		if strings.ContainsAny(ev.Content["Dtmf-Digit"], cl.validDigits) {
			*cl.collection += ev.Content["Dtmf-Digit"]
		}else{
			*cl.collection = ""
		}
	}

	if len(*cl.collection) == cl.numDigits {
		cl.wait <- *cl.collection
		*cl.collection = ""
	}
}
