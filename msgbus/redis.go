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
	OnOff     = "OnOff"
)
const (
	_GetAllHosts = iota
	_GetAllUsers
	_SubLoginState
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

func InitModel(addr string) {
	initRedix(addr)
}

func LoadUsers() error {
	hosts, err := GetAllHosts()
	glog.Infoln("allhosts:", hosts)
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
		glog.Infoln("got users %v from comet-%v", users, host)
		if e != nil {
			glog.Errorf("redis error %v", e)
			continue
		}
		GUserMap.load(users, hostName(host))
	}
	r.Close()
	return err
}

func SubUserState() (<-chan []byte, error) {
	r := Redix[_SubLoginState].Get()

	psc := redis.PubSubConn{Conn: r}
	psc.Subscribe(OnOff)
	ch := make(chan []byte, 128)

	// 单独处理Subscribe后的第一个消息（即返回值），用于保证在已监听到SubKey后，
	// 本函数返回，主函数继续载入已有的设备在线列表。用于避免Msgbus和Comet都刚刚
	// 启动时，Comet删除上次的旧设备列表和立刻登录的新设备之间的时间窗口中，Msgbus
	// 由于异步监听SubKey，导致先载入了旧的列表，后监听到SubKey，导致遗漏了Comet
	// 发出的删除旧列表通知。
	data := psc.Receive()
	switch n := data.(type) {
	case redis.Subscription:
		if n.Count == 0 {
			glog.Fatalf("Subscription: %s %s %d, %v\n", n.Kind, n.Channel, n.Count, n)
		}
	case error:
		glog.Errorf("subscribe on LoginState error: %v\n", n)
		return nil, n
	}

	go func() {
		defer psc.Close()
		for {
			data := psc.Receive()
			switch n := data.(type) {
			case redis.Message:
				ch <- n.Data
				//if glog.V(1) {
				//	glog.Infof("Message: %s %s\n", n.Channel, n.Data)
				//}
			case error:
				glog.Errorf("error: %v\n", n)
				return
			}
		}
	}()
	return ch, nil
}
