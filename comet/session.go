package main

import (
	"code.google.com/p/go.net/websocket"
	"github.com/cuixin/cloud/hlist"
	"github.com/golang/glog"
	"sync"
)

var (
	BlockSize int64 = 128
	MapSize   int64 = 1024
)

func getBlockID(uid int64) int64 {
	if uid > 0 {
		return uid % BlockSize
	} else {
		return -uid % BlockSize
	}
}

type Session struct {
	Uid       int64
	BindedIds []int64
	Conn      *websocket.Conn
}

func NewSession(uid int64, bindedIds []int64, conn *websocket.Conn) *Session {
	return &Session{Uid: uid, BindedIds: bindedIds, Conn: conn}
}

func (this *Session) Close() {
	this.Conn.Close()
}

func (this *Session) IsBinded(id int64) bool {
	for _, v := range this.BindedIds {
		if v == id {
			return true
		}
	}
	return false
}

type SessionList struct {
	mu []*sync.Mutex
	kv []map[int64]*hlist.Hlist
}

func InitSessionList() *SessionList {
	sl := &SessionList{
		mu: make([]*sync.Mutex, BlockSize),
		kv: make([]map[int64]*hlist.Hlist, BlockSize),
	}

	for i := int64(0); i < BlockSize; i++ {
		sl.mu[i] = &sync.Mutex{}
		sl.kv[i] = make(map[int64]*hlist.Hlist, MapSize)
	}
	return sl
}

func (this *SessionList) AddSession(s *Session) *hlist.Element {
	// 能想到的错误返回值是同一用户，同一mac多次登录，但这可能不算错误
	blockId := getBlockID(s.Uid)
	this.mu[blockId].Lock()
	h, ok := this.kv[blockId][s.Uid]
	var e *hlist.Element
	if ok {
		e = h.PushFront(s)
	} else {
		h = hlist.New()
		this.kv[blockId][s.Uid] = h
		e = h.PushFront(s)
	}
	this.mu[blockId].Unlock()
	return e
}

func (this *SessionList) RemoveSession(e *hlist.Element) {
	s, _ := e.Value.(*Session)
	blockId := getBlockID(s.Uid)
	if s.Conn != nil {
		s.Close()
	}
	this.mu[blockId].Lock()
	list, ok := this.kv[blockId][s.Uid]
	if ok {
		list.Remove(e)
		if list.Len() == 0 {
			delete(this.kv[blockId], s.Uid)
		}
	}
	this.mu[blockId].Unlock()
}

func (this *SessionList) PushMsg(uid int64, data []byte) {
	blockId := getBlockID(uid)

	lock := this.mu[blockId]
	lock.Lock()

	if list, ok := this.kv[blockId][uid]; ok {
		for e := list.Front(); e != nil; e = e.Next() {
			if session, ok := e.Value.(*Session); !ok {
				lock.Unlock()
				return
			} else {
				if len(data) <= 24 {
					glog.Errorf("[invalid data] [uid: %d] length less than 25 (%d)%v", uid, len(data), data)
				}
				err := websocket.Message.Send(session.Conn, data)
				if err != nil {
					// 不要在这里移除用户session，用户的websocket连接会处理这个情况
					if glog.V(1) {
						glog.Infof("[push failed] uid: %d, error: %v", session.Uid, err)
					}
				}
			}
		}
	}
	lock.Unlock()
}
