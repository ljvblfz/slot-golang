package main

import (
	"encoding/binary"
	"github.com/cuixin/cloud/hlist"
	"github.com/golang/glog"
)

func MainHandle(msg []byte) {
	glog.Infof("%v", msg)
	idsSize := binary.LittleEndian.Uint16(msg[:2])
	toIds := msg[2 : 2+idsSize*8]
	data := msg[2+idsSize*8:]
	if idsSize == 1 {
		uid := int64(binary.LittleEndian.Uint64(toIds))
		err := GUserMap.PushToComet(uid, data)
		if err != nil {
			glog.Errorf("Push to comet failed, [%d] %v", uid, err)
		}
		return
	}
	smap := make(map[string]*hlist.Hlist, idsSize)

	for i := uint16(0); i < idsSize; i++ {
		uid := int64(binary.LittleEndian.Uint64(toIds[i*8 : i*8+8]))
		cometHosts, err := GUserMap.GetUserComet(uid)
		if err != nil {
			glog.Errorf("%d %v", uid, err)
			continue
		}
		for e := cometHosts.Front(); e != nil; e = e.Next() {
			haddr, _ := e.Value.(string)
			hl, ok := smap[haddr]
			if !ok {
				hl = hlist.New()
				smap[haddr] = hl
			}
			glog.Info(haddr, "<<<", uid)
			hl.PushFront(uid)
		}
	}
	glog.Info("ToMap %v", smap, len(smap))
	for k, v := range smap {
		glog.Info(k, ">>>>> ", v)
		vSize := uint16(v.Len())
		pushData := make([]byte, 2+vSize*8+uint16(len(data)))
		binary.LittleEndian.PutUint16(pushData[:2], vSize)
		i := 0
		for e := v.Front(); e != nil; e = e.Next() {
			id, _ := e.Value.(int64)
			binary.LittleEndian.PutUint64(pushData[2+i*8:2+i*8+8], uint64(id))
			i++
		}
		copy(pushData[2+vSize*8:], data)
		err := GUserMap.BroadToComet(k, pushData)
		if err != nil {
			glog.Errorf("Broadcast to comet failed, [%s] %v", k, err)
		}
	}
	//glog.Info(string(msg[8:]))
}

func HandleClose(host string) {
	glog.Infof("[%s] server closed", host)
	GComets.RemoveServer(hostName(host))
}
