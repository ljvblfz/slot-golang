package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	privateKey = "black_crystal"
)

var (
	ErrorInvalidId = errors.New("Invalid parameter id")
	ErrorInvalidCode = errors.New("Invalid parameter code")

	kAesKey []byte
)

func init() {
	kAesKey = GenerateRandomKey(aes.BlockSize)
}

// Encode 计算id,expire的加密
func Encode(id int64, expire int64) (code []byte, err error) {
	if id == 0 {
		return nil, ErrorInvalidId
	}
	sha1Value := encodeSha1(id, expire)
	beforeAes := []byte(fmt.Sprintf("%d|%d|%s", id, expire, string(sha1Value)))

	block, err := aes.NewCipher(kAesKey)
	if err != nil {
		return nil, err
	}

	code = make([]byte, len(beforeAes))

	stream := cipher.NewCFBEncrypter(block, kAesKey)
	stream.XORKeyStream(code, beforeAes)

	return code, nil
}

// Decode 校验并解密id,expire
func Decode(code []byte) (int64, int64, error) {
	if len(code) < aes.BlockSize {
		return 0, 0, ErrorInvalidCode
	}

	block, err := aes.NewCipher(kAesKey)
	if err != nil {
		return 0, 0, err
	}

	stream := cipher.NewCFBDecrypter(block, kAesKey)
	stream.XORKeyStream(code, code)

	result := code[0:]

	parts := strings.SplitN(string(result), "|", 3)
	if len(parts) != 3 {
		return 0, 0, ErrorInvalidCode
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, ErrorInvalidCode
	}
	expire, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, 0, ErrorInvalidCode
	}

	sha1Value := encodeSha1(id, expire)
	if !bytes.Equal([]byte(parts[2]), sha1Value) {
		//msg := fmt.Sprintf("sha1 value not matched: %v != %v", []byte(parts[2]), sha1Value)
		return 0, 0, ErrorInvalidCode
	}
	return id, expire, nil
}

func encodeSha1(id int64, expire int64) []byte {
	beforeSha1 := fmt.Sprintf("%d|%d|%s", id, expire, privateKey)
	sha1Value := sha1.Sum([]byte(beforeSha1))
	return []byte(sha1Value[0:])
}
