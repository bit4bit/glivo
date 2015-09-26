//Implemanta Respuesta para Freeswitch Outbound e Inbound
//La idea inicial, es hacer un clon de plivoframework sencillo
//pero la diferencia es que recibe todo el mensaje o IVR
//y lo ejecuta
//
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

//HandlerCall active call
type HandlerCall func(call *Call, userData interface{})

//Una session representa un puerto escuchando peticiones de freeswitch
type server struct {
	listener  *net.Listener
	logger    *log.Logger
	waitCalls sync.WaitGroup
}

func (session *server) Serve(handler HandlerCall, userData interface{}) error {
	defer session.waitCalls.Wait()

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

		stopCall := make(chan bool)
		replyCh := make(chan CommandStatus, 100) //si +OK es "" de lo contrario se envia cade
		call := NewCall(&conn, header, replyCh, session.logger)
		session.waitCalls.Add(1)
		go HandleCall(call, buf, replyCh, stopCall)
		go DispatcherEvents(call)

		go func() {
			//preludio
			call.Write([]byte("linger\n\n"))
			call.Reply()
			call.Write([]byte("myevents\n\n"))
			call.Reply()
			call.Write([]byte("event plain CUSTOM\n\n"))
			call.Reply()

			handler(call, userData)
			session.waitCalls.Done()
			call.Close()
			close(call.queueEvents)
			stopCall <- true
		}()
	}

}

//Termina el servidor y bloquea hasta
//que se terminen todas las llamadas
func (session *server) Stop() {
	session.waitCalls.Wait()
	(*session.listener).Close()
}

type CommandStatus string

func DispatcherEvents(call *Call) {
	for ev := range call.queueEvents {
		eventDispatch(call, ev)
	}
}

func HandleCall(call *Call, buf *bufio.Reader,
	replyCh chan CommandStatus, stopedCall chan bool) {
	chnotification := make(chan textproto.MIMEHeader)
	cherror := make(chan error)
	reader := textproto.NewReader(buf)
	go func() {
		for {
			notification, err := reader.ReadMIMEHeader()
			if err != nil {
				cherror <- err
				break
			}
			chnotification <- notification
		}
	}()

loop:
	for {
		select {
		case <-stopedCall:
			break loop

		case <-cherror:
			break loop

		case notification := <-chnotification:
			notification_body := ""
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

}

//Crea el servidor en la interfaz y puerto seleccionado
func Listen(laddr string, logger *log.Logger) (*server, error) {
	srv, err := net.Listen("tcp", laddr)
	if err != nil {
		return nil, err
	}
	return &server{&srv, logger, sync.WaitGroup{}}, nil
}

func ListenAndserve(laddr string, logger *log.Logger, handler HandlerCall,
	userData interface{}) error {
	session, err := Listen(laddr, logger)
	if err != nil {
		return err
	}

	session.Serve(handler, userData)
	return nil
}
