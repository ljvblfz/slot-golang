package main

import (
	"encoding/binary"
	"github.com/cuixin/cloud/hlist"
	"github.com/golang/glog"
	"sync"
)

type MsgBusManager struct {
	list *hlist.Hlist
	curr *hlist.Element
	head *hlist.Element
	mu   *sync.Mutex
}

var GMsgBusManager = NewMsgBusManager()

func NewMsgBusManager() *MsgBusManager {
	return &MsgBusManager{list: hlist.New(), mu: &sync.Mutex{}}
}

func onMsgBusCloseEvent(s *MsgBusServer) {
	glog.Infof("[%s] closed", s.conn.RemoteAddr())
	GMsgBusManager.Offline(s)
}

func (this *MsgBusManager) Online(remoteAddr string) {
	this.mu.Lock()
	for e := this.list.Front(); e != nil; e = e.Next() {
		msgbus, _ := e.Value.(*MsgBusServer)
		if msgbus.conn.RemoteAddr().String() == remoteAddr {
			this.mu.Unlock()
			return
		}
	}

	g := NewMsgBusServer(gLocalAddr, remoteAddr)
	if g.Dail() == nil {
		go g.Reciver(onMsgBusCloseEvent)
	}
	e := this.list.PushFront(g)
	if this.list.Len() == 1 {
		this.head = e
		this.curr = e
	}
	this.mu.Unlock()
}

func (this *MsgBusManager) Offline(s *MsgBusServer) {
	this.mu.Lock()
	for e := this.list.Front(); e != nil; e = e.Next() {
		if srv, ok := e.Value.(*MsgBusServer); !ok {
			glog.Error("Fatal error on msg bus")
			this.mu.Unlock()
			return
		} else {
			if srv == s {
				glog.Infof("[%s] removed ok", s.conn.RemoteAddr())
				this.list.Remove(e)
				break
			}
		}
	}
	this.mu.Unlock()
}

func (this *MsgBusManager) Push2Backend(ids []int64, msg []byte) {
	size := uint16(len(ids))
	pushData := make([]byte, 2+size*8+uint16(len(msg)))
	binary.LittleEndian.PutUint16(pushData[:2], size)
	idsData := pushData[2 : 2+size*8]
	for i := uint16(0); i < size; i++ {
		binary.LittleEndian.PutUint64(idsData[i*8:i*8+8], uint64(ids[i]))
	}
	copy(pushData[2+size*8:], msg)

	this.mu.Lock()
	this.curr.Value.(*MsgBusServer).Send(pushData)
	next := this.curr.Next()
	if next != nil {
		this.curr = next
	} else {
		this.curr = this.head
	}
	this.mu.Unlock()
}
