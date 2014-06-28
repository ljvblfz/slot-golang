package main

import (
	"encoding/binary"
	"github.com/golang/glog"
)

func HandleMsg(msg []byte) {
	if len(msg) < 2 {
		glog.Fatalf("Invalidate msg length: [%d bytes] %v", len(msg), msg)
	}
	statIncDownStreamIn()
	size := binary.LittleEndian.Uint16(msg[:2])
	msg = msg[2:]
	data := msg[size*8:]
	for i := uint16(0); i < size; i++ {
		start := i * 8
		uid := binary.LittleEndian.Uint64(msg[start : start+8])
		gSessionList.PushMsg(int64(uid), data)
		statIncDownStreamOut()
		//glog.Infof("[push2] %d <- %v", uid, data)
	}
}
