package main

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	//"time"

	"github.com/garyburd/redigo/redis"
	"github.com/golang/glog"
	"github.com/hjr265/redsync.go/redsync"
)

const (
	HostUsers             = "Host:%s" // (1, 2, 3)
	PubKey                = "PubKey"
	SubDeviceUsersKey     = "PubDeviceUsers"
	SubModifiedPasswdKey  = "PubModifiedPasswdUser"
	SubCommonMsgKeyPrefix = "PubCommonMsg:"
	SubCommonMsgKey       = SubCommonMsgKeyPrefix + "*"

	RedisDeviceUsers = "bind:device"
	RedisUserDevices = "bind:user"

	// 用户正在使用的手机id
	RedisUserMobiles = "user:mobileid"

	RedisSessionDevice       = "sess:dev:%s"
	RedisSessionDeviceAddr   = "sess:dev:sid:%d"
	RedisSessionDeviceLocker = "sess:dev:%s:locker"
)
const (
	_SetUserOnline = iota
	_SetUserOffline
	_GetDeviceUsers
	_GetUserDevices
	_SelectMobileId
	_ReturnMobileId
	_SubDeviceUsersKey
	_SubModifiedPasswd
	_SubCommonMsg
	_GetDeviceSession
	_SetDeviceSession
	_DeleteDeviceSession
	//_ExpireDeviceSession
	_Max

	// eval, script, 2, htable, id, PubKey, cometIP
	_scriptOnline = `
local count = redis.call('hincrby', KEYS[1], KEYS[2], 1)
if count == 1 then
	redis.call('publish', ARGV[1], KEYS[2].."|"..ARGV[2].."|1")
end
return count
`

	_scriptOffline = `
local count = redis.call('hincrby', KEYS[1], KEYS[2], -1)
if count == 0 then
	redis.call('publish', ARGV[1], KEYS[2].."|"..ARGV[2].."|0")
	redis.call('hdel', KEYS[1], KEYS[2])
end
return count
`
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

	RedixAddr     string
	RedixAddrPool []net.Addr

	ScriptOnline         *redis.Script
	ScriptOffline        *redis.Script
	ScriptSelectMobileId *redis.Script
)
var redisAddr string

func InitRedix(addr string) {
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

	RedixAddr = addr
	netAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		panic(err)
	}
	RedixAddrPool = []net.Addr{netAddr}

	ScriptSelectMobileId = redis.NewScript(1, _scriptSelectMobileId)
	ScriptSelectMobileId.Load(Redix[_SelectMobileId])
	ScriptOnline = redis.NewScript(2, _scriptOnline)
	ScriptOnline.Load(Redix[_SetUserOnline])
	ScriptOffline = redis.NewScript(2, _scriptOffline)
	ScriptOffline.Load(Redix[_SetUserOffline])

	err = SubDeviceUsers()
	if err != nil {
		panic(err)
	}
	err = SubModifiedPasswd()
	if err != nil {
		panic(err)
	}
	if gCometType != CometUdp || gCometPushUdp {
		err = SubCommonMsg()
		if err != nil {
			panic(err)
		}
	}
}

func ClearRedis(ip string) error {
	r := Redix[_SetUserOnline]
	RedixMu[_SetUserOnline].Lock()
	defer RedixMu[_SetUserOnline].Unlock()

	err := r.Send("del", fmt.Sprintf("Host:%s", ip))
	if err != nil {
		return err
	}
	err = r.Send("publish", PubKey, fmt.Sprintf("0|%s|0", ip))
	if err != nil {
		return err
	}
	err = r.Flush()
	if err != nil {
		return err
	}
	_, err = r.Receive()
	if err != nil {
		return err
	}
	_, err = r.Receive()
	if err != nil {
		return err
	}
	return nil
}

func SetUserOnline(uid int64, host string) (bool, error) {
	r := Redix[_SetUserOnline]
	RedixMu[_SetUserOnline].Lock()
	defer RedixMu[_SetUserOnline].Unlock()

	// new
	return redis.Bool(ScriptOnline.Do(r,
		fmt.Sprintf(HostUsers, host),
		uid,
		PubKey,
		host,
	))

	// old
	//ret, err := redis.Int(r.Do("hincrby", fmt.Sprintf(HostUsers, host), uid, 1))
	//if err != nil {
	//	r.Close()
	//	return false, err
	//}
	//if ret == 1 {
	//	_, err = r.Do("publish", PubKey, fmt.Sprintf("%d|%s|%d", uid, host, 1))
	//	if err != nil {
	//		r.Close()
	//		return false, err
	//	}
	//}
	//return ret == 1, err
}

