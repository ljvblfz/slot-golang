package main

import (
	"fmt"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/golang/glog"
)

var (
	gPool *redis.Pool
)

const (
	// redis hash表名，及其key/value类型

	nsUserNewId		= "user:nextid"		// int64,用于记录下一个用户ID
	nsUserMailToId	= "user:mailtoid"	// key:mail(string), value:id(int64)
	nsUserEmail		= "user:email"		// key:id(int64), value:string
	nsUserPhone		= "user:phone"		// key:id(int64), value:string
	nsUserShadow	= "user:shadow"		// key:id(int64), value:string

	nsDeviceNewId	= "device:nextid"	// int64,用于记录下一个设备ID
	nsDeviceMacToId	= "device:mactoid"	// key:id(int64), value:bool
	nsDeviceMac		= "device:mac"		// key:id(int64), value:string
	nsDeviceSn		= "device:sn"		// key:id(int64), value:string
	//nsDeviceKey		= "device:key"		// key:id(int64), value:string
	nsDeviceRegTime = "device:regtime"	// key:id(int64), value:int64

	nsBindDevice	= "bind:device"		// sets, tablename+=deviceid
	nsBindUser		= "bind:user"		// sets, tablename+=userid
)

func InitRedis(addr string, passwd string) {
	gPool = &redis.Pool{
        MaxIdle: 5, // 可根据实际情况调整
        // MaxActive:   100,
        IdleTimeout: 10 * time.Second,
        Dial: func() (redis.Conn, error) {
            c, err := redis.Dial("tcp", addr)
            if err != nil {
                return nil, err
            }
            if passwd != "" {
                if _, err := c.Do("AUTH", passwd); err != nil {
                    c.Close()
                    return nil, err
                }
            }
            return c, err
        },
        TestOnBorrow: func(c redis.Conn, t time.Time) error {
            _, err := c.Do("PING")
            if err != nil {
                glog.Errorln(err)
            }
            return err
        },
    }
}

// Models
type User struct {
	Id				int64
	Email			string
	Passwd			string
	Phone			string
	BindedDevices	[]int64
}

// NewId get a new unique id from redis
func NewUserId() (int64, *ApiErr) {
	c := gPool.Get()
	defer c.Close()

	id, err := redis.Int64(c.Do("incr", nsUserNewId))
	if err != nil {
		return 0, NewError(ErrRedisError, err)
	}
	return id, nil
}

func (u *User) Register() *ApiErr {
	c := gPool.Get()
	defer c.Close()

	// do validation
	existId, err := redis.Int64(c.Do("hget", nsUserMailToId, u.Email))
	if err == nil || existId > 0 {
		return NewError(ErrEmailExist, nil)
	}
	emailExist, err := redis.String(c.Do("hget", nsUserEmail, u.Id, u.Email))
	if err == nil || len(emailExist) > 0 {
		return NewError(ErrUserIdExist, nil)
	}

	// do write
	err = c.Send("hsetnx", nsUserMailToId, u.Email, u.Id)
	if err != nil {
		glog.Fatalln(err)
		return NewError(ErrRedisError, err)
	}
	err = c.Send("hset", nsUserEmail, u.Id, u.Email)
	if err != nil {
		glog.Fatalln(err)
		return NewError(ErrRedisError, err)
	}
	if len(u.Phone) > 0 {
		err = c.Send("hset", nsUserPhone, u.Id, u.Phone)
		if err != nil {
			glog.Fatalln(err)
			return NewError(ErrRedisError, err)
		}
	}

	// generate password shadown
	pwdShadow := EncodePassword(u.Passwd, GenerateRandomString(kSaltLen))
	err = c.Send("hset", nsUserShadow, u.Id, pwdShadow)
	if err != nil {
		glog.Fatalln(err)
		return NewError(ErrRedisError, err)
	}
	err = c.Flush()
	if err != nil {
		glog.Fatalln(err)
		return NewError(ErrRedisError, err)
	}

	if _, err = c.Receive(); err != nil {
		return NewError(ErrRedisError, err)
	}
	if _, err = c.Receive(); err != nil {
		return NewError(ErrRedisError, err)
	}
	if len(u.Phone) > 0 {
		if _, err = c.Receive(); err != nil {
			return NewError(ErrRedisError, err)
		}
	}
	if _, err = c.Receive(); err != nil {
		return NewError(ErrRedisError, err)
	}
	return nil
}

