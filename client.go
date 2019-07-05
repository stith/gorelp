package relp

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

type Client struct {
	host       string
	port       int
	timeout    time.Duration
	connection net.Conn
	readwriter *bufio.ReadWriter
	offerData  []byte

	nextTxn int

	WillWaitAck bool
}

func NewClientConnection(host string, port int, timeout time.Duration) (net.Conn, *bufio.ReadWriter, error) {
	connection, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), timeout)
	if err != nil {
		connection = FailedConn{}
	}
	reader := bufio.NewReader(connection)
	writer := bufio.NewWriter(connection)
	readwriter := bufio.NewReadWriter(reader, writer)

	return connection, readwriter, err
}

// NewClientTimeout - Starts a new RELP client with Dial timeout set
func NewClientTimeout(host string, port int, timeout time.Duration, offerData map[string]string) (client Client, err error) {
	client.host = host
	client.port = port
	client.timeout = timeout
	client.nextTxn = 2
	client.WillWaitAck = true
	client.offerData = offerDataBytes(offerData)

	client.connection, client.readwriter, err = NewClientConnection(host, port, timeout)

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
	if _, err := msg.send(c.readwriter.Writer); err != nil {
		return err
	}

	if c.WillWaitAck {
		return c.WaitAck(&msg)
	} else {
		return nil
	}
}

func (c *Client) WaitAck(msg *Message) (err error) {
	ack, err := readMessage(c.readwriter.Reader)
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

// Recreate - recreates connection
func (c *Client) Recreate() error {
	var err error

	c.connection.Close()
	c.connection, c.readwriter, err = NewClientConnection(c.host, c.port, c.timeout)
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
	if _, err := offer.send(c.readwriter.Writer); err != nil {
		return err
	}

	offerResponse, err := readMessage(c.readwriter.Reader)
	if err != nil {
		return err
	}

	responseParts := strings.Split(string(offerResponse.Data), "\n")
	if !strings.HasPrefix(responseParts[0], "200 OK") {
		return fmt.Errorf("Server responded to offer with: %s", responseParts[0])
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
	_, err = closeMessage.send(c.readwriter.Writer)
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
