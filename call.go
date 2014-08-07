//Call representa la conexion a la llamada iniciada
//o bien el acceso Outbound de Freeswitch,
//donde se puede controlar o dirigir el IVR.
//
//http://wiki.freeswitch.org/wiki/Event_Socket_Outbound
package glivo

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"net/textproto"
	"net/url"
	"strings"
	"time"
)

type Channel struct {
	uuid   string
	Header map[string]string
}

//Termina, cuelga la llamada actual
func (session *Channel) Close() {
}

//La llamada que se controla actualmente
type Call struct {
	Conn     net.Conn
	UUID     string
	Header   map[string]string
	Variable map[string]string

	Caller    *Channel
	replyChan *chan CommandStatus

	handlers           []HandlerEvent
	handlersIdx        int64
	handlersDestroyIdx int64

	//Al registrar se elimina despues de
	//de haber encontrado el evento esperado
	handlerOnce []HandlerEvent

	//Se encolan los eventos
	//para ser procesados por *eventDispatch*
	queueEvents chan Event

	logger *log.Logger

	Closed bool
}

//Cantidad maxima antes de bloquear la gorutina
//aqui espero que no se retarde tanto para llenar
//la QUEUE
const CALL_MAX_QUEUE_EVENTS = 77

func NewCall(conn *net.Conn, header textproto.MIMEHeader, logger *log.Logger) *Call {

	call := &Call{
		Conn:               *conn,
		UUID:               "",
		Header:             make(map[string]string),
		Variable:           make(map[string]string),
		Caller:             &Channel{"", make(map[string]string)},
		replyChan:          nil,
		handlers:           make([]HandlerEvent, 0),
		handlersIdx:        0,
		handlersDestroyIdx: 0,
		handlerOnce:        make([]HandlerEvent, 0),
		queueEvents:        make(chan Event, CALL_MAX_QUEUE_EVENTS),
		logger:             logger,
		Closed:             false,
	}

	for k, v := range header {
		val, err := url.QueryUnescape(v[0])
		if err != nil {
			val = v[0]
		}

		if strings.HasPrefix("Caller-", k) {
			call.Caller.Header[strings.TrimPrefix(k, "Caller-")] = val
		} else if strings.HasPrefix("variable_", k) {
			call.Variable[strings.TrimPrefix(k, "variable_")] = val
		} else {
			call.Header[k] = val
		}
	}
	call.UUID = header.Get("Unique-ID")

	return call
}

func (call *Call) SetReply(rc *chan CommandStatus) {
	call.replyChan = rc
}

func (call *Call) ReplyChan() chan CommandStatus {
	return *call.replyChan
}

//Espera el reply del comando ejecutado
func (call *Call) Reply() CommandStatus {
	return <-*call.replyChan
}

//Envia al socket
func (call *Call) Write(p []byte) (int, error) {
	return call.Conn.Write(p)
}

//Executa app
func (call *Call) Execute(app string, arg string, lock bool) {
	evlock := "false"
	if lock {
		evlock = "true"
	}
	msg := map[string]string{
		"call-command":     "execute",
		"execute-app-name": app,
		"execute-app-arg":  arg,
		"event-lock":       evlock,
	}

	call.sendMSG(msg)
}

func (call *Call) SetVar(name string, value string) {

	msg := map[string]string{
		"call-command":     "set",
		"execute-app-name": fmt.Sprintf("%s=%s", name, value),
		"event-lock":       "true",
	}

	call.sendMSG(msg)
	call.Reply()
}

func (call *Call) sendMSG(data map[string]string) {
	msg := bytes.NewBufferString("sendmsg\n")
	for k, v := range data {
		msg.WriteString(k)
		msg.WriteString(": ")
		msg.WriteString(v)
		msg.WriteString("\n")
	}
	msg.WriteString("\n\n")
	call.logger.Println("====BEGIN SendMSG\n", msg.String(), "====END SendMSG\n")
	call.Write([]byte(msg.String()))
}

//Reproduce audio
//http://wiki.freeswitch.org/wiki/Misc._Dialplan_Tools_playback
func (call *Call) Playback(url string) {
	call.SetVar("playback_delimiter", "!")
	call.Execute("playback", url, false)
}

//Contesta
//http://wiki.freeswitch.org/wiki/Misc._Dialplan_Tools_answer
func (call *Call) Answer() {
	call.Execute("answer", "", true)
	call.Reply()
}

//Cuelga la llamada, al recibir evento CHANNEL_EXECUTE_COMPLETE de hangup
//o bien respuesta del comando
//http://wiki.freeswitch.org/wiki/Misc._Dialplan_Tools_hangup
func (call *Call) Hangup() {

	call.Execute("hangup", "", true)
	select {
	case <-call.ReplyChan():
		call.Caller.Close()
		call.Close()
	case <-call.WaitEventChan(map[string]string{"Event-Name": "CHANNEL_EXECUTE_COMPLETE", "Variable_current_application": "hangup"}):
		call.Caller.Close()
		call.Close()
	case <-time.After(time.Minute * 5):
		call.Caller.Close()
		call.Close()
	}
}

//Cierra llamada debe ser llamada siempre
func (call *Call) Close() {
	call.Conn.Close()
	call.Closed = true
}

//Registra observador a un evento
func (call *Call) RegisterEventHandle(hl HandlerEvent) int64 {
	call.handlersIdx += 1
	call.handlers = append(call.handlers, hl)
	return call.handlersIdx
}

func (call *Call) UnregisterEventHandle(idx int64) {
	//https://code.google.com/p/go-wiki/wiki/SliceTricks
	call.handlersDestroyIdx += 1
	idx -= call.handlersDestroyIdx
	copy(call.handlers[idx:], call.handlers[idx+1:])
	call.handlers[len(call.handlers)-1] = nil
	call.handlers = call.handlers[:len(call.handlers)-1]
}

func (call *Call) AddActionHandle(hl HandlerEvent) {
	call.handlerOnce = append(call.handlerOnce, hl)
}

func (call *Call) DoneActionHandle(idx int) {
	//https://code.google.com/p/go-wiki/wiki/SliceTricks
	copy(call.handlerOnce[idx:], call.handlerOnce[idx+1:])
	call.handlerOnce[len(call.handlerOnce)-1] = nil
	call.handlerOnce = call.handlerOnce[:len(call.handlerOnce)-1]
}

//Bloquea gorutina esperando evento CHANNEL_ANSWER
//retorna el chan glivo.Event
func (call *Call) WaitAnswer() <-chan interface{} {
	return call.WaitEventChan(map[string]string{"Event-Name": "CHANNEL_ANSWER"})
}

//Se espera un evento determinado
func (call *Call) WaitEventChan(filter map[string]string) chan interface{} {
	wait := make(chan interface{})
	waitEvent := NewWaitEventHandle(wait, filter)
	call.AddActionHandle(waitEvent)
	return wait
}

//Espera un evento y se ejecuta el handler una sola vez
func (call *Call) OnceEventHandle(hl func(chan interface{}) HandlerEvent) chan interface{} {
	res := make(chan interface{})
	call.AddActionHandle(hl(res))
	return res
}
