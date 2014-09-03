package main

import (
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/golang/glog"
	"strconv"
	"sync"
)

const (
	HostUsers = "Host:%s" // (1, 2, 3)
	PubKey    = "PubKey"

    RedisDeviceUsers = "bind:device"
	RedisUserDevices = "bind:user"

	// 用户正在使用的手机id
	RedisUserMobiles = "user:mobileid"
)
const (
	_SetUserOnline = iota
	_SetUserOffline
	_GetDeviceUsers
	_GetUserDevices
	_SelectMobileId
	_ReturnMobileId
	_Max

	// 为用户挑选一个[1,15]中未使用的手机id
	_scriptSelectMobileId = `
for mid=1,15,1 do
	if 1 == redis.call('sadd', KEYS[1], mid) then
		return mid
	end
end
return 0
`
)

var (
	Redix   []redis.Conn
	RedixMu []*sync.Mutex

	ScriptSelectMobileId *redis.Script
)
var redisAddr string

func initRedix(addr string) {
	Redix = make([]redis.Conn, _Max)
	RedixMu = make([]*sync.Mutex, _Max)

	var err error
	for i := 0; i < _Max; i++ {
		Redix[i], err = redis.Dial("tcp", addr)
		if err != nil {
			panic(err)
		}
		RedixMu[i] = &sync.Mutex{}
		glog.Infof("RedisPool[%d] Init OK on %s\n", i, addr)
	}
	ScriptSelectMobileId = redis.NewScript(1, _scriptSelectMobileId)
	ScriptSelectMobileId.Load(Redix[_SelectMobileId])
}

func SetUserOnline(uid int64, host string) (bool, error) {
	r := Redix[_SetUserOnline]
	RedixMu[_SetUserOnline].Lock()
	defer RedixMu[_SetUserOnline].Unlock()

	ret, err := redis.Int(r.Do("hincrby", fmt.Sprintf(HostUsers, host), uid, 1))
	if err != nil {
		r.Close()
		return false, err
	}
	if ret == 1 {
		_, err = r.Do("publish", PubKey, fmt.Sprintf("%d|%s|%d", uid, host, 1))
		if err != nil {
			r.Close()
			return false, err
		}
	}
	return ret == 1, err
}

func SetUserOffline(uid int64, host string) error {
	r := Redix[_SetUserOffline]
	RedixMu[_SetUserOffline].Lock()
	defer RedixMu[_SetUserOffline].Unlock()
	var (
		err error
		ret int
	)
	ret, err = redis.Int(r.Do("hincrby", fmt.Sprintf(HostUsers, host), uid, -1))
	if err != nil {
		r.Close()
		return err
	}
	if ret <= 0 {
		_, err = r.Do("publish", PubKey, fmt.Sprintf("%d|%s|%d", uid, host, 0))
		if err != nil {
			r.Close()
			return err
		}
		_, err = r.Do("hdel", fmt.Sprintf(HostUsers, host), uid)
		if err != nil {
			r.Close()
			return err
		}
	}
	return err
}

func GetDeviceUsers(deviceId int64) ([]int64, error) {
	r := Redix[_GetDeviceUsers]
	RedixMu[_GetDeviceUsers].Lock()
	defer RedixMu[_GetDeviceUsers].Unlock()
	users, err := redis.Strings(r.Do("smembers", fmt.Sprintf("%s:%d", RedisDeviceUsers, deviceId)))
	if err != nil {
		return nil, err
	}
	bindedIds := make([]int64, 0, len(users))
    for _, user_id := range users {
        u_id, err := strconv.ParseInt(user_id, 10, 64)
		if err != nil {
			continue
		}
		bindedIds = append(bindedIds, int64(u_id))
	}
	if len(bindedIds) == len(users) {
		err = nil
	}
	return bindedIds, err
}


func GetUserDevices(userId int64) ([]int64, error) {
	r := Redix[_GetUserDevices]
	RedixMu[_GetUserDevices].Lock()
	defer RedixMu[_GetUserDevices].Unlock()

	idStrs, err := redis.Strings(r.Do("smembers", fmt.Sprintf("%s:%d", RedisUserDevices, userId)))
	if err != nil {
		return nil, err
	}
	bindedIds := make([]int64, 0, len(idStrs))
    for _, v := range idStrs {
        id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			continue
		}
		bindedIds = append(bindedIds, id)
	}
	if len(bindedIds) == len(idStrs) {
		err = nil
	}
	return bindedIds, err
}

// 为用户uid挑选一个1到15内的未使用的手机子id
func SelectMobileId(uid int64) (int, error) {
	r := Redix[_SelectMobileId]
	RedixMu[_SelectMobileId].Lock()
	defer RedixMu[_SelectMobileId].Unlock()

	return redis.Int(ScriptSelectMobileId.Do(r, fmt.Sprintf("%s:%d", RedisUserMobiles, uid)))
}

func ReturnMobileId(userId int64, mid byte) error {
	r := Redix[_ReturnMobileId]
	RedixMu[_ReturnMobileId].Lock()
	defer RedixMu[_ReturnMobileId].Unlock()

	_, err := r.Do("srem", fmt.Sprintf("%s:%d", RedisUserMobiles, userId), mid)

	return err
}
