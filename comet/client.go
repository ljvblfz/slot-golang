package main

import (
	"encoding/binary"

	"cloud-socket/msgs"
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

	if gCometType == msgs.CometWs {
		for i := uint16(0); i < size; i++ {
			start := i * 8
			id := binary.LittleEndian.Uint64(msg[start : start+8])
			gSessionList.PushMsg(int64(id), data)
		}
	} else if gCometType == msgs.CometUdp {
		for i := uint16(0); i < size; i++ {
			start := i * 8
			id := binary.LittleEndian.Uint64(msg[start : start+8])
			gUdpSessions.PushMsg(int64(id), data)
		}
	}
}
