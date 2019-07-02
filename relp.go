package relp

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"time"
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
	host       string
	port       int
	timeout    time.Duration
	connection net.Conn
	reader     *bufio.Reader
	offerData  map[string]string

	nextTxn int

	WillWaitAck bool
}

func readMessage(reader *bufio.Reader) (message Message, err error) {
	txn, err := reader.ReadString(' ')

	if err == io.EOF {
		// A graceful EOF means the client closed the connection. Hooray!
		return message, err
	} else if err != nil && strings.HasSuffix(err.Error(), "connection reset by peer") {
		return
	} else if err != nil {
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
	reader := bufio.NewReader(conn)
	defer conn.Close()

	for {
		message, _ := readMessage(reader)
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

func NewClientConnection(host string, port int, timeout time.Duration) (net.Conn, *bufio.Reader, error) {
	connection, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), timeout)
	if err != nil {
		connection = FailedConn{}
	}
	reader := bufio.NewReader(connection)

	return connection, reader, err
}

// NewClientTimeout - Starts a new RELP client with Dial timeout set
func NewClientTimeout(host string, port int, timeout time.Duration, offerData map[string]string) (client Client, err error) {
	client.host = host
	client.port = port
	client.timeout = timeout
	client.nextTxn = 2
	client.WillWaitAck = true
	client.offerData = offerData

	client.connection, client.reader, err = NewClientConnection(host, port, timeout)

	if err != nil {
		return client, err
	}

	err = client.Open()

	// TODO: Parse the server's info/commands into the Client object
	return client, err
}

// NewClient - Starts a new RELP client with Dial timeout set
func NewClient(host string, port int, offerData map[string]string) (client Client, err error) {
	return NewClientTimeout(host, port, 0, offerData)
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

func (c *Client) PackageString(msg string) Message {
	return Message{
		Txn:     c.nextTxn,
		Command: "syslog",
		Data:    msg,
	}
}

// SendMessage - Sends a message using the client's connection
func (c *Client) SendMessage(msg Message) (err error) {

	c.nextTxn = c.nextTxn + 1
	_, err = msg.send(c.connection)
	if err != nil {
		return err
	}

	if c.WillWaitAck {
		return c.WaitAck(&msg)
	} else {
		return nil
	}
}

func (c *Client) WaitAck(msg *Message) (err error) {
	ack, err := readMessage(c.reader)
	if err != nil {
		return err
	}

	if ack.Command != "rsp" {
		return fmt.Errorf("Response to txn %d was %s: %s", msg.Txn, ack.Command, ack.Data)
	}

	if ack.Txn != msg.Txn {
		return fmt.Errorf("Response txn to %d was %d", msg.Txn, ack.Txn)
	}

	msg.Acked = true

	return nil
}

// SetDeadline - make the next operation timeout if not completed before the given time
func (c *Client) SetDeadline(t time.Time) error {
	return c.connection.SetDeadline(t)
}

func (c *Client) SetReadDeadline(t time.Time) error {
	return c.connection.SetReadDeadline(t)
}

func (c *Client) SetWriteDeadline(t time.Time) error {
	return c.connection.SetWriteDeadline(t)
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

// Recreate - recreates connection
func (c *Client) Recreate() error {
	var err error

	c.connection.Close()
	c.connection, c.reader, err = NewClientConnection(c.host, c.port, c.timeout)
	c.Open()

	if err != nil {
		log.Println("Error recreating client", err)
	}

	return err
}

func (c *Client) Open() error {
	offer := Message{
		Txn:     1,
		Command: "open",
		Data:    offerDataToString(c.offerData),
	}
	offer.send(c.connection)

	offerResponse, err := readMessage(c.reader)
	if err != nil {
		return err
	}

	responseParts := strings.Split(offerResponse.Data, "\n")
	if !strings.HasPrefix(responseParts[0], "200 OK") {
		return fmt.Errorf("Server responded to offer with: %s", responseParts[0])
	} else {
		return nil
	}
}

func offerDataToString(offerData map[string]string) string {
	data := fmt.Sprintf("relp_version=%d\nrelp_software=%s\ncommands=syslog", relpVersion, relpSoftware)

	for k, v := range offerData {
		data += fmt.Sprintf("\n%s=%s", k, v)
	}

	return data
}
