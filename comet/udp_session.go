package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"cloud-socket/msgs"
	"github.com/golang/glog"
	uuid "github.com/nu7hatch/gouuid"
)

var (
	kSidLen                     = 16
	kHeaderCheckPosInDataHeader = 8
	ErrSessNotExist             = fmt.Errorf("session not exists")

	gUdpSessions = &UdpSessionList{}
)

type UdpSession struct {
	//Session
	Sid           string       `json:"-"`
	DeviceId      int64        `json:"DeviceID"`
	Addr          *net.UDPAddr `json:"Addr"`
	LastHeartbeat time.Time    `json:"LastHeartbeat"`

	// 自身的包序号
	// 暂时不使用到该包序号，只有当服务器会主动推送消息给设备时才需要
	Sidx uint16 `json:"Sidx"`

	// 收取的包序号
	Ridx uint16 `json:"Ridx"`

	// 已绑定的用户ID列表
	BindedUsers []int64 `json:"BindedUsers"`
}

func NewUdpSession(addr *net.UDPAddr) *UdpSession {
	var u *UdpSession
	if addr == nil {
		u = &UdpSession{
			Addr:          &net.UDPAddr{},
			LastHeartbeat: time.Now(),
		}
	} else {
		u = &UdpSession{
			Addr:          addr,
			LastHeartbeat: time.Now(),
		}
	}
	return u
}

// check pack number and other things in session here
func (s *UdpSession) VerifySession(packNum uint16) error {
	// 现在由redis负责超时
	//if time.Now().Sub(s.LastHeartbeat) > 2*kHeartBeat {
	//	return ErrSessTimeout
	//}
	// pack number
	switch {
	case packNum > s.Ridx:
	case packNum < s.Ridx && packNum < 10 && s.Ridx > uint16(65530):
	default:
		return ErrSessPackSeq
	}

	// all ok
	s.Ridx = packNum

	return nil
}

func (s *UdpSession) isBinded(id int64) bool {
	// 当s代表板子时，检查id是否属于已绑定用户下的手机
	id = id - id%int64(kUseridUnit)
	for _, v := range s.BindedUsers {
		if v == id {
			return true
		}
	}
	return false
}

func (s *UdpSession) CalcDestIds(toId int64) []int64 {
	var destIds []int64
	if toId == 0 {
		for i, ci := 0, len(s.BindedUsers); i < ci; i++ {
			for j := int64(1); j < int64(kUseridUnit); j++ {
				destIds = append(destIds, s.BindedUsers[i]+j)
			}
		}
	} else {
		if !s.isBinded(int64(toId)) {
			glog.Errorf("[msg] src id [%d] not binded to dst id [%d], valid ids: %v", s.DeviceId, toId, s.BindedUsers)
			return nil
		}
		if toId%int64(kUseridUnit) == 0 {
			destIds = make([]int64, kUseridUnit-1)
			for i, c := 0, int(kUseridUnit-1); i < c; i++ {
				toId++
				destIds[i] = toId
			}
		}
	}
	return destIds
}

// Caller should have lock on UdpSession
func (s *UdpSession) Update(addr *net.UDPAddr) error {
	if s.Addr.String() != addr.String() {
		s.Addr = addr
	}
	s.LastHeartbeat = time.Now()
	return nil
}

func (s *UdpSession) String() string {
	buf, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(buf)
}

func (s *UdpSession) FromString(data string) error {
	return json.Unmarshal([]byte(data), s)
}

type UdpSessionList struct {
	server *UdpServer
}

func NewUdpSessionList() *UdpSessionList {
	sl := &UdpSessionList{}
	return sl
}

func (this *UdpSessionList) GetDeviceAddr(id int64) (string, error) {
	glog.Infoln("GetDeviceAddr ",id)
	sid, err := GetDeviceSid(id)
	if err != nil {
		return "", fmt.Errorf("get session of device [%d] error: %v", id, err)
	}

	locker := NewDeviceSessionLocker(sid)
	err = locker.Lock()
	if err != nil {
		return "", fmt.Errorf("lock session id [%s] failed: %v", sid, err)
	}
	defer locker.Unlock()

	i, err := uuid.ParseHex(sid)
	if err != nil {
		return "", fmt.Errorf("wrong session id format: %v", err)
	}
	sess, err := this.GetSession(i)
	if err != nil {
		return "", fmt.Errorf("get session %s error: %v", sid, err)
	}
	return sess.Addr.String(), nil
}

// Get existed session from DB
func (this *UdpSessionList) GetSession(sid *uuid.UUID) (*UdpSession, error) {
	glog.Infoln("GetSession:",sid)
	// Get from DB
	data, err := GetDeviceSession(sid.String())
	if err != nil {
		return nil, err
	}
	s := &UdpSession{
		Addr: &net.UDPAddr{},
	}
	err = s.FromString(data)
	if err != nil {
		return nil, err
	}
	s.Sid = sid.String()
	return s, nil
}

// Delete from DB
// 现在还没有需要调用该接口的地方
//func (this *UdpSessionList) DeleteSession(sid *uuid.UUID) error {
//	return DeleteDeviceSession(sid.String())
//}

// Save to DB
func (this *UdpSessionList) SaveSession(sid *uuid.UUID, s *UdpSession) error {
	glog.Infoln("SaveSession:",sid,s)
	return SetDeviceSession(sid.String(), gUdpTimeout, s.String(), s.DeviceId, s.Addr)
}

