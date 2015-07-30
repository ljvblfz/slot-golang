package main

import (
	"sync"

	//	"cloud-base/hlist"
	"cloud-socket/msgs"
	"fmt"
	"github.com/golang/glog"
	//"strings"
	//"strconv"
)

var (
	BlockSize int64 = 128
	MapSize   int64 = 1024
)

//func getBlockID(uid int64) int64 {
//	if uid > 0 {
//		return uid % BlockSize
//	} else {
//		return -uid % BlockSize
//	}
//}
//type SessionList struct {
//	onlined   []map[int64]*hlist.Hlist
//	onlinedMu []*sync.Mutex
//}
type SessionList struct {
	onlined   map[int64]*Session
	onlinedMu *sync.Mutex
}
type Session struct {
	Uid  int64
	devs []int64 // Uid为手机:包含所有已绑定的板子id;Uid为板子时，包含所有已绑定的用户id
	Conn Connection
	Adr  string
}

func NewWsSession(uid int64, bindedIds []int64, conn Connection, adr string) *Session {
	return &Session{Uid: uid, devs: bindedIds, Conn: conn, Adr: adr}
}

func (this *Session) Close() {
	this.Conn.Close()
}

func (this *Session) isBinded(id int64) bool {
	for _, v := range this.devs {
		if v == id {
			return true
		}
	}
	return false
}

func (this *Session) calcDestIds(toId int64) []int64 {
	var destIds []int64
	if toId == 0 {
		//		if this.Uid > 0 {
		destIds = this.devs
		//		} else {
		//			for i, ci := 0, len(this.BindedIds); i < ci; i++ {
		//				for j := int64(1); j < int64(kUseridUnit); j++ {
		//					destIds = append(destIds, this.BindedIds[i]+j)
		//					glog.Infoln("session.go destIds:", destIds)
		//				}
		//			}
		//		}

	} else {
		if !this.isBinded(int64(toId)) {
			glog.Errorf("[msg] src id [%d] not binded to dst id [%d], valid ids: %v", this.Uid, toId, this.devs)
			return nil
		}
		//		if this.Uid < 0 && toId%int64(kUseridUnit) == 0 {
		//			destIds = make([]int64, kUseridUnit-1)
		//			glog.Infoln("session.go destIds:", destIds)
		//			for i, c := 0, int(kUseridUnit-1); i < c; i++ {
		//				toId++
		//				destIds[i] = toId
		//				glog.Infoln("session.go toId:", toId)
		//			}
		//		} else {
		destIds = append(destIds, toId)
		//		}
	}
	if glog.V(3) {
		glog.Infof("%v became %v", toId, destIds)
	}
	return destIds
}

// 检查是否有已发生的设备列表变动，有的话一次性全部读取，但只保留最后一个最新的
//func (this *Session) UpdateBindedIds() {
//	newIds := ""
//	idsChanged := false
//	for {
//		done := false
//		select {
//		case s := <-this.MsgChan:
//			newIds = s
//			idsChanged = true
//		default:
//			done = true
//			break
//		}
//		if done {
//			break
//		}
//	}
//	if idsChanged {
//		ids := strings.Split(newIds, ",")
//		this.BindedIds = make([]int64, len(ids))
//		for i, _ := range ids {
//			if len(ids[i]) > 0 {
//				n, err := strconv.ParseInt(ids[i], 10, 64)
//				if err != nil {
//					glog.Errorf("[binded ids] id [%d] receive non-int id updated message [%s], error: %v", this.Uid, newIds, err)
//					continue
//				}
//				this.BindedIds[i] = n
//			}
//		}
//	}
//}

func InitSessionList() *SessionList {
	sl := &SessionList{
		onlined:   make(map[int64]*Session, MapSize),
		onlinedMu: new(sync.Mutex),
	}
	return sl
}

func (this *SessionList) AddSession(s *Session) {
	this.onlinedMu.Lock()
	this.onlined[s.Uid] = s
	this.onlinedMu.Unlock()
	return
}

func (this *SessionList) RemoveSession(s *Session) {
	this.onlinedMu.Lock()
	if s.Conn != nil {
		s.Close()
	}
	delete(this.onlined, s.Uid)
	this.onlinedMu.Unlock()
	if glog.V(3) {
		glog.Infof("[WSSESS:DEAD] usr:%v,%v.", s.Uid, s.Adr)
	}
}

