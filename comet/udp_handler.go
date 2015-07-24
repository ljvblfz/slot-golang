package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cloud-base/atomic"
	"cloud-socket/msgs"
	"github.com/golang/glog"
	uuid "github.com/nu7hatch/gouuid"
)

const (
	FrameHeaderLen = 24
	DataHeaderLen  = 12

	kHeartBeatSec = 120
	kHeartBeat    = kHeartBeatSec * time.Second

	kApiGetUAddr = "/api/device/udpaddr"
)

var (
	//ErrSessTimeout = fmt.Errorf("session timeout")
	ErrSessPackSeq = fmt.Errorf("wrong package sequence number")

	udpUrlPort atomic.AtomicInt32
)

func GetCometUdpUrl() string {
	//	return fmt.Sprintf("http://%s:%d%s", gLocalAddr, udpUrlPort.Get(), kApiGetUAddr)
	v, _ := net.ResolveUDPAddr("udp", gUdpSessions.server.addr)
	return fmt.Sprintf("%v:%v", gLocalAddr, v.Port)
}

type Handler struct {
	Server     *UdpServer
	listenAddr string

	kApiUrls map[uint16]string
}

func NewHandler(apiServerUrl string, listenAddr string) *Handler {
	urls := make(map[uint16]string)
	urls[CmdRegister] = apiServerUrl + UrlRegister
	urls[CmdLogin] = apiServerUrl + UrlLogin
	urls[CmdUnbind] = apiServerUrl + UrlUnbind
	urls[CmdRename] = apiServerUrl + UrlChangeName

	_, p, err := net.SplitHostPort(listenAddr)
	if err != nil {
		glog.Fatalf("Parse TCP address for API server failed: %v", err)
	}
	port, _ := strconv.Atoi(p)
	udpUrlPort.Set(int32(port))
	h := &Handler{
		listenAddr: listenAddr,
		kApiUrls:   urls,
	}
	return h
}

func (h *Handler) Run() {
	mux := http.NewServeMux()
	mux.HandleFunc(kApiGetUAddr, h.OnBackendGetDeviceAddr)

	s := &http.Server{
		Addr:    h.listenAddr,
		Handler: mux,
	}
	glog.Infof("Start API server on %s", s.Addr)
	err := s.ListenAndServe()
	if err != nil {
		glog.Fatalf("Start HTTP for UDP server failed: %v", err)
	}
}

// accept request from api server
// DON'T need it any more
//func (h *Handler) OnBindingRequest(w http.ResponseWriter, r *http.Request) {
//	var (
//		deviceId  int64
//		userId    int64
//		deviceMac []byte
//
//		seqNum    uint16
//		cmdSeqNum uint8
//	)
//	// TODO get seqNum and cmdSeqNum
//
//	// make a message
//	req := make([]byte, FrameHeaderLen+DataHeaderLen+16)
//	// frame header
//	frame := req[:FrameHeaderLen]
//	frame[0] = 1<<7 | 0x2
//	binary.LittleEndian.PutUint16(frame[2:4], seqNum)
//	binary.LittleEndian.PutUint32(frame[4:8], uint32(time.Now().Unix()))
//	binary.LittleEndian.PutUint64(frame[8:16], uint64(deviceId))
//	// data header
//	header := req[FrameHeaderLen : FrameHeaderLen+DataHeaderLen]
//	header[1] = cmdSeqNum
//	binary.LittleEndian.PutUint16(header[4:6], 0xE4)
//	header[6] = msgs.ChecksumHeader(req, FrameHeaderLen+8)
//	// data body
//	copy(req[FrameHeaderLen+DataHeaderLen:], deviceMac)
//	binary.LittleEndian.PutUint64(req[FrameHeaderLen+DataHeaderLen+8:], uint64(userId))
//
//	//header[7] = msgs.ChecksumBody(req[FrameHeaderLen+DataHeaderLen:], 16)
//
//	// send message
//
//	err := gUdpSessions.PushMsg(deviceId, req)
//	if err != nil {
//		w.WriteHeader(200)
//		// timeout
//		w.Write([]byte("{\"status\":2}"))
//		return
//	}
//
//	c := make(chan error)
//	err = AppendBindingRequest(c, deviceId, userId)
//	if err != nil {
//		w.WriteHeader(200)
//		w.Write([]byte("{\"status\":3}"))
//		return
//	}
//	// wait for async response on a chan
//	select {
//	case err := <-c:
//		if err != nil {
//			// if write response into HTTP
//			w.WriteHeader(200)
//			// parse
//			w.Write([]byte("{\"status\":4}"))
//		} else {
//			// if write response into HTTP
//			w.WriteHeader(200)
//			// parse
//			w.Write([]byte("{\"status\":0}"))
//		}
//	case <-time.After(2 * time.Minute):
//		w.WriteHeader(200)
//		// timeout
//		w.Write([]byte("{\"status\":1}"))
//	}
//}

