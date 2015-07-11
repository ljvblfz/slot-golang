package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"io/ioutil"
	"net"

	"github.com/golang/glog"
)

const (
	HEADER_SIZE = 4
	PAYLOAD_MAX = 1024
)

type MsgBusServer struct {
	localAddr  string
	remoteAddr string
	conn       *net.TCPConn
}

func NewMsgBusServer(localAddr, remoteAddr string) *MsgBusServer {
	return &MsgBusServer{localAddr: localAddr, remoteAddr: remoteAddr}
}

func (this *MsgBusServer) Dail() error {
	glog.Infof("Dail to [%s]->[%s]\n", this.localAddr, this.remoteAddr)
	var (
		err           error
		tcpLocalAddr  net.TCPAddr
		tcpRemoteAddr *net.TCPAddr
	)

	ip, err := net.ResolveIPAddr("ip", this.localAddr)
	if err != nil || ip.IP == nil {
		glog.Fatalf("Resovle Local TcpAddr [%s], error: %v\n", this.localAddr, err)
	}
	tcpLocalAddr.IP = ip.IP
	//tcpLocalAddr, err = net.ResolveTCPAddr("tcp", this.localAddr)
	//if err != nil {
	//	glog.Errorf("Resovle Local TcpAddr [%s] [%s]\n", this.localAddr, err.Error())
	//	return err
	//}

	tcpRemoteAddr, err = net.ResolveTCPAddr("tcp", this.remoteAddr)
	if err != nil {
		glog.Errorf("Resovle Remote TcpAddr [%s] [%s]\n", this.remoteAddr, err.Error())
		return err
	}
	this.conn, err = net.DialTCP("tcp", &tcpLocalAddr, tcpRemoteAddr)
	if err != nil {
		glog.Errorf("Dail [%s]->[%s] [%s]\n", this.localAddr, this.remoteAddr, err.Error())
		return err
	}
	buf := make([]byte, 1)
	buf[0] = byte(gCometType)
	_, err = this.conn.Write(buf)
	if err != nil {
		glog.Errorf("Send info error [%s]->[%s] [%s]\n", this.localAddr, this.remoteAddr, err.Error())
		return err
	}
	n, err := this.conn.Read(buf)
	if err != nil || n != 1 {
		glog.Errorf("Read ack error [%s]<-[%s] [%s]\n", this.localAddr, this.remoteAddr, err.Error())
		return err
	}

	// glog.Infof("Dail to [%s] ok\n", this.addr)
	return nil
}

func (this *MsgBusServer) Reciver(onCloseEventFunc func(s *MsgBusServer)) {
	defer this.conn.Close()

	header := make([]byte, HEADER_SIZE)
	buf := make([]byte, PAYLOAD_MAX)

	for {
		// header
		n, err := io.ReadFull(this.conn, header)
		if n == 0 && err == io.EOF {
			glog.Errorf("[MSGBUS:EOF] %v", this.remoteAddr)
			break
		} else if err != nil {
			glog.Errorf("[%s] error receiving header: %s\n", this.remoteAddr, err.Error())
			break
		}
		size := binary.LittleEndian.Uint32(header)
		if size > PAYLOAD_MAX {
			glog.Errorf("[%s] overload the max[%d]>[%d]\n", this.remoteAddr, size, PAYLOAD_MAX)
			_, err = io.CopyN(ioutil.Discard, this.conn, int64(size))
			if err != nil {
				break
			}
			continue
		}

		data := buf[:size]
		n, err = io.ReadFull(this.conn, data)
		glog.Infoln("[receive msg from msgbus]", data)
		if n == 0 && err == io.EOF {
			glog.Errorf("[EOF] %v", this.remoteAddr)
			break
		} else if err != nil {
			glog.Errorf("[%s] error receiving [%s]\n", this.remoteAddr, err.Error())
			break
		}
		HandleMsg(data)
	}
	onCloseEventFunc(this)
}

func (this *MsgBusServer) Send(msg []byte) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, uint32(len(msg)))
	binary.Write(buf, binary.LittleEndian, msg)
	_, err := this.conn.Write(buf.Bytes())
	if err != nil {
		glog.Infoln("forward msg error:", err)
		this.conn.Close()
	}
}
