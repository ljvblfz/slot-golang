package main

import (
	"encoding/binary"
	"github.com/golang/glog"
)

func MainHandle(msg []byte) {
	uid := binary.LittleEndian.Uint64(msg[:8])
	GUserMap.PushToComet(int64(uid), msg)
	glog.Info(msg)
}

func HandleClose(host string) {
	glog.Infof("server closed")
}
