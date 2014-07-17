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
	"sync"
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
	uuid     string
	Header   map[string]string
	Variable map[string]string

	Caller    *Channel
	replyChan *chan CommandStatus

	handlers map[string]HandlerEvent
	//Al registrar se elimina despues de
	//de haber encontrado el evento esperado
	handlerOnce    map[string]HandlerEvent
	muxHandlerOnce *sync.Mutex

	//Se encolan los eventos
	//para ser procesados por *eventDispatch*
	queueEvents chan Event

	logger *log.Logger
}

//Cantidad maxima antes de bloquear la gorutina
//aqui espero que no se retarde tanto para llenar
//la QUEUE
const CALL_MAX_QUEUE_EVENTS = 77

func NewCall(conn *net.Conn, header textproto.MIMEHeader, logger *log.Logger) *Call {

	call := &Call{
		Conn:           *conn,
		uuid:           "",
		Header:         make(map[string]string),
		Variable:       make(map[string]string),
		Caller:         &Channel{"", make(map[string]string)},
		replyChan:      nil,
		handlers:       make(map[string]HandlerEvent),
		handlerOnce:    make(map[string]HandlerEvent),
		muxHandlerOnce: &sync.Mutex{},
		queueEvents:    make(chan Event, CALL_MAX_QUEUE_EVENTS),
		logger:         logger,
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
	call.uuid = header.Get("Unique-ID")

	return call
}

func (call *Call) SetReply(rc *chan CommandStatus) {
	call.replyChan = rc
}

//Espera el reply del comando ejecutado
func (call *Call) Reply() CommandStatus {
	return <-*call.replyChan
}

//Envia al socket
func (call *Call) Write(p []byte) (int, error) {
	return call.Conn.Write(p)
}

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
		msg.WriteString(fmt.Sprintf("%s: %s\n", k, v))
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

//Cuelga la llamada
//http://wiki.freeswitch.org/wiki/Misc._Dialplan_Tools_hangup
func (call *Call) Hangup() {
	call.Execute("hangup", "", true)
	call.Reply()
	call.Caller.Close()
}

//Cierra llamada debe ser llamada siempre
func (call *Call) Close() {
	call.Conn.Close()
}

//Registra observador a un evento
func (call *Call) RegisterEventHandle(uuid string, hl HandlerEvent) {
	call.handlers[uuid] = hl
}

func (call *Call) UnregisterEventHandle(uuid string) {
	delete(call.handlers, uuid)
}

func (call *Call) AddActionHandle(uuid string, hl HandlerEvent) {
	call.muxHandlerOnce.Lock()
	defer call.muxHandlerOnce.Unlock()
	call.handlerOnce[uuid] = hl
}

func (call *Call) DoneActionHandle(uuid string) {
	call.muxHandlerOnce.Lock()
	defer call.muxHandlerOnce.Unlock()
	delete(call.handlerOnce, uuid)
}

//Bloquea gorutina esperando evento CHANNEL_ANSWER
//retorna el chan glivo.Event
func (call *Call) WaitAnswer() <-chan interface{} {
	return call.WaitExecute("wait_answer", map[string]string{"Event-Name": "CHANNEL_ANSWER"})
}

//Se un evento determinado
func (call *Call) WaitExecute(action string, filter map[string]string) chan interface{} {

	wait := make(chan interface{})
	waitEvent := NewWaitEventHandle(wait, filter)
	call.AddActionHandle(action, waitEvent)
	return wait
}

//Espera un evento y se ejecuta el handler una sola vez
func (call *Call) OnceEventHandle(action string, hl func(chan interface{}) HandlerEvent) chan interface{} {
	res := make(chan interface{})
	call.AddActionHandle(action, hl(res))
	return res
}
