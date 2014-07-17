//
//Application:bridge
//EventName:CHANNEL_HANGUP
//EventName:CHANNEL_EXECUTE_COMPLETE
//Application:bridge
//Variable_bridge_hangup_cause:NORMAL_CLEARING
//Variable_dialstatus:DONTCALL
//Channel-Call-State:HELD
//Variable_originate_disposition:CALL_REJECTED
//Hangup-Cause:CALL_REJECTED
package glivo

import (
	"fmt"
	"net/textproto"
	"strings"
)

//Representa un evento de Freeswitch
//Las claves estan Camelised
type Event struct {
	call    *Call
	Content map[string]string
}

//Permite llamar procedimiento cuando se cumpla el filro
//o bien cuando el evento cumpla las condiciones esperadas.
type HandlerEvent interface {
	//Delimita el handler a que eventos trabajar
	//retornar +true+ si es el evento esperado
	Filter(Event) bool

	//Recibe evento y reacciona a este si fue aceptado en el filtro
	Handle(Event)
}

//Crea evento apartir de el MIME leido
func EventFromMIME(call *Call, mime textproto.MIMEHeader) Event {
	return Event{call, mimeToMap(mime)}
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

	for k, handler := range call.handlerOnce {
		if handler == nil {
			continue
		}
		if handler.Filter(event) {
			handler.Handle(event)
			call.DoneActionHandle(k)
		}
	}

}

//Este permite esperar un evento determinado
//y bloquear la gorutina atraves del chan *wait*
//@todo como manejar un timeout por demora??
type WaitEventHandle struct {
	wait   chan interface{}
	filter map[string]string
}

func NewWaitEventHandle(wait chan interface{}, filter map[string]string) WaitEventHandle {
	return WaitEventHandle{wait, filter}
}

func (we WaitEventHandle) Filter(ev Event) bool {
	for fk, fv := range we.filter {
		if ev.Content[fk] != fv {
			return false
		}
	}
	return true
}

func (we WaitEventHandle) Handle(ev Event) {
	we.wait <- ev
}

//Este permite esperar un evento determinado
//y bloquear la gorutina atraves del chan *wait*
//@todo como manejar un timeout por demora??
type WaitAnyEventHandle struct {
	wait   chan interface{}
	filter []map[string]string
}

func NewWaitAnyEventHandle(wait chan interface{}, filter []map[string]string) WaitAnyEventHandle {
	return WaitAnyEventHandle{wait, filter}
}

func (we WaitAnyEventHandle) Filter(ev Event) bool {
	valid_and := true
	for _, filter := range we.filter {
		valid_and = true
		for fk, fv := range filter {
			if ev.Content[fk] != fv {
				valid_and = false
				break
			}
		}
		if valid_and {
			break
		}
	}
	return valid_and
}

func (we WaitAnyEventHandle) Handle(ev Event) {
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
	return CollectDTMFEventHandle{wait, terminator, validDigits, numDigits, new(string)}
}

func (cl CollectDTMFEventHandle) Filter(ev Event) bool {
	if ev.Content["Event-Name"] == "DTMF" {
		return true
	}

	if ev.Content["Event-Name"] == "CHANNEL_EXECUTE_COMPLETE" && ev.Content["Variable_current_application"] == "play_and_get_digits" {
		cl.wait <- *cl.collection
		*cl.collection = ""
	}

	return false
}

func (cl CollectDTMFEventHandle) Handle(ev Event) {
	switch ev.Content["Dtmf-Digit"] {
	case fmt.Sprintf("%c", cl.terminator):
		cl.wait <- *cl.collection
		*cl.collection = ""
	default:
		if strings.ContainsAny(ev.Content["Dtmf-Digit"], cl.validDigits) {
			*cl.collection += ev.Content["Dtmf-Digit"]
		} else {
			*cl.collection = ""
		}
	}

	if len(*cl.collection) == cl.numDigits {
		cl.wait <- *cl.collection
		*cl.collection = ""
	}
}
