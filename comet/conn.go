package main

import (
	"net"

	"cloud-base/websocket"
)

type Connection interface {
	Close() error
	Send(msg []byte) (int, error)
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

type udpConn struct {
	server *net.UDPConn
	peer   *net.UDPAddr
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
