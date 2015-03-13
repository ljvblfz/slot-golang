package main

import (
	"encoding/base64"
	"encoding/binary"
	//"encoding/json"
	"bytes"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cloud-socket/msgs"
	"github.com/golang/glog"
	uuid "github.com/nu7hatch/gouuid"
)

const (
	FrameHeaderLen = 24
	DataHeaderLen  = 16

	kHeartBeat = 20 * time.Second
)

var (
	ErrSessTimeout = fmt.Errorf("session timeout")
)

type Handler struct {
	Server     *UdpServer
	listenAddr string

	kApiUrls    map[uint16]string
	workerCount int
	msgQueue    chan *UdpMsg
}

func NewHandler(workerCount int, apiServerUrl string, listenAddr string) *Handler {
	urls := make(map[uint16]string)
	urls[CmdRegister] = apiServerUrl + UrlRegister
	urls[CmdLogin] = apiServerUrl + UrlLogin
	urls[CmdDoBind] = apiServerUrl + UrlBind
	urls[CmdRename] = apiServerUrl + UrlChangeName

	h := &Handler{
		listenAddr:  listenAddr,
		kApiUrls:    urls,
		workerCount: workerCount,
		msgQueue:    make(chan *UdpMsg, 1024*128),
	}
	return h
}

func (h *Handler) Go() {
	for i := 0; i < h.workerCount; i++ {
		go h.processer()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/binding", h.OnBindingRequest)
	go func() {
		if e := http.ListenAndServe(h.listenAddr, mux); e != nil {
			glog.Errorf("[handler|server] ListenAndServe error: %v", e)
		}
	}()
}

// accept request from api server
func (h *Handler) OnBindingRequest(w http.ResponseWriter, r *http.Request) {
	var (
		deviceId  int64
		userId    int64
		deviceMac []byte

		seqNum    uint16
		cmdSeqNum uint8
	)
	// TODO get seqNum and cmdSeqNum

	// make a message
	req := make([]byte, FrameHeaderLen+DataHeaderLen+16)
	// frame header
	frame := req[:FrameHeaderLen]
	frame[0] = 1<<7 | 0x2
	binary.LittleEndian.PutUint16(frame[2:4], seqNum)
	binary.LittleEndian.PutUint32(frame[4:8], uint32(time.Now().Unix()))
	binary.LittleEndian.PutUint64(frame[8:16], uint64(deviceId))
	// data header
	header := req[FrameHeaderLen : FrameHeaderLen+DataHeaderLen]
	header[1] = cmdSeqNum
	binary.LittleEndian.PutUint16(header[4:6], 0xE4)
	header[6] = msgs.ChecksumHeader(req, FrameHeaderLen+8)
	// data body
	copy(req[FrameHeaderLen+DataHeaderLen:], deviceMac)
	binary.LittleEndian.PutUint64(req[FrameHeaderLen+DataHeaderLen+8:], uint64(userId))

	//header[7] = msgs.ChecksumBody(req[FrameHeaderLen+DataHeaderLen:], 16)

	// send message

	err := gSessionList.PushUdpMsg(deviceId, req)
	if err != nil {
		w.WriteHeader(200)
		// timeout
		w.Write([]byte("{\"status\":2}"))
		return
	}

	c := make(chan error)
	err = AppendBindingRequest(c, deviceId, userId)
	if err != nil {
		w.WriteHeader(200)
		w.Write([]byte("{\"status\":3}"))
		return
	}
	// wait for async response on a chan
	select {
	case err := <-c:
		if err != nil {
			// if write response into HTTP
			w.WriteHeader(200)
			// parse
			w.Write([]byte("{\"status\":4}"))
		} else {
			// if write response into HTTP
			w.WriteHeader(200)
			// parse
			w.Write([]byte("{\"status\":0}"))
		}
	case <-time.After(2 * time.Minute):
		w.WriteHeader(200)
		// timeout
		w.Write([]byte("{\"status\":1}"))
	}
}

// 获取指定设备的外网UDP地址
// TODO 暂未实现
func (h *Handler) OnBackendGetDeviceAddr(w http.ResponseWriter, r *http.Request) {
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

	h.msgQueue <- t
}

func (h *Handler) processer() {
	for t := range h.msgQueue {
		err := h.handle(t)
		if err != nil {
			if glog.V(1) {
				glog.Errorf("[handler] handle msg (len[%d] %v) error: %v", len(t.Msg), t.Msg, err)
			} else {
				glog.Errorf("[handler] handle msg (len[%d] %v) error: %v", len(t.Msg), t.Msg[:5], err)
			}
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
		bodyLen := int(binary.LittleEndian.Uint16(t.Msg[FrameHeaderLen+6:]))
		// discard msg if found checking error
		if t.Msg[FrameHeaderLen+8] != msgs.ChecksumHeader(t.Msg, FrameHeaderLen+8) {
			return fmt.Errorf("checksum header error")
		}
		// TODO check data body, need check algorithm

		// parse data(udp)
		// 28 = FrameHeaderLen + 4
		c := binary.LittleEndian.Uint16(t.Msg[28:30])

		var (
			sess *UdpSession
			sid  *uuid.UUID
			err  error
		)
		// 34 = FrameHeaderLen + 10
		sidIndex := 34
		if c != CmdGetToken {
			sid, err = uuid.Parse(t.Msg[sidIndex : sidIndex+16])
			if err != nil {
				return fmt.Errorf("parse session id error: %v", err)
			}

			sess, err = gSessionList.GetUdpSession(sid)
			if err != nil {
				return fmt.Errorf("cmd: %X, sid: [%v], error: %v", c, sid, err)
			}
			err = h.VerifySession(sess)
			if err != nil {
				if err == ErrSessTimeout {
					gSessionList.RemoveUdpSession(sid)
				}
				gSessionList.ReleaseUdpSession(sess)
				return fmt.Errorf("cmd: %X, verify session error: %v", c, err)
			}
		}
		bodyIndex := sidIndex + 16
		body := t.Msg[bodyIndex : bodyIndex+bodyLen]

		output := make([]byte, bodyIndex, 128)
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
				gSessionList.ReleaseUdpSession(sess)
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
			gSessionList.ReleaseUdpSession(sess)
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
func (h *Handler) VerifySession(s *UdpSession) error {
	if time.Now().Sub(s.LastHeartbeat) > 2*kHeartBeat {
		return ErrSessTimeout
	}
	// TODO pack number
	// TODO cmd number
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
	s := NewUdpSession(NewUdpConnection(h.Server.socket, t.Peer), mac, sn, t.Peer.String())
	err = gSessionList.AddUdpSession(sid, s)
	if err != nil {
		glog.Fatalf("[onGetToken] AddSession failed: %v", err)
	}
	output := make([]byte, 20)
	binary.LittleEndian.PutUint32(output[:4], 0)
	copy(output[4:20], sid[:16])

	// TODO we don't need save session into redis now
	//sd, err := json.Marshal(s)
	//if err != nil {
	//	glog.Errorf("Marshal session into json failed: %v", err)
	//	return nil, err
	//}
	//err = SetDeviceSession(sid, sd)

	return output, nil
}

func (h *Handler) onRegister(t *UdpMsg, sess *UdpSession, body []byte) ([]byte, error) {
	if len(body) < 31 {
		return nil, fmt.Errorf("[onRegister] bad body length %d", len(body))
	}
	//sess, err := gSessionList.GetUdpSession(sid)
	//if err != nil {
	//	return nil, fmt.Errorf("[onRegister] sid: [%v], error: %v", sid, err)
	//}
	//defer gSessionList.ReleaseUdpSession(sess)
	//err = h.VerifySession(sess)
	//if err != nil {
	//	if err == ErrSessTimeout {
	//		gSessionList.RemoveUdpSession(sid)
	//	}
	//	return nil, fmt.Errorf("[onRegister] verify session error: %v", err)
	//}

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
		}
	}
	return output, nil
}

func (h *Handler) onLogin(t *UdpMsg, sess *UdpSession, body []byte) ([]byte, error) {
	if len(body) != 72 {
		return nil, fmt.Errorf("[onLogin] bad body length %v", len(body))
	}
	//sess, err := gSessionList.GetUdpSession(sid)
	//if err != nil {
	//	return nil, fmt.Errorf("[onLogin] sid: [%v], error: %v", sid, err)
	//}
	//defer gSessionList.ReleaseUdpSession(sess)
	//err = h.VerifySession(sess)
	//if err != nil {
	//	if err == ErrSessTimeout {
	//		gSessionList.RemoveUdpSession(sid)
	//	}
	//	return nil, fmt.Errorf("[onLogin] verify session error: %v", err)
	//}

	mac := base64.StdEncoding.EncodeToString(body[0:8])
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
	//sess, err := gSessionList.GetUdpSession(sid)
	//if err != nil {
	//	return nil, fmt.Errorf("[onRename] sid: [%v], error: %v", sid, err)
	//}
	//defer gSessionList.ReleaseUdpSession(sess)
	//err = h.VerifySession(sess)
	//if err != nil {
	//	if err == ErrSessTimeout {
	//		gSessionList.RemoveUdpSession(sid)
	//	}
	//	return nil, fmt.Errorf("[onRename] verify session error: %v", err)
	//}

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
	//sess, err := gSessionList.GetUdpSession(sid)
	//if err != nil {
	//	return nil, fmt.Errorf("[onDoBind] sid: [%v], error: %v", sid, err)
	//}
	//defer gSessionList.ReleaseUdpSession(sess)
	//err = h.VerifySession(sess)
	//if err != nil {
	//	if err == ErrSessTimeout {
	//		gSessionList.RemoveUdpSession(sid)
	//	}
	//	return nil, fmt.Errorf("[onDoBind] verify session error: %v", err)
	//}

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
	//sess, err := gSessionList.GetUdpSession(sid)
	//if err != nil {
	//	return nil, fmt.Errorf("[onHearBeat] sid: [%v], error: %v", sid, err)
	//}
	//defer gSessionList.ReleaseUdpSession(sess)
	//err = h.VerifySession(sess)
	//if err != nil {
	//	if err == ErrSessTimeout {
	//		gSessionList.RemoveUdpSession(sid)
	//	}
	//	return nil, fmt.Errorf("[onHearBeat] verify session error: %v", err)
	//}

	//id := int64(binary.LittleEndian.Uint64(body[:8]))

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
	//err := h.VerifySession(sid)
	//if err != nil {
	//	if err == ErrSessTimeout {
	//		gSessionList.RemoveUdpSession(sid)
	//	}
	//	return nil, fmt.Errorf("[session] verify session error: %v", err)
	//}

	//mac := int64(binary.LittleEndian.Uint64(body[:8]))
	//id, dstIds, err := gSessionList.GetDeviceIdAndDstIds(sid)
	//if err == nil {
	//	// TODO
	//	//destIds := gSessionList.GetDeviceIdAndBinding(mac)
	//	offlineMsg := msgs.NewAppMsg(0, id, msgs.MIDOffline)
	//	GMsgBusManager.Push2Backend(id, destIds, offlineMsg.MarshalBytes())
	//}

	//output := make([]byte, 12)
	//copy(output[4:12], body[:8])
	//if err != nil {
	//	binary.LittleEndian.PutUint32(output[16:20], 1)
	//} else {
	//	binary.LittleEndian.PutUint32(output[16:20], 0)
	//}
	//return output, nil
}
