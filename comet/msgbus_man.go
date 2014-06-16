package main

import (
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
	glog.Info(s.conn.RemoteAddr(), "has been closed")
	s.conn.Close()
}

func (this *MsgBusManager) Online(addr string) {
	this.mu.Lock()

	g := NewMsgBusServer(addr)
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
				glog.Info("Removed", s.conn.RemoteAddr(), "OK")
				this.list.Remove(e)
				break
			}
		}
	}
	this.mu.Unlock()
}

func (this *MsgBusManager) Push2Backend(msg []byte) {
	this.mu.Lock()
	this.curr.Value.(*MsgBusServer).Send(msg)
	next := this.curr.Next()
	if next != nil {
		this.curr = next
	} else {
		this.curr = this.head
	}
	this.mu.Unlock()
}
