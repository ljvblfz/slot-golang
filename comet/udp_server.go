package main

import (
	"github.com/golang/glog"
	"net"
	"sync"
)

const (
	kMaxPackageSize = 10240
)

type UdpServer struct {
	addr     string
	handler  *Handler
	socket   *net.UDPConn
	socketMu *sync.Mutex
}

func NewUdpServer(addr string, handler *Handler) *UdpServer {
	s := &UdpServer{
		addr:     addr,
		handler:  handler,
		socketMu: &sync.Mutex{},
	}
	return s
}

func (s *UdpServer) RunLoop() {
	localAddr, err := net.ResolveUDPAddr("udp", s.addr)
	if err != nil {
		glog.Fatalf("Resolve server addr failed: %v", err)
	}
	socket, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		glog.Fatalf("Listen on addr failed: %v", err)
	}
	s.socket = socket
	glog.Infof("UdpServer started on %v", socket.LocalAddr())

	input := make([]byte, kMaxPackageSize)
	for {
		n, peer, err := socket.ReadFromUDP(input)
		if err != nil {
			if nerr, ok := err.(*net.OpError); ok && !nerr.Temporary() {
				glog.Fatalf("[udp|received] Read failed: %v", nerr)
			}
			continue
		}
		glog.Infof("[udp|received] peer: %v, msg: len(%d)%v", peer, n, input[:n])
		s.handler.Process(peer, input[:n])
	}
}

func (s *UdpServer) Send(peer *net.UDPAddr, msg []byte) {
	s.socketMu.Lock()
	n, err := s.socket.WriteToUDP(msg, peer)
	s.socketMu.Unlock()
	glog.Infof("[udp|sended] peer: %v, msg: len(%d)%v,err:%v", peer.String(), n, msg, err)
}