// 获取指定设备的外网UDP地址
func (h *Handler) OnBackendGetDeviceAddr(w http.ResponseWriter, r *http.Request) {
	if strings.ToUpper(r.Method) != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.ParseInt(r.FormValue("deviceId"), 10, 64)
	if err != nil {
		w.WriteHeader(404)
		return
	}
	addr, err := gUdpSessions.GetDeviceAddr(id)
	response := make(map[string]interface{})
	if err == nil {
		response["status"] = 0
		response["deviceAddr"] = addr
	} else {
		response["status"] = 1
	}
	buf, err := json.Marshal(response)
	if err != nil {
		glog.Fatal("Marshal response %v to json failed: %v", response, err)
	}
	w.Header()["Content-Type"] = []string{"application/json; charset=utf-8"}
	w.Write(buf)
}

func (h *Handler) Process(peer *net.UDPAddr, input []byte) {
	msgCopy := make([]byte, len(input))
	copy(msgCopy, input)

	t := &UdpMsg{
		Peer:   peer,
		Msg:    msgCopy,
		Input:  make(map[string]string),
		Output: make(map[string]string),
	}

	go h.process(t)
}

func (h *Handler) process(t *UdpMsg) {
	err := h.handle(t)
	if err != nil {
		glog.Errorf("[udp:error occurs]   msg (len[%d] %v) error: %v", len(t.Msg), t.Msg, err)
	}
}

