package main

import (
	"net"
	"sync"

	"github.com/golang/glog"
)

const (
	kMaxPackageSize = 10240
)

type Server struct {
	addr     string
	handler  *Handler
	socket   *net.UDPConn
	socketMu *sync.Mutex
}

func NewServer(addr string, handler *Handler) *Server {
	s := &Server{
		addr:     addr,
		handler:  handler,
		socketMu: &sync.Mutex{},
	}
	handler.Server = s
	handler.Go()
	return s
}

func (s *Server) Start() {
	localAddr, err := net.ResolveUDPAddr("udp", s.addr)
	if err != nil {
		glog.Fatalf("Resolve server addr failed: %v", err)
	}
	socket, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		glog.Fatalf("Listen on addr failed: %v", err)
	}
	s.socket = socket
	glog.Infof("Server started on %v", socket.LocalAddr())

	buf := make([]byte, kMaxPackageSize)
	for {
		n, peer, err := socket.ReadFromUDP(buf)
		if err != nil {
			if nerr, ok := err.(*net.OpError); ok && !nerr.Temporary() {
				glog.Fatalf("Read failed: %v", nerr)
			}
			continue
		}

		if glog.V(3) {
			glog.Infof("[msg|in] peer: %v, msg: len(%d)%v", peer, n, buf[:n])
		} else if glog.V(2) {
			if n < 4 {
				glog.Infof("[msg|in] peer: %v, msg: len(%d)%v", peer, n, buf[:n])
			} else {
				glog.Infof("[msg|in] peer: %v, msg: len(%d)%v", peer, n, buf[:4])
			}
		}
		s.handler.Process(peer, buf[:n])
	}
}

func (s *Server) Send(peer *net.UDPAddr, msg []byte) {
	s.socketMu.Lock()
	n, err := s.socket.WriteToUDP(msg, peer)
	s.socketMu.Unlock()
	if n != len(msg) || err != nil {
		if glog.V(3) {
			glog.Errorf("[server] send udp msg (len(%d)%v) failed: %v", len(msg), msg, err)
		} else if glog.V(2) {
			glog.Errorf("[server] send udp msg (len(%d)%v) failed: %v", len(msg), msg[:4], err)
		} else {
			glog.Errorf("[server] send udp msg failed: %v", err)
		}
	} else {
		if glog.V(3) {
			glog.Infof("[msg|out] peer: %v, msg: len(%d)%v", peer, n, msg[:n])
		} else if glog.V(2) {
			if n < 4 {
				glog.Infof("[msg|out] peer: %v, msg: len(%d)%v", peer, n, msg[:n])
			} else {
				glog.Infof("[msg|out] peer: %v, msg: len(%d)%v", peer, n, msg[:4])
			}
		}
	}
}
