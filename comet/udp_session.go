package main

import (
	"cloud-socket/msgs"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/golang/glog"
	uuid "github.com/nu7hatch/gouuid"
	"net"
	"sync"
	"time"
)

var (
	kSidLen                     = 16
	kHeaderCheckPosInDataHeader = 8
	ErrSessNotExist             = fmt.Errorf("session not exists")

	gUdpSessions = NewUdpSessionList()
)

type UdpSession struct {
	//Session
	Sid           string       `json:"-"`
	DeviceId      int64        `json:"DeviceID"`
	Addr          *net.UDPAddr `json:"Addr"`
	LastHeartbeat time.Time    `json:"LastHeartbeat"`
	Owner         int64
	Mac           []byte
	GUID          *uuid.UUID
	// 自身的包序号
	// 暂时不使用到该包序号，只有当服务器会主动推送消息给设备时才需要
	Sidx uint16 `json:"Sidx"`

	// 收取的包序号
	Ridx uint16 `json:"Ridx"`

	// 已绑定的用户ID列表
	Users []int64 `json:"BindedUsers"`
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
	for _, v := range s.Users {
		if v == id {
			return true
		}
	}
	//s.BindedUsers = GetDeviceUsers(s.DeviceId)
	return false
}

func (s *UdpSession) CalcDestIds(toId int64) []int64 {
	if toId == 0 {
		return s.Users
	} else {
		if !s.isBinded(int64(toId)) {
			if glog.V(3) {
				glog.Infof("dev [%d] unbind to usr [%d], valid ids: %v", s.DeviceId, toId, s.Users)
			}
			return nil
		}
		return []int64{toId}
	}
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
	udplk  *sync.RWMutex
	sidlk  *sync.RWMutex
	devlk  *sync.RWMutex
	//k:ip;v:
	udpmap map[string]*time.Timer
	//k:sid
	sidmap map[string]*UdpSession
	//k:deviceId
	devmap map[int64]string
}

func NewUdpSessionList() *UdpSessionList {
	sl := &UdpSessionList{
		udplk:  new(sync.RWMutex),
		sidlk:  new(sync.RWMutex),
		devlk:  new(sync.RWMutex),
		udpmap: make(map[string]*time.Timer),
		sidmap: make(map[string]*UdpSession),
		devmap: make(map[int64]string),
	}
	return sl
}
func (this *UdpSessionList) GetDeviceAddr(id int64) (string, error) {
	this.devlk.RLock()
	sid, ok := this.devmap[id]
	this.devlk.RUnlock()
	if !ok {
		return "", fmt.Errorf("[ERR] no device [%d],%v", id, ok)
	}

	//	i, err := uuid.ParseHex(sid)
	//	if err != nil {
	//		return "", fmt.Errorf("wrong session id format: %v", err)
	//	}
	this.sidlk.RLock()
	sess, ok := this.sidmap[sid]
	this.sidlk.RUnlock()
	if !ok {
		return "", fmt.Errorf("[ERR] no sid %s,%v", sid, ok)
	}
	return sess.Addr.String(), nil
}

// Get existed session from DB
func (this *UdpSessionList) GetSession(sid *uuid.UUID) (*UdpSession, error) {
	this.sidlk.RLock()
	s, _ := this.sidmap[sid.String()]
	this.sidlk.RUnlock()
	if s == nil {
		return s, fmt.Errorf("[ERR] no sid %v", sid.String())
	}
	return s, nil
}

// Delete from DB
// 现在还没有需要调用该接口的地方
//func (this *UdpSessionList) DeleteSession(sid *uuid.UUID) error {
//	return DeleteDeviceSession(sid.String())
//}

// Save to DB
func (this *UdpSessionList) SaveSession(sid *uuid.UUID, s *UdpSession) error {
	this.sidlk.Lock()
	this.sidmap[sid.String()] = s
	s.GUID = sid
	this.sidlk.Unlock()
	return nil
}

func (this *UdpSessionList) PushCommonMsg(msgId uint16, did int64, msgBody []byte) error {
	msg := msgs.NewMsg(msgBody, nil)
	msg.FrameHeader.Opcode = 2
	msg.DataHeader.MsgId = msgId
	msg.FrameHeader.DstId = did

	this.devlk.RLock()
	sid, ok := this.devmap[did]
	this.devlk.RUnlock()
	if !ok {
		return fmt.Errorf("[udp:err] no dev [%d] error: %v", did, ok)
	}

	i, err := uuid.ParseHex(sid)
	if err != nil {
		return fmt.Errorf("[udp:err] sid format err: %v", sid)
	}

	sess, err := this.GetSession(i)
	if err != nil {
		return fmt.Errorf("[udp:err] no session %s error: %v", sid, err)
	}
	sess.Sidx++
	msg.FrameHeader.Sequence = sess.Sidx
	msgBytes := msg.MarshalBytes()
	this.server.Send(sess.Addr, msgBytes)
	return nil
}

func (this *UdpSessionList) PushMsg(did int64, msg []byte) error {

	this.devlk.RLock()
	sid, ok := this.devmap[did]
	glog.Infoln("sid:", sid)
	if !ok {
		return fmt.Errorf("[udp:err] [%d] can't got sid.", did)
	}
	this.devlk.RUnlock()
	siduuid, err := uuid.ParseHex(sid)
	if err != nil {
		return fmt.Errorf("[udp:err] sid format err: %v", sid)
	}
	this.sidlk.RLock()
	sess, ok := this.sidmap[sid]
	if !ok {
		return fmt.Errorf("[udp:err] no session %s", sid)
	}
	this.sidlk.RUnlock()

	sess.Sidx++
	binary.LittleEndian.PutUint16(msg[2:4], sess.Sidx)
	copy(msg[FrameHeaderLen:FrameHeaderLen+kSidLen], siduuid[:])
	//hcIndex := FrameHeaderLen + kSidLen + FrameHeaderLen + kHeaderCheckPosInDataHeader
	//glog.Infoln("PushMsg----------sess:",	sess)
	//msg[hcIndex] = msgs.ChecksumHeader(msg, hcIndex)
	//glog.Infoln("PushMsg:",did,len(msg),msg,hcIndex)
	this.server.Send(sess.Addr, msg)
	return nil
}

func (this *UdpSessionList) UpdateIds(deviceId int64, userId int64, bindType bool) {

	this.devlk.RLock()
	sid, ok := this.devmap[deviceId]
	this.devlk.RUnlock()
	if !ok {
		glog.Errorf("no device [%d] error: %v", deviceId, ok)
		return
	}

	i, err := uuid.ParseHex(sid)
	if err != nil {
		glog.Errorf("[udp:err] sid format err: %v", sid)
		return
	}
	sess, err := this.GetSession(i)
	if err != nil {
		glog.Errorln(err)
		return
	}
	if bindType {
		// 绑定
		sess.Users = append(sess.Users, userId)
		if glog.V(3) {
			glog.Infof("[udp:bind] dev:%d binded usr:%d", deviceId, userId)
		}
		GMsgBusManager.NotifyBindedIdChanged(deviceId, []int64{userId}, nil)
	} else {
		// 解绑
		for k, v := range sess.Users {
			if v != userId {
				continue
			}
			lastIndex := len(sess.Users) - 1
			sess.Users[k] = sess.Users[lastIndex]
			sess.Users = sess.Users[:lastIndex]
			if glog.V(3) {
				glog.Infof("[udp:unbind] dev:%d unbinded usr:%d", deviceId, userId)
			}
			break
		}
		GMsgBusManager.NotifyBindedIdChanged(deviceId, nil, []int64{userId})
	}
	this.SaveSession(i, sess)
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
