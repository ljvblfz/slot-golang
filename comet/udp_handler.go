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
	//	uuid "github.com/nu7hatch/gouuid"
)

const (
	FrameHeaderLen = 24
	DataHeaderLen  = 10

	kHeartBeatSec = 120
	kHeartBeat    = kHeartBeatSec * time.Second

	kApiGetUAddr = "/api/device/udpaddr"
)

var (
	//ErrSessTimeout = fmt.Errorf("session timeout")
	ErrSessPackSeq = fmt.Errorf("Wrong Package Sequence Number")

	udpUrlPort atomic.AtomicInt32
)

func GetCometUdpUrl() string {
	//	return fmt.Sprintf("http://%s:%d%s", gLocalAddr, udpUrlPort.Get(), kApiGetUAddr)
	v, _ := net.ResolveUDPAddr("udp", gUdpSessions.Server.addr)
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
	glog.Infof("HTTP server on %s", s.Addr)
	err := s.ListenAndServe()
	if err != nil {
		glog.Fatalf("Start HTTP for UDP server failed: %v", err)
	}
}

// accept request from api server
// DON'T need it any more
//func (h *Handler) OnBindingRequest(w http.ResponseWriter, r *http.Request) {
//	var (
//		DevId  int64
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
//	binary.LittleEndian.PutUint64(frame[8:16], uint64(DevId))
//	// data header
//	header := req[FrameHeaderLen : FrameHeaderLen+DataHeaderLen]
//	header[1] = cmdSeqNum
//	binary.LittleEndian.PutUint16(header[4:6], 0xE4)
//	header[6] = msgs.Crc(req, FrameHeaderLen+8)
//	// data body
//	copy(req[FrameHeaderLen+DataHeaderLen:], deviceMac)
//	binary.LittleEndian.PutUint64(req[FrameHeaderLen+DataHeaderLen+8:], uint64(userId))
//
//	//header[7] = msgs.ChecksumBody(req[FrameHeaderLen+DataHeaderLen:], 16)
//
//	// send message
//
//	err := gUdpSessions.PushMsg(DevId, req)
//	if err != nil {
//		w.WriteHeader(200)
//		// timeout
//		w.Write([]byte("{\"status\":2}"))
//		return
//	}
//
//	c := make(chan error)
//	err = AppendBindingRequest(c, DevId, userId)
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
	id, err := strconv.ParseInt(r.FormValue("DevId"), 10, 64)
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
		glog.Errorf("[UDPCOMET:PROCESS] ERROR!!, msg(len[%d]，%v)，%v", len(t.Msg), t.Msg, err)
	}
}

