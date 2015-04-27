package msgs

import (
	"encoding/binary"
	"errors"
)

const (
	// 状态消息0x36中的消息类型码
	MSTJoinedFamily      = 1
	MSTRemovedFromFamily = 2
	MSTQuitFamily        = 3
	MSTFamilyDismissed   = 4
	MSTInviteUser        = 5
	MSTRefusedInviting   = 6
	MSTBinded            = 7
	MSTUnbinded          = 8
	MSTDeviceOnline      = 9
	MSTDeviceOffline     = 10
	MSTActivitedEmail    = 11
	MSTBindedUserMobile  = 12
	MSTKickOff           = 13
)

var (
	ErrPayloadTooLong = errors.New("length of payload can't exceed 255 bytes")
)

type MsgStatus struct {
	Type byte
	Id   int64
	// 额外的消息，长度不能超过255
	Payload []byte
}

func (m *MsgStatus) Marshal() ([]byte, error) {
	if len(m.Payload) > 255 {
		return nil, ErrPayloadTooLong
	}
	buf := make([]byte, 1+8+1+len(m.Payload))

	buf[0] = m.Type
	binary.LittleEndian.PutUint64(buf[1:9], uint64(m.Id))
	buf[10] = byte(len(m.Payload))
	copy(buf[11:], m.Payload)
	return buf, nil
}
