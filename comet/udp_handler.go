package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
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
	DataHeaderLen  = 16

	kHeartBeat = 20 * time.Second

	kApiGetUAddr = "/api/device/udpaddr"
)

var (
	//ErrSessTimeout = fmt.Errorf("session timeout")
	ErrSessPackSeq = fmt.Errorf("wrong package sequence number")

	udpUrlPort atomic.AtomicInt32
)

func GetCometUdpUrl() string {
	return fmt.Sprintf("http://%s:%d%s", gLocalAddr, udpUrlPort.Get(), kApiGetUAddr)
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
	urls[CmdDoBind] = apiServerUrl + UrlBind
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
// TODO 暂未实现
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
	addr, err := GetDeviceAddr(id)
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

func (h *Handler) Process(peer *net.UDPAddr, msg []byte) {
	msgCopy := make([]byte, len(msg))
	copy(msgCopy, msg)

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
		if glog.V(1) {
			glog.Errorf("[handler] handle msg (len[%d] %v) error: %v", len(t.Msg), t.Msg, err)
		} else {
			glog.Errorf("[handler] handle msg (len[%d] %v) error: %v", len(t.Msg), t.Msg[:5], err)
		}
	}
}

func (h *Handler) handle(t *UdpMsg) error {
	// TODO need decryption later

	mlen := len(t.Msg)
	if mlen < FrameHeaderLen {
		return fmt.Errorf("[protocol] invalid message length for device proxy")
	}
	// check opcode
	op := (0x7 & t.Msg[0])
	if op != 0x2 && op != 0x3 {
		return fmt.Errorf("[protocol] invalid message protocol")
	}

	if op == 0x2 {
		if mlen < FrameHeaderLen+DataHeaderLen {
			return fmt.Errorf("[protocol] invalid message length for protocol")
		}
		packNum := binary.LittleEndian.Uint16(t.Msg[2:4])
		bodyLen := int(binary.LittleEndian.Uint16(t.Msg[FrameHeaderLen+6:]))
		// discard msg if found checking error
		if t.Msg[FrameHeaderLen+8] != msgs.ChecksumHeader(t.Msg, FrameHeaderLen+8) {
			return fmt.Errorf("checksum header error")
		}

		// parse data(udp)
		// 28 = FrameHeaderLen + 4
		c := binary.LittleEndian.Uint16(t.Msg[28:30])

		// 34 = FrameHeaderLen + 10
		sidIndex := 34
		bodyIndex := sidIndex + 16
		if bodyLen != len(t.Msg[bodyIndex:]) {
			return fmt.Errorf("wrong body length in data header: %d != %d", bodyLen, len(t.Msg[bodyIndex:]))
		}
		body := t.Msg[bodyIndex : bodyIndex+bodyLen]

		// check data body
		if t.Msg[FrameHeaderLen+9] != msgs.ChecksumHeader(t.Msg[sidIndex:], 16+bodyLen) {
			return fmt.Errorf("checksum data error")
		}

		var (
			sess   *UdpSession
			sid    *uuid.UUID
			err    error
			locker Locker
		)

		if c != CmdGetToken {
			sid, err = uuid.Parse(t.Msg[sidIndex : sidIndex+16])
			if err != nil {
				return fmt.Errorf("parse session id error: %v", err)
			}

			locker = NewDeviceSessionLocker(sid.String())
			err = locker.Lock()
			if err != nil {
				return fmt.Errorf("lock session id [%s] failed: %v", sid, err)
			}
			sess, err = gUdpSessions.GetSession(sid)
			if err != nil {
				locker.Unlock()
				return fmt.Errorf("cmd: %X, sid: [%v], error: %v", c, sid, err)
			}
			err = h.VerifySession(sess, packNum)
			if err != nil {
				//if err == ErrSessTimeout {
				//	gUdpSessions.DeleteSession(sid)
				//}
				locker.Unlock()
				return fmt.Errorf("cmd: %X, verify session error: %v", c, err)
			}
		}

		output := make([]byte, bodyIndex, 128)
		// copy same packNum into this ACK response
		copy(output[:bodyIndex], t.Msg[:bodyIndex])

		var res []byte
		switch c {
		case CmdGetToken:
			t.CmdType = c
			res, err = h.onGetToken(t, body)

		case CmdRegister:
			t.CmdType = c
			t.Url = h.kApiUrls[c]
			res, err = h.onRegister(t, sess, body)

		case CmdLogin:
			t.CmdType = c
			t.Url = h.kApiUrls[c]
			res, err = h.onLogin(t, sess, body)

		case CmdRename:
			t.CmdType = c
			t.Url = h.kApiUrls[c]
			res, err = h.onRename(t, sess, body)

		case CmdDoBind:
			t.CmdType = c
			t.Url = h.kApiUrls[c]
			res, err = h.onDoBind(t, sess, body)

		case CmdHeartBeat:
			t.CmdType = c
			res, err = h.onHearBeat(t, sess, body)

		case CmdSubDeviceOffline:
			t.CmdType = c
			res, err = h.onSubDeviceOffline(t, sess, body)

		default:
			glog.Warningf("invalid command type %v", c)
			if sess != nil {
				locker.Unlock()
			}
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
			return nil
		}
		if sess != nil {
			gUdpSessions.SaveSession(sid, sess)
			locker.Unlock()
		}
		if err != nil {
			if glog.V(1) {
				glog.Errorf("[handle] cmd: %X, error: %v", c, err)
			}
		}
		if res != nil {
			output = append(output, res...)
		}

		if glog.V(2) {
			glog.Infof("UDP SEND: %d, %v", len(output), output)
		}
		output[FrameHeaderLen+8] = msgs.ChecksumHeader(output, FrameHeaderLen+8)
		output[FrameHeaderLen] |= (msgs.FlagAck | msgs.FlagRead)
		h.Server.Send(t.Peer, output)

	} else {
		if mlen < FrameHeaderLen+FrameHeaderLen+12 {
			return fmt.Errorf("[protocol] invalid message length for protocol")
		}
		// discard msg if found checking error
		if t.Msg[FrameHeaderLen+FrameHeaderLen+8] != msgs.ChecksumHeader(t.Msg, FrameHeaderLen+FrameHeaderLen+8) {
			return fmt.Errorf("checksum header error")
		}
		// TODO check data body

		// TODO transfer message to dest id
		// maybe it needn't check pack number, because it belongs to dest mobile?

		return fmt.Errorf("[protocol] NOT IMPLEMENTED for transfer messages")
	}
	return nil
}

