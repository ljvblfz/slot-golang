package main

import (
	// "bufio"
	"encoding/binary"
	"github.com/golang/glog"
	"io"
	"net"
	"sync"
)

const (
	BUF_MAX     = 1024
	HEADER_SIZE = 4
	PAYLOAD_MAX = BUF_MAX - HEADER_SIZE
)

type Server struct {
	TcpServer
	wg sync.WaitGroup
}

func (this *Server) listen() {
	defer this.wg.Done()
	for {
		conn, err := this.accept()
		if err != nil {
			glog.Errorf("front server acceept failed:%s\n", err.Error())
			break
		}
		glog.V(1).Infof("front server, new connection from %v\n", conn.RemoteAddr())
		this.wg.Add(1)
		go this.handleClient(conn)
	}
}

func (this *Server) Start() error {
	err := this.buildListener()
	if err != nil {
		return err
	}

	this.wg.Add(1)
	go this.listen()
	return nil
}

func (this *Server) handleClient(conn *net.TCPConn) {
	defer this.wg.Done()

	defer conn.Close()
	// head := make([]byte, 4)
	// rd := bufio.NewReaderSize(conn, 1024)
	// for {
	// 	err = binary.Read(rd, binary.LittleEndian, &head)
	// 	if err != nil {
	// 		glog.Errorf("read tunnel failed:%s\n", err.Error())
	// 		break
	// 	}

	// 	var data []byte

	// 	// if header.Sz == 0, it's ok too
	// 	data = make([]byte, header.Sz)
	// 	c := 0
	// 	for c < int(header.Sz) {
	// 		var n int
	// 		n, err = rd.Read(data[c:])
	// 		if err != nil {
	// 			Error("read tunnel failed:%s", err.Error())
	// 			return
	// 		}
	// 		c += n
	// 	}

	// 	self.outputCh <- &TunnelPayload{header.Linkid, data}
	// }
	buf := make([]byte, BUF_MAX)
	for {
		// read header : 4-bytes
		header := buf[:HEADER_SIZE]
		n, err := io.ReadFull(conn, header)
		if n == 0 && err == io.EOF {
			break
		} else if err != nil {
			glog.Errorf("error receiving header:%s\n", err)
			break
		}

		// read payload, the size of the payload is given by header
		size := binary.BigEndian.Uint32(header)
		if size > PAYLOAD_MAX {
			// log
			break
		}

		data := buf[HEADER_SIZE : HEADER_SIZE+size]
		n, err = io.ReadFull(conn, data)

		if err != nil {
			glog.Errorf("error receiving payload:%s\n", err)
			break
		}
		MainHandle(data)
	}
	HandleClose(conn.RemoteAddr().String()) // 释放服务器所有用户的信息
}

func (self *Server) Stop() {
	self.closeListener()
}

// func (self *Server) Wait() {
// 	self.wg.Wait()
// 	Error("front door quit")
// }

func NewServer(addr string) *Server {
	server := new(Server)
	server.TcpServer.addr = addr
	return server
}
