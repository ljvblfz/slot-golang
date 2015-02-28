package main

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud-base/websocket"
	"cloud-socket/msgs"
	"github.com/golang/glog"
)

const (
	killedByOtherDevice  = "Another device login %d"
	wrongLoginParams     = "Wrong login params %s"
	wrongLoginType       = "Wrong login type %s"
	wrongLoginDevice     = "Wrong login deviceId %s"
	wrongLoginTimestamp  = "Wrong login timestamp %s"
	userLoginTimeout     = "User %d login timeout"
	userReconnectTimeout = "Reconnect timeout %d"
	wrongMd5Check        = "User %d has wrong md5"
	wrongLoginTimeout    = "Wrong login %d timeout %s"

	LOGIN_PARAM_COUNT = 4
	READ_TIMEOUT      = 10
	PING_MSG          = "p"
	PONG_MSG          = "P"
	TIME_OUT          = 150
	EXPIRE_TIME       = uint64(1 * 60) // 1 mins

	kLoginKey = "BlackCrystalWb14527" // 和http服务器约定好的私有盐

	// 用户id的间隔，这个间隔内可用的取值数量，就是该用户可同时登录的手机数量
	kUseridUnit uint = 16

	kDstIdOffset = 0
	kDstIdLen    = 8
	kDstIdEnd    = kDstIdOffset + kDstIdLen
)

var (
	LOGIN_PARAM_ERROR = errors.New("Login params parse error!")
	ParamsError       = &ErrorCode{2001, "登陆参数错误"}
	LoginFailed       = &ErrorCode{2002, "登陆失败"}

	AckLoginOK             = []byte{byte(0)}  // 登陆成功
	AckWrongParams         = []byte{byte(1)}  // 错误的登陆参数
	AckWrongLoginType      = []byte{byte(2)}  // 登陆类型解析错误
	AckWrongLoginDevice    = []byte{byte(3)}  // 登陆设备ID解析错误
	AckWrongLoginTimestamp = []byte{byte(4)}  // 登陆时间戳解析错误
	AckLoginTimeout        = []byte{byte(5)}  // 登陆超时
	AckWrongMD5            = []byte{byte(6)}  // 错误的md5
	AckOtherglogoned       = []byte{byte(7)}  // 您已在别处登陆
	AckWrongLoginTimeout   = []byte{byte(8)}  // 超时解析错误
	AckServerError         = []byte{byte(9)}  // 服务器错误
	AckModifiedPasswd      = []byte{byte(10)} // 密码已修改

	websocketUrl []string
	urlLock      sync.Mutex
)

type ErrorCode struct {
	ErrorId   int
	ErrorDesc string
}

// 获取本comet可接受websocket连接的所有地址
func GetCometUrl() []string {
	urlLock.Lock()
	urls := make([]string, len(websocketUrl))
	copy(urls, websocketUrl)
	urlLock.Unlock()
	return urls
}

// StartHttp start http listen.
func StartHttp(bindAddrs []string) {
	for _, bind := range bindAddrs {
		glog.Infof("start websocket listen addr:[%s]\n", bind)
		go websocketListen(bind)
	}
}

func websocketListen(bindAddr string) {
	httpServeMux := http.NewServeMux()

	wsHandler := websocket.Server{
		Handshake: func(config *websocket.Config, r *http.Request) error {
			if len(config.Protocol) > 0 {
				config.Protocol = config.Protocol[:1]
			}
			return nil
		},
		Handler:  WsHandler,
		MustMask: false,
	}
	path := "ws"
	httpServeMux.Handle("/"+path, wsHandler)

	addr, err := net.ResolveTCPAddr("tcp", bindAddr)
	if err != nil {
		glog.Errorf("[listen] net.ResolveTCPAddr %s failed: %v", bindAddr, err)
		return
	}
	bindAddr = fmt.Sprintf("%s:%d", gLocalAddr, addr.Port)

	server := &http.Server{
		Addr:        bindAddr,
		Handler:     httpServeMux,
		ReadTimeout: READ_TIMEOUT * time.Second,
	}

	urlServer := fmt.Sprintf("ws://%s/%s", bindAddr, path)

	urlLock.Lock()
	websocketUrl = append(websocketUrl, urlServer)
	urlLock.Unlock()

	err = server.ListenAndServe()
	if err != nil {
		glog.Errorf("server.Serve(\"%s\") error(%v)", bindAddr, err)
		panic(err)
	}
	urlLock.Lock()
	for i, v := range websocketUrl {
		if v != urlServer {
			continue
		}
		newUrl := make([]string, len(websocketUrl)-1)
		copy(newUrl[:i], websocketUrl[:i])
		copy(newUrl[i:], websocketUrl[i+1:])
		websocketUrl = newUrl
		break
	}
	urlLock.Unlock()
}

