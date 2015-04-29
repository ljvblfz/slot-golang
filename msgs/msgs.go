package msgs

import (
	"encoding/binary"
	"time"
)

const (
	// 消息头各部分的长度
	kFrameHeaderLen = 24
	kHeadForward    = kFrameHeaderLen
	kSidLen         = 16
	kHeadFrame      = kFrameHeaderLen
	kHeadData       = 12

	// 转发头中的标记字节
	kForwardFlagOffset = 0

	// 应用层协议的消息ID
	MIDOnlineOffline = 0x34
	MIDBindUnbind    = 0x35
	MIDStatus        = 0x36

	FlagAck  = 1 << 6
	FlagRead = 1 << 7

	// 协议定义为0x131，但go编译器禁止长类型强制转换为短类型，所以手动强制转换结果为0x31
	kPoly = 0x31
)

type FrameHeader struct {
	Fin    bool
	Mask   bool
	Ver    byte
	Opcode byte

	Reserve byte

	Sequence uint16

	Time uint32

	DstId int64

	SrcId int64

	// this is non-empty only if opcode is 3
	Guid []byte
}

func NewFrameHeader() *FrameHeader {
	return &FrameHeader{
		Fin:  true,
		Time: uint32(time.Now().Unix()),
	}
}

func (f *FrameHeader) marshalBytes() []byte {
	buf := make([]byte, kFrameHeaderLen)
	// byte 0
	if f.Fin {
		buf[0] |= 0x80 // 1<<7
	}
	if f.Mask {
		buf[0] |= 0x40 // 1<<6
	}
	if f.Ver != 0 {
		buf[0] |= (f.Ver & 0x7) << 3
	}
	buf[0] |= f.Opcode & 0x7

	// byte 1
	buf[1] = f.Reserve

	// byte [2:4]
	binary.LittleEndian.PutUint16(buf[2:4], f.Sequence)

	// byte [4:8]
	binary.LittleEndian.PutUint32(buf[4:8], f.Time)

	// byte [8:16]
	binary.LittleEndian.PutUint64(buf[8:16], uint64(f.DstId))

	// byte [16:24]
	binary.LittleEndian.PutUint64(buf[16:24], uint64(f.SrcId))

	// byte [24:24+16], optional
	if f.Guid != nil {
		buf = append(buf, f.Guid...)
	}
	return buf
}

type dataHeader struct {
	Read        bool
	Ack         bool
	DataFormat  byte
	KeyLevel    byte
	EncryptType byte

	DataSeq byte

	DevType uint16

	MsgId uint16

	Length uint16

	HeadCheck byte

	Check byte

	// this is 16 bytes only if this message is used between devices and servers
	SessionId uint16
}

func (d *dataHeader) marshalBytes(dataLen int, opcode byte) (buf []byte, headCheckPos int, bodyCheckPos int) {
	buf = make([]byte, 12)
	// byte 0
	if d.Read {
		buf[0] |= 0x80 // 1<<7
	}
	if d.Ack {
		buf[0] |= 0x40 // 1<<6
	}
	if d.DataFormat != 0 {
		buf[0] |= (d.DataFormat & 0x3) << 4
	}
	if d.KeyLevel != 0 {
		buf[0] |= (d.KeyLevel & 0x3) << 2
	}
	if d.EncryptType != 0 {
		buf[0] |= d.EncryptType & 0x3
	}

	// byte 1
	buf[1] = d.DataSeq

	// byte [2:4]
	binary.LittleEndian.PutUint16(buf[2:4], d.DevType)

	// byte [4:6]
	binary.LittleEndian.PutUint16(buf[4:6], d.MsgId)

	// byte [6:8]
	binary.LittleEndian.PutUint16(buf[6:8], uint16(dataLen))

	// byte 9
	headCheckPos = 9

	// byte 10
	bodyCheckPos = 10

	// byte [10:10+(2 or 16)]
	binary.LittleEndian.PutUint16(buf[10:12], d.SessionId)

	return buf, headCheckPos, bodyCheckPos
}

type AppMsg struct {
	ForwardHeader *FrameHeader
	FrameHeader   FrameHeader
	DataHeader    dataHeader
	Data          []byte
}

func NewMsg(data []byte, forwardHeader *FrameHeader) *AppMsg {
	m := &AppMsg{
		ForwardHeader: forwardHeader,
		FrameHeader:   *NewFrameHeader(),
		DataHeader: dataHeader{
			Read: true,
		},
		Data: data,
	}
	return m
}

// 编码消息至字节流，并更新帧头中的时间戳至当前时间
func (a *AppMsg) MarshalBytes() []byte {
	a.FrameHeader.Time = uint32(time.Now().Unix())

	var buf []byte

	if a.ForwardHeader != nil {
		buf = a.ForwardHeader.marshalBytes()
	}

	buf = append(buf, a.FrameHeader.marshalBytes()...)

	var opcode byte
	if a.ForwardHeader != nil {
		opcode = a.ForwardHeader.Opcode
	} else {
		opcode = a.FrameHeader.Opcode
	}
	bufFrame, hcheckPos, checkPos := a.DataHeader.marshalBytes(len(a.Data), opcode)
	hcheckPos += len(buf)
	checkPos += len(buf)

	buf = append(buf, bufFrame...)

	if len(a.Data) != 0 {
		buf = append(buf, a.Data...)
	}

	buf[hcheckPos] = ChecksumHeader(buf, hcheckPos)
	buf[checkPos] = ChecksumHeader(buf[checkPos+1:], len(buf)-checkPos-1)

	return buf
}

