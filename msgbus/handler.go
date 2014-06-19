package main

import (
	"encoding/binary"
	"github.com/golang/glog"
)

func MainHandle(msg []byte) {
	uid := binary.LittleEndian.Uint64(msg[:8])
	err := GUserMap.PushToComet(int64(uid), msg)
	if err != nil {
		glog.Errorf("Push to comet failed, [%d] %v", uid, err)
	}
	//glog.Info(string(msg[8:]))
}

func HandleClose(host string) {
	glog.Infof("[%s] server closed", host)
	GComets.RemoveServer(hostName(host))
}
