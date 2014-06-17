package main

import (
	//"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/golang/glog"
	//"strconv"
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
		//IdleTimeout: 10 * time.Second,
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

func InitModel(addr string) error {
	initRedix(addr)
	hosts, err := GetAllHosts()
	if err != nil {
		return err
	}
	return GetAllUsers(hosts)
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
		err error
		//users []string
	)
	for _, host := range hosts {
		users, e := redis.Strings(r.Do("hkeys", host))
		if e != nil {
			glog.Errorf("redis error %v", e)
			continue
		}
		GUserMap.Load(users, hostName(host))
	}
	r.Close()
	return err
}

func SubUserState() (<-chan []byte, error) {
	r := Redix[_SubKey].Get()

	//replys, err := redis.Strings(r.Do("config", "get", "timeout"))
	//if err != nil || len(replys) != 2 || replys[0] != "timeout" {
	//	glog.Errorf("Redis error %v", err)
	//	defer r.Close()
	//	return nil, err
	//}
	//timeout, _ := strconv.Atoi(replys[1])
	//if timeout > 0 {
	//	newTimeout := int(float64(timeout) * 0.9)
	//	if newTimeout == 0 {
	//		newTimeout = 1
	//	}
	//	if newTimeout > timeout {
	//		newTimeout = timeout
	//	}
	//	glog.Infof("start ping redis for %d sec", newTimeout)
	//	go func() {
	//		for {
	//			select {
	//			case <-time.After(time.Duration(newTimeout) * time.Second):
	//				_, err := r.Do("PING")
	//				glog.Info("ping...")
	//				if err != nil {
	//					return
	//				}
	//			}
	//		}
	//	}()
	//} else {
	//	glog.Infof("redis has no timeout")
	//}
	psc := redis.PubSubConn{Conn: r}
	psc.Subscribe(SubKey)
	ch := make(chan []byte, 128)
	go func() {
		defer psc.Close()
		for {
			data := psc.Receive()
			switch n := data.(type) {
			case redis.Message:
				glog.Infof("Message: %s %s\n", n.Channel, n.Data)
				ch <- n.Data
			// case redis.PMessage:
			// 	fmt.Printf("PMessage: %s %s %s\n", n.Pattern, n.Channel, n.Data)
			case redis.Subscription:
				if n.Count == 0 {
					glog.Fatalf("Subscription: %s %s %d, %v\n", n.Kind, n.Channel, n.Count, n)
					return
				}
			case error:
				glog.Errorf("error: %v\n", n)
				return
			}
		}
	}()
	return ch, nil
}
