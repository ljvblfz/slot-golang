package msg

import (
	"encoding/binary"
	"time"
)

const (
	// 消息头各部分的长度
	kHeadForward = 20
	kHeadFrame = 24
	kHeadData = 12

	// 应用层协议的消息ID
	MIDKickout	= 0x23
	MIDOnline	= 0x35
	MIDOffline	= 0x36
	MIDBind		= 0x37
	MIDUnbind	= 0x38
)

type AppMsg struct {
	DstId int64
	SrcId int64
	MsgId uint16

	buf []byte
}

func NewAppMsg(dstId int64, srcId int64, msgId uint16) *AppMsg {
	a := &AppMsg{
		buf: make([]byte, kHeadForward + kHeadFrame + kHeadData),
	}
	for i, _ := range a.buf {
		a.buf[i] = 0
	}
	// 预填的后续不会改变的数据
	frame := a.buf[20:20+24]
	frame[0] = 1 << 7
	a.SetDstId(dstId)
	a.SetSrcId(srcId)
	a.SetMsgId(msgId)
	return a
}

func (a *AppMsg) SetDstId(dstId int64) {
	a.DstId = dstId
	// 转发头
	binary.LittleEndian.PutUint64(a.buf[:], uint64(a.DstId))
	// 帧头
	binary.LittleEndian.PutUint64(a.buf[kHeadForward+8:], uint64(a.DstId))
	// 数据头
	a.buf[kHeadForward+kHeadFrame] = byte(1 << 7)
}

func (a *AppMsg) SetSrcId(srcId int64) {
	a.SrcId = srcId
	// 帧头
	binary.LittleEndian.PutUint64(a.buf[kHeadForward+16:], uint64(a.SrcId))
}

func (a *AppMsg) SetMsgId(msgId uint16) {
	a.MsgId = msgId
	// 数据头
	binary.LittleEndian.PutUint16(a.buf[kHeadForward+kHeadFrame+4:], a.MsgId)
}

func (a *AppMsg) MarshalBytes() []byte {
	// 帧头
	binary.LittleEndian.PutUint32(a.buf[kHeadForward+4:], uint32(time.Now().Unix()))

	// 数据头

	// 异或校验HeadCheck之前的帧头和数据头，不满16位的补0
	var headCheck [2]uint8
	i := kHeadForward
	count := kHeadForward + kHeadFrame + 8
	for ; i + 1 < count; i += 2 {
		headCheck[0] ^= a.buf[i]
		headCheck[1] ^= a.buf[i+1]
	}
	if i < count {
		headCheck[0] ^= a.buf[i]
	}
	copy(a.buf[kHeadForward+kHeadFrame+8:], headCheck[:2])

	return a.buf
}