// 这个ACK消息只应在OpCode等于2时使用，否则会写错ACK标记为位置
func NewAckMsg(message []byte) []byte {
	buf := make([]byte, len(message))
	copy(buf, message)
	// 数据头中的ACK头
	buf[kHeadForward+kHeadFrame] |= FlagAck
	return buf
}

// 数据头中校验位的校验算法
// 实现协议：智能家居通讯协议V2.3.2
func ChecksumHeader(msg []byte, n int) byte {
	var headerCheck uint8
	for i := 0; i < n; i++ {
		headerCheck ^= msg[i]
		for bit := 8; bit > 0; bit-- {
			if headerCheck&0x80 != 0 {
				headerCheck = (headerCheck << 1) ^ kPoly
			} else {
				headerCheck = (headerCheck << 1)
			}
		}
	}
	return headerCheck
}

// 消息的转发类型
// 返回true时，转发，不上传，服务器不回复ACK
// 返回false时，不转发，上传，服务器回复ACK
func IsForwardType(msg []byte) bool {
	return msg[kForwardFlagOffset]&0x7 == 3
}

func ForwardSrcId(msg []byte) int64 {
	return int64(binary.LittleEndian.Uint64(msg[16:18]))
}

func GetMsgId(msg []byte) uint16 {
	offset := 0
	if IsForwardType(msg) {
		offset = kHeadForward + kSidLen + kHeadFrame + 4
	} else {
		offset = kHeadFrame + 4
	}
	return uint16(binary.LittleEndian.Uint16(msg[offset : offset+2]))
}

// old protocol
//type AppMsg struct {
//	dstId int64
//	srcId int64
//	msgId uint16
//
//	buf []byte
//}
//
//func NewAppMsg(dstId int64, srcId int64, msgId uint16, msgBody []byte) *AppMsg {
//	a := &AppMsg{
//		buf: make([]byte, kHeadForward+kHeadFrame+kHeadData+len(msgBody)),
//	}
//	for i, _ := range a.buf {
//		a.buf[i] = 0
//	}
//	a.buf[16] |= 1
//	// 预填的后续不会改变的数据
//	frame := a.buf[kHeadForward : kHeadForward+kHeadFrame]
//	frame[0] = 1 << 7
//	a.SetDstId(dstId)
//	a.SetSrcId(srcId)
//	a.SetMsgId(msgId)
//
//	copy(a.buf[kHeadForward+kHeadFrame+kHeadData:], msgBody)
//
//	return a
//}
//
//func (a *AppMsg) SetDstId(dstId int64) {
//	a.dstId = dstId
//	// 转发头
//	binary.LittleEndian.PutUint64(a.buf[:], uint64(a.dstId))
//	// 帧头
//	binary.LittleEndian.PutUint64(a.buf[kHeadForward+8:], uint64(a.dstId))
//	// 数据头
//	a.buf[kHeadForward+kHeadFrame] = byte(1 << 7)
//}
//
//func (a *AppMsg) SetSrcId(srcId int64) {
//	a.srcId = srcId
//	// 帧头
//	binary.LittleEndian.PutUint64(a.buf[kHeadForward+16:], uint64(a.srcId))
//}
//
//func (a *AppMsg) SetMsgId(msgId uint16) {
//	a.msgId = msgId
//	// 数据头
//	binary.LittleEndian.PutUint16(a.buf[kHeadForward+kHeadFrame+4:], a.msgId)
//}
//
//func (a *AppMsg) MarshalBytes() []byte {
//	// 帧头
//	binary.LittleEndian.PutUint32(a.buf[kHeadForward+4:], uint32(time.Now().Unix()))
//
//	// 数据头
//
//	// 异或校验HeadCheck之前的帧头和数据头，不满16位的补0
//	i := kHeadForward
//	last := kHeadForward + kHeadFrame + 8
//	// new protocol
//	var headerCheck uint8
//	for ; i <= last; i++ {
//		headerCheck ^= a.buf[i]
//		for bit := 8; bit > 0; bit-- {
//			if headerCheck&0x80 != 0 {
//				headerCheck = (headerCheck << 1) ^ kPoly
//			} else {
//				headerCheck = (headerCheck << 1)
//			}
//		}
//	}
//	a.buf[kHeadForward+kHeadFrame+8] = headerCheck
//	// old protocol
//	//var headCheck [2]uint8
//	//for ; i+1 < count; i += 2 {
//	//	headCheck[0] ^= a.buf[i]
//	//	headCheck[1] ^= a.buf[i+1]
//	//}
//	//if i < count {
//	//	headCheck[0] ^= a.buf[i]
//	//}
//	//copy(a.buf[kHeadForward+kHeadFrame+8:], headCheck[:2])
//
//	return a.buf
//}
