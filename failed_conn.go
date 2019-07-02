package relp

import (
	"errors"
	"net"
	"time"
)

type FailedConn struct{}

var FailedConnError = errors.New("this is a failed connection")

func (f FailedConn) Read(b []byte) (n int, err error) {
	return 0, FailedConnError
}

func (f FailedConn) Write(b []byte) (n int, err error) {
	return 0, FailedConnError
}

func (f FailedConn) Close() error {
	return nil
}

func (f FailedConn) LocalAddr() net.Addr {
	return nil
}

func (f FailedConn) RemoteAddr() net.Addr {
	return nil
}

func (f FailedConn) SetDeadline(t time.Time) error {
	return FailedConnError
}

func (f FailedConn) SetReadDeadline(t time.Time) error {
	return FailedConnError
}

func (f FailedConn) SetWriteDeadline(t time.Time) error {
	return FailedConnError
}
