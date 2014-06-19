package main

import (
	"encoding/binary"
	"github.com/golang/glog"
	"net"
	"strings"
	"sync"
)

var HostPrefix = "Host:"

var GComets = NewComets()

type CometServer struct {
	mu   *sync.Mutex
	kv   map[int64]struct{} // uid
	conn *net.TCPConn
}

func (this *CometServer) addUser(uid int64) {
	this.mu.Lock()
	this.kv[uid] = struct{}{}
	this.mu.Unlock()
}

func (this *CometServer) delUser(uid int64) {
	this.mu.Lock()
	delete(this.kv, uid)
	this.mu.Unlock()
}

func (this *CometServer) Push(msg []byte) (err error) {
	this.mu.Lock()
	// 这里不用buffer，是为了避免一次额外的内存分配，以时间换空间
	err = binary.Write(this.conn, binary.LittleEndian, uint32(len(msg)))
	if err == nil {
		err = binary.Write(this.conn, binary.LittleEndian, msg)
	}
	this.mu.Unlock()
	return
}

type Comets struct {
	mu      *sync.Mutex
	Servers map[string]*CometServer // addr -> comet
}

func NewComets() *Comets {
	return &Comets{mu: &sync.Mutex{}, Servers: make(map[string]*CometServer, 512)}
}

// host不包括Host:等前缀
func (this *Comets) AddServer(host string, conn *net.TCPConn) {
	this.mu.Lock()
	this.Servers[host] = &CometServer{mu: &sync.Mutex{}, kv: make(map[int64]struct{}, 10240), conn: conn}
	this.mu.Unlock()
}

// addr传入的是ip
func (this *Comets) RemoveServer(host string) {
	this.mu.Lock()
	if _, ok := this.Servers[host]; ok {
		delete(this.Servers, host)
	} else {
		// TODO add error log
		glog.Errorf("[%s] Removed", host)
	}
	this.mu.Unlock()
}

func (this *Comets) AddUserToHost(uid int64, host string) {
	this.mu.Lock()
	if h, ok := this.Servers[host]; ok {
		h.addUser(uid)
	} else {
		// TODO add error log
		glog.Errorf("[%s] duplicated add user [%d]", host, uid)
	}
	this.mu.Unlock()
}

func (this *Comets) RemoveUserFromHost(uid int64, host string) {
	this.mu.Lock()
	if h, ok := this.Servers[host]; ok {
		h.delUser(uid)
	} else {
		// TODO add error log
		glog.Errorf("[%s] cannot find user [%d]", host, uid)
	}
	this.mu.Unlock()
}

func (this *Comets) PushMsg(msg []byte, host string) {
	this.mu.Lock()
	server, ok := this.Servers[host]
	if !ok {
		glog.Errorf("unexpected uninitialized server(host: %s), %v", host, this.Servers)
	}
	//glog.Info(host, msg)
	server.Push(msg)
	this.mu.Unlock()
}

// hostName Comets中Servers存储的字段来自tcp连接的ip，但来自redis的host名为Host:ip
func hostName(hostName string) string {
	n := strings.Index(hostName, HostPrefix)
	if n != 0 {
		return hostName
	}
	return hostName[len(HostPrefix):]
}
