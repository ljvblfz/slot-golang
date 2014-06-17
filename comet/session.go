package main

import (
	"code.google.com/p/go.net/websocket"
	"github.com/cuixin/cloud/hlist"
	"sync"
)

var (
	BlockSize int64 = 128
	MapSize   int64 = 1024
)

func getBlockID(uid int64) int64 {
	return uid % BlockSize
}

type Session struct {
	Uid			int64
	Alias		string
	Mac			string
	BindedIds	[]int64
	Conn		*websocket.Conn
}

func NewSession(uid int64, alias string, mac string, bindedIds []int64, conn *websocket.Conn) *Session {
	return &Session{Uid: uid, Alias: alias, Mac: mac, BindedIds: bindedIds, Conn: conn}
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

func (this *SessionList) AddSession(s *Session) {
	// 能想到的错误返回值是同一用户，同一mac多次登录，但这可能不算错误
	blockId := getBlockID(s.Uid)
	this.mu[blockId].Lock()
	h, ok := this.kv[blockId][s.Uid]
	if ok {
		h.PushFront(s)
	} else {
		h = hlist.New()
		this.kv[blockId][s.Uid] = h
		h.PushFront(s)
	}
	this.mu[blockId].Unlock()
}

func (this *SessionList) RemoveSession(s *Session) {
	blockId := getBlockID(s.Uid)
	this.mu[blockId].Lock()
	if list, ok := this.kv[blockId][s.Uid]; ok {
		for e := list.Front(); e != nil; e = e.Next() {
			if session, ok := e.Value.(*Session); !ok {
				this.mu[blockId].Unlock()
				return
			} else {
				session.Close()
				list.Remove(e)
			}
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
				err := websocket.Message.Send(session.Conn, data)
				if err != nil {
					session.Close()
					list.Remove(e)
				}
			}
		}
	}
	lock.Unlock()
}
