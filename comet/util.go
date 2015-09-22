package main

import (
	binary "encoding/binary"
	uuid "github.com/nu7hatch/gouuid"
	"unsafe"
)

var (
//kBalabala = []byte{'h', 'a', 'd', 't', 'o', 'b', 'e', '8'}
)

func NewUuid() *uuid.UUID {
	id, _ := uuid.NewV4()
	return id
}

func Byte2Sess(adr []byte) *UdpSession {
	y := uintptr(binary.LittleEndian.Uint64(adr))
	sess := (*UdpSession)(unsafe.Pointer(y))
	return sess
}
func Sess2Byte(o *UdpSession) []byte {
	d := make([]byte, 8)
	binary.LittleEndian.PutUint64(d, uint64(uintptr(unsafe.Pointer(o))))
	return d
}
