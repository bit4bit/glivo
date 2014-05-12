//Implemanta Respuesta para Freeswitch Outbound e Inbound
//La idea inicial, es hacer un clon de plivoframework sencillo
//pero la diferencia es que recibe todo el mensaje o IVR
//y lo ejecuta
package glivo

import (
	"net"
	"net/textproto"
	"bufio"
	"strings"
	"io"
	"io/ioutil"
	"strconv"
	"log"
)

//Una session representa un puerto escuchando peticiones de freeswitch
type Session struct {
	listener *net.Listener
	done chan bool
	logger *log.Logger
}


func NewSession(srv *net.Listener, logger *log.Logger) *Session {
	return &Session{srv,  make(chan bool), logger}
}

func (session *Session) Start(handler func(call *Call)) {

	go func(session *Session){
		calls_active := make([]*Call, 100, 254)
		for {
			select{
			case <-session.done:
				session.logger.Print("Closing server")
				return;
			default:
			}

			conn, err := (*session.listener).Accept()
			if err != nil {
				continue
			}

			conn.Write([]byte("connect\n\n"))
			buf := bufio.NewReaderSize(conn, 4096)
			reader := textproto.NewReader(buf)
	
			header, err := reader.ReadMIMEHeader()
			if err != nil {
				session.logger.Fatalf("Error reading call info: %s", err.Error())
				continue
			}

			call := NewCall(&conn, header, session.logger)
			calls_active = append(calls_active, call)

			replyCh := make(chan CommandStatus, 100) //si +OK es "" de lo contrario se envia cade
			call.SetReply(&replyCh)

			go HandleCall(call, buf, replyCh)
			//preludio
			call.Write([]byte("linger\n\n"))
			call.Reply()
			call.Write([]byte("myevents\n\n"))
			call.Reply()

			go handler(call)
		}
		
		//esperamos que terminen todas las llamadas activas
		//antes de cerrar
		for _,call_active := range calls_active {
			if call_active != nil {
				<- call_active.done
			}
		}
		session.done <- true
	}(session)

}

//Termina el servidor y bloquea hasta
//que se terminen todas las llamadas
func (session *Session) Stop() bool {
	session.done <- true
	return <- session.done
}


type CommandStatus string



func HandleCall(call *Call, buf *bufio.Reader, replyCh chan CommandStatus){
	defer call.Conn.Close()

	reader := textproto.NewReader(buf)
	for {
		notification_body := ""
		notification,err := reader.ReadMIMEHeader()
		if err != nil {
			call.logger.Println("Failed read: ", err.Error())
			break
		}
		if Scontent_length := notification.Get("Content-Length"); Scontent_length != "" {
			content_length, _ := strconv.Atoi(Scontent_length)
			lreader := io.LimitReader(buf, int64(content_length))
			body, err := ioutil.ReadAll(lreader)
			if err != nil {
				call.logger.Fatalf("Failed read body: %s" ,err.Error())
				break
			}else{
				notification_body = string(body)
			}
			
		}


		switch notification.Get("Content-Type") {
		case "command/reply":
			if strings.HasPrefix(notification.Get("Reply-Text"), "+OK") {
				replyCh <- ""
			}else{
				replyCh <- CommandStatus(strings.TrimPrefix(notification.Get("Reply-Text"), "-ERR"))
			}
		case "text/event-plain":
			buf := bufio.NewReader(strings.NewReader(notification_body))
			reader := textproto.NewReader(buf)
			mime_body, _ := reader.ReadMIMEHeader()
			eventDispatch(call, EventFromMIME(call, mime_body))
		}
	}
}

//Crea el servidor en la interfaz y puerto seleccionado
func NewFS(laddr string, logger *log.Logger ) (* Session, error) {
	srv, err := net.Listen("tcp", laddr)
	if err != nil {
		return nil, err
	}
	return NewSession(&srv, logger), nil
}


