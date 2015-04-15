package main

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	uuid "github.com/nu7hatch/gouuid"
)

var (
	// 客户端UDP端口失效时长（秒)
	gUdpTimeout = 40

	ErrSessNotExist = fmt.Errorf("session not exists")

	gUdpSessions = &UdpSessionList{}
)

type UdpSession struct {
	//Session
	Sid           *uuid.UUID   `json:"Sid"`
	Addr          *net.UDPAddr `json:"Addr"`
	LastHeartbeat time.Time    `json:"LastHeartbeat"`

	// 自身的包序号
	Sidx uint16 `json:"Sidx"`

	// 收取的包序号
	Ridx uint16 `json:"Ridx"`
}

func NewUdpSession(addr *net.UDPAddr) *UdpSession {
	u := &UdpSession{
		Addr:          addr,
		LastHeartbeat: time.Now(),
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
}

func NewUdpSessionList() *UdpSessionList {
	sl := &UdpSessionList{}
	return sl
}

func (this *UdpSessionList) GetSession(sid *uuid.UUID) (*UdpSession, error) {
	// Get from DB
	data, err := GetDeviceSession(sid.String())
	if err != nil {
		return nil, err
	}
	s := &UdpSession{}
	err = s.FromString(data)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (this *UdpSessionList) DeleteSession(sid *uuid.UUID) error {
	// Delete from DB
	return DeleteDeviceSession(sid.String())
}

func (this *UdpSessionList) SaveSession(sid *uuid.UUID, s *UdpSession) error {
	// Save to DB
	return SetDeviceSession(sid.String(), gUdpTimeout, s.String())
}

//func (this *UdpSessionList) PushMsg(uid int64, msg []byte) error {
//	_, err := sess.Conn.Send(msg)
//	return err
//}

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