func (h *Handler) handle(t *UdpMsg) error {
	// TODO need decryption later
	busi := ""
	mlen := len(t.Msg)
	if mlen < FrameHeaderLen {
		return fmt.Errorf("[UDPCOMET:HANDLE] invalid message length for device proxy,reason: mlen < FrameHeaderLen | %v < %v.", mlen, FrameHeaderLen)
	}
	// check opcode
	op := (0x7 & t.Msg[0])
	if op != 0x2 && op != 0x3 {
		return fmt.Errorf("[UDPCOMET:HANDLE] reason: wrong opcode, op!=2&&op!=3, op=%v", op)
	}
	if op == 0x2 {
		var (
			sess      *UdpSession
			err       error
			body      []byte
			bodyIndex int
		)
		if mlen < FrameHeaderLen+DataHeaderLen {
			return fmt.Errorf("[UDPCOMET:02] invalid message length for protocol,  mlen < FrameHeaderLen+DataHeaderLen ,%v < %v", mlen, FrameHeaderLen+DataHeaderLen)
		}
		packNum := binary.LittleEndian.Uint16(t.Msg[2:4])
		bodyLen := int(binary.LittleEndian.Uint16(t.Msg[FrameHeaderLen+4 : FrameHeaderLen+6]))
		bodyIndex = FrameHeaderLen + DataHeaderLen
		if bodyLen != len(t.Msg[bodyIndex:]) { //报文实际长度与报文体内设置的长度不一致
			return fmt.Errorf("[UDPCOMET:02] wrong body length in data header: %d != %d", bodyLen, len(t.Msg[bodyIndex:]))
		}
		// discard msg if found checking error
		if t.Msg[FrameHeaderLen+6] != msgs.Crc(t.Msg, FrameHeaderLen+6) {
			return fmt.Errorf("[UDPCOMET:02] checksum header error %v!=%v", t.Msg[FrameHeaderLen+8], msgs.Crc(t.Msg, FrameHeaderLen+8))
		}

		// check body
		if t.Msg[FrameHeaderLen+7] != msgs.Crc(t.Msg[FrameHeaderLen+8:], 2+bodyLen) {
			return fmt.Errorf("[UDPCOMET:02] checksum body error %v!=%v", t.Msg[FrameHeaderLen+7], msgs.Crc(t.Msg[FrameHeaderLen+8:], 2+bodyLen))
		}

		body = t.Msg[bodyIndex : bodyIndex+bodyLen]

		// parse data(udp)
		// 28 = FrameHeaderLen + 4
		MsgId := binary.LittleEndian.Uint16(t.Msg[28:30])
		if glog.V(3) {
			glog.Infof("业务%v", fmt.Sprintf("[%x]", MsgId))
		}
		if MsgId == 0x31 {
			if glog.V(3) {
				glog.Infoln("[UDPCOMET:02] 执行报警信息同步")
			}
			busi = "报警信息同步"
			GMsgBusManager.Push2Msgbus(0, nil, t.Msg)
			return nil
			//TODO alarm msg synchronization
		} else if MsgId != CmdSess {
			sess = Byte2Sess(t.Msg[bodyIndex : bodyIndex+8])
			err = sess.VerifySession(t.Peer.String())
			if err != nil {
				return err
			}
			err = sess.VerifyPack(packNum)
			if err != nil {
				return err
			}
		}

		output := make([]byte, bodyIndex)
		// copy same packNum into this ACK response
		copy(output[:bodyIndex], t.Msg[:bodyIndex])

		var res []byte
		t.CmdType = MsgId
		switch MsgId {
		case CmdSess:
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

		case CmdHeartBeat:
			res, err = h.onHearBeat(t, sess, body)
			busi = "CmdHeartBeat"
		case CmdLoginout:
			res, err = h.onLoginout(t, sess, body)
			busi = "onLoginout"
		default:
			return fmt.Errorf("[UDPCOMET:02] invalid command type [%v], %v", MsgId, sess)
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
		}
		if err != nil {
			glog.Errorf("[UDPCOMET:02] cmd: [%X], [%v]", MsgId, err)
		}
		if MsgId != CmdSess {
			copy(res[4:20], t.Msg[bodyIndex:bodyIndex+8])
		}
		if res != nil {
			output = append(output, res...)
		}

		output[FrameHeaderLen] |= (msgs.FlagAck | msgs.FlagRead)
		binary.LittleEndian.PutUint32(output[4:8], uint32(time.Now().Unix()))
		binary.LittleEndian.PutUint16(output[FrameHeaderLen+6:], uint16(len(res)))
		output[FrameHeaderLen+6] = msgs.Crc(output, FrameHeaderLen+6)
		output[FrameHeaderLen+7] = msgs.Crc(output[FrameHeaderLen+8:], 2+len(res))

		if sess != nil {
			h.Server.Send2(t.Peer, output, sess, busi)
		} else {
			h.Server.Send(t.Peer, output)
		}

	} else if op == 0x3 {
		glog.Infoln("---------------------opcode 3-------------------")
		if mlen < FrameHeaderLen+kSidLen+FrameHeaderLen+DataHeaderLen {
			return fmt.Errorf("[UDPCOMET:03] invalid message length, %v<%v", mlen, FrameHeaderLen+kSidLen+FrameHeaderLen+DataHeaderLen)
		}
		packNum := binary.LittleEndian.Uint16(t.Msg[2:4])
		//		bodyLen := int(binary.LittleEndian.Uint16(t.Msg[FrameHeaderLen+kSidLen+FrameHeaderLen+6:]))
		// discard msg if found checking error
		//		if t.Msg[FrameHeaderLen+kSidLen+FrameHeaderLen+8] != msgs.Crc(t.Msg, FrameHeaderLen+kSidLen+FrameHeaderLen+8) {
		//			return fmt.Errorf("checksum header error")
		//		}

		// check data body
		//		if t.Msg[FrameHeaderLen+kSidLen+FrameHeaderLen+9] != msgs.Crc(t.Msg[FrameHeaderLen+kSidLen+FrameHeaderLen+10:], 2+bodyLen) {
		//			return fmt.Errorf("checksum data error")
		//		}

		var (
			sess *UdpSession
			err  error
		)

		//bodyIndex = FrameHeaderLen + DataHeaderLen
		//if bodyLen != len(t.Msg[bodyIndex:]) {
		//	return fmt.Errorf("wrong body length in data header: %d != %d", bodyLen, len(t.Msg[bodyIndex:]))
		//}
		//		body = t.Msg[bodyIndex : bodyIndex+bodyLen]
		sess = Byte2Sess(t.Msg[FrameHeaderLen : FrameHeaderLen+kSidLen])
		err = sess.VerifySession(t.Peer.String())
		if err != nil {
			return err
		}

		err = sess.VerifyPack(packNum)
		if err != nil {
			return fmt.Errorf("[UDPCOMET:03] verify session error: %v", err)
		}

		toId := int64(binary.LittleEndian.Uint64(t.Msg[8:16]))
		srcId := int64(binary.LittleEndian.Uint64(t.Msg[16:24]))

		// check binded ids
		destIds := sess.CalcDestIds(toId)

		glog.Infof("[UDPCOMET:03] %d -> %d udp, calc to: %v, data: (len: %d)%v", srcId, toId, destIds, len(t.Msg), t.Msg)
		if len(destIds) > 0 {
			GMsgBusManager.Push2Msgbus(srcId, destIds, t.Msg)
		} else {
			glog.Infof("[UDPCOMET:03] from [%v] to [%v] | destination is empty", srcId, toId)
		}
	}
	return nil
}

