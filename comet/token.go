package main

import (
//"encoding/binary"
//"fmt"
//"time"

//"cloud-base/crypto"
)

//const (
//	k5Sec  = 5 * 1000000000
//	k30Sec = 30 * 1000000000
//)

var (
//kBalabala = []byte{'h', 'a', 'd', 't', 'o', 'b', 'e', '8'}
)

//var (
//	ErrTokenTooOld = fmt.Errorf("token too old")
//	ErrTokenTooNew = fmt.Errorf("token from future")
//)

//func VerifyToken(sn []byte, token []byte) error {
//	msg, err := crypto.DesDecrypt(token, kBalabala)
//	//fmt.Printf("DecodeToken: (%d)%x -> (%d)%x\n", len(token), token, len(msg), msg)
//	if err != nil {
//		return err
//	}
//	ls := int64(binary.LittleEndian.Uint64(msg[8:16])) ^
//		int64(binary.LittleEndian.Uint64(sn[8:16]))
//
//	//fmt.Printf("Decrypt: sn: %d ^ res: %d = tm: %d\n", int64(binary.LittleEndian.Uint64(sn[8:16])), int64(binary.LittleEndian.Uint64(msg[8:16])), ls)
//
//	ns := time.Now().UnixNano()
//	if ns > ls {
//		if ns-ls > k30Sec {
//			return ErrTokenTooOld
//		}
//	} else if ns < ls {
//		if ls-ns > k5Sec {
//			return ErrTokenTooNew
//		}
//	}
//	return nil
//}