// check pack number and other things in session here
func (h *Handler) VerifySession(s *UdpSession, packNum uint16) error {
	// 现在由redis负责超时
	//if time.Now().Sub(s.LastHeartbeat) > 2*kHeartBeat {
	//	return ErrSessTimeout
	//}
	// pack number
	switch {
	case packNum > s.Ridx:
	case packNum < s.Ridx && packNum < 10 && s.Ridx > uint16(65530):
	default:
		return ErrSessPackSeq
	}

	// TODO cmd number

	// all ok
	s.Ridx = packNum

	return nil
}

func (h *Handler) onGetToken(t *UdpMsg, body []byte) ([]byte, error) {
	if len(body) != 24 {
		return nil, fmt.Errorf("[onGetToken] bad body length %d", len(body))
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
	locker := NewDeviceSessionLocker(sid.String())
	err = locker.Lock()
	if err != nil {
		e := fmt.Errorf("Lock redis failed on new session id [%s], error: %v", sid, err)
		glog.Error(e)
		return nil, e
	}
	defer locker.Unlock()

	s := NewUdpSession(t.Peer)
	err = gUdpSessions.SaveSession(sid, s)
	if err != nil {
		glog.Fatalf("[onGetToken] SaveSession failed: %v", err)
	}
	output := make([]byte, 20)
	binary.LittleEndian.PutUint32(output[:4], 0)
	copy(output[4:20], sid[:16])

	return output, nil
}

func (h *Handler) onRegister(t *UdpMsg, sess *UdpSession, body []byte) ([]byte, error) {
	if len(body) < 31 {
		return nil, fmt.Errorf("[onRegister] bad body length %d", len(body))
	}

	dv := body[0:2]
	mac := base64.StdEncoding.EncodeToString(body[2:10])
	produceTime := binary.LittleEndian.Uint32(body[10:14])
	sn := body[14:30]
	nameLen := body[30]
	var name []byte
	if nameLen > 0 {
		name = body[31 : 31+nameLen]
	}

	t.Input["mac"] = mac
	t.Input["dv"] = fmt.Sprintf("%x", dv)
	t.Input["pt"] = fmt.Sprintf("%d", produceTime)

	t.Input["sn"] = fmt.Sprintf("%x", sn)
	t.Input["name"] = fmt.Sprintf("%x", name)

	output := make([]byte, 76)
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
			copy(output[12:76], []byte(cookie))
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
			binary.LittleEndian.PutUint64(output[4:12], uint64(id))
			sess.DeviceId = id
		}
	}
	return output, nil
}

