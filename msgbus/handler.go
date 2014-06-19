package main

import (
	"encoding/binary"
	"github.com/golang/glog"
)

func MainHandle(msg []byte) {
	toSize := binary.LittleEndian.Uint16(msg[:2])
	toIds := msg[2 : toSize*8]
	data := msg[2+toSize*8:]
	for i := uint16(0); i < toSize; i++ {
		uid := binary.LittleEndian.Uint64(toIds[i*8 : i*8+8])
		err := GUserMap.PushToComet(int64(uid), data)
		if err != nil {
			glog.Errorf("Push to comet failed, [%d] %v", uid, err)
		}
	}
	//glog.Info(string(msg[8:]))
}

func HandleClose(host string) {
	glog.Infof("[%s] server closed", host)
	GComets.RemoveServer(hostName(host))
}
