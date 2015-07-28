package main

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	//"time"
	"bytes"
	"cloud-socket/msgs"
	"encoding/binary"
	"github.com/garyburd/redigo/redis"
	"github.com/golang/glog"
	//	"github.com/hjr265/redsync.go/redsync"
)

const (
	HostUsers             = "Host:%s" // (1, 2, 3)
	OnOff                 = "OnOff"
	SubDeviceUsersKey     = "PubDeviceUsers"
	SubModifiedPasswdKey  = "PubModifiedPasswdUser"
	SubCommonMsgKeyPrefix = "PubCommonMsg:"
	SubCommonMsgKey       = SubCommonMsgKeyPrefix + "*"
	DevOnlineChannel      = "DevOnlineChannel"
	RedisDeviceUsers      = "device:owner"
	RedisUserDevices      = "u:%v:devices"

	// 用户正在使用的手机id
	RedisUserMobiles = "user:mobileid:%v"

	RedisSessionDevice       = "sess:dev:%v"
	RedisSessionDeviceAddr   = "sess:dev:sid:%v"
	RedisSessionDeviceLocker = "sess:dev:%v:locker"
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
	_SubOffline
	_DevOnlineCh
	_Short
	//_ExpireDeviceSession
	_Max

	// eval, script, 2, htable, id, OnOff, cometIP
	_scriptOnline = `
local count = redis.call('hincrby', KEYS[1], KEYS[2], 1)
if count == 1 then
	redis.call('publish', ARGV[1], KEYS[2].."|"..ARGV[2].."|1")
end
return count
`

	_scriptOffline = `
redis.call('publish', ARGV[1], KEYS[2].."|"..ARGV[2].."|0")
redis.call('hdel', KEYS[1], KEYS[2])
`
	_scriptOnlineb = `
local count = redis.call('hincrby', KEYS[1], KEYS[2], 1)
if count == 1 then
	redis.call('publish', ARGV[1], KEYS[2].."|"..ARGV[2].."|1")
end
return count
`

	_scriptOfflineb = `
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

	if gCometType != msgs.CometUdp || gCometUdpSubBindingEvent {
		glog.Infoln("SubDeviceUsers")
		err = SubDeviceUsers()
		if err != nil {
			panic(err)
		}
	}
	if gCometType != msgs.CometUdp || gCometPushUdp {
		glog.Infoln("SubCommonMsg")
		err = SubCommonMsg()
		if err != nil {
			panic(err)
		}
	}
	if gCometType == msgs.CometWs {
		glog.Infoln("SubModifiedPasswd SubOffline")
		err = SubModifiedPasswd()
		if err != nil {
			panic(err)
		}
		SubOffline()
	}
	if gCometType == msgs.CometUdp {
		SubDevOnlineChannel()
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
	err = r.Send("publish", OnOff, fmt.Sprintf("0|%s|0", ip))
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
		OnOff,
		host,
	))
}

func SetUserOffline(uid int64, host string) error {
	r := Redix[_SetUserOffline]
	RedixMu[_SetUserOffline].Lock()
	defer RedixMu[_SetUserOffline].Unlock()
	// new
	_, err := redis.Bool(ScriptOffline.Do(r,
		fmt.Sprintf(HostUsers, host),
		uid,
		OnOff,
		host,
	))
	return err
}
func ForceUserOffline(uid int64) {
	glog.Infof("force %v offline begin", uid)
	r := Redix[_SetUserOffline]
	RedixMu[_SetUserOffline].Lock()
	defer RedixMu[_SetUserOffline].Unlock()
	r.Do("publish", []byte("UserOffline"), fmt.Sprint(uid))
}

func SubOffline() error {
	r := Redix[_SubOffline]
	RedixMu[_SubOffline].Lock()
	defer RedixMu[_SubOffline].Unlock()

	psc := redis.PubSubConn{Conn: r}
	err := psc.Subscribe("UserOffline")
	if err != nil {
		return err
	}
	ch := make(chan []byte, 8)
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
				glog.Errorf("[bind|redis] sub of error: %v\n", data)
				return
			}
		}
	}()
	go HandleOffline(ch)
	return nil
}

func HandleOffline(ch <-chan []byte) {
	for buf := range ch {
		if buf == nil {
			continue
		}
		uid, err := strconv.ParseInt(string(buf), 10, 64)
		glog.Infoln("HandleOffline:", uid)
		if err != nil {
			glog.Errorf("[HandleSubOffline] invalid uid %s, error: %v", string(buf), err)
			continue
		}
		if v, ok := gSessionList.onlined[uid]; ok {
			SetUserOffline(uid, fmt.Sprintf("%v-%v", gLocalAddr, gCometType))
			v.Conn.Send(AckForceUserOffline) //
			gSessionList.RemoveSession(v)
			glog.Infof("force user %v offline, finish.", uid)
		} else {
			glog.Infof("this comet has not user's ws session [%v]", uid)
		}
	}
}
func UpdateDevAdr(sess *UdpSession) {
	r := Redix[_GetDeviceUsers]
	RedixMu[_GetDeviceUsers].Lock()
	defer RedixMu[_GetDeviceUsers].Unlock()
	r.Do("hset", "device:adr", fmt.Sprintf("%d", sess.DeviceId), sess.Addr.String())
}
func PushDevOnlineMsgToUsers(sess *UdpSession) {
	r := Redix[_GetDeviceUsers]
	RedixMu[_GetDeviceUsers].Lock()
	defer RedixMu[_GetDeviceUsers].Unlock()
	r.Do("hset", "device:adr", fmt.Sprintf("%d", sess.DeviceId), sess.Addr.String())
	if len(sess.BindedUsers) == 0 {
		glog.Infof("[dev online msg] dev {%v} send to usr:{%v} for dev online msg, dest is empty", sess.DeviceId, sess.BindedUsers)
		return
	}
	var DevOnlineMsg []byte
	var strIds []string
	for _, v := range sess.BindedUsers {
		strIds = append(strIds, fmt.Sprintf("%d", v))
	}
	userIds := strings.Join(strIds, ",") + "|"
	DevOnlineMsg = append(DevOnlineMsg, []byte(userIds)...)
	DevOnlineMsg = append(DevOnlineMsg, byte(9 /**上线标识*/))
	b_buf := bytes.NewBuffer([]byte{})
	binary.Write(b_buf, binary.LittleEndian, sess.DeviceId)
	DevOnlineMsg = append(DevOnlineMsg, b_buf.Bytes()...)
	DevOnlineMsg = append(DevOnlineMsg, byte(0 /**内容长度*/))
	r.Do("publish", []byte("PubCommonMsg:0x36"), DevOnlineMsg)

}
func PushDevOfflineMsgToUsers(sess *UdpSession) {
	r := Redix[_GetDeviceUsers]
	RedixMu[_GetDeviceUsers].Lock()
	defer RedixMu[_GetDeviceUsers].Unlock()
	r.Do("hdel", "device:adr", fmt.Sprintf("%d", sess.DeviceId), sess.Addr.Network())
	if len(sess.BindedUsers) == 0 {
		glog.Infof("[dev offline msg] dev {%v} send to usr:{%v} for dev offline msg, dest is empty", sess.DeviceId, sess.BindedUsers)
		return
	}
	var DevOfflineMsg []byte
	var strIds []string
	for _, v := range sess.BindedUsers {
		strIds = append(strIds, fmt.Sprintf("%d", v))
	}
	userIds := strings.Join(strIds, ",") + "|"
	DevOfflineMsg = append(DevOfflineMsg, []byte(userIds)...)
	DevOfflineMsg = append(DevOfflineMsg, byte(10 /**下线标识*/))
	b_buf := bytes.NewBuffer([]byte{})
	binary.Write(b_buf, binary.LittleEndian, sess.DeviceId)
	DevOfflineMsg = append(DevOfflineMsg, b_buf.Bytes()...)
	DevOfflineMsg = append(DevOfflineMsg, byte(0 /**内容长度*/))
	r.Do("publish", []byte("PubCommonMsg:0x36"), DevOfflineMsg)

}
func GetDeviceUsers(deviceId int64) ([]int64, int64, error) {
	r := Redix[_GetDeviceUsers]
	RedixMu[_GetDeviceUsers].Lock()
	defer RedixMu[_GetDeviceUsers].Unlock()
	user, err := redis.String(r.Do("hget", RedisDeviceUsers, deviceId))
	if err != nil {
		return nil, 0xFF, err
	}
	u_id, _ := strconv.ParseInt(user, 10, 64)
	host, err2 := redis.String(r.Do("hget", "user:family", user))
	//如果找不到host，说明此用户是孤儿，那么只返回此设备的直接关联用户
	//如果找到host,就返回此设备直接关联用户所属家庭所有成员
	if host == "" || err2 != nil {
		bindedIds := make([]int64, 0, 1)

		bindedIds = append(bindedIds, int64(u_id))
		return bindedIds, u_id, nil
	} else {
		mems, _ := redis.Strings(r.Do("smembers", fmt.Sprintf("family:%v", host)))
		bindedIds := make([]int64, 0, len(mems))
		for _, m := range mems {
			u_id, err := strconv.ParseInt(m, 10, 64)
			if err == nil {
				bindedIds = append(bindedIds, int64(u_id))
			}
		}
		return bindedIds, int64(u_id), nil
	}
}
func GetUserDevices(userId int64) ([]int64, error) {
	r := Redix[_GetUserDevices]
	RedixMu[_GetUserDevices].Lock()
	defer RedixMu[_GetUserDevices].Unlock()
	var idStrs []string //字串类型的设备数组
	hostId, err := redis.String(r.Do("hget", "user:family", userId))
	//hostId为空说明此用户为孤儿
	if err != nil || hostId == "" {
		idStrs, _ = redis.Strings(r.Do("smembers", fmt.Sprintf(RedisUserDevices, userId)))
	} else {
		mems, _ := redis.Strings(r.Do("smembers", fmt.Sprintf("family:%v", hostId)))
		for _, m := range mems {
			devs, err := redis.Strings(r.Do("smembers", fmt.Sprintf(RedisUserDevices, m)))
			if err == nil && len(devs) > 0 {
				idStrs = append(idStrs, devs...)
			}
		}
	}
	bindedIds := make([]int64, 0, len(idStrs))
	for _, v := range idStrs {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			continue
		}
		bindedIds = append(bindedIds, id)
	}
	return bindedIds, nil
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
		glog.Infoln("HandleDeviceUsers ", string(buf))
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
		// new code for udp
		if gCometType == msgs.CometUdp {
			go gUdpSessions.UpdateIds(deviceId, userId, msgType != 0)
		}

		if gCometType == msgs.CometWs {

			go gSessionList.UpdateIds(deviceId, userId, msgType != 0)
		}
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
func PubDevOnlineChannel(devid int64) {
	r := Redix[_Short]
	RedixMu[_Short].Lock()
	defer RedixMu[_Short].Unlock()
	r.Do("publish", []byte(DevOnlineChannel), fmt.Sprint(devid))
}
func GotDevAddr(devId int64) string {
	r := Redix[_Short]
	RedixMu[_Short].Lock()
	defer RedixMu[_Short].Unlock()
	v, _ := redis.String(r.Do("hget", "device:adr", fmt.Sprint(devId)))
	return v
}
func SubDevOnlineChannel() error {
	r := Redix[_DevOnlineCh]
	RedixMu[_DevOnlineCh].Lock()
	defer RedixMu[_DevOnlineCh].Unlock()

	psc := redis.PubSubConn{Conn: r}
	err := psc.PSubscribe(DevOnlineChannel)
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
	go func(ch <-chan redis.PMessage) {
		for m := range ch {
			glog.Infof("[SubDevOnlineChannel received] [%v][%v]", string(m.Data), m.Data)
			strDevId := string(m.Data)
			devId, _ := strconv.ParseInt(strDevId, 10, 64)
			glog.Infof("stop dev %v", devId)
			if strSid, ok := gUdpSessions.devmap[devId]; ok {
				gUdpSessions.udpmap[gUdpSessions.sidmap[strSid].Addr.String()].Stop()
				glog.Infof("%v stop the offline event", strDevId)
			} else {
				glog.Infof("%v stop the offline event fail.", strDevId)
			}
		}
	}(ch)
	return nil
}

func HandleCommonMsg(ch <-chan redis.PMessage) {
	for m := range ch {
		glog.Infof("[comon ch received] [%v][%v]", string(m.Data), m.Data)
		msgid := strings.TrimPrefix(m.Channel, SubCommonMsgKeyPrefix)
		if len(msgid) == 0 {
			continue
		}
		mid, err := strconv.ParseUint(msgid, 0, 16)
		if err != nil {
			glog.Errorf("[channel err] Cannot parse wrong MsgId %s, error: %v", msgid, err)
			continue
		}

		fields := strings.SplitN(string(m.Data), "|", 2)
		if len(fields) != 2 || len(fields) == 0 {
			glog.Errorf("[channel err] invalid pub-sub msg format: %s", string(m.Data))
			continue
		}

		ids := strings.Split(fields[0], ",")
		dstIds := make([]int64, 0, len(ids))
		for _, i := range ids {
			id, err := strconv.ParseInt(i, 10, 64)
			if err != nil {
				glog.Errorf("[channel err] invalid dest id [%s], error: %v", i, err)
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
		if gCometType == msgs.CometWs {
			gSessionList.PushCommonMsg(msgId, id, msgBody)
		} else if gCometType == msgs.CometUdp {
			gUdpSessions.PushCommonMsg(msgId, id, msgBody)
		}
	}
}

func GetDeviceSession(sid string) (string, error) {
	r := Redix[_GetDeviceSession]
	RedixMu[_GetDeviceSession].Lock()
	defer RedixMu[_GetDeviceSession].Unlock()

	return redis.String(r.Do("get", fmt.Sprintf(RedisSessionDevice, sid)))
}

func SetDeviceSession(sid string, expire int, json string, deviceId int64, addr *net.UDPAddr) error {
	r := Redix[_SetDeviceSession]
	RedixMu[_SetDeviceSession].Lock()
	defer RedixMu[_SetDeviceSession].Unlock()

	_, err := r.Do("setex", fmt.Sprintf(RedisSessionDevice, sid), expire, json)
	if err != nil {
		return err
	}
	// 对于刚刚建立，还未调用过注册或登录接口的会话，deviceId是0，不要为这个状态的
	// session设置deviceId和UDP地址的表映射，因为这个状态的session还不满足P2P的业务功能
	if deviceId != 0 {
		_, err = r.Do("setex", fmt.Sprintf(RedisSessionDeviceAddr, deviceId), expire, sid)
		if err != nil {
			return err
		}
	}
	return err
}

func GetDeviceSid(deviceId int64) (string, error) {

	r := Redix[_SetDeviceSession]
	RedixMu[_SetDeviceSession].Lock()
	defer RedixMu[_SetDeviceSession].Unlock()
	v, e := redis.String(r.Do("get", fmt.Sprintf(RedisSessionDeviceAddr, deviceId)))
	return v, e
}

func DeleteDeviceSession(sid string) error {
	r := Redix[_DeleteDeviceSession]
	RedixMu[_DeleteDeviceSession].Lock()
	defer RedixMu[_DeleteDeviceSession].Unlock()

	_, err := r.Do("del", fmt.Sprintf(RedisSessionDevice, sid))
	return err
}
func SetLoginFlag(usrId int64, flag string) {
	r := Redix[_Short]
	RedixMu[_Short].Lock()
	defer RedixMu[_Short].Unlock()
	r.Do("hset", []byte("user:logined"), fmt.Sprintf("%v", usrId), flag)
}
