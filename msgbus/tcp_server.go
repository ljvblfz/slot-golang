package main

import (
	"errors"
	"github.com/golang/glog"
	"net"
)

type TcpServer struct {
	addr string
	ln   *net.TCPListener
}

func (this *TcpServer) accept() (conn *net.TCPConn, err error) {
	conn, err = this.ln.AcceptTCP()
	return
}

func (this *TcpServer) buildListener() error {
	if this.ln != nil {
		return errors.New("server has started")
	}

	laddr, err := net.ResolveTCPAddr("tcp", this.addr)
	if err != nil {
		glog.Fatalf("resolve local addr failed:%s\n", err.Error())
		return err
	}

	ln, err := net.ListenTCP("tcp", laddr)
	if err != nil {
		glog.Fatalf("build listener failed:%s\n", err.Error())
		return err
	}

	glog.Infof("listen %s\n", this.addr)
	this.ln = ln
	return nil
}

func (this *TcpServer) closeListener() {
	if this.ln != nil {
		this.ln.Close()
	}
}