func getLoginParams(req string) (id int64, timestamp, timeout uint64, md5Str string, err error) {
	args := strings.Split(req, "|")
	if len(args) != LOGIN_PARAM_COUNT {
		err = LOGIN_PARAM_ERROR
		return
	}
	// skip the 0
	id, err = strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return
	}
	if len(args[1]) == 0 {
		err = errors.New("login needs timestamp")
		return
	}
	timestamp, err = strconv.ParseUint(args[1], 10, 64)
	if err != nil {
		return
	}
	if len(args[2]) == 0 {
		err = errors.New("login needs timeout")
		return
	}
	timeout, err = strconv.ParseUint(args[2], 10, 64)
	if err != nil {
		return
	}

	md5Str = args[3]
	return
}

func isAuth(id int64, timestamp uint64, timeout uint64, md5Str string) error {
	if timestamp > 0 && uint64(time.Now().Unix())-timestamp >= EXPIRE_TIME {
		return fmt.Errorf("login timeout, %d - %d >= %d", uint64(time.Now().Unix()), timestamp, EXPIRE_TIME)
		// return false
	}
	// check hmac is equal
	md5Buf := md5.Sum([]byte(fmt.Sprintf("%d|%d|%d|%s", id, timestamp, timeout, kLoginKey)))
	md5New := base64.StdEncoding.EncodeToString(md5Buf[0:])
	if md5New != md5Str {
		if glog.V(1) {
			glog.Warningf("auth not equal: %s != %s", md5New, md5Str)
		}
		return errors.New("login parameter is not verified")
	}
	return nil
}

func setReadTimeout(conn *websocket.Conn, delaySec int) error {
	return conn.SetReadDeadline(time.Now().Add(time.Second * time.Duration(delaySec)))
}

