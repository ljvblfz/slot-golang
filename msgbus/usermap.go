package main

import (
	"fmt"
	"github.com/cuixin/cloud/hlist"
	"github.com/golang/glog"
	"strconv"
	"sync"
)

var (
	BlockSize int64 = 128
	MapSize         = 10240

	GUserMap *UserMap
)

func getBlockID(uid int64) int {
	return int(uid % BlockSize)
}

type UserMap struct {
	mu []*sync.Mutex
	kv []map[int64]*hlist.Hlist
}

func InitUserMap() {
	GUserMap = &UserMap{
		mu: make([]*sync.Mutex, BlockSize),
		kv: make([]map[int64]*hlist.Hlist, BlockSize),
	}
	for i := int64(0); i < BlockSize; i++ {
		GUserMap.mu[i] = &sync.Mutex{}
		GUserMap.kv[i] = make(map[int64]*hlist.Hlist, MapSize)
	}
}

func (this *UserMap) Load(users []string, host string) {
	for i := 0; i < len(users); i++ {
		uid, _ := strconv.ParseInt(users[i], 10, 64)
		bn := getBlockID(uid)
		h, ok := this.kv[bn][uid]
		if !ok {
			h = hlist.New()
			this.kv[bn][uid] = h
		}
		h.PushFront(host)
	}
}

func (this *UserMap) Online(uid int64, host string) {
	bn := getBlockID(uid)
	this.mu[bn].Lock()
	hostlist, ok := this.kv[bn][uid]
	if !ok {
		hostlist = hlist.New()
		this.kv[bn][uid] = hostlist
	}
	hostlist.PushFront(host)
	this.mu[bn].Unlock()
}

func (this *UserMap) Offline(uid int64, host string) {
	bn := getBlockID(uid)
	this.mu[bn].Lock()
	hostlist, ok := this.kv[bn][uid]
	if !ok {
		// TODO Error log
		this.mu[bn].Unlock()
		return
	}
	err := 0
	for e := hostlist.Front(); e != nil; e = e.Next() {
		if _host, ok := e.Value.(string); !ok {
			// TODO error log
			glog.Fatal("Fatal error")
			this.mu[bn].Unlock()
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
	this.mu[bn].Unlock()
}

func (this *UserMap) GetUserComet(uid int64) (*hlist.Hlist, error) {
	bn := getBlockID(uid)
	this.mu[bn].Lock()
	hosts, ok := this.kv[bn][uid]
	if !ok {
		this.mu[bn].Unlock()
		return nil, fmt.Errorf("Cannot found %d comet", uid)
	}
	newList := hlist.New()
	for e := hosts.Front(); e != nil; e = e.Next() {
		// glog.Info("UID", uid, e.Value)
		newList.PushFront(e.Value)
	}
	this.mu[bn].Unlock()
	return newList, nil
}

func (this *UserMap) BroadToComet(addr string, msg []byte) error {
	err := GComets.PushMsg(msg, addr)
	return err
}

func (this *UserMap) PushToComet(uid int64, msg []byte) error {
	bn := getBlockID(uid)
	this.mu[bn].Lock()
	hostlist, ok := this.kv[bn][uid]
	if !ok {
		// TODO Error log
		this.mu[bn].Unlock()
		return fmt.Errorf("%d not found", uid)
	}
	var err error
	for e := hostlist.Front(); e != nil; e = e.Next() {
		if h, ok := e.Value.(string); !ok {
			// TODO error log
			this.mu[bn].Unlock()
			return fmt.Errorf("wrong type error [%v] uid[%d]", e.Value, uid)
		} else {
			err = GComets.PushMsg(msg, h)
		}
	}
	this.mu[bn].Unlock()
	return err
}
