package main

import (
	"net"
	"sync"

	"github.com/golang/glog"
)

const (
	kMaxPackageSize = 10240
)

type (
	Server interface {
		Send(peer *net.UDPAddr, msg []byte) // send msg to client/device
		RunLoop() bool
		GetProxySeriveAddr() string
	}

	myServer struct {
		isActive bool
		addr     string
		socket   *net.UDPConn
		socketMu *sync.Mutex
	}
)

func NewServer(addr string) Server {
	return &myServer{
		addr:     addr,
		socketMu: &sync.Mutex{},
	}
}

func (this *myServer) RunLoop() bool {
	if this.isActive {
		return false
	}

	localAddr, err := net.ResolveUDPAddr("udp", this.addr)
	if err != nil {
		glog.Fatalf("Resolve server addr failed: %v", err)
	}

	socket, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		glog.Fatalf("Listen on addr failed: %v", err)
	}
	this.socket = socket

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
		adr := selectUDPServer()
		this.Send(peer, []byte(adr))
		glog.Infoln(n, peer, adr)
	}
	return true
}

func (this *myServer) Send(peer *net.UDPAddr, msg []byte) {
	this.socketMu.Lock()
	n, err := this.socket.WriteToUDP(msg, peer)
	this.socketMu.Unlock()
	glog.Infof("[udp|sended] peer: %v, msg: len(%d)%v,err:%v", peer.String(), n, msg, err)
}

func (this *myServer) GetProxySeriveAddr() string {
	return this.addr
}
