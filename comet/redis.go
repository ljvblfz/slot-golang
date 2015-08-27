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
	if glog.V(3) {
		glog.Infoln("Subscribed DeviceUsers")
	}
	err = SubDeviceUsers()
	if err != nil {
		panic(err)
	}
	if gCometType != msgs.CometUdp || gCometPushUdp {
		if glog.V(3) {
			glog.Infoln("Subscribed SubCommonMsg")
		}
		err = SubCommonMsg()
		if err != nil {
			panic(err)
		}
	}
	if gCometType == msgs.CometWs {
		if glog.V(3) {
			glog.Infoln("Subscribed SubModifiedPasswd SubOffline")
		}
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

/**
*	执行lua，更新MSGBUS的用户与WSCOMET映射表，uid可以是硬件或手机用户
 */
func SetUserOnline(uid int64, host string) (bool, error) {
	r := Redix[_SetUserOnline]
	RedixMu[_SetUserOnline].Lock()
	defer RedixMu[_SetUserOnline].Unlock()
	return redis.Bool(ScriptOnline.Do(r,
		fmt.Sprintf(HostUsers, host),
		uid,
		OnOff,
		host,
	))
}

/**
*	执行lua，更新MSGBUS的用户与WSCOMET映射表，uid可以是硬件或手机用户
 */
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
	if glog.V(3) {
		glog.Infof("Publish forcing usr[%v] offline msg", uid)
	}
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
		if glog.V(3) {
			glog.Infof("Handling usr[%v] Offline MSG", uid)
		}
		if err != nil {
			glog.Errorf("[HandleUSROffline] invalid uid[%s], %v", string(buf), err)
			continue
		}
		if v, ok := gSessionList.onlined[uid]; ok {
			SetUserOffline(uid, fmt.Sprintf("%v-%v", gLocalAddr, gCometType))
			v.Conn.Send(AckForceUserOffline) //
			gSessionList.RemoveSession(v)
			if glog.V(3) {
				glog.Infof("Handling usr[%v] offline event has DONE.", uid)
			}
		} else {
			if glog.V(3) {
				glog.Infof("Handling usr[%v] offline event has FAILED.", uid)
			}
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
	if glog.V(3) {
		glog.Infof("[PUB DEV ONLINE MSG] dev[%v] send to usr[%v] for dev online msg", sess.DeviceId, sess.Users)
	}
	r := Redix[_GetDeviceUsers]
	RedixMu[_GetDeviceUsers].Lock()
	defer RedixMu[_GetDeviceUsers].Unlock()
	r.Do("hset", "device:adr", fmt.Sprintf("%d", sess.DeviceId), sess.Addr.String())
	if len(sess.Users) == 0 {
		if glog.V(3) {
			glog.Infof("[PUB DEV ONLINE MSG] dev {%v} can't send to usr:{%v} for dev online msg, dest is empty", sess.DeviceId, sess.Users)
		}
		return
	}
	var DevOnlineMsg []byte
	var strIds []string
	for _, v := range sess.Users {
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
	if glog.V(3) {
		glog.Infof("[PUB DEV ONLINE MSG] dev[%v] send to usr[%v] for dev online msg, DONE", sess.DeviceId, sess.Users)
	}
}
func PushDevOfflineMsgToUsers(sess *UdpSession) {
	if glog.V(3) {
		glog.Infof("[PUB DEV OFFLINE MSG] dev[%v] send to usr[%v] for dev offline msg", sess.DeviceId, sess.Users)
	}
	r := Redix[_GetDeviceUsers]
	RedixMu[_GetDeviceUsers].Lock()
	defer RedixMu[_GetDeviceUsers].Unlock()
	r.Do("hdel", "device:adr", fmt.Sprintf("%d", sess.DeviceId), sess.Addr.Network())
	if len(sess.Users) == 0 {
		glog.Infof("[PUB DEV OFFLINE MSG] dev {%v} can't send to usr:{%v} for dev offline msg, dest is empty", sess.DeviceId, sess.Users)
		return
	}
	var DevOfflineMsg []byte
	var strIds []string
	for _, v := range sess.Users {
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
	if glog.V(3) {
		glog.Infof("[PUB DEV OFFLINE MSG] dev[%v] send to usr[%v] for dev offline msg, DONE", sess.DeviceId, sess.Users)
	}
}

/**
*返回参数1，与硬件相关的用户列表
*返回参数2，硬件的拥有者
 */
func GetUsersByDev(deviceId int64) ([]int64, int64, error) {
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
func GetDevByUsr(userId int64) ([]int64, error) {
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
		if glog.V(3) {
			glog.Infof("[UPDATING BINDING LIST] received [%v]", string(buf))
		}
		strs := strings.SplitN(string(buf), "|", 3)
		if len(strs) != 3 || len(strs) == 0 {
			glog.Errorf("[UPDATING BINDING LIST] invalid pub-sub msg format: %s", string(buf))
			continue
		}
		deviceId, err := strconv.ParseInt(strs[0], 10, 64)
		if err != nil {
			glog.Errorf("[UPDATING BINDING LIST] invalid deviceId %s, error: %v", strs[0], err)
			continue
		}
		userId, err := strconv.ParseInt(strs[1], 10, 64)
		if err != nil {
			glog.Errorf("[UPDATING BINDING LIST] invalid userId %s, error: %v", strs[1], err)
			continue
		}
		msgType, err := strconv.ParseInt(strs[2], 10, 32)
		if err != nil {
			glog.Errorf("[UPDATING BINDING LIST] invalid bind type %s, error: %v", strs[2], err)
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
					glog.Fatalf("[SUB MODIFY PWD EVENT] |%s| |%s| |%d| |%v|", n.Kind, n.Channel, n.Count, n)
					return
				}
			case error:
				glog.Errorf("[SUB MODIFY PWD EVENT] |%v|", n)
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
		if glog.V(3) {
			glog.Infof("[HANDLING MODIFY PWD EVENT] received [%v]", userId)
		}
		if err != nil {
			glog.Errorf("[HANDLING MODIFY PWD EVENT] invalid userId %s, error: %v", string(buf), err)
			continue
		}
		go gSessionList.OffliningUsr(userId)
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
			switch n := data.(type) {
			case redis.PMessage:
				ch <- n
			case redis.Subscription:
				if n.Count == 0 {
					glog.Fatalf("[SUB COMMON MSG EVENT] |%s| |%s| |%d| |%v|", n.Kind, n.Channel, n.Count)
					return
				}
			case error:
				glog.Errorf("[SUB COMMON MSG EVENT] |%v|", n)
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
			switch n := data.(type) {
			case redis.PMessage:
				ch <- n
			case redis.Subscription:
				if n.Count == 0 {
					glog.Fatalf("[SUB DevOnline EVENT] |%s| |%s| |%d| |%v|", n.Kind, n.Channel, n.Count)
					return
				}
			case error:
				glog.Errorf("[SUB DevOnline EVENT] |%v|", n)
				return
			}
		}
	}()
	go func(ch <-chan redis.PMessage) {
		for m := range ch {
			if glog.V(3) {
				glog.Infof("[Dealing DevOnline] Received [%v][%v]", string(m.Data), m.Data)
			}
			devId, _ := strconv.ParseInt(string(m.Data), 10, 64)
			if glog.V(3) {
				glog.Infof("[Dealing DevOnline] Received [%v]", devId)
			}
			if strSid, ok := gUdpSessions.devmap[devId]; ok {
				gUdpSessions.udpmap[gUdpSessions.sidmap[strSid].Addr.String()].Stop()
				if glog.V(3) {
					glog.Infof("[Dealing DevOnline] Terminating the %v's offline event", devId)
				}
			} else {
				if glog.V(3) {
					glog.Infof("[Dealing DevOnline] Fail to terminate the %v's offline event fail. Because there is not this sid of %v on the UDPCOMET", devId, devId)
				}
			}
		}
	}(ch)
	return nil
}

func HandleCommonMsg(ch <-chan redis.PMessage) {
	for m := range ch {
		glog.Infof("[HANDLING COMMON MSG]received\n[%v]\n[%v]", string(m.Data), m.Data)
		msgid := strings.TrimPrefix(m.Channel, SubCommonMsgKeyPrefix)
		if len(msgid) == 0 {
			continue
		}
		mid, err := strconv.ParseUint(msgid, 0, 16)
		if err != nil {
			glog.Errorf("[HANDLING COMMON MSG] Cannot parse wrong MsgId %s, error: %v", msgid, err)
			continue
		}

		fields := strings.SplitN(string(m.Data), "|", 2)
		if len(fields) != 2 || len(fields) == 0 {
			glog.Errorf("[HANDLING COMMON MSG] invalid pub-sub msg format: %s", string(m.Data))
			continue
		}

		ids := strings.Split(fields[0], ",")
		dstIds := make([]int64, 0, len(ids))
		for _, i := range ids {
			id, err := strconv.ParseInt(i, 10, 64)
			if err != nil {
				glog.Errorf("[HANDLING COMMON MSG] invalid dest id [%s], error: %v", i, err)
				continue
			}
			dstIds = append(dstIds, id)
		}
		msgBody := []byte(fields[1])
		if glog.V(3) {
			glog.Infof("[HANDLING COMMON MSG] pushing [%d] message to %v, message: len(%d)%v", mid, dstIds, len(msgBody), msgBody)
		}
		go Send2Dest(uint16(mid), dstIds, msgBody)
	}
}

/**
* 发送消息到目标，目标可能是手机或硬件
 */
func Send2Dest(msgId uint16, dstIds []int64, msgBody []byte) {
	for _, id := range dstIds {
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
func SaveDvName(dvId int64, name string) {
	r := Redix[_Short]
	RedixMu[_Short].Lock()
	defer RedixMu[_Short].Unlock()
	r.Do("hset", []byte("dv:name"), fmt.Sprintf("%v", dvId), name)
}
func GetDvName(dvId int64) string {
	r := Redix[_Short]
	RedixMu[_Short].Lock()
	defer RedixMu[_Short].Unlock()
	DeviceName, _ := redis.String(r.Do("hget", []byte("dv:name"), fmt.Sprintf("%v", dvId)))
	return DeviceName
}
