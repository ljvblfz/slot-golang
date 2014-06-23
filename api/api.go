// 实现所有的api逻辑
package main

import (
	"errors"
	"time"

	"github.com/golang/glog"
)

// RegisterUser 用户注册
func RegisterUser(email string, passwd string, phone string) (*User, *ApiErr) {
	if len(passwd) < 6 {
		return nil, NewError(ErrPasswordTooShort, errors.New("password too short"))
	}
	newId, err := NewUserId()
	if err != nil {
		return nil, err
	}
	u := &User{
		Id: int64(newId),
		Email: email,
		Passwd: passwd,
		Phone: phone,
	}
	glog.Infof("[RegisterUser] email: %s\n", email)
	err = u.Register()
	if err != nil {
		glog.Infof("[RegisterUser failed] email: %s, error: %v", email, err)
		return nil, err
	}
	return u, err
}

// LoginUser 用户登录
func LoginUser(email string, passwd string) (*User, *ApiErr) {
	u := &User{
		Email: email,
		Passwd: passwd,
	}
	glog.Infof("[LoginUser] email: %s\n", email)
	err := u.Login()
	if err != nil {
		glog.Infof("[LoginUser failed] email: %s, error: %v", email, err)
		return nil, err
	}
	return u, err
}

// GetUserDevices 获取用户绑定的所有设备
func GetUserDevices(uid int64) ([]int64, *ApiErr) {
	u := &User{Id: uid}
	glog.Infof("[GetUserDevices] uid: %d\n", uid)
	err := u.GetDevices()
	if err != nil {
		glog.Infof("[GetUserDevices failed] uid: %, error: %v", uid, err)
		return nil, err
	}
	return u.BindedDevices, nil
}

// RegisterDevice 设备注册
func RegisterDevice(mac string, sn string) (*Device, *ApiErr) {
	if len(mac) == 0 {
		return nil, NewError(ErrInvalidMac, nil)
	}
	if len(sn) == 0 {
		return nil, NewError(ErrInvalidSn, nil)
	}

	//newId, err := NewDeviceId()
	//if err != nil {
	//	return nil, err
	//}
	d := &Device{
		//Id: newId,
		Mac: mac,
		Sn: sn,
		RegisterTime: time.Now(),
	}

	glog.Infof("[RegisterDevice] mac: %s, sn: %s\n", mac, sn)
	apiErr := d.Register()
	if apiErr != nil {
		// 如果是重复注册，会把原有第一次注册的id取出来
		if apiErr.Code == ErrMacRegistered && d.Id > 0 {
			d.Key = GenerateDeviceKey(mac, d.Id)
			glog.Infof("[RegisterDevice repeat] mac: %s, sn: %s", mac, sn)
			return d, nil
		}
		glog.Infof("[RegisterDevice failed] mac: %s, sn: %s, error: %v", mac, sn, apiErr)
		return nil, apiErr
	}
	d.Key = GenerateDeviceKey(mac, d.Id)
	return d, apiErr
}

// VerifySn 校验sn是否合法
func VerifySn(sn string) bool {
	// TODO(yy) 暂时缺少实现
	return true
}

// LoginDevice 设备登录
func LoginDevice(mac string) (*Device, *ApiErr) {
	d := &Device{
		Mac: mac,
	}
	glog.Infof("[LoginDevice] mac: %s\n", mac)
	err := d.Login()
	if err != nil {
		glog.Infof("[LoginDevice failed] mac: %s, error: %v", mac, err)
		return nil, err
	}
	return d, err
}

// BindDevice 绑定用户和设备
func BindDevice(dId int64, userId int64) *ApiErr {
	if dId <= 0 || userId <= 0 {
		return NewError(ErrInvalidId, nil)
	}
	b := &Bind{
		DeviceId: dId,
		UserId: userId,
	}
	glog.Infof("[Bind] deviceId: %d, userid: %d\n", dId, userId)
	err := b.Bind()
	if err != nil {
		glog.Infof("[Bind failed] deviceId: %d, userid: %d, error: %v\n", dId, userId, err)
	}
	return err
}

// UnbindDevice 解绑用户和设备
func UnbindDevice(dId int64, userId int64) *ApiErr {
	if dId <= 0 || userId <= 0 {
		return NewError(ErrInvalidId, nil)
	}
	b := &Bind{
		DeviceId: dId,
		UserId: userId,
	}
	glog.Infof("[Unbind] deviceId: %d, userid: %d\n", dId, userId)
	err := b.Unbind()
	if err != nil {
		glog.Infof("[Unbind failed] deviceId: %d, userid: %d, error: %v\n", dId, userId, err)
	}
	return err
}