func (u *User) Login() *ApiErr {
	c := gPool.Get()
	defer c.Close()

	// Fetch info for login
	id, err := redis.Int64(c.Do("hget", nsUserMailToId, u.Email))
	if err != nil || id == 0 {
		return NewError(ErrEmailNotExist, nil)
	}

	encodedPwd, err := redis.String(c.Do("hget", nsUserShadow, id))
	if err != nil {
		return NewError(ErrRedisError, err)
	}
	if len(encodedPwd) == 0 {
		return NewError(ErrServerWrong1, nil)
	}

	// 邮箱或密码任一不匹配，返回同一个值
	if !VerifyPassword(u.Passwd, encodedPwd) {
		return NewError(ErrEmailOrPwdWrong, nil)
	}
	u.Id = id

	return u.getDevices(&c)
}

func (u *User) GetDevices() *ApiErr {
	c := gPool.Get()
	defer c.Close()

	return u.getDevices(&c)
}

func (u *User) getDevices(c *redis.Conn) *ApiErr {
	ids, e := redis.Values((*c).Do("smembers", fmt.Sprintf("%s:%d", nsBindUser, u.Id)))
	if e != nil {
		return NewError(ErrRedisError, nil)
	}
	for _, v := range ids {
		did, err1 := redis.Int64(v, e)
		if err1 == nil {
			u.BindedDevices = append(u.BindedDevices, did)
		} else {
			glog.Warningf("Bind error: wrong device'id type, user table: %s, value: %v\n", fmt.Sprintf("%s:%d", nsBindUser, u.Id), v)
		}
	}
	return nil
}

// Device 设备
type Device struct {
	Id				int64
	Mac				string
	Sn				string
	Key				string
	RegisterTime	time.Time
	BindedUsers		[]int64
}

func NewDeviceId() (int64, *ApiErr) {
	c := gPool.Get()
	defer c.Close()

	id, err := redis.Int64(c.Do("incr", nsDeviceNewId))
	if err != nil {
		return 0, NewError(ErrRedisError, err)
	}
	return id, nil
}

func (d *Device) GetIdByMac() *ApiErr {
	if len(d.Mac) == 0 {
		return NewError(ErrInvalidMac, nil)
	}
	c := gPool.Get()
	defer c.Close()

	id, err := redis.Int64(c.Do("hget", nsDeviceMacToId, d.Mac))
	if err != nil {
		return NewError(ErrRedisError, err)
	}
	if id <= 0 {
		return NewError(ErrNotRegistered, nil)
	}
	d.Id = id
	return nil
}

func (d *Device) Register() *ApiErr {
	c := gPool.Get()
	defer c.Close()

	err := c.Send("hsetnx", nsDeviceMacToId, d.Mac, d.Id)
	if err != nil {
		return NewError(ErrRedisError, err)
	}
	err = c.Send("hsetnx", nsDeviceMac, d.Id, d.Mac)
	if err != nil {
		return NewError(ErrRedisError, err)
	}
	err = c.Send("hset", nsDeviceSn, d.Id, d.Mac)
	if err != nil {
		return NewError(ErrRedisError, err)
	}
	//err = c.Send("hset", nsDeviceKey, d.Id, d.Key)
	//if err != nil {
	//	return NewError(ErrRedisError, err)
	//}
	err = c.Send("hset", nsDeviceRegTime, d.Id, d.RegisterTime.Unix())
	if err != nil {
		return NewError(ErrRedisError, err)
	}

	c.Flush()

	if _, err = c.Receive(); err != nil {
		return NewError(ErrRedisError, err)
	}
	if _, err = c.Receive(); err != nil {
		return NewError(ErrRedisError, err)
	}
	if _, err = c.Receive(); err != nil {
		return NewError(ErrRedisError, err)
	}
	//if _, err = c.Receive(); err != nil {
	//	return NewError(ErrRedisError, err)
	//}
	if _, err = c.Receive(); err != nil {
		return NewError(ErrRedisError, err)
	}
	return nil
}

