package main

import (
	"encoding/binary"
	"github.com/golang/glog"
)

func HandleMsg(msg []byte) {
	// glog.Info(msg)
	if len(msg) <= 8 {
		glog.Fatalf("Invalidate msg length: [%d bytes] %v", len(msg), msg)
	}
	uid := binary.LittleEndian.Uint64(msg[:8])
	gSessionList.PushMsg(int64(uid), msg[8:])
}
