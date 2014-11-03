package main

import (
	"encoding/binary"
	"sync"

	"cloud-base/hlist"
	"cloud-socket/msgs"
	"github.com/golang/glog"
)

type MsgBusManager struct {
	list *hlist.Hlist
	curr *hlist.Element
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
	//this.head = this.list.Front() 有必要缓存一个head元素？
	this.curr = e
	this.mu.Unlock()
	statIncMsgbusConns()
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
				if this.curr == e {
					this.curr = nil
				}
				statDecMsgbusConns()
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

	//glog.Infof("[push] %v", pushData)
	if this.curr != nil {
		this.curr.Value.(*MsgBusServer).Send(pushData)
		this.mu.Lock()
		next := this.curr.Next()
		if next != nil {
			this.curr = next
		} else {
			this.curr = this.list.Front()
		}
		this.mu.Unlock()
	} else {
		glog.Errorf("[msgbus] curr == nil, list: %v", this.list)
	}
	statIncUpStreamOut()
}

func (this *MsgBusManager) NotifyBindedIdChanged(deviceId int64, newBindIds []int64, unbindIds []int64) {
	m := msgs.NewAppMsg(0, deviceId, msgs.MIDBind)
	if len(newBindIds) > 0 {
		GMsgBusManager.Push2Backend(newBindIds, m.MarshalBytes())
	}
	if len(unbindIds) > 0 {
		m.SetMsgId(msgs.MIDUnbind)
		GMsgBusManager.Push2Backend(unbindIds, m.MarshalBytes())
	}
}