func SetUserOffline(uid int64, host string) error {
	r := Redix[_SetUserOffline]
	RedixMu[_SetUserOffline].Lock()
	defer RedixMu[_SetUserOffline].Unlock()
	// new
	_, err := redis.Bool(ScriptOffline.Do(r,
		fmt.Sprintf(HostUsers, host),
		uid,
		PubKey,
		host,
	))
	return err

	// old
	//var (
	//	err error
	//	ret int
	//)
	//ret, err = redis.Int(r.Do("hincrby", fmt.Sprintf(HostUsers, host), uid, -1))
	//if err != nil {
	//	r.Close()
	//	return err
	//}
	//if ret <= 0 {
	//	_, err = r.Do("publish", PubKey, fmt.Sprintf("%d|%s|%d", uid, host, 0))
	//	if err != nil {
	//		r.Close()
	//		return err
	//	}
	//	_, err = r.Do("hdel", fmt.Sprintf(HostUsers, host), uid)
	//	if err != nil {
	//		r.Close()
	//		return err
	//	}
	//}
	//return err
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

func SubDeviceUsers() error {
	r := Redix[_SubDeviceUsersKey]
	RedixMu[_SubDeviceUsersKey].Lock()
	defer RedixMu[_SubDeviceUsersKey].Unlock()

	psc := redis.PubSubConn{Conn: r}
	err := psc.Subscribe(SubDeviceUsersKey)
	if err != nil {
		return err
	}
	ch := make(chan []byte, 128)
	go func() {
		defer psc.Close()
		for {
			data := psc.Receive()
			switch n := data.(type) {
			case redis.Message:
				ch <- n.Data
			case redis.Subscription:
				if n.Count == 0 {
					glog.Fatalf("Subscription: %s %s %d, %v\n", n.Kind, n.Channel, n.Count, n)
					return
				}
			case error:
				glog.Errorf("[bind|redis] sub of error: %v\n", n)
				return
			}
		}
	}()
	go HandleDeviceUsers(ch)
	return nil
}

func HandleDeviceUsers(ch <-chan []byte) {
	for buf := range ch {
		if buf == nil {
			continue
		}

		strs := strings.SplitN(string(buf), "|", 3)
		if len(strs) != 3 || len(strs) == 0 {
			glog.Errorf("[binded id] invalid pub-sub msg format: %s", string(buf))
			continue
		}
		deviceId, err := strconv.ParseInt(strs[0], 10, 64)
		if err != nil {
			glog.Errorf("[binded id] invalid deviceId %s, error: %v", strs[0], err)
			continue
		}
		userId, err := strconv.ParseInt(strs[1], 10, 64)
		if err != nil {
			glog.Errorf("[binded id] invalid userId %s, error: %v", strs[1], err)
			continue
		}
		msgType, err := strconv.ParseInt(strs[2], 10, 32)
		if err != nil {
			glog.Errorf("[binded id] invalid bind type %s, error: %v", strs[2], err)
			continue
		}
		go gSessionList.UpdateIds(deviceId, userId, msgType != 0)
	}
}

func SubModifiedPasswd() error {
	r := Redix[_SubModifiedPasswd]
	RedixMu[_SubModifiedPasswd].Lock()
	defer RedixMu[_SubModifiedPasswd].Unlock()

	psc := redis.PubSubConn{Conn: r}
	err := psc.Subscribe(SubModifiedPasswdKey)
	if err != nil {
		return err
	}
	ch := make(chan []byte, 128)
	go func() {
		defer psc.Close()
		for {
			data := psc.Receive()
			switch n := data.(type) {
			case redis.Message:
				ch <- n.Data
			case redis.Subscription:
				if n.Count == 0 {
					glog.Fatalf("Subscription: %s %s %d, %v\n", n.Kind, n.Channel, n.Count, n)
					return
				}
			case error:
				glog.Errorf("[modifypwd|redis] sub of error: %v\n", n)
				return
			}
		}
	}()
	go HandleModifiedPasswd(ch)
	return nil
}

func HandleModifiedPasswd(ch <-chan []byte) {
	for buf := range ch {
		if buf == nil {
			continue
		}

		userId, err := strconv.ParseInt(string(buf), 10, 64)
		if err != nil {
			glog.Errorf("[modifiedpasswd] invalid userId %s, error: %v", string(buf), err)
			continue
		}
		go gSessionList.KickOffline(userId)
	}
}

func SubCommonMsg() error {
	r := Redix[_SubCommonMsg]
	RedixMu[_SubCommonMsg].Lock()
	defer RedixMu[_SubCommonMsg].Unlock()

	psc := redis.PubSubConn{Conn: r}
	err := psc.PSubscribe(SubCommonMsgKey)
	if err != nil {
		return err
	}
	ch := make(chan redis.PMessage, 128)
	go func() {
		defer psc.Close()
		for {
			data := psc.Receive()
			switch m := data.(type) {
			case redis.PMessage:
				ch <- m
			case redis.Subscription:
				if m.Count == 0 {
					glog.Fatalf("Subscription: %s %s %d, %v\n", m.Kind, m.Channel, m.Count, m)
					return
				}
			case error:
				glog.Errorf("[modifypwd|redis] sub of error: %v\n", m)
				return
			}
		}
	}()
	go HandleCommonMsg(ch)
	return nil
}

func HandleCommonMsg(ch <-chan redis.PMessage) {
	for m := range ch {
		msgid := strings.TrimPrefix(m.Channel, SubCommonMsgKeyPrefix)
		if len(msgid) == 0 {
			continue
		}
		mid, err := strconv.ParseUint(msgid, 0, 16)
		if err != nil {
			glog.Errorf("[common msg] Cannot parse wrong MsgId %s, error: %v", msgid, err)
			continue
		}

		fields := strings.SplitN(string(m.Data), "|", 2)
		if len(fields) != 2 || len(fields) == 0 {
			glog.Errorf("[common msg] invalid pub-sub msg format: %s", string(m.Data))
			continue
		}

		ids := strings.Split(fields[0], ",")
		dstIds := make([]int64, 0, len(ids))
		for _, i := range ids {
			id, err := strconv.ParseInt(i, 10, 64)
			if err != nil {
				glog.Errorf("[common msg] invalid dest id %s, error: %v", i, err)
				continue
			}
			dstIds = append(dstIds, id)
		}
		msgBody := []byte(fields[1])
		//if glog.V(2) {
		//	glog.Infof("[common msg] pushing [%d] message to %v, message: len(%d)%v", mid, dstIds, len(msgBody), msgBody)
		//}
		go PushMsg(uint16(mid), dstIds, msgBody)
	}
}

func PushMsg(msgId uint16, dstIds []int64, msgBody []byte) {
	for _, id := range dstIds {
		// 整数为手机，负数为板子
		if gCometType == CometWs {
			gSessionList.PushCommonMsg(msgId, id, msgBody)
		} else if gCometType == CometUdp {
			gUdpSessions.PushCommonMsg(msgId, id, msgBody)
		}
	}
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

type Locker interface {
	Lock() error
	Unlock()
}

func NewDeviceSessionLocker(sid string) Locker {
	locker, err := redsync.NewMutex(fmt.Sprintf(RedisSessionDeviceLocker, sid), RedixAddrPool)
	if err != nil {
		panic(err)
	}
	return locker
}

func GetDeviceSession(sid string) (string, error) {
	r := Redix[_GetDeviceSession]
	RedixMu[_GetDeviceSession].Lock()
	defer RedixMu[_GetDeviceSession].Unlock()

	return redis.String(r.Do("get", fmt.Sprintf(RedisSessionDevice, sid)))
}

func SetDeviceSession(sid string, expire int, data string, deviceId int64, addr *net.UDPAddr) error {
	r := Redix[_SetDeviceSession]
	RedixMu[_SetDeviceSession].Lock()
	defer RedixMu[_SetDeviceSession].Unlock()

	err := r.Send("set", fmt.Sprintf(RedisSessionDevice, sid), data)
	if err != nil {
		return err
	}
	err = r.Send("expire", fmt.Sprintf(RedisSessionDevice, sid), expire)
	if err != nil {
		return err
	}
	// 对于刚刚建立，还未调用过注册或登录接口的会话，deviceId是0，不要为这个状态的
	// session设置deviceId和UDP地址的表映射，因为这个状态的session还不满足P2P的业务功能
	if deviceId != 0 {
		err = r.Send("set", fmt.Sprintf(RedisSessionDeviceAddr, deviceId), sid)
		if err != nil {
			return err
		}
		err = r.Send("expire", fmt.Sprintf(RedisSessionDeviceAddr, deviceId), expire)
		if err != nil {
			return err
		}
	}
	err = r.Flush()
	if err != nil {
		return err
	}
	_, err = r.Receive()
	if err != nil {
		return err
	}
	_, err = r.Receive()
	if err != nil {
		return err
	}
	if deviceId != 0 {
		_, err = r.Receive()
		if err != nil {
			return err
		}
		_, err = r.Receive()
	}
	return err
}

func GetDeviceSid(deviceId int64) (string, error) {
	r := Redix[_SetDeviceSession]
	RedixMu[_SetDeviceSession].Lock()
	defer RedixMu[_SetDeviceSession].Unlock()

	return redis.String(r.Do("get", fmt.Sprintf(RedisSessionDeviceAddr, deviceId)))
}

func DeleteDeviceSession(sid string) error {
	r := Redix[_DeleteDeviceSession]
	RedixMu[_DeleteDeviceSession].Lock()
	defer RedixMu[_DeleteDeviceSession].Unlock()

	_, err := r.Do("del", fmt.Sprintf(RedisSessionDevice, sid))
	return err
}

//func ExpireDeviceSession(sid string, expire int) error {
//	r := Redix[_ExpireDeviceSession]
//	RedixMu[_ExpireDeviceSession].Lock()
//	defer RedixMu[_ExpireDeviceSession].Unlock()
//
//	_, err := r.Do("expire", fmt.Sprintf(RedisSessionDevice, sid), expire)
//	return err
//}
