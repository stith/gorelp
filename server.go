package relp

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"net"
)

/*
Server - Contains info about the RELP listener

  MessageChannel - Emits messages as they are received
*/
type Server struct {
	MessageChannel chan Message

	AutoAck bool

	listener net.Listener
	done     bool
}

// NewServer - Fire up a server to accept connections and emit messages
// Returns a Server
func NewServer(host string, port int, autoAck bool) (server Server, err error) {
	server.listener, err = net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	server.AutoAck = autoAck

	if err != nil {
		return server, err
	}

	server.MessageChannel = make(chan Message)
	go acceptConnections(server)
	return server, nil
}

// Close - Stops listening for connections and closes the message channel
func (s Server) Close() {
	s.done = true
	s.listener.Close()
	close(s.MessageChannel)
}

func handleConnection(conn net.Conn, server Server) {
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	defer conn.Close()

	for {
		message, err := readMessage(reader)
		if err != nil {
			log.Println(err)
			continue
		}
		message.writer = writer

		response := Message{
			Txn:     message.Txn,
			Command: "rsp",
		}

		switch message.Command {
		case "open":
			var buffer bytes.Buffer
			buffer.WriteString("200 OK\n")
			buffer.WriteString(defaultOffer)
			response.Data = buffer.Bytes()

			_, err := response.send(writer)
			if err != nil {
				log.Println(err)
				return
			}
		case "syslog":
			server.MessageChannel <- message
			if server.AutoAck {
				err := message.Ack()
				if err != nil {
					fmt.Println("Error sending syslog ok:", err)
					return
				}
			}
		case "close":
			fmt.Println("Got a close, closing!")
			return
		default:
			log.Println("Got unknown command:", message.Command)
			response.Data = []byte("500 ERR")
			_, err := response.send(writer)
			if err != nil {
				log.Println("Error sending 500 ERR:", err)
				return
			}
		}
	}
}

func acceptConnections(server Server) {
	for {
		conn, err := server.listener.Accept()
		if err != nil {
			return
		}
		if conn != nil {
			go handleConnection(conn, server)
		}
		if server.done {
			return
		}
	}
}

// Ack - Acknowledges a message
func (m *Message) Ack() (err error) {
	if m.Acked {
		return fmt.Errorf("Called Ack on already-acknowledged message %d.", m.Txn)
	}

	if m.writer == nil {
		// If the source connection is gone, we don't need to do any work.
		return nil
	}

	ackMessage := Message{
		Txn:     m.Txn,
		Command: "rsp",
		Data:    []byte("200 OK"),
	}
	_, err = ackMessage.send(m.writer)
	if err != nil {
		return err
	}
	m.Acked = true
	return
}
