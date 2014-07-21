package main

import (
	"code.google.com/p/go.net/websocket"
	"crypto/md5"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/golang/glog"
	"html/template"
	"io"
	//"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var homeTempl = template.Must(template.ParseFiles("home.html"))

func homeHandle(w http.ResponseWriter, r *http.Request) {
	homeTempl.Execute(w, r.Host)
}

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
)

var (
	LOGIN_PARAM_ERROR = errors.New("Login params parse error!")
	ParamsError       = &ErrorCode{2001, "登陆参数错误"}
	LoginFailed       = &ErrorCode{2002, "登陆失败"}

	AckLoginOK             = []byte{byte(0)} // 登陆成功
	AckWrongParams         = []byte{byte(1)} // 错误的登陆参数
	AckWrongLoginType      = []byte{byte(2)} // 登陆类型解析错误
	AckWrongLoginDevice    = []byte{byte(3)} // 登陆设备ID解析错误
	AckWrongLoginTimestamp = []byte{byte(4)} // 登陆时间戳解析错误
	AckLoginTimeout        = []byte{byte(5)} // 登陆超时
	AckWrongMD5            = []byte{byte(6)} // 错误的md5
	AckOtherglogoned       = []byte{byte(7)} // 您已在别处登陆
	AckWrongLoginTimeout   = []byte{byte(8)} // 超时解析错误
)

type ErrorCode struct {
	ErrorId   int
	ErrorDesc string
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
	httpServeMux.HandleFunc("/", homeHandle)

	wsHandler := websocket.Server{
		Handshake: nil,
		Handler: WsHandler,
		MustMask: false,
	}
	httpServeMux.Handle("/ws", wsHandler)
	server := &http.Server{
		Addr:        bindAddr,
		Handler:     httpServeMux,
		ReadTimeout: READ_TIMEOUT * time.Second,
	}
	err := server.ListenAndServe()
	if err != nil {
		glog.Errorf("server.Serve(\"%s\") error(%v)", bindAddr, err)
		panic(err)
	}
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
	if timestamp > 0 && uint64(time.Now().Unix()) - timestamp >= EXPIRE_TIME {
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
	reply := make([]byte, 0, 1024)
	if err = websocket.Message.Receive(ws, &reply); err != nil {
		glog.Errorf("[%s] websocket.Message.Receive() error(%s)\n", addr, err)
		ws.Close()
		return
	}

	glog.Infof("Recv login %s\n", string(reply))
	// parse login params
	id, timestamp, timeout, encryShadow, loginErr := getLoginParams(string(reply))
	if loginErr != nil {
		glog.Errorf("[%s] params (%s) error (%v)\n", addr, string(reply), loginErr)
		websocket.Message.Send(ws, AckWrongParams)
		ws.Close()
		return
	}
	// check login
	if err = isAuth(id, timestamp, timeout, encryShadow); err != nil {
		glog.Errorf("[%s] auth failed:\"%s\", error: %v", addr, string(reply), err)
		websocket.Message.Send(ws, LoginFailed.ErrorId)
		ws.Close()
		return
	}
	var bindedIds []int64
	if id > 0 {
		bindedIds, err = GetUserDevices(id)
	} else if id < 0 {
		bindedIds, err = GetDeviceUsers(id)
	}
	if err != nil {
		glog.Errorf("[getIds] id [%d] get ids error: %v, ids: %v", id, err, bindedIds)
	} else {
		if glog.V(2) {
			glog.Infof("[getIds] get ids for id [%d]: count(%d)%v", id, len(bindedIds), bindedIds)
		}
	}

	statIncConnTotal()
	statIncConnOnline()
	defer statDecConnOnline()

	// 旧程序需要成功登陆后的一次回复
	err = websocket.Message.Send(ws, []byte{0})
	if err != nil {
		glog.Errorf("[%s] [uid: %d] sent login-ack error (%v)\n", addr, id, err)
		ws.Close()
		return
	}

	_, err = SetUserOnline(id, gLocalAddr)
	if err != nil {
		glog.Errorf("redis online error [uid: %d] %v\n", id, err)
		ws.Close()
		return
	}
	if glog.V(2) {
		glog.Infof("[online] id %d on %s", id, gLocalAddr)
	}

	s := NewSession(id, bindedIds, ws)
	selement := gSessionList.AddSession(s)

	if timeout <= 0 {
		timeout = TIME_OUT
	}
	ws.ReadTimeout = (time.Duration(timeout) + 30) * time.Second
	//start := time.Now().UnixNano()
	//end := int64(start + int64(time.Second))
	for {
		// more than 1 sec, reset the timer
		//if end-start >= int64(time.Second) {
		//	if err = setReadTimeout(ws, timeout + 30); err != nil {
		//		glog.Errorf("<%s> user_id:\"%d\" websocket.SetReadDeadline() error(%s)\n", addr, id, err)
		//		break
		//	}
		//	start = end
		//}

		if err = websocket.Message.Receive(ws, &reply); err != nil {
			if err == io.EOF {
				if glog.V(1) {
					glog.Errorf("[connection] user [%d] quit on EOF", id)
				}
			} else {
				glog.Errorf("[connection] <%s> user_id:\"%d\" websocket.Message.Receive() error(%s)\n", addr, id, err)
			}
			break
		}
		s.UpdateBindedIds()
		if len(reply) == 1 && string(reply) == PING_MSG {
			if err = websocket.Message.Send(ws, PONG_MSG); err != nil {
				glog.Errorf("<%s> user_id:\"%d\" write heartbeat to client error(%s)\n", addr, id, err)
				break
			}
			//glog.Debugf("<%s> user_id:\"%s\" receive heartbeat\n", addr, id)
		} else {
			statIncUpStreamIn()
			//glog.Debugf("<%s> user_id:\"%s\" recv msg %s\n", addr, id, reply)
			// Send to Message Bus
			msg := reply

			if len(msg) < 12 {
				glog.Infof("Invalid msg lenght %d bytes, %v", len(msg), msg)
				break
			}
			// 提取消息中的目标id
			toId := int64(binary.LittleEndian.Uint64(msg[4:12]))

			if glog.V(2) {
				glog.Infof("[msg in] %d <- %d, binded(%v), data: (len: %d)%v...", toId, id, s.BindedIds, len(msg), msg[0:3])
			} else if glog.V(3) {
				glog.Infof("[msg in] %d <- %d, binded(%v), data: (len: %d)%v...", toId, id, s.BindedIds, len(msg), msg)
			}
			if toId != 0 {
				if !s.IsBinded(int64(toId)) {
					glog.Errorf("[msg] src id [%d] not binded to dst id [%d], valid ids: %v", id, toId, s.BindedIds)
					// TODO: 初期测试时不需要校验，但实际插座逻辑需要校验
					//continue
				}
				GMsgBusManager.Push2Backend([]int64{int64(toId)}, msg)
			} else {
				GMsgBusManager.Push2Backend(s.BindedIds, msg)
			}
			// glog.Infof("%v Recv %v [%#T] [%#T] [%v] [%v] [%v]", s.Uid, reply, reply, PING_MSG,
			//  reply == PING_MSG, []byte(reply), []byte(PING_MSG))
		}
		//end = time.Now().UnixNano()
	}
	err = SetUserOffline(id, gLocalAddr)
	if err != nil {
		glog.Errorf("[offline error] uid %d, error: %v", id, err)
	}
	if glog.V(2) {
		glog.Infof("[offline] id %d on %s", id, gLocalAddr)
	}
	gSessionList.RemoveSession(selement)
	return
}