func (h *Handler) handle(t *UdpMsg) error {
	// TODO need decryption later
	busi := ""
	mlen := len(t.Msg)
	if mlen < FrameHeaderLen {
		return fmt.Errorf("[protocol] invalid message length for device proxy,reason: mlen < FrameHeaderLen | %v < %v.", mlen, FrameHeaderLen)
	}
	// check opcode
	op := (0x7 & t.Msg[0])
	if op != 0x2 && op != 0x3 {
		return fmt.Errorf("[protocol] reason: wrong opcode, op!=2&&op!=3, op=%v", op)
	}
	glog.Infoln("转发类型为", op)
	if op == 0x2 {
		if mlen < FrameHeaderLen+DataHeaderLen {
			return fmt.Errorf("[protocol] invalid message length for protocol,  mlen < FrameHeaderLen+DataHeaderLen ,%v < %v", mlen, FrameHeaderLen+DataHeaderLen)
		}
		packNum := binary.LittleEndian.Uint16(t.Msg[2:4])
		bodyLen := int(binary.LittleEndian.Uint16(t.Msg[FrameHeaderLen+6:]))
		// discard msg if found checking error
		if t.Msg[FrameHeaderLen+8] != msgs.ChecksumHeader(t.Msg, FrameHeaderLen+8) {
			return fmt.Errorf("checksum header error %v!=%v", t.Msg[FrameHeaderLen+8], msgs.ChecksumHeader(t.Msg, FrameHeaderLen+8))
		}

		// check body
		if t.Msg[FrameHeaderLen+9] != msgs.ChecksumHeader(t.Msg[FrameHeaderLen+10:], 2+bodyLen) {
			return fmt.Errorf("checksum body error %v!=%v", t.Msg[FrameHeaderLen+9], msgs.ChecksumHeader(t.Msg[FrameHeaderLen+10:], 2+bodyLen))
		}

		var (
			sess      *UdpSession
			sid       *uuid.UUID
			err       error
			body      []byte
			bodyIndex int
		)

		bodyIndex = FrameHeaderLen + DataHeaderLen
		if bodyLen != len(t.Msg[bodyIndex:]) { //报文实际长度与报文体内设置的长度不一致
			return fmt.Errorf("wrong body length in data header: %d != %d", bodyLen, len(t.Msg[bodyIndex:]))
		}
		body = t.Msg[bodyIndex : bodyIndex+bodyLen]

		// parse data(udp)
		// 28 = FrameHeaderLen + 4
		MsgId := binary.LittleEndian.Uint16(t.Msg[28:30])
		glog.Infof("业务%v", fmt.Sprintf("[%x]", MsgId))
		if MsgId == CmdSyncState {
			busi = "CmdSyncState"
			glog.Infoln("执行设备状态同步")
			GMsgBusManager.Push2Bus(0, nil, t.Msg)
			return nil
		} else if MsgId == 0x31 {
			glog.Infoln("执行报警信息同步")
			busi = "报警信息同步"
			GMsgBusManager.Push2Bus(0, nil, t.Msg)
			return nil
			//TODO alarm msg synchronization
		} else if MsgId != CmdGetToken {
			sid, err = uuid.Parse(t.Msg[bodyIndex : bodyIndex+16])
			if err != nil {
				return fmt.Errorf("parse session id error: %v", err)
			}
			sess, err = gUdpSessions.GetSession(sid)
			if err != nil || sess == nil {
				return fmt.Errorf("cmd: %X, sid: [%v], error: %v", MsgId, sid, err)
			}
			err = sess.VerifySession(packNum)
			if err != nil {
				return fmt.Errorf("cmd: %X, verify session error: %v", MsgId, err)
			}
			sess.Sid = sid.String()
		}

		output := make([]byte, bodyIndex)
		// copy same packNum into this ACK response
		copy(output[:bodyIndex], t.Msg[:bodyIndex])

		var res []byte
		t.CmdType = MsgId
		switch MsgId {
		case CmdGetToken:
			res, err = h.onGetToken(t, body)
			busi = "CmdGetToken"
		case CmdRegister:
			t.Url = h.kApiUrls[MsgId]
			res, err = h.onRegister(t, sess, body)
			busi = "CmdRegister"
		case CmdLogin:
			t.Url = h.kApiUrls[MsgId]
			res, err = h.onLogin(t, sess, body)
			busi = "CmdLogin"
		case CmdRename:
			t.Url = h.kApiUrls[MsgId]
			res, err = h.onRename(t, sess, body)
			busi = "CmdRename"
		case CmdUnbind:
			t.Url = h.kApiUrls[MsgId]
			res, err = h.onUnbind(t, sess, body)
			busi = "CmdUnbind"

		case CmdHeartBeat:
			res, err = h.onHearBeat(t, sess, body)
			busi = "CmdHeartBeat"
		case CmdSubDeviceOffline:
			res, err = h.onSubDeviceOffline(t, sess, body)
			busi = "CmdSubDeviceOffline"
			//		case CmdSyncState:
			//			GMsgBusManager.Push2Bus(0, nil, t.Msg)

		default:
			glog.Warningf("invalid command type %v", MsgId)
			// don't reply on wrong msgid

			//b := make([]byte, 4)
			//binary.LittleEndian.PutUint32(b, uint32(DAckBadCmd))
			//output = append(output, b...)

			//err = computeCheck(output)
			//if err == nil {
			//	h.Server.Send(t.Peer, output)
			//} else {
			//	if glog.V(1) {
			//		glog.Warningf("[handle] check message failed: %v", err)
			//	}
			//}
			return fmt.Errorf("invalid command type [%v], %v", MsgId, sess)
		}
		if sess != nil {
			gUdpSessions.SaveSession(sid, sess)
		}
		if err != nil {
			glog.Errorf("[handle] cmd: [%X], error: [%v]", MsgId, err)
		}
		if MsgId != CmdGetToken {
			//			copy(res[4:20], t.Msg[bodyIndex:bodyIndex+16])
			copy(res[4:20], sid[:])
		}
		if res != nil {
			output = append(output, res...)
		}

		output[FrameHeaderLen] |= (msgs.FlagAck | msgs.FlagRead)
		binary.LittleEndian.PutUint16(output[FrameHeaderLen+6:], uint16(len(res)))
		output[FrameHeaderLen+8] = msgs.ChecksumHeader(output, FrameHeaderLen+8)
		output[FrameHeaderLen+9] = msgs.ChecksumHeader(output[FrameHeaderLen+10:], 2+len(res))

		if sess != nil {
			h.Server.Send2(t.Peer, output, sess.DeviceId, busi)
		} else {
			h.Server.Send2(t.Peer, output, 0, busi)
		}

	} else if op == 0x3 {
		glog.Infoln("开始转发消息")
		if mlen < FrameHeaderLen+kSidLen+FrameHeaderLen+DataHeaderLen {
			return fmt.Errorf("[udp|forward] invalid message length for protocol")
		}
		packNum := binary.LittleEndian.Uint16(t.Msg[2:4])
		//		bodyLen := int(binary.LittleEndian.Uint16(t.Msg[FrameHeaderLen+kSidLen+FrameHeaderLen+6:]))
		// discard msg if found checking error
		//		if t.Msg[FrameHeaderLen+kSidLen+FrameHeaderLen+8] != msgs.ChecksumHeader(t.Msg, FrameHeaderLen+kSidLen+FrameHeaderLen+8) {
		//			return fmt.Errorf("checksum header error")
		//		}

		// check data body
		//		if t.Msg[FrameHeaderLen+kSidLen+FrameHeaderLen+9] != msgs.ChecksumHeader(t.Msg[FrameHeaderLen+kSidLen+FrameHeaderLen+10:], 2+bodyLen) {
		//			return fmt.Errorf("checksum data error")
		//		}

		var (
			sess *UdpSession
			sid  *uuid.UUID
			err  error
		)

		//bodyIndex = FrameHeaderLen + DataHeaderLen
		//if bodyLen != len(t.Msg[bodyIndex:]) {
		//	return fmt.Errorf("wrong body length in data header: %d != %d", bodyLen, len(t.Msg[bodyIndex:]))
		//}
		//		body = t.Msg[bodyIndex : bodyIndex+bodyLen]

		sid, err = uuid.Parse(t.Msg[FrameHeaderLen : FrameHeaderLen+kSidLen])
		if err != nil {
			return fmt.Errorf("parse session id error: %v", err)
		}

		sess, err = gUdpSessions.GetSession(sid)
		glog.Infof("sid:%v | sess:%v", sid.String(), sess)
		if err != nil || sess == nil {
			return fmt.Errorf("[udp|forward] sid: [%v], error: %v", sid, err)
		}
		err = sess.VerifySession(packNum)
		if err != nil {
			return fmt.Errorf("[udp|forward] verify session error: %v", err)
		}

		toId := int64(binary.LittleEndian.Uint64(t.Msg[8:16]))
		srcId := int64(binary.LittleEndian.Uint64(t.Msg[16:24]))

		// check binded ids
		destIds := sess.CalcDestIds(toId)

		glog.Infof("[udp|forward] %d -> %d udp, calc to: %v, data: (len: %d)%v", srcId, toId, destIds, len(t.Msg), t.Msg)
		if len(destIds) > 0 {
			GMsgBusManager.Push2Bus(srcId, destIds, t.Msg)
		} else {
			glog.Infof("[udp|forward] from %v to %v | destination is empty", srcId, toId)
		}
	}
	return nil
}