func WsHandler(ws *websocket.Conn) {
	addr := ws.Request().RemoteAddr
	var err error
	if err = setReadTimeout(ws, 60); err != nil {
		glog.Errorf("[%s] websocket.SetReadDeadline() error(%s)\n", addr, err)
		ws.Close()
		return
	}
	reply := make([]byte, 0, 256)
	if err = websocket.Message.Receive(ws, &reply); err != nil {
		glog.Errorf("[%s] websocket.Message.Receive() error(%s)\n", addr, err)
		ws.Close()
		return
	}

	// parse login params
	id, timestamp, timeout, encryShadow, loginErr := getLoginParams(string(reply))
	if loginErr != nil {
		glog.Errorf("[online|check] [%s] params (%s) error (%v)\n", addr, string(reply), loginErr)
		websocket.Message.Send(ws, AckWrongParams)
		ws.Close()
		return
	}
	// check login
	if err = isAuth(id, timestamp, timeout, encryShadow); err != nil {
		glog.Errorf("[online|check] [%s] auth failed:\"%s\", error: %v", addr, string(reply), err)
		websocket.Message.Send(ws, LoginFailed.ErrorId)
		ws.Close()
		return
	}
	var bindedIds []int64
	var mid byte
	if id > 0 {
		// 用户登录，检查其id是否为16整数倍，并为其分配一个1到15内的未使用的手机子id，相加后作为手机
		// id，用于本session
		if id%int64(kUseridUnit) != 0 {
			glog.Warningf("[online|mobileid] invalid user id %d, low byte is not zero", id)
			err = websocket.Message.Send(ws, AckWrongLoginDevice)
			ws.Close()
			return
		}
		mobileid, err := SelectMobileId(id)
		if err != nil {
			glog.Warningf("[online|mobileid] select mobile id for user %d failed: %v", id, err)
			err = websocket.Message.Send(ws, AckServerError)
			ws.Close()
			return
		}
		if mobileid <= 0 {
			glog.Warningf("[online|mobileid] no valid mobile id for user %d, the user may have 15 clients now.", id)
			err = websocket.Message.Send(ws, AckWrongLoginDevice)
			ws.Close()
			return
		}
		newId := id + int64(mobileid%int(kUseridUnit))
		if id > newId {
			glog.Errorf("[online|mobileid] user id overflow, origin id: %d, newId %d with mid %d", id, newId, mobileid)
		}
		mid = byte(mobileid)
		// 先用原始的用户id获取设备列表
		bindedIds, err = GetUserDevices(id)
		id = newId // 防止错误的手机id溢出可用的范围
	} else if id < 0 {
		bindedIds, err = GetDeviceUsers(id)
	}
	if err != nil {
		glog.Errorf("[online|getIds] id [%d] get ids error: %v, ids: %v", id, err, bindedIds)
		websocket.Message.Send(ws, LoginFailed.ErrorId)
		ws.Close()
		return
	}

	statIncConnTotal()
	statIncConnOnline()
	defer statDecConnOnline()

	// 成功登陆后的一次回复
	if id > 0 {
		err = websocket.Message.Send(ws, []byte{0, mid})
	} else {
		err = websocket.Message.Send(ws, []byte{0})
	}
	if err != nil {
		glog.Errorf("[online|error] [%s] [uid: %d] sent login-ack error (%v)\n", addr, id, err)
		ws.Close()
		return
	}

	_, err = SetUserOnline(id, gLocalAddr)
	if err != nil {
		glog.Errorf("[online|check] redis online error [uid: %d] %v\n", id, err)
		ws.Close()
		return
	}
	if glog.V(2) {
		glog.Infof("[online] id: %d, ip: %v, comet: %s, param: %s, binded ids: %v", id, addr, gLocalAddr, reply, bindedIds)
	}

	s := NewSession(id, bindedIds, ws)
	selement := gSessionList.AddSession(s)

	if id < 0 {
		destIds := gSessionList.CalcDestIds(s, 0)
		onlineMsg := msgs.NewAppMsg(0, id, msgs.MIDOnline)
		GMsgBusManager.Push2Backend(id, destIds, onlineMsg.MarshalBytes())
	}

	if timeout <= 0 {
		timeout = TIME_OUT
	}
	ws.ReadTimeout = time.Duration(3*timeout) * time.Second
	for {
		if err = websocket.Message.Receive(ws, &reply); err != nil {
			break
		}
		gSessionList.GetBindedIds(s, &bindedIds)
		if len(reply) == 1 && string(reply) == PING_MSG {
			if err = websocket.Message.Send(ws, PONG_MSG); err != nil {
				glog.Errorf("<%s> user_id:\"%d\" write heartbeat to client error(%s)\n", addr, id, err)
				break
			}
		} else {
			statIncUpStreamIn()
			// Send to Message Bus
			msg := reply

			if len(msg) < kDstIdEnd {
				glog.Infof("Invalid msg lenght %d bytes, %v", len(msg), msg)
				break
			}
			// 根据手机与嵌入式协议，提取消息中的目标id
			toId := int64(binary.LittleEndian.Uint64(msg[kDstIdOffset:kDstIdEnd]))

			destIds := gSessionList.CalcDestIds(s, toId)

			if glog.V(3) {
				glog.Infof("[msg|in] %d <- %d, binded(%v), calc to: %v, data: (len: %d)%v...", toId, id, s.BindedIds, destIds, len(msg), msg)
			} else if glog.V(2) {
				glog.Infof("[msg|in] %d <- %d, binded(%v), calc to: %v, data: (len: %d)%v...", toId, id, s.BindedIds, destIds, len(msg), msg[0:kDstIdEnd])
			}
			GMsgBusManager.Push2Backend(id, destIds, msg)
		}
		//end = time.Now().UnixNano()
	}
	offlineErr := err
	err = SetUserOffline(id, gLocalAddr)
	if err != nil {
		glog.Errorf("[offline|error] uid %d, error: %v", id, err)
	}
	if glog.V(2) {
		glog.Infof("[offline] id:%d, comet: %s, reason: %v", id, gLocalAddr, offlineErr)
	}
	if id > 0 && mid > 0 {
		id -= int64(mid)
		err = ReturnMobileId(id, mid)
		if err != nil {
			glog.Errorf("[mid|return] return mid %d for user %d failed, error: %v", mid, id, err)
		}
	}
	if id < 0 {
		destIds := gSessionList.CalcDestIds(s, 0)
		offlineMsg := msgs.NewAppMsg(0, id, msgs.MIDOffline)
		GMsgBusManager.Push2Backend(id, destIds, offlineMsg.MarshalBytes())
	}
	gSessionList.RemoveSession(selement)
	return
}