func (this *UdpSessionList) PushCommonMsg(msgId uint16, did int64, msgBody []byte) error {
	glog.Infoln("PushCommonMsg:",msgId,did,msgBody)
	msg := msgs.NewMsg(msgBody, nil)
	glog.Infoln("PushCommonMsg msg:",msg)
	msg.FrameHeader.Opcode = 2
	msg.DataHeader.MsgId = msgId
	msg.FrameHeader.DstId = did

	sid, err := GetDeviceSid(did)
	glog.Infoln("PushCommonMsg sid:",sid,err)
	if err != nil {
		return fmt.Errorf("get session of device [%d] error: %v", did, err)
	}

	locker := NewDeviceSessionLocker(sid)
	glog.Infoln("PushCommonMsg locker:",locker)
	err = locker.Lock()
	if err != nil {
		return fmt.Errorf("lock session id [%s] failed: %v", sid, err)
	}
	defer locker.Unlock()

	i, err := uuid.ParseHex(sid)
	glog.Infoln("PushCommonMsg i:",i,err)
	if err != nil {
		return fmt.Errorf("wrong session id format: %v", err)
	}
	sess, err := this.GetSession(i)
	glog.Infoln("PushCommonMsg sess:",sess,err)
	if err != nil {
		return fmt.Errorf("get session %s error: %v", sid, err)
	}
	sess.Sidx++
	msg.FrameHeader.Sequence = sess.Sidx
	glog.Infoln("PushCommonMsg sess.Sidx:",sess.Sidx)
	msgBytes := msg.MarshalBytes()
	glog.Infoln("PushCommonMsg:",msgId,"|",did,"|",msgBody)
	this.server.Send(sess.Addr, msgBytes)

	this.SaveSession(i, sess)
	return nil
}

func (this *UdpSessionList) PushMsg(did int64, msg []byte) error {
	glog.Infoln("PushMsg----------:",did,msg)
	sid, err := GetDeviceSid(did)
	glog.Infoln("PushMsg---------sid-:",sid)
	if err != nil {
		return fmt.Errorf("get session of device [%d] error: %v", did, err)
	}

	locker := NewDeviceSessionLocker(sid)
	glog.Infoln("PushMsg----------locker:",locker)
	err = locker.Lock()
	if err != nil {
		return fmt.Errorf("lock session id [%s] failed: %v", sid, err)
	}
	defer locker.Unlock()

	i, err := uuid.ParseHex(sid)
	glog.Infoln("PushMsg----------sid:",	sid, []byte(sid), i, err)
	if err != nil {
		return fmt.Errorf("wrong session id format: %v", err)
	}
	sess, err := this.GetSession(i)
	glog.Infoln("PushMsg----------sess:",	sess)
	if err != nil {
		return fmt.Errorf("get session %s error: %v", sid, err)
	}
	sess.Sidx++

	binary.LittleEndian.PutUint16(msg[2:4], sess.Sidx)
	glog.Infoln("PushMsg----------sess:",	sess)
	copy(msg[FrameHeaderLen:FrameHeaderLen+kSidLen], i[:])
	glog.Infoln("PushMsg----------sess:",	sess)
	//hcIndex := FrameHeaderLen + kSidLen + FrameHeaderLen + kHeaderCheckPosInDataHeader
	//glog.Infoln("PushMsg----------sess:",	sess)
	//msg[hcIndex] = msgs.ChecksumHeader(msg, hcIndex)
	//glog.Infoln("PushMsg:",did,len(msg),msg,hcIndex)
	this.server.Send(sess.Addr, msg)
	glog.Infoln("PushMsg----------sess:",	sess)
	this.SaveSession(i, sess)
	glog.Infoln("PushMsg----------sess:",	sess)
	return nil
}

func (this *UdpSessionList) UpdateIds(deviceId int64, userId int64, bindType bool) {
	sid, err := GetDeviceSid(deviceId)
	if err != nil {
		glog.Errorf("get session of device [%d] error: %v", deviceId, err)
		return
	}

	locker := NewDeviceSessionLocker(sid)
	err = locker.Lock()
	if err != nil {
		glog.Errorf("lock session id [%s] failed: %v", sid, err)
		return
	}
	defer locker.Unlock()

	i, err := uuid.ParseHex(sid)
	if err != nil {
		glog.Errorf("wrong session id format: %v", err)
		return
	}
	sess, err := this.GetSession(i)
	if err != nil {
		glog.Errorf("get session %s error: %v", sid, err)
		return
	}
	if bindType {
		// 绑定
		sess.BindedUsers = append(sess.BindedUsers, userId)
		glog.Infof("[bind|bind] deviceId %d add userId %d", deviceId, userId)

	} else {
		// 解绑
		for k, v := range sess.BindedUsers {
			if v != userId {
				continue
			}
			lastIndex := len(sess.BindedUsers) - 1
			sess.BindedUsers[k] = sess.BindedUsers[lastIndex]
			sess.BindedUsers = sess.BindedUsers[:lastIndex]
			glog.Infof("[bind|unbind] deviceId %d remove userId %d", deviceId, userId)
			break
		}
	}
	this.SaveSession(i, sess)

	go func() {
		mids := TransId(userId)
		if bindType {
			GMsgBusManager.NotifyBindedIdChanged(deviceId, mids, nil)
		} else {
			GMsgBusManager.NotifyBindedIdChanged(deviceId, nil, mids)
		}
	}()
}

//func (this *UdpSessionList) GetDeviceIdAndDstIds(sid *uuid.UUID) (int64, []int64, error) {
//	this.udpsMu.RLock()
//	defer this.udpsMu.RUnlock()
//	s, ok := this.udps[*sid]
//	if !ok {
//		return 0, nil, fmt.Errorf("session [%s] not exists", sid)
//	}
//	s.mu.Lock()
//	defer s.mu.Unlock()
//	binds := s.Session.calcDestIds(0)
//	return s.Session.Uid, binds, nil
//}