func (h *Handler) onGetToken(t *UdpMsg, body []byte) ([]byte, error) {
	output := make([]byte, 28)
	if len(body) != 24 {
		return output, fmt.Errorf("[udp:onGetToken] bad body length %d", len(body))
	}

	var err error

	mac := make([]byte, 8)
	copy(mac, body[:8])
	sn := make([]byte, 16)
	copy(sn, body[8:24])

	// TODO verify mac and sn by real production's information
	// if err := VerifyDeviceInfo(mac, sn); err != nil {
	//	return nil, fmt.Errorf("invalid device error: %v", err)
	// }

	sid := NewUuid()
	s := NewUdpSession(t.Peer)
	err = gUdpSessions.SaveSession(sid, s)
	if err != nil {
		glog.Fatalf("[udp:onGetToken] SaveSession failed: %v", err)
	}

	binary.LittleEndian.PutUint32(output[:4], 0) //返回结果
	copy(output[4:20], sid[:16])                 //把sid传给板子

	return output, nil
}

func (h *Handler) onRegister(t *UdpMsg, sess *UdpSession, body []byte) ([]byte, error) {
	if len(body) < 310 {
		return nil, fmt.Errorf("[udp:onRegister] bad body length %d", len(body))
	}

	dv := body[16:18]
	//comType := body[18:20] //通讯类型
	produceTime := binary.LittleEndian.Uint32(body[20:24])
	mac := hex.EncodeToString(body[24:32])
	sn := body[32:48]
	//sign := body[48:308] //签名
	nameLen := body[308]

	var name []byte
	if nameLen > 0 {
		name = body[309 : 309+int(nameLen)]
	}

	t.Input["mac"] = mac
	t.Input["dv"] = fmt.Sprintf("%d", binary.LittleEndian.Uint16(dv))
	t.Input["pt"] = fmt.Sprintf("%d", produceTime)

	t.Input["sn"] = fmt.Sprintf("%x", sn)
	t.Input["name"] = string(name)

	output := make([]byte, 92)
	httpStatus, rep, err := t.DoHTTPTask()
	if err != nil {
		binary.LittleEndian.PutUint32(output[0:4], uint32(httpStatus))
		return output, err
	}

	if s, ok := rep["status"]; ok {
		if status, ok := s.(float64); ok {
			binary.LittleEndian.PutUint32(output[0:4], uint32(int32(status)))
		}
	}
	// device need id in protocol
	if c, ok := rep["cookie"]; ok {
		if cookie, ok := c.(string); ok {
			//fmt.Println(mac, len(cookie), cookie)
			copy(output[28:92], []byte(cookie))
			ss := strings.SplitN(cookie, "|", 2)
			if len(ss) == 0 {
				binary.LittleEndian.PutUint32(output[0:4], uint32(DAckServerError))
				return output, nil
			}
			id, err := strconv.ParseInt(ss[0], 10, 64)
			if err != nil {
				binary.LittleEndian.PutUint32(output[0:4], uint32(DAckServerError))
				return output, nil
			}
			binary.LittleEndian.PutUint64(output[20:28], uint64(id))
			//sess.DeviceId = id
		}
	}
	return output, nil
}

