// 定义所有的错误值，用于和前段交流
package main

import "fmt"

const (
	// 错误码表，新增错误值必须放到末尾
	ErrOk = iota
	ErrRedisError
	ErrUserIdExist
	ErrEmailExist
	ErrEmailNotExist
	ErrInvalidId
	ErrEmailOrPwdWrong
	ErrPasswordTooShort
	ErrInvalidMac
	ErrInvalidSn
	ErrNotRegistered
	ErrRepeatBind
	ErrNeverBind

	// 复杂不易描述的错误，主要用于内部错误定义
	ErrServerWrong1		// Shadow is empty

	// Add new code here:
)

// ApiErr 预定义的错误类型，用于具体错误的定义和描述
type ApiErr struct {
	Code	int
	Msg		error
}

func (e ApiErr) Error() string {
	m := ""
	if e.Msg != nil {
		m = e.Msg.Error()
	}
	return fmt.Sprintf("%d(%v)", e.Code, m)
}

// NewError 生成错误码，并携带具体描述:w
func NewError(code int, msg error) *ApiErr {
	return &ApiErr{Code: code, Msg: msg}
}
