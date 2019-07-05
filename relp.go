package relp

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
)

const relpVersion = 0
const relpSoftware = "gorelp,0.2.0,https://github.com/sebisujar/gorelp"

var defaultOffer = fmt.Sprintf("relp_version=%d\nrelp_software=%s\ncommands=syslog", relpVersion, relpSoftware)

// Message - A single RELP message
type Message struct {
	// The transaction ID that the message was sent in
	Txn int
	// The command that was run. Will be "syslog" pretty much always under normal
	// operation
	Command string
	// The actual message data
	Data []byte

	// true if the message has been acked
	Acked bool

	// Used internally for acking.
	writer *bufio.Writer
}

func readMessage(reader *bufio.Reader) (message Message, err error) {
	txn, err := reader.ReadString(' ')

	if err == io.EOF {
		// A graceful EOF means the client closed the connection. Hooray!
		return
	} else if err != nil && strings.HasSuffix(err.Error(), "connection reset by peer") {
		return
	} else if err != nil {
		return
	}

	message.Txn, err = strconv.Atoi(strings.TrimSpace(txn))
	if err != nil {
		return
	}

	cmd, err := reader.ReadString(' ')
	if err != nil {
		return
	}

	message.Command = strings.TrimSpace(cmd)

	dataLenBytes, err := reader.ReadString(' ')
	if err != nil {
		log.Println("Error reading dataLen:", err)
		return message, err
	}

	dataLen, err := strconv.Atoi(strings.TrimSpace(dataLenBytes))
	if err != nil {
		log.Println("Error converting dataLen to int:", err)
		return message, err
	}

	message.Data = make([]byte, dataLen)
	_, err = io.ReadFull(reader, message.Data)
	if err != nil {
		log.Println("Error reading message:", err)
		return message, err
	}

	return message, err
}

// Send - Sends a message
func (m Message) send(writer *bufio.Writer) (int, error) {
	// format: txn command datalength data\n
	var buffer bytes.Buffer
	buffer.WriteString(string(m.Txn))
	buffer.WriteRune(' ')
	buffer.WriteString(m.Command)
	buffer.WriteRune(' ')
	buffer.WriteString(string(len(m.Data)))
	buffer.WriteRune(' ')
	buffer.Write(m.Data)
	buffer.WriteRune('\n')

	return writer.Write(buffer.Bytes())
}
