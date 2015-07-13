package main

import (
	// "bufio"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"sync"

	"cloud-socket/msgs"
	"github.com/golang/glog"
)

const (
	HEADER_SIZE = 4
	INIT_MAX    = 1024
	PAYLOAD_MAX = 32 * 1024
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

func (this *Server) login(conn *net.TCPConn) (msgs.CometType, error) {
	buf := make([]byte, 1)
	n, err := conn.Read(buf)
	if err != nil || n != 1 {
		return 0, fmt.Errorf("Read info error [%s]<-[%s] [%s]\n", conn.LocalAddr(), conn.RemoteAddr(), err.Error())
	}
	switch msgs.CometType(buf[0]) {
	case msgs.CometWs:
	case msgs.CometUdp:
	default:
		return 0, fmt.Errorf("wrong CometType %v", buf[0])
	}
	_, err = conn.Write(buf)
	if err != nil {
		return 0, fmt.Errorf("Send ack error [%s]->[%s] [%s]\n", conn.LocalAddr(), conn.RemoteAddr(), err.Error())
	}
	return msgs.CometType(buf[0]), nil
}

func (this *Server) handleClient(conn *net.TCPConn) {
	defer this.wg.Done()
	defer conn.Close()
	addr, err := net.ResolveTCPAddr(conn.RemoteAddr().Network(), conn.RemoteAddr().String())
	if err != nil {
		glog.Errorf("ResolveTCPAddr failed [%v], %v", conn.RemoteAddr(), err)
		return
	}

	// check login
	cometType, err := this.login(conn)
	if err != nil {
		glog.Errorf("Login comet failed [%v], %v", conn.RemoteAddr(), err)
		return
	}

	GComets.AddServer(addr.IP.String(), conn, cometType)
	header := make([]byte, HEADER_SIZE)
	var bufLen uint32 = INIT_MAX
	buf := make([]byte, bufLen)
	for {
		// TODO 这里有一个可优化的空间，在接受数据之前创建一个buffer，前四字节是长度，后面
		// 该长度的字节是内容，然后将这个buffer整体传给后续的处理程序，后续的转发就可以直接
		// 使用该buffer转发内容，不需要再单独拼接长度头和内容，或者调用两次发送，这样避免了
		// 多余的一次内存分配或多余的一次系统调用

		// read header : 4-bytes
		n, err := io.ReadFull(conn, header)
		glog.Infoln("来货了")
		if n == 0 && err == io.EOF {
			break
		} else if err != nil {
			glog.Errorf("[%s] error receiving header:%s\n", conn.RemoteAddr().String(), err)
			break
		}

		// read payload, the size of the payload is given by header
		size := binary.LittleEndian.Uint32(header)
		glog.Infof("[mb:received] msg length:%v", size)
		if size > bufLen {
			if size > PAYLOAD_MAX {
				// 数据意外的过长，扔掉本次消息
				glog.Errorf("[msg] discard this message from comet [%v] too long, %d bytes", conn.RemoteAddr(), size)
				_, err = io.CopyN(ioutil.Discard, conn, int64(size))
				if err != nil {
					break
				}
				continue
			}
			bufLen = size
			buf = make([]byte, bufLen)
		}

		data := buf[:size]
		glog.Infof("[mb:received] msg %v", data)
		n, err = io.ReadFull(conn, data)

		if err != nil {
			glog.Errorf("error receiving payload:%s\n", err)
			break
		}
		MainHandle(data)
	}
	HandleClose(addr.String()) // 释放服务器所有用户的信息
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