func (h *Handler) onRegister2(t *UdpMsg, sess *UdpSession, body []byte) ([]byte, error) {
	output := make([]byte, 92)
	if len(body) < 296 {
		return output, fmt.Errorf("[udp:onRegister] bad body length %d", len(body))
	}

	dv := body[16:18]
	//comType := body[18:20] //通讯类型
	mac := hex.EncodeToString(body[18:26])
	sn := body[26:34]
	sign := hex.EncodeToString(body[34:294]) //签名
	nameLen := body[295]

	var name []byte
	if nameLen > 0 {
		name = body[296 : 296+int(nameLen)]
	}

	t.Input["mac"] = mac
	t.Input["dv"] = fmt.Sprintf("%d", binary.LittleEndian.Uint16(dv))
	t.Input["sign"] = sign
	t.Input["sn"] = fmt.Sprintf("%x", sn)
	t.Input["name"] = string(name)

	httpStatus, rep, err := t.DoHTTPTask()
	if err != nil {
		binary.LittleEndian.PutUint32(output[0:4], uint32(httpStatus))
		return output, err
	}

	if s, ok := rep["status"]; ok {
		if status, ok := s.(float64); ok {
			binary.LittleEndian.PutUint32(output[0:4], uint32(int32(status)))
		}
	}
	// device need id in protocol
	if c, ok := rep["cookie"]; ok {
		if cookie, ok := c.(string); ok {
			//fmt.Println(mac, len(cookie), cookie)
			copy(output[28:92], []byte(cookie))
			ss := strings.SplitN(cookie, "|", 2)
			if len(ss) == 0 {
				binary.LittleEndian.PutUint32(output[0:4], uint32(DAckServerError))
				return output, nil
			}
			id, err := strconv.ParseInt(ss[0], 10, 64)
			if err != nil {
				binary.LittleEndian.PutUint32(output[0:4], uint32(DAckServerError))
				return output, nil
			}
			binary.LittleEndian.PutUint64(output[20:28], uint64(id))
			//sess.DeviceId = id
		}
	}
	return output, nil
}

