package main

import (
	"bytes"
	"encoding/binary"
	"github.com/golang/glog"
	"io"
	"net"
)

const (
	HEADER_SIZE = 4
	PAYLOAD_MAX = 1024
)

type MsgBusServer struct {
	addr string
	conn *net.TCPConn
}

func NewMsgBusServer(addr string) *MsgBusServer {
	return &MsgBusServer{addr: addr}
}

func (this *MsgBusServer) Dail() error {
	glog.Infof("Dail to [%s]\n", this.addr)
	var (
		err     error
		tcpAddr *net.TCPAddr
	)
	tcpAddr, err = net.ResolveTCPAddr("tcp", this.addr)
	if err != nil {
		glog.Errorf("ResovleTcpAddr [%s] [%s]\n", this.addr, err.Error())
		return err
	}
	this.conn, err = net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		glog.Errorf("Dail [%s] [%s]\n", this.addr, err.Error())
		return err
	}
	// glog.Infof("Dail to [%s] ok\n", this.addr)
	return nil
}

func (this *MsgBusServer) Reciver() {
	defer this.conn.Close()

	header := make([]byte, HEADER_SIZE)
	buf := make([]byte, PAYLOAD_MAX)

	for {
		// header
		n, err := io.ReadFull(this.conn, header)
		if n == 0 && err == io.EOF {
			break
		} else if err != nil {
			glog.Errorf("[%s] error receiving header: %s\n", this.addr, err.Error())
			break
		}
		size := binary.LittleEndian.Uint32(header)
		if size > PAYLOAD_MAX {
			glog.Errorf("[%s] overload the max[%d]>[%d]\n", this.addr, size, PAYLOAD_MAX)
			break
		}

		data := buf[:size]
		n, err = io.ReadFull(this.conn, data)
		if n == 0 && err == io.EOF {
			break
		} else if err != nil {
			glog.Errorf("[%s] error receiving [%s]\n", this.addr, err.Error())
			break
		}
		HandleMsg(data)
	}
}

func (this *MsgBusServer) Send(msg []byte) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, uint32(len(msg)))
	binary.Write(buf, binary.LittleEndian, msg)
	_, err := this.conn.Write(buf.Bytes())
	if err != nil {
		this.conn.Close()
	}
}
