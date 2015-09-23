package main

import (
	"net"
	"sync"
	"github.com/golang/glog"
	"crypto/md5"
	"encoding/hex"
	"cloud-socket/msgs"
)

const (
	kMaxPackageSize = 10240
	
)
var (
	md5Ctx = md5.New()
)
func begmd5(ctn []byte) []byte {
	defer md5Ctx.Reset()
	md5Ctx.Write(ctn)
	cipher := md5Ctx.Sum(nil)
	return cipher
}
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
		//请求报文不够长，24为帧头长度，10为数据头长度，16为mac的md5长度，32为登录代理用的key长度
		if n<24+10+16+32 {
			continue
		}
		params :=input[0:n]
		pMac:=params[16:24]
		pMacMd5:=params[24+10:24+10+16]
//		pLoginProxyKey:=params[24+10+16:24+10+16+32]
		macmd5:=begmd5(pMac)
		//校验mac
		if hex.EncodeToString(pMacMd5)!=hex.EncodeToString(macmd5){
			continue
		}
		pHeadChecksum:=params[24+6]
		pDataChecksum:=params[24+6+1]
		head:=params[:24+6]
		data:=params[24+8:]
		//帧头crc检验，检验域：从0到数据关的length字段
		if pHeadChecksum != msgs.Crc(head,len(head)){
			continue
		}
		//数据crc检验，检验域：从数据头的SessinId字段到结尾
		if pDataChecksum != msgs.Crc(data,len(data)){
			continue
		}
		
		adr := chooseAUDPServer()
		this.Send(peer,adr)
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