func (h *Handler) onLogin2(t *UdpMsg, sess *UdpSession, body []byte) ([]byte, error) {
	output := make([]byte, 29)
	if len(body) != 88 {
		return output, fmt.Errorf("[udp:onLogin] bad body length %v", len(body))
	}

	mac := hex.EncodeToString(body[16:24])
	sess.Mac = body[16:24]
	// C program sent cookie string without trim zero bytes
	cookie := string(bytes.TrimRight(body[24:88], "\x00"))
	t.Input["mac"] = mac
	t.Input["cookie"] = cookie
	//fmt.Println(mac, len(cookie), cookie)
	//glog.Infof("LOGIN: mac: %s, cookie: %v", mac, string(cookie))

	httpStatus, rep, err := t.DoHTTPTask()
	if err != nil {
		glog.Infoln("err occurs when device login", httpStatus, err)
		binary.LittleEndian.PutUint32(output[0:4], uint32(httpStatus))
		return output, err
	}
	var status int32 = DAckHTTPError
	firstLogin := true
	if s, ok := rep["status"]; ok {
		if n, ok := s.(float64); ok {
			status = int32(n)

			if n == 0 {
				ss := strings.SplitN(cookie, "|", 2)

				if len(ss) > 0 {
					id, err := strconv.ParseInt(ss[0], 10, 64)
					if err == nil {
						sess.DeviceId = id
						if strSid, ok := gUdpSessions.devmap[sess.DeviceId]; ok {
							glog.Infoln("杀死上一个")
							gUdpSessions.udpmap[gUdpSessions.sidmap[strSid].Addr.String()].Reset(0)
							firstLogin = false
						} else {
							glog.Infoln("nothing to kill")
						}
					}
				}

				bindedIds, owner, err := GetDeviceUsers(sess.DeviceId)
				glog.Infoln(bindedIds, owner)
				sess.Owner = owner
				if err != nil {
					glog.Errorf("[udp|getIds] id [%d] get ids error: %v, ids: %v", sess.DeviceId, err, bindedIds)
				} else {
					if len(bindedIds) > 0 {
						sess.BindedUsers = bindedIds
						binary.LittleEndian.PutUint64(output[20:28], uint64(sess.Owner))
					}
				}
				glog.Infof("[udp|getIds] id [%d] get ids: %v", sess.DeviceId, bindedIds)

				_, err = SetUserOnline(sess.DeviceId, fmt.Sprintf("%v-%v", gLocalAddr, gCometType))
				if err != nil {
					glog.Errorf("[udp:err] SetUserOnline error [uid: %d] %v\n", sess.DeviceId, err)
				} else {
					/**向用户推此设备在线消息*/
					PushDevOnlineMsgToUsers(sess)
					gUdpSessions.udpmap[sess.Addr.String()] = time.AfterFunc(time.Duration(gUdpTimeout)*time.Second, func() {
						if firstLogin {
							PushDevOfflineMsgToUsers(sess)
						}
						gUdpSessions.udplk.Lock()
						delete(gUdpSessions.udpmap, sess.Addr.String())
						gUdpSessions.udplk.Unlock()
						gUdpSessions.sidlk.Lock()
						delete(gUdpSessions.sidmap, gUdpSessions.devmap[sess.DeviceId])
						gUdpSessions.sidlk.Unlock()
						gUdpSessions.devlk.Lock()
						delete(gUdpSessions.devmap, sess.DeviceId)
						gUdpSessions.devlk.Unlock()
						//					SetUserOffline(sess.DeviceId, h.Server.con.LocalAddr().String())
						SetUserOffline(sess.DeviceId, fmt.Sprintf("%v-%v", gLocalAddr, gCometType))
					})
					glog.Infoln("--------------------------------sess.Sid:", sess.Sid)
					gUdpSessions.sidlk.Lock()
					gUdpSessions.sidmap[sess.Sid] = sess
					gUdpSessions.sidlk.Unlock()
					gUdpSessions.devlk.Lock()
					gUdpSessions.devmap[sess.DeviceId] = sess.Sid
					gUdpSessions.devlk.Unlock()
				}
			}
		}
	}
	binary.LittleEndian.PutUint32(output[0:4], uint32(status))
	output[28] = kHeartBeatSec
	return output, nil
}