func (h *Handler) onGetToken(t *UdpMsg, body []byte) ([]byte, error) {
	output := make([]byte, 20)
	if len(body) != 48 {
		return output, fmt.Errorf("[udp:onGetToken] bad body length %d", len(body))
	}

	mac := make([]byte, 16)
	copy(mac, body[:16])
	key := make([]byte, 32)
	copy(key, body[16:48])

	// TODO verify mac and sn by real production's information
	// if err := VerifyDeviceInfo(mac, sn); err != nil {
	//	return nil, fmt.Errorf("invalid device error: %v", err)
	// }

	s := NewUdpSession(t.Peer)
	sid := Sess2Byte(s)
	gUdpSessions.SaveSessionByAdr(s)
	binary.LittleEndian.PutUint32(output[:4], 0) //返回结果
	copy(output[4:12], sid)                      //把sid传给板子

	return output, nil
}

func (h *Handler) onRegister(t *UdpMsg, sess *UdpSession, body []byte) ([]byte, error) {
	if len(body) < 310 {
		return nil, fmt.Errorf("[udp:onRegister] %v bad body length %d", sess.Sid, len(body))
	}

	dv := body[0:4]
	mac := hex.EncodeToString(body[4:12])
	sn := body[12:20]
	doUnbind := int(body[20])
	signature := body[21:uint32(body[21])]

	t.Input["mac"] = mac
	t.Input["dv"] = fmt.Sprintf("%d", binary.LittleEndian.Uint16(dv))
	t.Input["sn"] = fmt.Sprintf("%x", sn)
	if doUnbind == 1 {
		t.Input["doUnbind"] = "y"
	} else {
		t.Input["doUnbind"] = "n"
	}
	t.Input["sign"] = string(signature)

	output := make([]byte, 92)
	httpStatus, rep, err := t.DoHTTPTask()
	if glog.V(3) {
		glog.Infof("[udp:onRegister] %v httpstatus:%v,rep:%v,%v", sess.Sid, httpStatus, rep, err)
	}
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
			cki, _ := hex.DecodeString(cookie)
			copy(output[4:36], cki)
		}
	}
	return output, nil
}

