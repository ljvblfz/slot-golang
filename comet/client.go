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
			id := int64(binary.LittleEndian.Uint64(msg[start : start+8]))
			gSessionList.PushMsg(id, data)
		}
	} else if gCometType == msgs.CometUdp {
		for i := uint16(0); i < size; i++ {
			start := i * 8
			id := int64(binary.LittleEndian.Uint64(msg[start : start+8]))
			err := gUdpSessions.PushMsg(id, data)
			if err != nil {
				glog.Errorf("[udp:sended] fail to device: %d , data: len(%d)%v| error: %v", id, len(data), data, err)
			} else {
				glog.Infof("[udp:sended] success to device: %d , data: len(%d)%v", id, len(data), data)
			}
		}
	}
}
