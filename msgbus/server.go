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
	HEADER_SIZE = 4
	PAYLOAD_MAX = 1024
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
			glog.Errorf("comet accept failed:%s\n", err.Error())
			break
		}
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
	addr, err := net.ResolveTCPAddr(conn.RemoteAddr().Network(), conn.RemoteAddr().String())
	if err != nil {
		glog.Errorf("ResolveTCPAddr failed [%v], %v", conn.RemoteAddr(), err)
		return
	}
	GComets.AddServer(addr.IP.String(), conn)
	glog.Infof("New comet [%s]", addr.IP.String())
	header := make([]byte, HEADER_SIZE)
	buf := make([]byte, PAYLOAD_MAX)
	for {
		// TODO 这里有一个可优化的空间，在接受数据之前创建一个buffer，前四字节是长度，后面
		// 该长度的字节是内容，然后将这个buffer整体传给后续的处理程序，后续的转发就可以直接
		// 使用该buffer转发内容，不需要再单独拼接长度头和内容，或者调用两次发送，这样避免了
		// 多余的一次内存分配或多余的一次系统调用

		// read header : 4-bytes
		n, err := io.ReadFull(conn, header)
		if n == 0 && err == io.EOF {
			break
		} else if err != nil {
			glog.Errorf("[%s] error receiving header:%s\n", conn.RemoteAddr().String(), err)
			break
		}

		// read payload, the size of the payload is given by header
		size := binary.LittleEndian.Uint32(header)
		if size > PAYLOAD_MAX {
			// log
			glog.Error("Overflow the max size", size, PAYLOAD_MAX)
			// 超过最大长度不应该出现这样的问题，一旦出现只能关闭服务器，或者忽略这个消息内容
			// 如果忽略消息内容，应该将内容读取完毕再break

			break
		}

		data := buf[:size]
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
