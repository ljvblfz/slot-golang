package main

import (
	//"cloud-base/hlist"
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

type UserMap struct {
	mu *sync.RWMutex
	kv map[int64]string
}

func InitUserMap() {
	GUserMap = &UserMap{
		mu: new(sync.RWMutex),
		kv: make(map[int64]string, BlockSize),
	}
}

func (this *UserMap) load(users []string, host string) {
	for i := 0; i < len(users); i++ {
		uid, _ := strconv.ParseInt(users[i], 10, 64)
		this.kv[uid] = host
		glog.Infoln("load", uid, host)
		if glog.V(1) {
			glog.Infof("[load online] user %d on %s", uid, host)
		}
	}
}

func (this *UserMap) Online(uid int64, host string) {
	this.mu.Lock()
	glog.Infoln("online", uid, host)
	this.kv[uid] = host
	this.mu.Unlock()
}

func (this *UserMap) Offline(uid int64, host string) {
	this.mu.Lock()
	delete(this.kv, uid)
	this.mu.Unlock()
}

func (this *UserMap) OfflineHost(host string) {
	this.mu.Lock()
	for uid, _host := range this.kv {
		err := 0
		if _host == host {
			err++
			delete(this.kv, uid)
		}
		if err != 1 {
			glog.Errorf("[offline] id %d: count %d with host %s", uid, err, host)
		}
	}
	this.mu.Unlock()
}

func (this *UserMap) GetUserComet(uid int64) (string, error) {
	this.mu.RLock()
	adr, ok := this.kv[uid]
	if !ok {
		this.mu.RUnlock()
		return "", fmt.Errorf("[bus|err] Cannot found %d comet", uid)
	}
	this.mu.RUnlock()
	return adr, nil
}

func (this *UserMap) PushToComet(uid int64, msg []byte) error {
	this.mu.Lock()
	wscomet, ok := this.kv[uid]
	if !ok {
		this.mu.Unlock()
		return fmt.Errorf("[bus|err] user %d not found", uid)
	}
	var err error
	err = GComets.PushMsg(msg, wscomet)
	if err != nil {
		glog.Errorf("[bus|down] to: (%d)%v", uid, wscomet)
	} else {
		statIncDownStreamOut()
		if glog.V(3) {
			glog.Infof("[bus|down] to: %d, data: (len: %d)%v", uid, len(msg), msg)
		} else if glog.V(2) {
			glog.Infof("[bus|down] to: %d, data: (len: %d)%v", uid, len(msg), msg[:3])
		}
	}
	this.mu.Unlock()
	return err
}

func (this *UserMap) GetAll() map[string]string {
	usermap := make(map[string]string)
	this.mu.RLock()
	for k, v := range this.kv {
		if v == "" {
			continue
		}
		id := strconv.FormatInt(k, 10)
		usermap[id] = v
	}
	this.mu.RUnlock()
	return usermap
}
