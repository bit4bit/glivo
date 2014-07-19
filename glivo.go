//Implemanta Respuesta para Freeswitch Outbound e Inbound
//La idea inicial, es hacer un clon de plivoframework sencillo
//pero la diferencia es que recibe todo el mensaje o IVR
//y lo ejecuta
//
//BUG() Para terminar el servidor se corta la conexion lo mismo que para los clientes, y es debido al
//bloqueo de E/S, como implementar un non-I/O Bloque
package glivo

import (
	"bufio"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
)

//Una session representa un puerto escuchando peticiones de freeswitch
type Session struct {
	listener  *net.Listener
	logger    *log.Logger
	waitCalls sync.WaitGroup
}

func NewSession(srv *net.Listener, logger *log.Logger) *Session {
	return &Session{listener: srv, logger: logger}
}

func (session *Session) Start(handler func(call *Call, userData interface{}), userData interface{}) {
	defer session.waitCalls.Wait()

	go func(session *Session) {

		for {

			conn, err := (*session.listener).Accept()
			if err != nil {
				continue
			}

			conn.Write([]byte("connect\n\n"))
			buf := bufio.NewReaderSize(conn, 4096)
			reader := textproto.NewReader(buf)

			header, err := reader.ReadMIMEHeader()
			if err != nil {
				session.logger.Printf("Error reading Call Start info: %s", err.Error())
				continue
			}

			call := NewCall(&conn, header, session.logger)

			replyCh := make(chan CommandStatus, 100) //si +OK es "" de lo contrario se envia cade
			call.SetReply(&replyCh)
			session.waitCalls.Add(1)
			go HandleCall(call, buf, replyCh, &session.waitCalls)
			go DispatcherEvents(call)

			//preludio
			call.Write([]byte("linger\n\n"))
			call.Reply()
			call.Write([]byte("myevents\n\n"))
			call.Reply()
			call.Write([]byte("event plain CUSTOM\n\n"))
			call.Reply()

			go handler(call, userData)

		}

	}(session)

}

//Termina el servidor y bloquea hasta
//que se terminen todas las llamadas
func (session *Session) Stop() {
	session.waitCalls.Wait()
	(*session.listener).Close()
}

type CommandStatus string

func DispatcherEvents(call *Call) {
	for ev := range call.queueEvents {
		eventDispatch(call, ev)
	}
}

func HandleCall(call *Call, buf *bufio.Reader, replyCh chan CommandStatus, waitCall *sync.WaitGroup) {
	defer call.Conn.Close()
	defer func() { close(call.queueEvents) }()
	defer waitCall.Done()

	reader := textproto.NewReader(buf)

	for {
		notification_body := ""
		notification, err := reader.ReadMIMEHeader()
		if err != nil {
			call.logger.Println("Failed read: ", err.Error())
			break
		}
		if Scontent_length := notification.Get("Content-Length"); Scontent_length != "" {
			content_length, _ := strconv.Atoi(Scontent_length)
			lreader := io.LimitReader(buf, int64(content_length))
			body, err := ioutil.ReadAll(lreader)
			if err != nil {
				call.logger.Printf("Failed read body closing: %s", err.Error())
				break
			} else {
				notification_body = string(body)
			}

		}

		switch notification.Get("Content-Type") {
		case "command/reply":
			if strings.HasPrefix(notification.Get("Reply-Text"), "+OK") {
				replyCh <- ""
			} else {
				replyCh <- CommandStatus(strings.TrimPrefix(notification.Get("Reply-Text"), "-ERR"))
			}
		case "text/event-plain":
			buf := bufio.NewReader(strings.NewReader(notification_body))
			reader := textproto.NewReader(buf)
			mime_body, _ := reader.ReadMIMEHeader()
			event := EventFromMIME(call, mime_body)
			call.queueEvents <- event
		}
	}

}

//Crea el servidor en la interfaz y puerto seleccionado
func Listen(laddr string, logger *log.Logger) (*Session, error) {
	srv, err := net.Listen("tcp", laddr)
	if err != nil {
		return nil, err
	}
	return NewSession(&srv, logger), nil
}
