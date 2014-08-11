package main

import (
	"encoding/binary"
	"github.com/cuixin/cloud/hlist"
	"github.com/golang/glog"
)

func MainHandle(msg []byte) {
	statIncUpStreamIn()

	idsSize := binary.LittleEndian.Uint16(msg[:2])
	toIds := msg[2 : 2+idsSize*8]
	data := msg[2+idsSize*8:]

	if glog.V(2) {
		glog.Infof("[msg|in] ids count: %d, to ids: %v, total len: %d, data: %v...", idsSize, toIds, len(msg), msg[:3])
	}

	// Write into rabbitmq
	if glog.V(2) {
		glog.Infof("[rmq] write %s", data)
	}
	GRmqs.Push(data)

	if idsSize == 1 {
		uid := int64(binary.LittleEndian.Uint64(toIds))
		err := GUserMap.PushToComet(uid, msg)
		if err != nil {
			statIncDownStreamOutBad()
			glog.Errorf("Push to comet failed, [%d] %v", uid, err)
		} else {
			statIncDownStreamOut()
		}
		return
	}
	smap := make(map[string]*hlist.Hlist, idsSize)
	//mapInfo := make(map[int32] []string)

	for i := uint16(0); i < idsSize; i++ {
		uid := int64(binary.LittleEndian.Uint64(toIds[i*8 : i*8+8]))
		cometHosts, err := GUserMap.GetUserComet(uid)
		if err != nil {
			glog.Errorf("id: %d, error: %v", uid, err)
			continue
		}
		for e := cometHosts.Front(); e != nil; e = e.Next() {
			haddr, _ := e.Value.(string)
			hl, ok := smap[haddr]
			if !ok {
				hl = hlist.New()
				smap[haddr] = hl
			}
			hl.PushFront(uid)
			//mapInfo[uid] = append(mapInfo[uid], haddr)
		}
	}
	//glog.Infof("[msg|down] to: (%d)%v", len(mapInfo), mapInfo)
	for k, v := range smap {
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
		statIncDownStreamOut()
	}
}

func HandleClose(host string) {
	glog.Infof("[%s] server closed", host)
	GComets.RemoveServer(hostName(host))
}
