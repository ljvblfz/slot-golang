package main

import (
	"cloud-base/hlist"
	"fmt"
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
	if uid > 0 {
		return int(uid % BlockSize)
	} else {
		return int(-uid % BlockSize)
	}
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
		if glog.V(1) {
			glog.Infof("[load online] user %d on %s", uid, host)
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
	doAdd := true
	if hostlist.Len() > 0 {
		for e := hostlist.Front(); e != nil; e = e.Next() {
			if _host, ok := e.Value.(string); ok {
				if _host == host {
					doAdd = false
					break
				}
			}
		}
	}
	if doAdd {
		hostlist.PushFront(host)
	} else {
		glog.Warningf("[online|repeat] id %d online repeatly on host %v which could be a mistake between LoadAllUser and SubOnlineUser", uid, host)
	}
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
		glog.Errorln("id", uid, "can't be found in host", host)
	}
	this.mu[bn].Unlock()
}

func (this *UserMap) OfflineHost(host string) {
	for bn := int64(0); bn < BlockSize; bn++ {
		this.mu[bn].Lock()
		for uid, hostlist := range this.kv[bn] {
			if hostlist == nil {
				continue
			}
			err := 0
			for e := hostlist.Front(); e != nil; e = e.Next() {
				_host, ok := e.Value.(string)
				if !ok {
					glog.Fatal("Fatal error")
					continue
				}
				if _host == host {
					err++
					hostlist.Remove(e)
				}
			}
			if err != 1 {
				glog.Errorf("[offline] id %d: count %d with host %s", uid, err, host)
			}
		}
		this.mu[bn].Unlock()
	}
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

func (this *UserMap) PushToComet(uid int64, msg []byte) error {
	bn := getBlockID(uid)
	this.mu[bn].Lock()
	hostlist, ok := this.kv[bn][uid]
	if !ok {
		this.mu[bn].Unlock()
		return fmt.Errorf("user %d not found", uid)
	}
	var err error
	for e := hostlist.Front(); e != nil; e = e.Next() {
		if h, ok := e.Value.(string); !ok {
			this.mu[bn].Unlock()
			return fmt.Errorf("wrong type error [%v] uid[%d]", e.Value, uid)
		} else {
			err = GComets.PushMsg(msg, h)
			if err != nil {
				glog.Errorf("[msg|down] to: (%d)%v", 1, h)
			} else {
				statIncDownStreamOut()
				if glog.V(3) {
					glog.Infof("[msg|down] to: %d, data: (len: %d)%v", uid, len(msg), msg)
				} else if glog.V(2) {
					glog.Infof("[msg|down] to: %d, data: (len: %d)%v", uid, len(msg), msg[:3])
				}
			}
		}
	}
	this.mu[bn].Unlock()
	return err
}

func (this *UserMap) GetAll() map[string][]string {
	usermap := make(map[string][]string)
	for block := 0; block < len(this.kv); block++ {
		this.mu[block].Lock()
		for k, v := range this.kv[block] {
			if v == nil {
				continue
			}
			for p := v.Front(); p != nil; p = p.Next() {
				h, ok := p.Value.(string)
				if !ok {
					glog.Errorf("Type error, [%v] should be string of ip", p.Value)
					continue
				}
				id := strconv.FormatInt(k, 10)
				usermap[id] = append(usermap[id], h)
			}
		}
		this.mu[block].Unlock()
	}
	return usermap
}