func (h *Handler) onLogin(t *UdpMsg, sess *UdpSession, body []byte) ([]byte, error) {
	if len(body) != 72 {
		return nil, fmt.Errorf("[onLogin] bad body length %v", len(body))
	}

	mac := base64.StdEncoding.EncodeToString(body[0:8])
	// C program sent cookie string without trim zero bytes
	cookie := string(bytes.TrimRight(body[8:72], "\x00"))
	t.Input["mac"] = mac
	t.Input["cookie"] = cookie
	//fmt.Println(mac, len(cookie), cookie)
	//glog.Infof("LOGIN: mac: %s, cookie: %v", mac, string(cookie))

	output := make([]byte, 4)
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
	return output, nil
}

func (h *Handler) onRename(t *UdpMsg, sess *UdpSession, body []byte) ([]byte, error) {
	mac := base64.StdEncoding.EncodeToString(body[:8])
	nameLen := body[8]
	name := body[9 : 9+nameLen]

	t.Input["name"] = string(name)
	t.Input["mac"] = mac

	output := make([]byte, 12)
	copy(output[4:12], body[:8])

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

// TODO 业务流程未定义
func (h *Handler) onDoBind(t *UdpMsg, sess *UdpSession, body []byte) ([]byte, error) {
	uid := body[16:24]
	result := binary.LittleEndian.Uint32(body[24:28])

	t.Input["uid"] = fmt.Sprintf("%d", uid)
	t.Input["result"] = fmt.Sprintf("%d", result)

	output := make([]byte, 4)

	httpStatus, rep, err := t.DoHTTPTask()
	if err != nil {
		binary.LittleEndian.PutUint32(output[0:4], uint32(httpStatus))
		return output, err
	}

	// TODO status not in protocol
	if s, ok := rep["status"]; ok {
		if status, ok := s.(float64); ok {
			binary.LittleEndian.PutUint32(output[0:4], uint32(int32(status)))
			if status != 0 {
				return output, nil
			}
		}
	}
	return output, nil
}

func (h *Handler) onHearBeat(t *UdpMsg, sess *UdpSession, body []byte) ([]byte, error) {
	err := sess.Update(t.Peer)

	output := make([]byte, 4)
	if err != nil {
		binary.LittleEndian.PutUint32(output[:4], 1)
	} else {
		binary.LittleEndian.PutUint32(output[:4], 0)
	}

	return output, err
}

// TODO 下线消息的业务逻辑还未详细定义
func (h *Handler) onSubDeviceOffline(t *UdpMsg, sess *UdpSession, body []byte) ([]byte, error) {
	return nil, fmt.Errorf("[onSubDeviceOffline]NOT IMPLEMENTED API: onSubDeviceOffline")
}