func (h *Handler) onLogin(t *UdpMsg, sess *UdpSession, body []byte) ([]byte, error) {
	isFirstLogin := false
	if sess.DevId == 0 {
		isFirstLogin = true
	}

	output := make([]byte, 21)
	if len(body) != 88 {
		return output, fmt.Errorf("[udp:onLogin] %v bad body length %v", sess.Sid, len(body))
	}

	mac := hex.EncodeToString(body[0:8])

	cookie := string(bytes.TrimRight(body[8:72], "\x00"))
	t.Input["mac"] = mac
	t.Input["cookie"] = cookie

	httpStatus, rep, err := t.DoHTTPTask()
	if glog.V(3) {
		glog.Infof("[udp:onLogin] %v httpstatus:%v,rep:%v,%v", sess.Sid, httpStatus, rep, err)
	}
	if err != nil {
		binary.LittleEndian.PutUint32(output[0:4], uint32(httpStatus))
		return output, err
	}
	devAdr := ""
	var id int64
	var status int32 = DAckHTTPError
	if s, ok := rep["status"]; ok {
		if n, ok := s.(float64); ok {
			status = int32(n)

			if n == 0 {
				ss := strings.SplitN(cookie, "|", 2)

				if len(ss) > 0 {
					id, err := strconv.ParseInt(ss[0], 10, 64)
					if err == nil {
						if isFirstLogin || sess.DevId == id {
							sess.Mac = body[0:8]
							sess.DevId = id
							go PubDevOnlineChannel(id)
							time.Sleep(1 * time.Second)
							devAdr = GotDevAddr(id)
						} else {
							sess.Devs[id] = body[16:24]
							gUdpSessions.Sesses[id] = sess
						}
					}
				}
				//如果是第一次登录说明是主设备，如果登录的id等于sess中的id说明是主设备,是主设备就把设备拥有者返回
				if isFirstLogin || sess.DevId == id {
					bindedIds, owner, err := GetUsersByDev(sess.DevId)
					if err == nil {
						sess.Owner = owner
						binary.LittleEndian.PutUint64(output[4:12], uint64(sess.Owner))
						if len(bindedIds) > 0 {
							sess.Users = bindedIds
						}
					}
					go func() {
						go SetUserOnline(sess.DevId, fmt.Sprintf("%v-%v", gLocalAddr, gCometType))
						/**向用户推此设备在线消息*/
						if devAdr == "" {
							go PushDevOnlineMsgToUsers(sess)
						} else {
							go UpdateDevAdr(sess)
						}
						glog.Infoln("")
						sess.OfflineEvent = time.AfterFunc(time.Duration(gUdpTimeout)*time.Second, func() {
							glog.Infoln("")
							go PushDevOfflineMsgToUsers(sess)
							glog.Infoln("")
							gUdpSessions.Devlk.Lock()
							delete(gUdpSessions.Sesses, sess.DevId)
							for k, _ := range sess.Devs {
								delete(gUdpSessions.Sesses, k)
							}
							gUdpSessions.Devlk.Unlock()
							glog.Infoln("")
							//					SetUserOffline(sess.DevId, h.Server.con.LocalAddr().String())
							go SetUserOffline(sess.DevId, fmt.Sprintf("%v-%v", gLocalAddr, gCometType))
							glog.Infoln("")
							if glog.V(3) {
								glog.Infof("[udp:onLogin] %v %v loginout success.", sess.Sid, sess.DevId)
							}
						})

						gUdpSessions.Devlk.Lock()
						gUdpSessions.Sesses[sess.DevId] = sess
						delete(gUdpSessions.Sesses, int64(sess.Sid))
						gUdpSessions.Devlk.Unlock()

						glog.Infoln("")
						if glog.V(3) {
							glog.Infoln("")
							glog.Infof("[udp:onLogin] %v %v loginout success.", sess.Sid, sess.DevId)
							glog.Infoln("")
						}
					}()
				}
			}
		}
	}
	glog.Infoln("")
	binary.LittleEndian.PutUint32(output[0:4], uint32(status))
	glog.Infoln("")
	output[20] = kHeartBeatSec
	if glog.V(3) {
		defer glog.Infof("[udp:onLogin] SUCCESS LOGIN %v,%v,%v", sess.Sid, sess.DevId, sess.Addr.String())
	}
	return output, nil
}

func (h *Handler) onHearBeat(t *UdpMsg, sess *UdpSession, body []byte) ([]byte, error) {
	if glog.V(3) {
		glog.Infof("onHearBeat:%v,%v,%v", sess.Sid, sess.DevId, sess.Addr.String())
	}
	sess.Update(t.Peer)
	output := make([]byte, 12)
	binary.LittleEndian.PutUint32(output[:4], 0)
	copy(output[4:12], sess.Mac)
	sess.OfflineEvent.Reset(time.Duration(gUdpTimeout) * time.Second)
	return output, nil
}
func (h *Handler) onLoginout(t *UdpMsg, sess *UdpSession, body []byte) ([]byte, error) {
	if glog.V(3) {
		glog.Infof("onHearBeat:%v,%v,%v", sess.Sid, sess.DevId, sess.Addr.String())
	}
	output := make([]byte, 12)
	binary.LittleEndian.PutUint32(output[:4], 0)
	copy(output[4:12], sess.Mac)
	sess.OfflineEvent.Reset(0 * time.Second)
	return output, nil
}
