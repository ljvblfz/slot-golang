package main

import (
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/golang/glog"
	"time"
)

const (
	HostUsers = "Host:%s" // (1, 2, 3)
	PubKey    = "PubKey"
)
const (
	_SetUserOnline = iota
	_SetUserOffline
	_PubState
	_Max
)

var (
	Redix []*redis.Pool
)
var redisAddr string

func newPool(server, password string) *redis.Pool {
	return &redis.Pool{
		MaxIdle: 100,
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
	Redix = make([]*redis.Pool, _Max)
	redisAddr = addr
	for i := 0; i < _Max; i++ {
		Redix[i] = newPool(redisAddr, "")
		glog.Infof("RedisPool[%d] Init OK\n", i)
	}
}

func SetUserOnline(uid int64, host string) (bool, error) {
	r := Redix[_SetUserOnline].Get()
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
	r.Close()
	return ret == 1, err
}

func SetUserOffline(uid int64, host string) error {
	r := Redix[_SetUserOnline].Get()
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
	r.Close()
	return err
}
