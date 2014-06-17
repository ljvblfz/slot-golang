package main

import (
	"fmt"
	"github.com/cuixin/cloud/hlist"
	"github.com/golang/glog"
	"strconv"
	"sync"
)

var GUserMap = &UserMap{mu: &sync.Mutex{}, kv: make(map[int64]*hlist.Hlist, 10240)}

type UserMap struct {
	mu *sync.Mutex
	kv map[int64]*hlist.Hlist
}

func (this *UserMap) Load(users []string, host string) {
	for i := 0; i < len(users); i++ {
		uid, _ := strconv.ParseInt(users[i], 10, 64)
		h, ok := this.kv[uid]
		if !ok {
			h = hlist.New()
			this.kv[uid] = h
		}
		h.PushFront(host)
	}
	//fmt.Println(this.kv)
}

func (this *UserMap) Online(uid int64, host string) {
	this.mu.Lock()
	hostlist, ok := this.kv[uid]
	if !ok {
		hostlist = hlist.New()
		this.kv[uid] = hostlist
	}
	hostlist.PushFront(host)
	this.mu.Unlock()
}

func (this *UserMap) Offline(uid int64, host string) {
	this.mu.Lock()
	hostlist, ok := this.kv[uid]
	if !ok {
		// TODO Error log
		this.mu.Unlock()
		return
	}
	err := 0
	for e := hostlist.Front(); e != nil; e = e.Next() {
		if _host, ok := e.Value.(string); !ok {
			// TODO error log
			glog.Fatal("Fatal error")
			this.mu.Unlock()
			return
		} else {
			if _host == host {
				err++
				hostlist.Remove(e)
			}
		}
	}
	// TODO check err is equals 0
	if err != 1 {
		// LOG
		glog.Errorln(uid, "canot find in host", host)
	}
	this.mu.Unlock()
}

func (this *UserMap) PushToComet(uid int64, msg []byte) error {
	this.mu.Lock()
	hostlist, ok := this.kv[uid]
	if !ok {
		// TODO Error log
		this.mu.Unlock()
		return fmt.Errorf("%d not found", uid)
	}

	for e := hostlist.Front(); e != nil; e = e.Next() {
		if h, ok := e.Value.(string); !ok {
			// TODO error log
			this.mu.Unlock()
			return fmt.Errorf("wrong type error [%v] uid[%d]", e.Value, uid)
		} else {
			GComets.PushMsg(msg, h)
		}
	}
	this.mu.Unlock()
	return nil
}
