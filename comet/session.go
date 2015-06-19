package main

import (
	"sync"

	//	"cloud-base/hlist"
	"cloud-socket/msgs"
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
	Uid       int64
	BindedIds []int64 // Uid为手机:包含所有已绑定的板子id;Uid为板子时，包含所有已绑定的用户id
	Conn      Connection
	Adr       string
}

func NewWsSession(uid int64, bindedIds []int64, conn Connection, adr string) *Session {
	s, ok := gSessionList.onlined[uid]
	if ok {
		s.Conn.Send([]byte{1})
		s.Close()
		gSessionList.RemoveSession(s)
	}
	return &Session{Uid: uid, BindedIds: bindedIds, Conn: conn, Adr: adr}
}

func (this *Session) Close() {
	this.Conn.Close()
}

func (this *Session) isBinded(id int64) bool {
	//	if this.Uid < 0 {
	//		// 当this代表板子时，检查id是否属于已绑定用户下的手机
	//		id = id - id%int64(kUseridUnit)
	//	}
	for _, v := range this.BindedIds {
		if v == id {
			return true
		}
	}
	return false
}

func (this *Session) calcDestIds(toId int64) []int64 {
	glog.Infoln("session.go told:", toId)
	var destIds []int64
	if toId == 0 {
		//		if this.Uid > 0 {
		destIds = this.BindedIds
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
			glog.Errorf("[msg] src id [%d] not binded to dst id [%d], valid ids: %v", this.Uid, toId, this.BindedIds)
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
	_, ok := this.onlined[s.Uid]
	if ok {
		delete(this.onlined, s.Uid)
	}
	this.onlinedMu.Unlock()
	glog.Infoln("[ws:over] sess dead. uid:", s.Uid, s.Adr)
}

func (this *SessionList) GetBindedIds(session *Session, ids *[]int64) {
	this.onlinedMu.Lock()
	*ids = session.BindedIds
	this.onlinedMu.Unlock()
}

func (this *SessionList) CalcDestIds(s *Session, toId int64) []int64 {
	this.onlinedMu.Lock()
	var ids []int64
	_, ok := this.onlined[s.Uid]
	if ok {
		ids = s.calcDestIds(toId)
	}
	this.onlinedMu.Unlock()
	glog.Infoln("session.go ids:", ids)
	return ids
}
func (this *SessionList) UpdateIds(deviceId int64, userId int64, bindType bool) {
	// add or remove deviceId from mobileIds session
	mids := []int64{userId}
	lock := this.onlinedMu
	lock.Lock()
	if s, ok := this.onlined[userId]; ok {
		if !ok {
			glog.Infof("[ws:bind] no session userId %d bind device %d", userId, deviceId)
			return
		}
		if bindType {
			// 绑定
			s.BindedIds = append(s.BindedIds, deviceId)
			glog.Infof("[ws:bind] userId %d bind device %d", userId, deviceId)

		} else {
			// 解绑
			for k, v := range s.BindedIds {
				if v != deviceId {
					continue
				}
				lastIndex := len(s.BindedIds) - 1
				s.BindedIds[k] = s.BindedIds[lastIndex]
				s.BindedIds = s.BindedIds[:lastIndex]
				glog.Infof("[ws|unbind] userId %d unbind device %d", userId, deviceId)
				break
			}
		}
	}
	lock.Unlock()
	// if found deviceId, send bind/unbind message to mids
	if bindType {
		GMsgBusManager.NotifyBindedIdChanged(deviceId, mids, nil)
	} else {
		GMsgBusManager.NotifyBindedIdChanged(deviceId, nil, mids)
	}
}

func (this *SessionList) PushCommonMsg(msgid uint16, dstId int64, msgBody []byte) {
	glog.Infof("[ch:sending] msgid:%v | dstId:%v | len(msgBody):%v | msgBody:%v\n", msgid, dstId, len(msgBody), msgBody)
	msg := msgs.NewMsg(msgBody, nil)
	msg.FrameHeader.Opcode = 2
	msg.DataHeader.MsgId = msgid

	s, ok := this.onlined[dstId]
	if !ok {
		glog.Errorf("[ch:sending] no session, msgid:%v | dstId:%v | len(msgBody):%v | msgBody:%v\n", msgid, dstId, len(msgBody), msgBody)
		return
	}

	msg.FrameHeader.DstId = dstId
	msgBytes := msg.MarshalBytes()
	glog.Infoln("session.go Push msgBytes:", len(msgBytes), msgBytes)

	_, err := s.Conn.Send(msgBytes)
	if err != nil {
		glog.Warningf("[ch:err] id: %d, MsgId %d, error: %v", dstId, msgid, err)
		err = s.Conn.Close()
		if err != nil && glog.V(2) {
			glog.Warningf("[ch:err] id: %d, MsgId: %d, error: %v", dstId, msgid, err)
		}
	}
	glog.Infof("[ch:sended]%v->%v, MsgID:%v,ctn:%v ", s.Uid, dstId, msgid, msgBytes)
}

func (this *SessionList) KickOffline(uid int64) {
	body := msgs.MsgStatus{}
	body.Type = msgs.MSTKickOff
	kickMsg := msgs.NewMsg(nil, nil)
	kickMsg.FrameHeader.Opcode = 2
	kickMsg.DataHeader.MsgId = msgs.MIDStatus

	body.Id = uid
	kickMsg.FrameHeader.DstId = uid
	kickMsg.Data, _ = body.Marshal()
	msgBody := kickMsg.MarshalBytes()
	s, ok := this.onlined[uid]
	if !ok {
		glog.Errorln("[kick] no session :",uid)
		return
	}
	_, err := s.Conn.Send(msgBody)
	if err != nil && glog.V(2) {
		glog.Warningf("[kicking msg sending fail] user: %d, error: %v", s.Uid, err)
	}
	err = s.Conn.Close()
	if err != nil && glog.V(2) {
		glog.Warningf("[ws closing fail]  user: %d, error: %v", s.Uid, err)
	}
	glog.Infof("[kick] user %d modified password", s.Uid)
}

func (this *SessionList) PushMsg(uid int64, data []byte) {
	if s, ok := this.onlined[uid]; ok {
		if len(data) < 24 {
			glog.Errorf("[ws|err] [user: %d] length less than 24 (%d)%v", uid, len(data), data)
		}
		_, err := s.Conn.Send(data)
		if err != nil {
			// 不要在这里移除用户session，用户的websocket连接会处理这个情况
			glog.Infof("[ws|down] fail user: %d, error: %v", s.Uid, err)
		} else {
			statIncDownStreamOut()
			glog.Infof("[ws|down] success to user: %d, data: (len %d)%v", s.Uid, len(data), data[:3])
		}
	}
}
