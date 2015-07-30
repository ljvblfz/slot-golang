package main

import (
	"fmt"
	"net"
	"time"

	"cloud-base/websocket"
)

const (
	kUDPHearteat = 15 * time.Second
)

type Connection interface {
	Close() error
	Send(msg []byte) (int, error)
	Timeout() bool
	Update(net.Addr) error
}

type wsConn struct {
	conn *websocket.Conn
}

func NewWsConn(conn *websocket.Conn) Connection {
	return &wsConn{conn: conn}
}

func (w *wsConn) Close() error {
	if w.conn == nil {
		return nil
	}
	return w.conn.Close()
}

func (w *wsConn) Send(msg []byte) (int, error) {
	return w.conn.Write(msg)
}

func (w *wsConn) Timeout() bool {
	return false
}

//func (w *wsConn) Verify() error {
//	return nil
//}

func (w *wsConn) Update(net.Addr) error {
	return nil
}

type udpConn struct {
	server       *net.UDPConn
	peer         *net.UDPAddr
	sn           []byte
	lastHearbeat time.Time
}

func NewUdpConnection(socket *net.UDPConn, peer *net.UDPAddr) Connection {
	return &udpConn{server: socket, peer: peer}
}

func (u *udpConn) Close() error {
	return nil
}

func (u *udpConn) Send(msg []byte) (int, error) {
	return u.server.WriteToUDP(msg, u.peer)
}

func (u *udpConn) Timeout() bool {
	return time.Now().Sub(u.lastHearbeat) > 3*kUDPHearteat
}

//func (u *udpConn) Verify(token []byte) error {
//	return VerifyToken(token, u.sn)
//}

func (u *udpConn) Update(addr net.Addr) error {
	if !u.Timeout() {
		return fmt.Errorf("session timeout")
	}

	uaddr, ok := addr.(*net.UDPAddr)
	if !ok {
		return fmt.Errorf("not udp addr")
	}
	if u.peer.String() != uaddr.String() {
		u.peer = uaddr
	}
	return nil
}
