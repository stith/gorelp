package relp

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"net"
	"time"
)

var okResponse = []byte("200 OK")

type Client struct {
	host      string
	port      int
	timeout   time.Duration
	offerData []byte

	buffer     *bytes.Buffer
	connection net.Conn
	reader     *bufio.Reader

	nextTxn int

	WillWaitAck bool
}

func NewClientConnection(host string, port int, timeout time.Duration) (net.Conn, error) {
	connection, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), timeout)
	if err != nil {
		connection = FailedConn{}
	}
	return connection, err
}

// NewClientTimeout - Starts a new RELP client with Dial timeout set
func NewClientTimeout(host string, port int, timeout time.Duration, offerData map[string]string) (client Client, err error) {
	client.host = host
	client.port = port
	client.timeout = timeout
	client.nextTxn = 2
	client.WillWaitAck = true
	client.offerData = offerDataBytes(offerData)
	client.buffer = new(bytes.Buffer)

	client.connection, err = NewClientConnection(host, port, timeout)
	client.reader = bufio.NewReader(client.connection)

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

// SendMessage - Sends a message using the client's connection
func (c *Client) SendMessage(msg Message) error {
	c.nextTxn = c.nextTxn + 1
	if _, err := msg.send(c.buffer, c.connection); err != nil {
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

// Recreate - recreates connection
func (c *Client) Recreate() error {
	var err error

	c.connection.Close()
	c.connection, err = NewClientConnection(c.host, c.port, c.timeout)
	c.reader = bufio.NewReader(c.connection)
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
		Data:    c.offerData,
	}
	if _, err := offer.send(c.buffer, c.connection); err != nil {
		return err
	}

	offerResponse, err := readMessage(c.reader)
	if err != nil {
		return err
	}

	if !bytes.HasPrefix(offerResponse.Data, okResponse) {
		return fmt.Errorf("Server responded to offer with: %s", offerResponse.Data)
	} else {
		return nil
	}
}

// Close - Closes the connection gracefully
func (c Client) Close() (err error) {
	closeMessage := Message{
		Txn:     c.nextTxn,
		Command: "close",
	}
	_, err = closeMessage.send(c.buffer, c.connection)
	return
}

func offerDataBytes(offerData map[string]string) []byte {
	var buffer bytes.Buffer
	buffer.WriteString(defaultOffer)

	for k, v := range offerData {
		buffer.WriteRune('\n')
		buffer.WriteString(k)
		buffer.WriteRune('=')
		buffer.WriteString(v)
	}

	return buffer.Bytes()
}

func (c *Client) PackageString(msg string) Message {
	return Message{
		Txn:     c.nextTxn,
		Command: "syslog",
		Data:    []byte(msg),
	}
}

func (c *Client) PackageBytes(data []byte) Message {
	return Message{
		Txn:     c.nextTxn,
		Command: "syslog",
		Data:    data,
	}
}
