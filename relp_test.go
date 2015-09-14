package relp

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	_, err := NewServer("127.0.0.1", 3333)
	if err != nil {
		t.Error("Error setting up listener:", err)
		return
	}

	conn, err := net.DialTimeout("tcp", "127.0.0.1:3333", time.Second)
	if err != nil {
		t.Error("Error connecting to socket:", err)
		return
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)

	offerBytes := []byte("relp_version=0\nrelp_software=gorelp,0.0.1,https://git.stith.me/mstith/relp\ncommands=syslog")
	outString := fmt.Sprintf("1 open %d %s\n", len(offerBytes), offerBytes)
	_, err = conn.Write([]byte(outString))
	if err != nil {
		t.Error("Error sending offer", err)
		return
	}

	txn, err := reader.ReadString(' ')
	if err != nil {
		t.Error("Error reading offer response", err)
		return
	}

	txn = strings.TrimSpace(txn)
	if txn != "1" {
		t.Error("Offer response txn != 1", txn)
		return
	}

	cmd, err := reader.ReadString(' ')
	if err != nil {
		t.Error("Error reading offer response command", err)
		return
	}

	cmd = strings.TrimSpace(cmd)
	if cmd != "rsp" {
		t.Error("Offer response command != rsp", cmd)
		return
	}

	dataLenS, err := reader.ReadString(' ')
	if err != nil {
		t.Error("Error reading dataLen:", err)
		return
	}

	dataLen, err := strconv.Atoi(strings.TrimSpace(dataLenS))
	if err != nil {
		t.Error("Error converting dataLenS to int:", err)
		return
	}

	dataBytes := make([]byte, dataLen)
	_, err = io.ReadFull(reader, dataBytes)
	if err != nil {
		t.Error("Error reading message:", err)
		return
	}
	dataString := string(dataBytes[:dataLen])

	if dataString != "200 OK\nrelp_version=0\nrelp_software=gorelp,0.0.1,https://git.stith.me/mstith/relp\ncommands=syslog" {
		t.Error("Offer response != 200 OK\\nrelp_version=0\\nrelp_software=gorelp,0.0.1,https://git.stith.me/mstith/relp\\ncommands=syslog\n", dataString)
		return
	}
}
