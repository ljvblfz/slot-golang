package main

import (
	"encoding/binary"

	"cloud-base/hlist"
	"cloud-base/rmqs"
	"cloud-socket/msgs"
	"github.com/golang/glog"
)

const (
	// 消息ID偏移位置，根据手机与板子的协议，整个消息中，MsgId前包括20字节的转发头，
	// 24字节的帧头，数据头中的4字节其他数据
	kMsgIdOffset = 20 + 24 + 4
	kMsgIdLen    = 2
	kMsgIdEnd    = kMsgIdOffset + kMsgIdLen
)

var (
	GRmqs = rmqs.NewRmqs(onNewRmq, onClosedRmq, onSentMsg)
)

func onNewRmq(addr string) {
	statIncRmqCount()
}

func onClosedRmq(addr string) {
	statDecRmqCount()
}

func onSentMsg(msg []byte) {
	statIncMsgToRmq()
}

func MainHandle(srcMsg []byte) {
	statIncUpStreamIn()

	srcId := int64(binary.LittleEndian.Uint64(srcMsg[:8]))
	msg := srcMsg[8:]
	idsSize := binary.LittleEndian.Uint16(msg[:2])
	toIds := msg[2 : 2+idsSize*8]
	data := msg[2+idsSize*8:]

	if glog.V(2) {
		var ids []int64
		for i := uint16(0); i < idsSize; i++ {
			ids = append(ids, int64(binary.LittleEndian.Uint64(toIds[i*8:i*8+8])))
		}
		glog.Infof("[msg|in] from: %d, ids count: %d, to ids: %v, total len: %d, data: %v...", srcId, idsSize, ids, len(msg), msg[:3])
	}

	if srcId < 0 {
		shouldForward := msgs.IsForwardType(data)

		// Write into rabbitmq
		if !shouldForward {
			if len(data) >= kMsgIdEnd {
				msgId := int(binary.LittleEndian.Uint16(data[kMsgIdOffset:kMsgIdEnd]))
				GRmqs.Push(data, msgId)
			}

			id := msgs.ForwardSrcId(data)
			m := msgs.NewAckMsg(id, data)
			ackMsg := m.MarshalBytes()
			pushBuf := make([]byte, 2+8+len(ackMsg))
			binary.LittleEndian.PutUint16(pushBuf[:8], 1)
			binary.LittleEndian.PutUint64(pushBuf[2:2+8], uint64(id))
			copy(pushBuf[2+8:], ackMsg)
			err := GUserMap.PushToComet(id, pushBuf)
			if err != nil {
				statIncDownStreamOutBad()
				glog.Errorf("[msg|ack] ACK to [%d] error: %v", id, err)
			} else {
				statIncRmqCount()
			}

			return
		}
	}

	if idsSize == 1 {
		uid := int64(binary.LittleEndian.Uint64(toIds))
		err := GUserMap.PushToComet(uid, msg)
		if err != nil {
			statIncDownStreamOutBad()
			glog.Errorf("Push to comet failed, [%d] %v", uid, err)
		}
		return
	}
	smap := make(map[string]*hlist.Hlist, idsSize)

	for i := uint16(0); i < idsSize; i++ {
		uid := int64(binary.LittleEndian.Uint64(toIds[i*8 : i*8+8]))
		cometHosts, err := GUserMap.GetUserComet(uid)
		if err != nil {
			if glog.V(4) {
				glog.Errorf("id: %d, error: %v", uid, err)
			}
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
		}
	}
	for k, v := range smap {
		vSize := uint16(v.Len())
		pushData := make([]byte, 2+vSize*8+uint16(len(data)))
		binary.LittleEndian.PutUint16(pushData[:2], vSize)
		i := 0
		var idsDebug []int64
		for e := v.Front(); e != nil; e = e.Next() {
			id, _ := e.Value.(int64)
			binary.LittleEndian.PutUint64(pushData[2+i*8:2+i*8+8], uint64(id))
			idsDebug = append(idsDebug, id)
			i++
		}
		copy(pushData[2+vSize*8:], data)

		err := GComets.PushMsg(pushData, k)
		if err != nil {
			glog.Errorf("[msg|down] Broadcast to comet failed, comet: %s, ids: %s, err: %v", k, v, err)
		} else {
			if glog.V(3) {
				glog.Infof("[msg|down] to comet: %s, ids: %s, data: (len: %d)%v", k, v, len(msg), pushData)
			} else {
				glog.Infof("[msg|down] to comet: %s, ids: %s, data: (len: %d)%v...", k, v, len(msg), pushData[:3])
			}
		}
	}
}

func HandleClose(host string) {
	glog.Infof("[%s] server closed", host)
	GComets.RemoveServer(hostName(host))
}