func (h *Handler) onLogin(t *UdpMsg, sess *UdpSession, body []byte) ([]byte, error) {
	output := make([]byte, 21)
	if len(body) != 88 {
		return output, fmt.Errorf("[udp:onLogin] bad body length %v", len(body))
	}

	mac := hex.EncodeToString(body[16:24])
	sess.Mac = body[16:24]
	// C program sent cookie string without trim zero bytes
	cookie := string(bytes.TrimRight(body[24:88], "\x00"))
	t.Input["mac"] = mac
	t.Input["cookie"] = cookie
	//fmt.Println(mac, len(cookie), cookie)
	//glog.Infof("LOGIN: mac: %s, cookie: %v", mac, string(cookie))

	httpStatus, rep, err := t.DoHTTPTask()
	if err != nil {
		glog.Infoln("err occurs when device login", httpStatus, err)
		binary.LittleEndian.PutUint32(output[0:4], uint32(httpStatus))
		return output, err
	}
	devAdr := ""
	var status int32 = DAckHTTPError
	if s, ok := rep["status"]; ok {
		if n, ok := s.(float64); ok {
			status = int32(n)

			if n == 0 {
				ss := strings.SplitN(cookie, "|", 2)

				if len(ss) > 0 {
					id, err := strconv.ParseInt(ss[0], 10, 64)
					if err == nil {
						sess.DeviceId = id
						PubDevOnlineChannel(id)
						devAdr = GotDevAddr(id)
					}
				}

				bindedIds, owner, err := GetDeviceUsers(sess.DeviceId)
				glog.Infoln(bindedIds, owner)
				sess.Owner = owner
				if err == nil {
					if len(bindedIds) > 0 {
						sess.BindedUsers = bindedIds
						//						binary.LittleEndian.PutUint64(output[20:28], uint64(sess.Owner))
					}
				}
				glog.Infof("[udp|getIds] id [%d] get ids: %v", sess.DeviceId, bindedIds)

				_, err = SetUserOnline(sess.DeviceId, fmt.Sprintf("%v-%v", gLocalAddr, gCometType))
				if err != nil {
					glog.Errorf("[udp:err] SetUserOnline error [uid: %d] %v\n", sess.DeviceId, err)
				} else {
					/**向用户推此设备在线消息*/
					glog.Infoln("--------------------------------sess.Sid:", sess.Sid,devAdr)
					if devAdr == "" {
						PushDevOnlineMsgToUsers(sess)
					}
					gUdpSessions.udpmap[sess.Addr.String()] = time.AfterFunc(time.Duration(gUdpTimeout)*time.Second, func() {
						PushDevOfflineMsgToUsers(sess)
						gUdpSessions.udplk.Lock()
						delete(gUdpSessions.udpmap, sess.Addr.String())
						gUdpSessions.udplk.Unlock()
						gUdpSessions.sidlk.Lock()
						delete(gUdpSessions.sidmap, gUdpSessions.devmap[sess.DeviceId])
						gUdpSessions.sidlk.Unlock()
						gUdpSessions.devlk.Lock()
						delete(gUdpSessions.devmap, sess.DeviceId)
						gUdpSessions.devlk.Unlock()
						//					SetUserOffline(sess.DeviceId, h.Server.con.LocalAddr().String())
						SetUserOffline(sess.DeviceId, fmt.Sprintf("%v-%v", gLocalAddr, gCometType))
					})
					gUdpSessions.sidlk.Lock()
					gUdpSessions.sidmap[sess.Sid] = sess
					gUdpSessions.sidlk.Unlock()
					gUdpSessions.devlk.Lock()
					gUdpSessions.devmap[sess.DeviceId] = sess.Sid
					gUdpSessions.devlk.Unlock()
				}
			}
		}
	}
	binary.LittleEndian.PutUint32(output[0:4], uint32(status))
	output[20] = kHeartBeatSec
	return output, nil
}
func (h *Handler) onRename(t *UdpMsg, sess *UdpSession, body []byte) ([]byte, error) {
	mac := hex.EncodeToString(body[16:24])
	nameLen := body[24]
	name := body[25 : 25+nameLen]

	t.Input["name"] = string(name)
	t.Input["mac"] = mac

	output := make([]byte, 28)
	copy(output[20:28], body[16:24])

	httpStatus, rep, err := t.DoHTTPTask()
	if err != nil {
		binary.LittleEndian.PutUint32(output[0:4], uint32(httpStatus))
		return output, err
	}

	var status int32 = DAckHTTPError
	if s, ok := rep["status"]; ok {
		if n, ok := s.(float64); ok {
			status = int32(n)
		}
	}
	binary.LittleEndian.PutUint32(output[0:4], uint32(status))
	if status != 0 {
		return output, nil
	}
	return output, nil
}
func (h *Handler) onUnbind(t *UdpMsg, sess *UdpSession, body []byte) ([]byte, error) {
	id := int64(binary.LittleEndian.Uint64(body[16:24]))
	glog.Infoln("onUbind : ", id, sess.DeviceId)
	t.Input["id"] = fmt.Sprint(sess.DeviceId)

	output := make([]byte, 28)
	copy(output[20:28], sess.Mac)

	httpStatus, rep, err := t.DoHTTPTask()
	if err != nil {
		binary.LittleEndian.PutUint32(output[0:4], uint32(httpStatus))
		return output, err
	}

	var status int32 = DAckHTTPError
	if s, ok := rep["status"]; ok {
		if n, ok := s.(float64); ok {
			status = int32(n)
		}
	}
	binary.LittleEndian.PutUint32(output[0:4], uint32(status))
	if status != 0 {
		return output, nil
	}
	return output, nil
}

