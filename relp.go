package relp

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
)

const relpVersion = 0
const relpSoftware = "gorelp,0.2.0,https://github.com/stith/gorelp"

// Message - A single RELP message
type Message struct {
	// The transaction ID that the message was sent in
	Txn int
	// The command that was run. Will be "syslog" pretty much always under normal
	// operation
	Command string
	// The actual message data
	Data string

	// true if the message has been acked
	Acked bool

	// Used internally for acking.
	sourceConnection net.Conn
}

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

// Client - A client to a RELP server
type Client struct {
	connection net.Conn

	nextTxn int
}

func readMessage(conn io.Reader) (message Message, err error) {
	reader := bufio.NewReader(conn)

	txn, err := reader.ReadString(' ')
	if err == io.EOF {
		// A graceful EOF means the client closed the connection. Hooray!
		fmt.Println("Done!")
		return
	} else if err != nil && strings.HasSuffix(err.Error(), "connection reset by peer") {
		fmt.Println("Client rudely disconnected, but that's fine.")
		return
	} else if err != nil {
		log.Println("Error reading txn:", err, txn)
		return
	}
	message.Txn, _ = strconv.Atoi(strings.TrimSpace(txn))

	cmd, err := reader.ReadString(' ')
	if err != nil {
		log.Println("Error reading cmd:", err)
		return
	}
	message.Command = strings.TrimSpace(cmd)

	// Check for dataLen == 0
	peekLen, err := reader.Peek(1)
	message.Data = ""
	if string(peekLen[:]) != "0" {
		dataLenS, err := reader.ReadString(' ')
		if err != nil {
			log.Println("Error reading dataLen:", err)
			return message, err
		}

		dataLen, err := strconv.Atoi(strings.TrimSpace(dataLenS))
		if err != nil {
			log.Println("Error converting dataLenS to int:", err)
			return message, err
		}

		dataBytes := make([]byte, dataLen)
		_, err = io.ReadFull(reader, dataBytes)
		if err != nil {
			log.Println("Error reading message:", err)
			return message, err
		}
		message.Data = string(dataBytes[:dataLen])
	}
	return message, err
}

func handleConnection(conn net.Conn, server Server) {
	defer conn.Close()

	for {
		message, _ := readMessage(conn)
		message.sourceConnection = conn

		response := Message{
			Txn:     message.Txn,
			Command: "rsp",
		}

		switch message.Command {
		case "open":
			response.Data = fmt.Sprintf("200 OK\nrelp_version=%d\nrelp_software=%s\ncommands=syslog", relpVersion, relpSoftware)
			_, err := response.send(conn)
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
			response.Data = "500 ERR"
			_, err := response.send(conn)
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

// NewClient - Starts a new RELP client
func NewClient(host string, port int) (client Client, err error) {
	client.connection, err = net.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return client, err
	}

	offer := Message{
		Txn:     1,
		Command: "open",
		Data:    fmt.Sprintf("relp_version=%d\nrelp_software=%s\ncommands=syslog", relpVersion, relpSoftware),
	}
	offer.send(client.connection)

	offerResponse, err := readMessage(client.connection)
	if err != nil {
		return client, err
	}

	responseParts := strings.Split(offerResponse.Data, "\n")
	if !strings.HasPrefix(responseParts[0], "200 OK") {
		err = fmt.Errorf("Server responded to offer with: %s", responseParts[0])
	} else {
		err = nil
	}

	client.nextTxn = 2
	// TODO: Parse the server's info/commands into the Client object
	return client, err
}

// Close - Stops listening for connections and closes the message channel
func (s Server) Close() {
	s.done = true
	s.listener.Close()
	close(s.MessageChannel)
}

// Send - Sends a message
func (m Message) send(out io.Writer) (nn int, err error) {
	outLength := len([]byte(m.Data))
	outString := fmt.Sprintf("%d %s %d %s\n", m.Txn, m.Command, outLength, m.Data)
	return out.Write([]byte(outString))
}

// Ack - Acknowledges a message
func (m *Message) Ack() (err error) {
	if m.Acked {
		return fmt.Errorf("Called Ack on already-acknowledged message %d.", m.Txn)
	}

	if m.sourceConnection == nil {
		// If the source connection is gone, we don't need to do any work.
		return nil
	}

	ackMessage := Message{
		Txn:     m.Txn,
		Command: "rsp",
		Data:    "200 OK",
	}
	_, err = ackMessage.send(m.sourceConnection)
	if err != nil {
		return err
	}
	m.Acked = true
	return
}

// SendString - Convenience method which constructs a Message and sends it
func (c *Client) SendString(msg string) (err error) {
	message := Message{
		Txn:     c.nextTxn,
		Command: "syslog",
		Data:    msg,
	}

	err = c.SendMessage(message)

	return err
}

// SendMessage - Sends a message using the client's connection
func (c *Client) SendMessage(msg Message) (err error) {
	c.nextTxn = c.nextTxn + 1
	_, err = msg.send(c.connection)

	// TODO: Make waiting for an ack optional
	ack, err := readMessage(c.connection)
	if ack.Command != "rsp" {
		return fmt.Errorf("Response to txn %d was %s: %s", msg.Txn, ack.Command, ack.Data)
	}
	if ack.Txn != msg.Txn {
		return fmt.Errorf("Response txn to %d was %d", msg.Txn, ack.Txn)
	}

	return err
}

// Close - Closes the connection gracefully
func (c Client) Close() (err error) {
	closeMessage := Message{
		Txn:     c.nextTxn,
		Command: "close",
	}
	_, err = closeMessage.send(c.connection)
	return
}
