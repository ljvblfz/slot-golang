package main

import (
	"net"
	"sync"
)

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
	_, err = this.conn.Write(msg)
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

func (this *Comets) AddServer(addr string, conn *net.TCPConn) {
	this.mu.Lock()
	this.Servers[addr] = &CometServer{mu: &sync.Mutex{}, kv: make(map[int64]struct{}, 10240), conn: conn}
	this.mu.Unlock()
}

func (this *Comets) RemoveServer(addr string) {
	this.mu.Lock()
	delete(this.Servers, addr)
	this.mu.Unlock()
}

func (this *Comets) AddUserToHost(uid int64, host string) {
	this.mu.Lock()
	if h, ok := this.Servers[host]; ok {
		h.addUser(uid)
	} else {
		// TODO add error log
	}
	this.mu.Unlock()
}

func (this *Comets) RemoveUserFromHost(uid int64, host string) {
	this.mu.Lock()
	if h, ok := this.Servers[host]; ok {
		h.delUser(uid)
	} else {
		// TODO add error log
	}
	this.mu.Unlock()
}

func (this *Comets) PushMsg(msg []byte, host string) {
	this.mu.Lock()
	this.Servers[host].Push(msg)
	this.mu.Unlock()
}
