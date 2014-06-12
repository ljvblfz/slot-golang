package main

import (
	"encoding/binary"
)

func HandleMsg(msg []byte) {
	// glog.Info(msg)
	uid := binary.LittleEndian.Uint64(msg[:8])
	gSessionList.PushMsg(int64(uid), msg[8:])
}
