package main

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"sync"

	"cloud-socket/msgs"
	"github.com/golang/glog"
)

var HostPrefix = "Host:"

var GComets = NewComets()

type CometServer struct {
	mu        *sync.Mutex
	kv        map[int64]struct{} // uid
	conn      *net.TCPConn
	cometType msgs.CometType
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

	muUdp         *sync.Mutex
	UdpCometNames []string
	ServersUdp    map[string]*CometServer // addr -> comet
}

func NewComets() *Comets {
	return &Comets{
		mu:            &sync.Mutex{},
		Servers:       make(map[string]*CometServer, 16),
		muUdp:         &sync.Mutex{},
		UdpCometNames: make([]string, 0, 16),
		ServersUdp:    make(map[string]*CometServer, 16),
	}
}

// host不包括Host:等前缀
func (this *Comets) AddServer(host string, conn *net.TCPConn, cometType msgs.CometType) {
	if cometType == msgs.CometWs {
		this.mu.Lock()
		this.Servers[host] = &CometServer{
			mu:        &sync.Mutex{},
			kv:        make(map[int64]struct{}, 10240),
			conn:      conn,
			cometType: cometType,
		}
		this.mu.Unlock()
	} else if cometType == msgs.CometUdp {
		this.muUdp.Lock()
		this.UdpCometNames = append(this.UdpCometNames, host)
		this.ServersUdp[host] = &CometServer{
			mu:        &sync.Mutex{},
			kv:        make(map[int64]struct{}, 10240),
			conn:      conn,
			cometType: cometType,
		}
		this.muUdp.Unlock()
	}
	statIncCometConns()
}

// addr传入的是ip
func (this *Comets) RemoveServer(host string) {
	found := false

	this.mu.Lock()
	if _, ok := this.Servers[host]; ok {
		found = true
		delete(this.Servers, host)
		statDecCometConns()
	} else {
		glog.Errorf("[%s] Removed", host)
	}
	this.mu.Unlock()

	if found {
		return
	}

	this.muUdp.Lock()
	if _, ok := this.ServersUdp[host]; ok {
		delete(this.ServersUdp, host)
		for k, v := range this.UdpCometNames {
			if v == host {
				this.UdpCometNames[k] = this.UdpCometNames[len(this.UdpCometNames)-1]
				this.UdpCometNames = this.UdpCometNames[:len(this.UdpCometNames)-1]
				return
			}
		}
		statDecCometConns()
	} else {
		glog.Errorf("Cannot remove [%s] not found", host)
	}
	this.muUdp.Unlock()
}

func (this *Comets) AddUserToHost(uid int64, host string) {
	this.mu.Lock()
	if h, ok := this.Servers[host]; ok {
		h.addUser(uid)
	} else {
		glog.Errorf("[%s] duplicated add user [%d]", host, uid)
	}
	this.mu.Unlock()
}

func (this *Comets) RemoveUserFromHost(uid int64, host string) {
	this.mu.Lock()
	if h, ok := this.Servers[host]; ok {
		h.delUser(uid)
	} else {
		glog.Errorf("[%s] cannot find user [%d]", host, uid)
	}
	this.mu.Unlock()
}

func (this *Comets) PushMsg(msg []byte, host string) (err error) {
	this.mu.Lock()
	server, ok := this.Servers[host]
	if !ok {
		glog.Errorf("unexpected uninitialized server(host: %s), %v", host, this.Servers)
		this.mu.Unlock()
		return fmt.Errorf("cannot find %s", host)
	} else {
		//glog.Info(host, msg)
		err = server.Push(msg)
	}
	this.mu.Unlock()
	return err
}

func (this *Comets) PushUdpMsg(msg []byte) (err error) {
	this.muUdp.Lock()

	// 简单的随机找一个的UDP comet
	host := this.UdpCometNames[rand.Intn(len(this.UdpCometNames))]
	server, ok := this.ServersUdp[host]
	if !ok {
		glog.Errorf("unexpected uninitialized server(host: %s), %v", host, this.Servers)
		this.muUdp.Unlock()
		return fmt.Errorf("cannot find %s", host)
	} else {
		err = server.Push(msg)
		if err != nil {
			glog.Errorf("[msg|down] to udp comet %s error: %v", host, err)
		} else {
			statIncDownStreamOut()
		}
	}
	this.muUdp.Unlock()
	return err
}

// hostName Comets中Servers存储的字段来自tcp连接的ip，但来自redis的host名为Host:ip
func hostName(hostName string) string {
	n := strings.Index(hostName, HostPrefix)
	if n != 0 {
		return hostName
	}
	return hostName[len(HostPrefix):]
}
