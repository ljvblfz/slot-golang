package main

import (
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/golang/glog"
	"strconv"
	"sync"
	"time"
)

const (
	HostUsers = "Host:%s" // (1, 2, 3)
	PubKey    = "PubKey"

    RedisDeviceUsers = "bind:device"
	RedisUserDevices = "bind:user"
)
const (
	_SetUserOnline = iota
	_SetUserOffline
	_GetDeviceUsers
	_GetUserDevices
	_Max
)

var (
	Redix   []redis.Conn
	RedixMu []*sync.Mutex
)
var redisAddr string

func newPool(server, password string) *redis.Pool {
	return &redis.Pool{
		MaxIdle:     100,
		MaxActive:   100,
		IdleTimeout: 10 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", server)
			if err != nil {
				return nil, err
			}
			if password != "" {
				if _, err := c.Do("AUTH", password); err != nil {
					c.Close()
					return nil, err
				}
			}
			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			if err != nil {
				glog.Error(err)
			}
			return err
		},
	}
}

func initRedix(addr string) {
	Redix = make([]redis.Conn, _Max)
	RedixMu = make([]*sync.Mutex, _Max)

	// redisAddr = addr
	var err error
	for i := 0; i < _Max; i++ {
		Redix[i], err = redis.Dial("tcp", addr)
		if err != nil {
			panic(err)
		}
		RedixMu[i] = &sync.Mutex{}
		// Redix[i] = newPool(redisAddr, "")
		glog.Infof("RedisPool[%d] Init OK on %s\n", i, addr)
	}
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
	// r.Close()
	return ret == 1, err
}

func SetUserOffline(uid int64, host string) error {
	// r := Redix[_SetUserOffline].Get()
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
	// TODO 增加脚本保证事务全部执行
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
	// r.Close()
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
