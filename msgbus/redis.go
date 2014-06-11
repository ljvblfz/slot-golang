package main

import (
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/golang/glog"
	"time"
)

const (
	Hosts     = "Host:*"
	HostUsers = "Host:%s"
	SubKey    = "PubKey"
)
const (
	_GetAllHosts = iota
	_GetAllUsers
	_SubKey
	_Max
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
				glog.Info(err)
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
		r := Redix[i].Get()
		_, err := r.Do("PING")
		if err != nil {
			glog.Fatalf("Connect Redis [%s] failed [%s]\n", addr, err.Error())
		}
		r.Close()
		glog.Infof("RedisPool[%d] Init OK\n", i)
	}
}

func GetAllHosts() ([]string, error) {
	r := Redix[_GetAllHosts].Get()
	ret, err := redis.Strings(r.Do("keys", Hosts))
	r.Close()
	return ret, err
}

// 设置设备的状态，上线还是下线
func GetAllUsers(hosts []string) error {
	r := Redix[_GetAllUsers].Get()
	var (
		err   error
		users []string
	)
	for _, host := range hosts {
		users, _ = redis.Strings(r.Do("hkeys", fmt.Sprintf(HostUsers, host)))
		GUserMap.Load(users, host)
	}
	r.Close()
	return err
}

func SubUserState() (<-chan []byte, error) {
	r := Redix[_SubKey].Get()
	psc := redis.PubSubConn{Conn: r}
	ch := make(chan []byte, 128)
	go func() {
		defer r.Close()
		for {
			switch n := psc.Receive().(type) {
			case redis.Message:
				glog.Infof("Message: %s %s\n", n.Channel, n.Data)
				ch <- n.Data
			// case redis.PMessage:
			// 	fmt.Printf("PMessage: %s %s %s\n", n.Pattern, n.Channel, n.Data)
			case redis.Subscription:
				if n.Count == 0 {
					glog.Fatalf("Subscription: %s %s %d\n", n.Kind, n.Channel, n.Count)
					return
				}
			case error:
				glog.Errorf("error: %v\n", n)
				return
			}
		}
	}()
	psc.Subscribe(SubKey)
	return ch, nil
}
