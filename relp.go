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

/*
Server - Contains info about the RELP listener

  MessageChannel - Emits messages as they are received
*/
type Server struct {
	MessageChannel chan string

	listener net.Listener
	done     bool
}

func handleConnection(conn net.Conn, server Server) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	for {
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
		txn = strings.TrimSpace(txn)

		cmd, err := reader.ReadString(' ')
		if err != nil {
			log.Println("Error reading cmd:", err)
			return
		}
		cmd = strings.TrimSpace(cmd)

		// Check for dataLen == 0
		peekLen, err := reader.Peek(1)
		dataString := ""
		if string(peekLen[:]) != "0" {
			dataLenS, err := reader.ReadString(' ')
			if err != nil {
				log.Println("Error reading dataLen:", err)
				return
			}

			dataLen, err := strconv.Atoi(strings.TrimSpace(dataLenS))
			if err != nil {
				log.Println("Error converting dataLenS to int:", err)
				return
			}

			dataBytes := make([]byte, dataLen)
			_, err = io.ReadFull(reader, dataBytes)
			if err != nil {
				log.Println("Error reading message:", err)
				return
			}
			dataString = string(dataBytes[:dataLen])
		}

		switch cmd {
		case "open":
			outBytes := []byte("200 OK\nrelp_version=0\nrelp_software=gorelp,0.0.1,https://git.stith.me/mstith/relp\ncommands=syslog")
			outString := fmt.Sprintf("%s rsp %d %s\n", txn, len(outBytes), outBytes)
			_, err := conn.Write([]byte(outString))
			if err != nil {
				log.Println(err)
				return
			}
		case "syslog":
			server.MessageChannel <- dataString
			_, err := conn.Write([]byte(fmt.Sprintf("%s rsp 6 200 OK\n", txn)))
			if err != nil {
				fmt.Println("Error sending syslog ok:", err)
				return
			}
		case "close":
			fmt.Println("Got a close, closing!")
			return
		default:
			log.Println("Got unknown command:", cmd)
			_, err := conn.Write([]byte(fmt.Sprintf("%s rsp 7 500 ERR\n", txn)))
			if err != nil {
				log.Println(err)
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
func NewServer(host string, port int) (server Server, err error) {
	server.listener, err = net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return server, err
	}

	server.MessageChannel = make(chan string)
	go acceptConnections(server)
	return server, nil
}

// Close - Stops listening for connections and closes the message channel
func (s Server) Close() {
	s.done = true
	s.listener.Close()
	close(s.MessageChannel)
}