func (d *Device) Login() *ApiErr {
	if len(d.Mac) == 0 {
		return NewError(ErrInvalidMac, nil)
	}
	c := gPool.Get()
	defer c.Close()

	// get deviceId
	id, err := redis.Int64(c.Do("hget", nsDeviceMacToId, d.Mac))
	if err != nil {
		return NewError(ErrRedisError, err)
	}
	if id <= 0 {
		return NewError(ErrNotRegistered, nil)
	}

	d.Id = id

	ids, e := redis.Values(c.Do("smembers", fmt.Sprintf("%s:%d", nsBindDevice, id)))
	if e != nil {
		return NewError(ErrRedisError, nil)
	}
	for _, v := range ids {
		uid, err1 := redis.Int64(v, err)
		if err1 == nil {
			d.BindedUsers = append(d.BindedUsers, uid)
		} else {
			glog.Warningf("bind error: wrong user'id type, device table: %s, value: %v\n", fmt.Sprintf("%s:%d", nsBindDevice, id), v)
		}
	}

	return nil
}

// Bind 设备与用户的绑定关系
type Bind struct {
	DeviceId	int64
	UserId		int64
}

func (b *Bind) Bind() *ApiErr {
	c := gPool.Get()
	defer c.Close()

	// device table <- user id
	n, err := redis.Int(c.Do("sadd", fmt.Sprintf("%s:%d", nsBindDevice, b.DeviceId), b.UserId))
	if err != nil {
		return NewError(ErrRedisError, err)
	}
	if n != 1 {
		return NewError(ErrRepeatBind, nil)
	}

	// user table <- device id
	n, err = redis.Int(c.Do("sadd", fmt.Sprintf("%s:%d", nsBindUser, b.UserId), b.DeviceId))
	if err != nil {
		glog.Errorf("Bind failed, second redis 'Do' failed, but first 'Do' successful, deviceid: %d, userid: %d, error: %v\n", b.DeviceId, b.UserId, err)
		return NewError(ErrRedisError, err)
	}
	if n != 1 {
		glog.Errorf("Bind device to user failed, but user to device successful, deviceid: %d, userid: %d\n", b.DeviceId, b.UserId)
		return NewError(ErrRepeatBind, nil)
	}
	return nil
}

func (b *Bind) Unbind() *ApiErr {
	c := gPool.Get()
	defer c.Close()

	// pop from device table
	fmt.Println("srem", fmt.Sprintf("%s:%d", nsBindDevice, b.DeviceId), b.UserId)
	n, err := redis.Int(c.Do("srem", fmt.Sprintf("%s:%d", nsBindDevice, b.DeviceId), b.UserId))
	if err != nil {
		return NewError(ErrRedisError, err)
	}
	if n != 1 {
		return NewError(ErrNeverBind, nil)
	}

	// pop from user table
	n, err = redis.Int(c.Do("srem", fmt.Sprintf("%s:%d", nsBindUser, b.UserId), b.DeviceId))
	if err != nil {
		glog.Errorf("Unbind failed, second redis 'Do' failed, but first 'Do' successful, deviceid: %d, userid: %d, error: %v\n", b.DeviceId, b.UserId, err)
		return NewError(ErrRedisError, err)
	}
	if n != 1 {
		glog.Errorf("Unbind device to user failed, but user to device successful, deviceid: %d, userid: %d\n", b.DeviceId, b.UserId)
		return NewError(ErrNeverBind, nil)
	}
	return nil
}
