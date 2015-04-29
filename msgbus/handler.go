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
			msgId := msgs.GetMsgId(data)
			GRmqs.Push(data, int(msgId))

			id := msgs.ForwardSrcId(data)

			ackMsg := msgs.NewAckMsg(data)
			pushBuf := make([]byte, 2+8+len(ackMsg))
			binary.LittleEndian.PutUint16(pushBuf[:8], 1)
			binary.LittleEndian.PutUint64(pushBuf[2:2+8], uint64(id))
			copy(pushBuf[2+8:], ackMsg)

			err := GUserMap.PushToComet(id, pushBuf)
			if err != nil {
				statIncDownStreamOutBad()
				glog.Errorf("[msg|ack] ACK to [%d] error: %v", id, err)
			}

			return
		}
	}

	if idsSize == 1 {
		uid := int64(binary.LittleEndian.Uint64(toIds))
		var err error
		if uid > 0 {
			err = GUserMap.PushToComet(uid, msg)
		} else if uid < 0 {
			err = GComets.PushUdpMsg(msg)
			if err == nil {
				if glog.V(3) {
					glog.Infof("[msg|down] to: %d, data: (len: %d)%v", uid, len(msg), msg)
				} else if glog.V(2) {
					glog.Infof("[msg|down] to: %d, data: (len: %d)%v", uid, len(msg), msg[:3])
				}
			}
		}
		if err != nil {
			statIncDownStreamOutBad()
			glog.Errorf("Push to comet failed, [%d] %v", uid, err)
		}
		return
	}

	// key: cometName, value: idList
	wsCometToIds := make(map[string]*hlist.Hlist, idsSize)
	var udpIds *hlist.Hlist

	for i := uint16(0); i < idsSize; i++ {
		id := int64(binary.LittleEndian.Uint64(toIds[i*8 : i*8+8]))
		if id > 0 {
			cometHosts, err := GUserMap.GetUserComet(id)
			if err != nil {
				if glog.V(4) {
					glog.Errorf("id: %d, error: %v", id, err)
				}
				continue
			}
			for e := cometHosts.Front(); e != nil; e = e.Next() {
				haddr, _ := e.Value.(string)
				hl, ok := wsCometToIds[haddr]
				if !ok {
					hl = hlist.New()
					wsCometToIds[haddr] = hl
				}
				hl.PushFront(id)
			}
		} else if id < 0 {
			if udpIds == nil {
				udpIds = hlist.New()
			}
			udpIds.PushFront(id)
		}
	}
	for k, v := range wsCometToIds {
		vSize := uint16(v.Len())
		pushData := make([]byte, 2+vSize*8+uint16(len(data)))
		binary.LittleEndian.PutUint16(pushData[:2], vSize)
		//i := 0
		//var idsDebug []int64
		//for e := v.Front(); e != nil; e = e.Next() {
		//	id, _ := e.Value.(int64)
		//	binary.LittleEndian.PutUint64(pushData[2+i*8:2+i*8+8], uint64(id))
		//	idsDebug = append(idsDebug, id)
		//	i++
		//}
		copy(pushData[2+vSize*8:], data)

		err := GComets.PushMsg(pushData, k)
		if err != nil {
			glog.Errorf("[msg|down] Broadcast to websocket comet failed, comet: %s, ids: %s, err: %v", k, v, err)
		} else {
			if glog.V(3) {
				glog.Infof("[msg|down] to comet: %s, ids: %s, data: (len: %d)%v", k, v, len(msg), pushData)
			} else {
				glog.Infof("[msg|down] to comet: %s, ids: %s, data: (len: %d)%v...", k, v, len(msg), pushData[:3])
			}
		}
	}
	if udpIds != nil {
		vSize := uint16(udpIds.Len())
		pushData := make([]byte, 2+vSize*8+uint16(len(data)))
		binary.LittleEndian.PutUint16(pushData[:2], vSize)
		copy(pushData[2+vSize*8:], data)

		err := GComets.PushUdpMsg(pushData)
		if err != nil {
			glog.Errorf("[msg|down] Broadcast to udp comet failed, ids: %s, err: %v", udpIds, err)
		} else {
			if glog.V(3) {
				glog.Infof("[msg|down] to udp comet failed, ids: %s, data: (len: %d)%v", udpIds, len(msg), pushData)
			} else {
				glog.Infof("[msg|down] to udp comet failed, ids: %s, data: (len: %d)%v...", udpIds, len(msg), pushData[:3])
			}
		}
	}
}

func HandleClose(host string) {
	glog.Infof("[%s] server closed", host)
	GComets.RemoveServer(hostName(host))
}
