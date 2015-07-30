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
	if glog.V(3) {
		glog.Infof("[COMET:MSGBUS] Dail MsgBusSrv [%s]->[%s]", this.localAddr, this.remoteAddr)
	}
	var (
		err           error
		tcpLocalAddr  net.TCPAddr
		tcpRemoteAddr *net.TCPAddr
	)

	ip, err := net.ResolveIPAddr("ip", this.localAddr)
	if err != nil || ip.IP == nil {
		glog.Fatalf("[COMET:MSGBUS] Resovle Local TcpAddr [%s], error: %v", this.localAddr, err)
	}
	tcpLocalAddr.IP = ip.IP
	//tcpLocalAddr, err = net.ResolveTCPAddr("tcp", this.localAddr)
	//if err != nil {
	//	glog.Errorf("Resovle Local TcpAddr [%s] [%s]\n", this.localAddr, err.Error())
	//	return err
	//}

	tcpRemoteAddr, err = net.ResolveTCPAddr("tcp", this.remoteAddr)
	if err != nil {
		glog.Errorf("[COMET:MSGBUS] Resovle Remote TcpAddr [%s] [%s]", this.remoteAddr, err.Error())
		return err
	}
	this.conn, err = net.DialTCP("tcp", &tcpLocalAddr, tcpRemoteAddr)
	if err != nil {
		glog.Errorf("[COMET:MSGBUS] Dail [%s]->[%s] [%s]", this.localAddr, this.remoteAddr, err.Error())
		return err
	}
	buf := make([]byte, 1)
	buf[0] = byte(gCometType)
	_, err = this.conn.Write(buf)
	if err != nil {
		glog.Errorf("[COMET:MSGBUS] Send info error [%s]->[%s] [%s]", this.localAddr, this.remoteAddr, err.Error())
		return err
	}
	n, err := this.conn.Read(buf)
	if err != nil || n != 1 {
		glog.Errorf("[COMET:MSGBUS] Read ack error [%s]<-[%s] [%s]", this.localAddr, this.remoteAddr, err.Error())
		return err
	}
	if glog.V(3) {
		glog.Infof("[COMET:MSGBUS] Conecting MsgBusSrv [%s] ok", this.remoteAddr)
	}
	return nil
}

func (this *MsgBusServer) Reciver(onCloseEventFunc func(s *MsgBusServer)) {
	defer this.conn.Close()

	header := make([]byte, HEADER_SIZE)
	buf := make([]byte, PAYLOAD_MAX)

	for {
		// header
		_, err := io.ReadFull(this.conn, header)
		if err != nil {
			glog.Errorf("[COMET:MSGBUS] %v,%v", this.remoteAddr, err)
			break
		}
		size := binary.LittleEndian.Uint32(header)
		if size > PAYLOAD_MAX {
			glog.Errorf("[COMET:MSGBUS] [%s] HeaderLength ERR [%d]>[%d]", this.remoteAddr, size, PAYLOAD_MAX)
			_, err = io.CopyN(ioutil.Discard, this.conn, int64(size))
			if err != nil {
				break
			}
			continue
		}

		data := buf[:size]
		_, err = io.ReadFull(this.conn, data)
		if glog.V(3) {
			glog.Infof("[COMET:MSGBUS] Received |%v|", data)
		}
		if err != nil {
			glog.Errorf("[COMET:MSGBUS] [%s] receiving [%v]", this.remoteAddr, err)
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
		if glog.V(3) {
			glog.Infof("[COMET:MSGBUS] FAILED!! Forward msg |%v| to MsgBusSrv.|%v|", msg, err)
		}
		this.conn.Close()
	} else {
		if glog.V(3) {
			glog.Infof("[COMET:MSGBUS] DONE!!,Forward msg|%v| to MsgBusSrv.", msg)
		}
	}
}
