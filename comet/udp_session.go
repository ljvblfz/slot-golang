package main

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"cloud-socket/msgs"
	uuid "github.com/nu7hatch/gouuid"
)

var (
	ErrSessNotExist = fmt.Errorf("session not exists")

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

	i, err := uuid.Parse([]byte(sid))
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
	return SetDeviceSession(sid.String(), gUdpTimeout, s.String(), s.DeviceId, s.Addr)
}

func (this *UdpSessionList) PushCommonMsg(msgId uint16, did int64, msgBody []byte) error {
	msg := msgs.NewMsg(msgBody, nil)
	msg.FrameHeader.Opcode = 2
	msg.DataHeader.MsgId = msgId
	msg.FrameHeader.DstId = did

	sid, err := GetDeviceSid(did)
	if err != nil {
		return fmt.Errorf("get session of device [%d] error: %v", did, err)
	}

	locker := NewDeviceSessionLocker(sid)
	err = locker.Lock()
	if err != nil {
		return fmt.Errorf("lock session id [%s] failed: %v", sid, err)
	}
	defer locker.Unlock()

	i, err := uuid.Parse([]byte(sid))
	if err != nil {
		return fmt.Errorf("wrong session id format: %v", err)
	}
	sess, err := this.GetSession(i)
	if err != nil {
		return fmt.Errorf("get session %s error: %v", sid, err)
	}
	sess.Sidx++
	msg.FrameHeader.Sequence = sess.Sidx
	msgBytes := msg.MarshalBytes()
	this.server.Send(sess.Addr, msgBytes)
	return nil
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
