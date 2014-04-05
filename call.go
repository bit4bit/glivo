//Call representa la conexion a la llamada iniciada
//o bien el acceso Outbound de Freeswitch,
//donde se puede controlar o dirigir el IVR.
//
//http://wiki.freeswitch.org/wiki/Event_Socket_Outbound
package glivo

import (
	"bytes"
	"net"
	"net/url"
	"net/textproto"
	"strings"
	"fmt"
)


type Channel struct {
	uuid string
	Header map[string]string
}

//Termina, cuelga la llamada actual
func (session *Channel) Close() {
}


//La llamada que se controla actualmente
type Call struct {
	Conn net.Conn
	uuid string
	Header map[string]string
	Variable map[string]string

	Caller *Channel
	replyChan *chan CommandStatus
	done chan bool

	handlers map[string]HandlerEvent
}


func NewCall(conn *net.Conn, header textproto.MIMEHeader) *Call {

	call := &Call{*conn, "", make(map[string]string),
		make(map[string]string),
		&Channel{"", make(map[string]string)},
		nil, make(chan bool), 
		make(map[string]HandlerEvent),
	}

	for k,v := range header {
		val, err := url.QueryUnescape(v[0])
		if err != nil {
			val = v[0]
		}

		if strings.HasPrefix("Caller-", k) {
			call.Caller.Header[strings.TrimPrefix(k, "Caller-")] = val
		}else if strings.HasPrefix("variable_", k) {
			call.Variable[strings.TrimPrefix(k, "variable_")] = val
		}else{
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
	return <- *call.replyChan
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
		"call-command" : "execute",
		"execute-app-name" : app,
		"execute-app-arg" : arg,
		"event-lock" : evlock,

	}

	call.sendMSG(msg)
}

func (call *Call) SetVar(name string, value string){

	msg := map[string]string{
		"call-command" : "set",
		"execute-app-name" : fmt.Sprintf("%s=%s", name, value),
		"event-lock" : "true",

	}
	
	call.sendMSG(msg)
	call.Reply()
}

func (call *Call) sendMSG(data map[string]string) {
	msg := bytes.NewBufferString("sendmsg\n")
	for k,v := range data {
		msg.WriteString(fmt.Sprintf("%s: %s\n", k, v))
	}
	msg.WriteString("\n\n")
	fmt.Println("SendMSG:", msg.String())
	call.Write([]byte(msg.String()))
}

//Reproduce audio
//http://wiki.freeswitch.org/wiki/Misc._Dialplan_Tools_playback
func (call *Call) Playback(url string) {
	call.SetVar("playback_delimiter","!")
	call.Execute("playback", url, false)
}

//Contesta
//http://wiki.freeswitch.org/wiki/Misc._Dialplan_Tools_answer
func (call *Call) Answer() error {
	call.Execute("answer","", true)
	call.Reply()
	return nil
}

//Cuelga la llamada
//http://wiki.freeswitch.org/wiki/Misc._Dialplan_Tools_hangup
func (call *Call) Hangup() {
	call.Execute("hangup", "", true)
	call.Reply()
	call.Caller.Close()
}



//Cierra llamada debe ser llamada siempre
func (call *Call) Close(){
	call.done <- true
}

//Registra observador a un evento
func (call *Call) RegisterEventHandle(uuid string, hl HandlerEvent) {
	call.handlers[uuid] = hl
}

func (call *Call) UnregisterEventHandle(uuid string) {
	delete(call.handlers, uuid)
}