// 绑定过程不涉及板子
//func (h *Handler) onDoBind(t *UdpMsg, sess *UdpSession, body []byte) ([]byte, error) {
//	uid := body[16:24]
//	result := binary.LittleEndian.Uint32(body[24:28])
//
//	t.Input["uid"] = fmt.Sprintf("%d", uid)
//	t.Input["result"] = fmt.Sprintf("%d", result)
//
//	output := make([]byte, 4)
//
//	httpStatus, rep, err := t.DoHTTPTask()
//	if err != nil {
//		binary.LittleEndian.PutUint32(output[0:4], uint32(httpStatus))
//		return output, err
//	}
//
//	// TODO status not in protocol
//	if s, ok := rep["status"]; ok {
//		if status, ok := s.(float64); ok {
//			binary.LittleEndian.PutUint32(output[0:4], uint32(int32(status)))
//			if status != 0 {
//				return output, nil
//			}
//		}
//	}
//	return output, nil
//}

func (h *Handler) onHearBeat(t *UdpMsg, sess *UdpSession, body []byte) ([]byte, error) {
	glog.Infoln("OnHeartBeat is dealing.")
	err := sess.Update(t.Peer)

	output := make([]byte, 20)
	if err != nil {
		binary.LittleEndian.PutUint32(output[:4], 1)
	} else {
		binary.LittleEndian.PutUint32(output[:4], 0)
	}

	//set device offine event's time
	v, ok := gUdpSessions.udpmap[sess.Addr.String()]
	if ok {
		glog.Infof("%v's session has been refresh.", sess.DeviceId)
		v.Reset(time.Duration(gUdpTimeout) * time.Second)
	} else {
		glog.Infof("there is no session to refresh. %v", sess)
	}
	return output, err
}

// TODO 下线消息的业务逻辑还未详细定义，收到消息后是应该用websocket还是udp转发该消息至手机？
func (h *Handler) onSubDeviceOffline(t *UdpMsg, sess *UdpSession, body []byte) ([]byte, error) {
	return nil, fmt.Errorf("[udp:onSubDeviceOffline]NOT IMPLEMENTED API: onSubDeviceOffline")
}
