package main

import (
	"encoding/binary"
	"github.com/golang/glog"
	"io"
	"net"
)

const (
	HEADER_SIZE = 4
	PAYLOAD_MAX = 1024
)

func HandleMsg(msg []byte) {
	glog.Info(msg)
}

type MsgBusServer struct {
	addr string
	conn *net.TCPConn
}

func (this *MsgBusServer) Dail() error {
	glog.Infof("Connecting to [%s]\n", this.addr)
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
		glog.Errorf("DailTcp [%s] [%s]\n", this.addr, err.Error())
		return err
	}
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
		size := binary.BigEndian.Uint32(header)
		if size > PAYLOAD_MAX {
			glog.Errorf("[%s] overload the max[%d]>[%d]\n", this.addr, size, PAYLOAD_MAX)
			break
		}

		data := buf[:size]
		// packet seq_id uint32
		n, err = io.ReadFull(this.conn, data)
		if n == 0 && err == io.EOF {
			break
		} else if err != nil {
			glog.Errorf("[%s] error receiving [%s]", this.addr, err.Error())
			break
		}
		HandleMsg(data)
	}
}

func (this *MsgBusServer) Send(msg []byte) {
	_, err := this.conn.Write(msg)
	if err != nil {
		this.conn.Close()
	}
}
