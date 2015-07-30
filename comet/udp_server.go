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
	addr    string
	handler *Handler
	con     *net.UDPConn
	conlk   *sync.Mutex
}

func NewUdpServer(addr string, handler *Handler) *UdpServer {
	s := &UdpServer{
		addr:    addr,
		handler: handler,
		conlk:   &sync.Mutex{},
	}
	return s
}

func (s *UdpServer) RunLoop() {
	localAddr, err := net.ResolveUDPAddr("udp", s.addr)
	if err != nil {
		glog.Fatalf("Resolve server addr failed: %v", err)
	}
	s.con, err = net.ListenUDP("udp", localAddr)
	if err != nil {
		glog.Fatalf("Listen on addr failed: %v", err)
	}
	glog.Infof("UDPCOMET started on %v successfully", s.con.LocalAddr())

	input := make([]byte, kMaxPackageSize)
	for {
		n, peer, err := s.con.ReadFromUDP(input)
		if err != nil {
			if nerr, ok := err.(*net.OpError); ok && !nerr.Temporary() {
				glog.Fatalf("[udp|received] Read failed: %v", nerr)
			}
			continue
		}
		if glog.V(3) {
			glog.Infof("[UDPCOMET] received dev:%v,msg:len(%d)%v", peer, n, input[:n])
		}
		s.handler.Process(peer, input[:n])
	}
}

func (s *UdpServer) Send(peer *net.UDPAddr, msg []byte) {
	s.conlk.Lock()
	n, err := s.con.WriteToUDP(msg, peer)
	s.conlk.Unlock()
	if glog.V(3) {
		glog.Infof("[udp|sended] peer: %v, msg: len(%d)%v,err:%v", peer.String(), n, msg, err)
		glog.Infof("-----------------------------------------------------------------------------------------------------------------------------")
	}
}
func (s *UdpServer) Send2(peer *net.UDPAddr, msg []byte, id int64, busi string) {
	s.conlk.Lock()
	n, err := s.con.WriteToUDP(msg, peer)
	s.conlk.Unlock()
	if glog.V(3) {
		glog.Infof("[udp|sended %v] [dev:%v-%v], response: len(%d)%v,%v", busi, id, peer.String(), n, msg, err)
		glog.Infof("-----------------------------------------------------------------------------------------------------------------------------")
	}
}
