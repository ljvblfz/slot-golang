package msgs

import (
	"encoding/binary"
	"time"
)

const (
	// 消息头各部分的长度
	kHeadForward = 20
	kHeadFrame   = 24
	kHeadData    = 12

	// 转发头中的标记字节
	kForwardFlagOffset = 16

	// 应用层协议的消息ID
	MIDKickout = 0x23
	MIDOnline  = 0x35
	MIDOffline = 0x36
	MIDBind    = 0x37
	MIDUnbind  = 0x38

	kPoly = 0x66
)

type AppMsg struct {
	DstId int64
	SrcId int64
	MsgId uint16

	buf []byte
}

func NewAppMsg(dstId int64, srcId int64, msgId uint16) *AppMsg {
	a := &AppMsg{
		buf: make([]byte, kHeadForward+kHeadFrame+kHeadData),
	}
	for i, _ := range a.buf {
		a.buf[i] = 0
	}
	a.buf[16] |= 1
	// 预填的后续不会改变的数据
	frame := a.buf[kHeadForward : kHeadForward+kHeadFrame]
	frame[0] = 1 << 7
	a.SetDstId(dstId)
	a.SetSrcId(srcId)
	a.SetMsgId(msgId)
	return a
}

func NewAckMsg(srcId int64, message []byte) *AppMsg {
	a := &AppMsg{
		buf: make([]byte, len(message)),
	}
	copy(a.buf, message)
	// 数据头中的ACK头
	a.buf[kHeadForward+kHeadFrame] |= 1 << 6
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
	i := kHeadForward
	last := kHeadForward + kHeadFrame + 8
	// new protocol
	var headerCheck uint8
	for ; i <= last; i++ {
		headerCheck ^= a.buf[i]
		for bit := 8; bit > 0; bit-- {
			if headerCheck&0x80 != 0 {
				headerCheck = (headerCheck << 1) ^ kPoly
			} else {
				headerCheck = (headerCheck << 1)
			}
		}
	}
	a.buf[kHeadForward+kHeadFrame+8] = headerCheck
	// old protocol
	//var headCheck [2]uint8
	//for ; i+1 < count; i += 2 {
	//	headCheck[0] ^= a.buf[i]
	//	headCheck[1] ^= a.buf[i+1]
	//}
	//if i < count {
	//	headCheck[0] ^= a.buf[i]
	//}
	//copy(a.buf[kHeadForward+kHeadFrame+8:], headCheck[:2])

	return a.buf
}

// 消息的转发类型
// 返回true时，转发，不上传，服务器不回复ACK
// 返回false时，不转发，上传，服务器回复ACK
func IsForwardType(msg []byte) bool {
	return msg[kForwardFlagOffset]&0x1 != 0
}

func ForwardSrcId(msg []byte) int64 {
	return int64(binary.LittleEndian.Uint64(msg[8:16]))
}
