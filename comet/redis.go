package main

import (
	"fmt"
	"github.com/garyburd/redigo/redis"
	"log"
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
)

var (
	Redix []*redis.Pool
)
var redisAddr string

func newPool(server, password string) *redis.Pool {
	return &redis.Pool{
		MaxIdle: 5,
		// MaxActive:   100,
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
				log.Println(err)
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
		log.Printf("RedisPool[%d] Init OK\n", i)
	}
}

// 设置设备的状态，上线还是下线
func SetUserOnline(uid int64, host int32) (bool, error) {
	r := Redix[_SetUserOnline].Get()
	var (
		err error
		ret int
	)
	ret, err = redis.Bool(r.Do("hincrby", HostUsers, uid, 1))
	if ret == 1 {
		_, err = r.Do("publish", PubKey, fmt.Sprintf("%d|%d", uid, host))
		if err != nil {
			r.Close()
			return false, err
		}
	}
	r.Close()
	return exist, err
}

// 记录设备最后一次的状态
func SetUserOffline(uid uint32, data string) error {
	r := Redix[_SetUserOnline].Get()
	var (
		err error
		ret int
	)
	ret, err = redis.Int(r.Do("hincrby", HostUsers, uid, -1))
	if err != nil {
		r.Close()
		return err
	}
	// TODO 增加脚本保证事务全部执行
	if ret <= 0 {
		_, err = r.Do("publish", PubKey, fmt.Sprintf("%d|%d", -uid, host))
		if err != nil {
			r.Close()
			return false, err
		}
		_, err = r.Do("hdel", HostUsers, uid)
		if err != nil {
			r.Close()
			return false, err
		}
	}
	return err
}
