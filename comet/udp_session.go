package main

import (
	"cloud-socket/msgs"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/golang/glog"
	//	uuid "github.com/nu7hatch/gouuid"
	"net"
	"sync"
	"time"
	"unsafe"
)

var (
	kSidLen                     = 8
	kHeaderCheckPosInDataHeader = 8
	ErrSessNotExist             = fmt.Errorf("session not exists")

	gUdpSessions = NewUdpSessionList()
)

type UdpSession struct {
	Sid           uint64
	DevId         int64       `json:"DeviceID"`
	Addr          *net.UDPAddr `json:"Addr"`
	LastHeartbeat time.Time    `json:"LastHeartbeat"`
	Owner         int64
	Mac           []byte
	Users         []int64
	Devs          map[int64][]byte
	OfflineEvent  *time.Timer `json:"-"`

	Sidx uint16 `json:"-"`
	Ridx uint16 `json:"-"`
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
	u.Sid = uint64(uintptr(unsafe.Pointer(u)))
	return u
}

// check pack number and other things in session here
func (s *UdpSession) VerifyPack(packNum uint16) error {
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

func (s *UdpSession) VerifySession(adr string) error {
	if s.Addr.String() != adr {
		return fmt.Errorf("Wrong Sess!")
	}
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
				glog.Infof("dev [%d] unbind to usr [%d], current users: %v", s.DevId, toId, s.Users)
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
	return fmt.Sprintf("%x", s.Mac) + string(buf)
}

func (s *UdpSession) FromString(data string) error {
	return json.Unmarshal([]byte(data), s)
}

type UdpSessionList struct {
	Server *UdpServer
	Devlk  *sync.RWMutex
	Sesses map[int64]*UdpSession
}

func NewUdpSessionList() *UdpSessionList {
	return &UdpSessionList{
		Devlk:  new(sync.RWMutex),
		Sesses: make(map[int64]*UdpSession),
	}
}
func (this *UdpSessionList) GetDeviceAddr(id int64) (string, error) {
	this.Devlk.RLock()
	sess, ok := this.Sesses[id]
	this.Devlk.RUnlock()
	if !ok {
		return "", fmt.Errorf("[ERR] NO SESS %s,%v", id, ok)
	}
	return sess.Addr.String(), nil
}

func (this *UdpSessionList) SaveSessionByAdr(o *UdpSession) {
	this.Devlk.Lock()
	this.Sesses[int64(uintptr(unsafe.Pointer(o)))] = o
	this.Devlk.Unlock()
}
func (this *UdpSessionList) DelSessionByAdr(o *UdpSession) {
	this.Devlk.Lock()
	delete(this.Sesses,int64(uintptr(unsafe.Pointer(o))))
	this.Devlk.Unlock()
}
func (this *UdpSessionList) SaveSessionById(id int64,s *UdpSession) {
	this.Devlk.Lock()
	this.Sesses[id] = s
	this.Devlk.Unlock()
}
func (this *UdpSessionList) DelSessionById(id int64,s *UdpSession) {
	this.Devlk.Lock()
	delete(this.Sesses,id)
	this.Devlk.Unlock()
}


func (this *UdpSessionList) PushCommonMsg(msgId uint16, did int64, msgBody []byte) error {
	msg := msgs.NewMsg(msgBody, nil)
	msg.FrameHeader.Opcode = 2
	msg.DataHeader.MsgId = msgId
	msg.FrameHeader.DstId = did

	sess, _ := this.Sesses[did]
	if sess == nil {
		return fmt.Errorf("[udp:err] no session %v", did)
	}

	sess.Sidx++
	msg.FrameHeader.Sequence = sess.Sidx
	msgBytes := msg.MarshalBytes()
	this.Server.Send(sess.Addr, msgBytes)
	return nil
}

func (this *UdpSessionList) PushMsg(did int64, msg []byte) error {
	sess, _ := this.Sesses[did]
	if sess == nil {
		return fmt.Errorf("[udp:err] no session %s", did)
	}

	sess.Sidx++
	binary.LittleEndian.PutUint16(msg[2:4], sess.Sidx)
	copy(msg[FrameHeaderLen:FrameHeaderLen+kSidLen], Sess2Byte(sess))
	//hcIndex := FrameHeaderLen + kSidLen + FrameHeaderLen + kHeaderCheckPosInDataHeader
	//glog.Infoln("PushMsg----------sess:",	sess)
	//msg[hcIndex] = msgs.ChecksumHeader(msg, hcIndex)
	//glog.Infoln("PushMsg:",did,len(msg),msg,hcIndex)
	this.Server.Send(sess.Addr, msg)
	return nil
}

func (this *UdpSessionList) UpdateIds(deviceId int64, userId int64, bindType bool) {
	sess, _ := this.Sesses[deviceId]
	if sess == nil {
		glog.Infof("no udp session %v when updating mapping dev<->usr",deviceId)
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
