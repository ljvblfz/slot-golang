package main

import (
	"crypto/hmac"
	"crypto/md5"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	//"github.com/golang/glog"
)

var (
	kTimeStart = time.Date(2014, 5, 27, 17, 37, 0, 0, time.Local).UnixNano()
	kWbSalt = "BlackCrystalWb14527"
	kDeviceSalt []byte = []byte("BlackCrystalDevice14529")
)

const (
	kSaltLen = 10
)

func EncodePassword(rawPwd string, salt string) string {
	pwd := PBKDF2([]byte(rawPwd), []byte(salt), 10000, 50, sha256.New)
	return salt + hex.EncodeToString(pwd)
}

func VerifyPassword(rawPwd string, encodedPwd string) bool {
	if len(encodedPwd) <= kSaltLen {
		return false
	}
	salt := encodedPwd[:kSaltLen]
	//fmt.Printf("---raw:%s, ---salt:%s, ---encode:%s,---shadow:%s\n", rawPwd, salt, EncodePassword(rawPwd, salt), encodedPwd)
	return EncodePassword(rawPwd, salt) == encodedPwd
}

// http://code.google.com/p/go/source/browse/pbkdf2/pbkdf2.go?repo=crypto
func PBKDF2(password, salt []byte, iter, keyLen int, h func() hash.Hash) []byte {
	prf := hmac.New(h, password)
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen

	var buf [4]byte
	dk := make([]byte, 0, numBlocks*hashLen)
	U := make([]byte, hashLen)
	for block := 1; block <= numBlocks; block++ {
		// N.B.: || means concatenation, ^ means XOR
		// for each block T_i = U_1 ^ U_2 ^ ... ^ U_iter
		// U_1 = PRF(password, salt || uint(i))
		prf.Reset()
		prf.Write(salt)
		buf[0] = byte(block >> 24)
		buf[1] = byte(block >> 16)
		buf[2] = byte(block >> 8)
		buf[3] = byte(block)
		prf.Write(buf[:4])
		dk = prf.Sum(dk)
		T := dk[len(dk)-hashLen:]
		copy(U, T)

		// U_n = PRF(password, U_(n-1))
		for n := 2; n <= iter; n++ {
			prf.Reset()
			prf.Write(U)
			U = U[:0]
			U = prf.Sum(U)
			for x := range U {
				T[x] ^= U[x]
			}
		}
	}
	return dk[:keyLen]
}

// n2b int64->base64
func n2b(n int64) string {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(n))
	return base64.StdEncoding.EncodeToString(buf)
}

// b2n base64->int64
func b2n(b string) int64 {
	buf, err := base64.StdEncoding.DecodeString(b)
	if err != nil {
		return -1
	}
	return int64(binary.LittleEndian.Uint64(buf))
}

// ns2b []byte->base64
func bs2b(bs []byte) string {
	return base64.StdEncoding.EncodeToString(bs)
}

// b2ns base64->int64
//func b2ns(b string) ([]byte, err) {
//	return base64.StdEncoding.DecodeString(b)
//}

func writeCommonResp(w http.ResponseWriter) {
	w.Header().Add("Content-Type", "application/json; charset=UTF-8")
	w.Header().Add("Server", "blackapi")
}

// 生成用于登录另一个websocket服务器的key
// TODO(yy) 测试实现，不完善
func GenerateKey(id int64, timestamp int64, expire int64) string {
	timestampB := n2b(timestamp)
	buf := fmt.Sprintf("0|%x|%s|%x", id, timestampB, expire)
	md5_buf := fmt.Sprintf("%x|%s|%x|%s", id, timestampB, expire, kWbSalt)
	bytes := md5.Sum([]byte(md5_buf))
	md5Str := bs2b(bytes[0:])
	return fmt.Sprintf("%s|%s", buf, md5Str)
}

// GenerateDeviceKey 生成设备key
func GenerateDeviceKey(mac string, id int64) string {
	h := hmac.New(sha256.New, kDeviceSalt)
	h.Reset()
	h.Write([]byte(mac))
	return fmt.Sprintf("%x|%s", id, base64.StdEncoding.EncodeToString(h.Sum([]byte("bc"))))
}

func extractDeviceId(cookie string) int64 {
	strs := strings.SplitN(cookie, "|", 2)
	if len(strs) != 2 {
		return -1
	}
	id, err := strconv.ParseInt(strs[0], 16, 64)
	if err != nil {
		return -1
	}
	return id
}

// VerifyDeviceKey 校验设备的key
func VerifyDeviceKey(mac string, cookie string) bool {
	id := extractDeviceId(cookie)
	if id <= 0 {
		return false
	}
	return GenerateDeviceKey(mac, id) == cookie
}

// 依据时间尽可能生成顺序增长的ID，但仍有重复的几率，需要测试
func GenerateTimeKey() uint64 {
	var id uint64 = ((uint64(time.Now().UnixNano() - kTimeStart) << 24) |
					 ((uint64(os.Getpid()) & 0xFF) << 8) |
					 (uint64(rand.Int31n(256) & 0xFF)))
	return id
}

func GenerateRandomKey(length int) []byte {
	key := make([]byte, length)
	for i, c := 0, len(key); i < c; i++ {
		key[i] = byte(rand.Int() % 256)
	}
	return key
}

func GenerateRandomString(length int) string {
	const alphanum = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	var bytes = make([]byte, length)
	crand.Read(bytes)
	for i, b := range bytes {
		bytes[i] = alphanum[b%byte(len(alphanum))]
	}
	return string(bytes)
}
