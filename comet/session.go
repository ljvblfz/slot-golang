package main

import (
	"cloud-base/websocket"
	"cloud-base/hlist"
	"github.com/golang/glog"
	"sync"
	//"strings"
	//"strconv"
)

var (
	BlockSize int64 = 128
	MapSize   int64 = 1024
)

func getBlockID(uid int64) int64 {
	if uid > 0 {
		return uid % BlockSize
	} else {
		return -uid % BlockSize
	}
}

type Session struct {
	Uid       int64
	BindedIds []int64	// Uid为手机:包含所有已绑定的板子id;Uid为板子时，包含所有已绑定的用户id
	Conn      *websocket.Conn
}

func NewSession(uid int64, bindedIds []int64, conn *websocket.Conn) *Session {
	return &Session{Uid: uid, BindedIds: bindedIds, Conn: conn}
}

func (this *Session) Close() {
	this.Conn.Close()
}

func (this *Session) isBinded(id int64) bool {
	if this.Uid < 0 {
		// 当this代表板子时，检查id是否属于已绑定用户下的手机
		id = id - id % int64(kUseridUnit)
	}
	for _, v := range this.BindedIds {
		if v == id {
			return true
		}
	}
	return false
}

func (this *Session) calcDestIds(toId int64) []int64 {
	var destIds []int64
	if toId == 0 {
		if this.Uid > 0 {
			destIds = this.BindedIds
		} else {
			for i, ci := 0, len(this.BindedIds); i < ci; i++ {
				for j := int64(1); j < int64(kUseridUnit); j++ {
					destIds = append(destIds, this.BindedIds[i] + j)
				}
			}
		}

	} else {
		if !this.isBinded(int64(toId)) {
			glog.Errorf("[msg] src id [%d] not binded to dst id [%d], valid ids: %v", this.Uid, toId, this.BindedIds)
			return nil
		}
		if this.Uid < 0 && toId % int64(kUseridUnit) == 0 {
			destIds = make([]int64, kUseridUnit - 1)
			for i, c := 0, int(kUseridUnit - 1); i < c; i++ {
				toId++
				destIds[i] = toId
			}
		} else {
			destIds = append(destIds, toId)
		}
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
//		case s := <-this.TaskChan:
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

type SessionList struct {
	mu []*sync.Mutex
	kv []map[int64]*hlist.Hlist
}

func InitSessionList() *SessionList {
	sl := &SessionList{
		mu: make([]*sync.Mutex, BlockSize),
		kv: make([]map[int64]*hlist.Hlist, BlockSize),
	}

	for i := int64(0); i < BlockSize; i++ {
		sl.mu[i] = &sync.Mutex{}
		sl.kv[i] = make(map[int64]*hlist.Hlist, MapSize)
	}
	return sl
}

func (this *SessionList) AddSession(s *Session) *hlist.Element {
	// 能想到的错误返回值是同一用户，同一mac多次登录，但这可能不算错误
	blockId := getBlockID(s.Uid)
	this.mu[blockId].Lock()
	h, ok := this.kv[blockId][s.Uid]
	var e *hlist.Element
	if ok {
		e = h.PushFront(s)
	} else {
		h = hlist.New()
		this.kv[blockId][s.Uid] = h
		e = h.PushFront(s)
	}
	this.mu[blockId].Unlock()
	return e
}

func (this *SessionList) RemoveSession(e *hlist.Element) {
	s, _ := e.Value.(*Session)
	blockId := getBlockID(s.Uid)
	if s.Conn != nil {
		s.Close()
	}
	this.mu[blockId].Lock()
	list, ok := this.kv[blockId][s.Uid]
	if ok {
		list.Remove(e)
		if list.Len() == 0 {
			delete(this.kv[blockId], s.Uid)
		}
	}
	this.mu[blockId].Unlock()
}

func (this *SessionList) GetBindedIds(session *Session, ids *[]int64) {
	blockId := getBlockID(session.Uid)

	lock := this.mu[blockId]
	lock.Lock()
	*ids = session.BindedIds
	lock.Unlock()
}

func (this *SessionList) CalcDestIds(s *Session, toId int64) []int64 {
	blockId := getBlockID(s.Uid)
	this.mu[blockId].Lock()
	_, ok := this.kv[blockId][s.Uid]
	var ids []int64
	if ok {
		ids = s.calcDestIds(toId)
	} else {
		glog.Error("[calc|fatal] should never happened")
	}
	this.mu[blockId].Unlock()
	return ids
}

func (this *SessionList) UpdateIds(deviceId int64, userId int64, bindType bool) {
	mids := TransId(userId)
	// add or remove deviceId from mobileIds session
	for _, mid := range mids {
		blockId := getBlockID(mid)
		lock := this.mu[blockId]
		lock.Lock()
		if list, ok := this.kv[blockId][mid]; ok {
			for e := list.Front(); e != nil; e = e.Next() {
				s, ok := e.Value.(*Session)
				if !ok {
					break
				}
				if bindType {
					// 绑定
					s.BindedIds = append(s.BindedIds, deviceId)
					glog.Infof("[bind|bind] mid %d add device %d", mid, deviceId)

				} else {
					// 解绑
					for k, v := range s.BindedIds {
						if v != deviceId {
							continue
						}
						lastIndex := len(s.BindedIds) - 1
						s.BindedIds[k] = s.BindedIds[lastIndex]
						s.BindedIds = s.BindedIds[:lastIndex]
						glog.Infof("[bind|unbind] mid %d remove device %d", mid, deviceId)
						break
					}
				}
			}
		}
		lock.Unlock()
	}

	// add or remove mobile id from deviceId's session
	blockId := getBlockID(deviceId)
	lock := this.mu[blockId]
	foundDevice := false
	lock.Lock()
	if list, ok := this.kv[blockId][deviceId]; ok {
		foundDevice = true
		for e := list.Front(); e != nil; e = e.Next() {
			s, ok := e.Value.(*Session)
			if !ok {
				break
			}
			if bindType {
				// 绑定
				s.BindedIds = append(s.BindedIds, userId)
				glog.Infof("[bind|bind] deviceId %d add userId %d", deviceId, userId)

			} else {
				// 解绑
				for k, v := range s.BindedIds {
					if v != userId {
						continue
					}
					lastIndex := len(s.BindedIds) - 1
					s.BindedIds[k] = s.BindedIds[lastIndex]
					s.BindedIds = s.BindedIds[:lastIndex]
					glog.Infof("[bind|unbind] deviceId %d remove userId %d", deviceId, userId)
					break
				}
			}
		}
	}
	lock.Unlock()

	// if found deviceId, send bind/unbind message to mids
	if foundDevice {
		if bindType {
			GMsgBusManager.NotifyBindedIdChanged(deviceId, mids, nil)
		} else {
			GMsgBusManager.NotifyBindedIdChanged(deviceId, nil, mids)
		}
	}
}

func (this *SessionList) PushMsg(uid int64, data []byte) {
	blockId := getBlockID(uid)

	lock := this.mu[blockId]
	lock.Lock()

	if list, ok := this.kv[blockId][uid]; ok {
		for e := list.Front(); e != nil; e = e.Next() {
			if session, ok := e.Value.(*Session); !ok {
				lock.Unlock()
				return
			} else {
				if len(data) < 24 {
					glog.Errorf("[invalid data] [uid: %d] length less than 24 (%d)%v", uid, len(data), data)
				}
				err := websocket.Message.Send(session.Conn, data)
				if err != nil {
					// 不要在这里移除用户session，用户的websocket连接会处理这个情况
					if glog.V(1) {
						glog.Infof("[push failed] uid: %d, error: %v", session.Uid, err)
					}
				} else {
					statIncDownStreamOut()
					glog.Infof("[msg|down] to id: %d, data: (len %d)%v", session.Uid, len(data), data[:3])
				}
			}
		}
	}
	lock.Unlock()
}