func (this *SessionList) UpdateIds(deviceId int64, userId int64, bindType bool) {
	lock := this.onlinedMu
	lock.Lock()
	if s, ok := this.onlined[userId]; ok {
		if !ok {
			if glog.V(3) {
				glog.Infof("[WS UPDATE DEVLIST] no session of %d, fail to bind/unbind device %d", userId, deviceId)
			}
			return
		}
		if bindType {
			// 绑定
			s.devs = append(s.devs, deviceId)
			if glog.V(3) {
				glog.Infof("[WS UPDATE DEVLIST] usr:%d has binded dev:%d", userId, deviceId)
			}

		} else {
			// 解绑
			for k, v := range s.devs {
				if v != deviceId {
					continue
				}
				lastIndex := len(s.devs) - 1
				s.devs[k] = s.devs[lastIndex]
				s.devs = s.devs[:lastIndex]
				if glog.V(3) {
					glog.Infof("[WS UPDATE DEVLIST] usr:%d has unbinded dev:%d", userId, deviceId)
				}
				break
			}
		}
	}
	lock.Unlock()
	//	// if found deviceId, send bind/unbind message to mids
	//	if bindType {
	//		GMsgBusManager.NotifyBindedIdChanged(deviceId, mids, nil)
	//	} else {
	//		GMsgBusManager.NotifyBindedIdChanged(deviceId, nil, mids)
	//	}
}

func (this *SessionList) PushCommonMsg(msgid uint16, dstId int64, msgBody []byte) {
	if glog.V(3) {
		glog.Infof("[SEND COMMON MSG TO USR] Received=msgid:%v,usr:%v,len(msgBody):%v,msgBody:%v", msgid, dstId, len(msgBody), msgBody)
	}
	msg := msgs.NewMsg(msgBody, nil)
	msg.FrameHeader.Opcode = 2
	msg.DataHeader.MsgId = msgid

	s, ok := this.onlined[dstId]
	if !ok {
		if glog.V(3) {
			glog.Infof("[SEND COMMON MSG TO USR] MSG can't send, Destination is not in this WSCOMET,ADDITIONAL MSG msgid:%v,usr:%v,len(msgBody):%v,msgBody:%v", msgid, dstId, len(msgBody), msgBody)
		}
		return
	}

	msg.FrameHeader.DstId = dstId
	msgBytes := msg.MarshalBytes()

	if glog.V(3) {
		glog.Infof("[SEND COMMON MSG TO USR] Final Msg |len:%v| |%v|", len(msgBytes), msgBytes)
	}

	_, err := s.Conn.Send(msgBytes)
	if err != nil {
		if glog.V(3) {
			glog.Infof("[SEND COMMON MSG TO USR] Failed |%v|", err)
		}
		SetUserOffline(dstId, fmt.Sprintf("%v-%v", gLocalAddr, gCometType))
		this.RemoveSession(s)
		return
	}
	if glog.V(3) {
		glog.Infof("[SEND COMMON MSG TO USR] DONE %v->%v,MsgID:%v,ctn:%v", s.Adr, dstId, msgid, msgBytes)
	}
}

func (this *SessionList) OffliningUsr(uid int64) {
	if glog.V(3) {
		glog.Infof("[OFFLINE USR] %v", uid)
	}
	sess, ok := this.onlined[uid]
	if !ok {
		if glog.V(3) {
			glog.Infof("[OFFLINE USR] no usr:%v is in this WSCOMET", uid)
		}
		return
	}
	body := msgs.MsgStatus{}
	body.Type = msgs.MSTKickOff
	kickMsg := msgs.NewMsg(nil, nil)
	kickMsg.FrameHeader.Opcode = 2
	kickMsg.DataHeader.MsgId = msgs.MIDStatus

	body.Id = uid
	kickMsg.FrameHeader.DstId = uid
	kickMsg.Data, _ = body.Marshal()
	msgBody := kickMsg.MarshalBytes()

	_, err := sess.Conn.Send(msgBody)
	if err != nil {
		glog.Warningf("[OFFLINE USR] usr:%d,%v", sess.Uid, err)
	}
	err = sess.Conn.Close()
	if err != nil {
		glog.Warningf("[OFFLINE USR] usr:%d, error: %v", sess.Uid, err)
	}
	SetUserOffline(uid, fmt.Sprintf("%v-%v", gLocalAddr, gCometType))
	this.RemoveSession(sess)
	if glog.V(3) {
		glog.Infof("[OFFLINE USR] Done. usr:%v", uid)
	}
}

func (this *SessionList) PushMsg(uid int64, data []byte) {
	if s, ok := this.onlined[uid]; ok {
		if len(data) < 24 {
			glog.Errorf("[WSCOMET REFUSE SENDING MSG] Reason is msg's length less than 24,(%d)%v, destination is [usr:%d].", len(data), data, uid)
		}
		_, err := s.Conn.Send(data)
		if err != nil {
			// 不要在这里移除用户session，用户的websocket连接会处理这个情况
			if glog.V(3) {
				glog.Infof("[WSCOMET:SEND] FAILED! usr:%d,%v,%v,%v", s.Uid, len(data), data, err)
			}
		} else {
			statIncDownStreamOut()
			glog.Infof("[WSCOMET:SEND] DONE! usr:%d, data: (len %d)%v", s.Uid, len(data), data)
		}
	}
}
