package main

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/golang/glog"
	uuid "github.com/nu7hatch/gouuid"
)

type Handler struct {
	Server     *Server
	listenAddr string

	kApiUrls    map[uint16]string
	workerCount int
	msgQueue    chan *Task
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
		msgQueue:    make(chan *Task, 1024*128),
	}
	return h
}

func (h *Handler) Go() {
	for i := 0; i < h.workerCount; i++ {
		go h.processer()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/binding", h.OnBinding)
	go func() {
		if e := http.ListenAndServe(h.listenAddr, mux); e != nil {
			glog.Errorf("[handler|server] ListenAndServe error: %v", e)
		}
	}()
}

func (h *Handler) OnBinding(w http.ResponseWriter, r *http.Request) {
	// TODO accept request from api server
}

func (h *Handler) Process(peer *net.UDPAddr, msg []byte) {
	msgCopy := make([]byte, len(msg))
	copy(msgCopy, msg)

	t := &Task{
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

// TODO
func checkHeader(data []byte) error {
	return nil
}

// TODO
func checkData(data []byte) error {
	return nil
}

// TODO
func computeCheck(data []byte) error {
	return nil
}

func (h *Handler) handle(t *Task) error {
	// TODO decrypt
	mlen := len(t.Msg)
	if mlen < 24 {
		return fmt.Errorf("[protocol] invalid message length for device proxy")
	}
	// check opcode
	op := (0x7 & t.Msg[0])
	if op != 0x2 && op != 0x3 {
		return fmt.Errorf("[protocol] invalid message protocol")
	}

	// check xor
	if mlen < 24+24+12 {
		return fmt.Errorf("[protocol] invalid message length for protocol")
	}
	// discard msg if found checking error
	err := checkHeader(t.Msg)
	if err != nil {
		return err
	}
	err = checkData(t.Msg)
	if err != nil {
		return err
	}
	if op == 0x2 {
		// parse data(udp)
		// 51 = 24 + 24 + 3
		c := binary.LittleEndian.Uint16(t.Msg[51:53])

		// 60 = 24 + 24 + 12
		sidIndex := 60
		sid, err := uuid.Parse(t.Msg[sidIndex : sidIndex+16])
		if err != nil {
			return fmt.Errorf("parse session id error: %v", err)
		}
		// 76 = 60 + 16
		bodyIndex := sidIndex + 16
		body := t.Msg[bodyIndex:]

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
			res, err = h.onRegister(t, sid, body)

		case CmdLogin:
			t.CmdType = c
			t.Url = h.kApiUrls[c]
			res, err = h.onLogin(t, sid, body)

		case CmdRename:
			t.CmdType = c
			t.Url = h.kApiUrls[c]
			res, err = h.onRename(t, sid, body)

		case CmdDoBind:
			t.CmdType = c
			t.Url = h.kApiUrls[c]
			res, err = h.onDoBind(t, sid, body)

		case CmdHeartBeat:
			t.CmdType = c
			res, err = h.onHearBeat(t, sid, body)

		case CmdSubDeviceOffline:
			t.CmdType = c
			res, err = h.onSubDeviceOffline(t, sid, body)

		default:
			glog.Warningf("invalid command type %v", c)
			b := make([]byte, 4)
			binary.LittleEndian.PutUint32(b, uint32(DAckBadCmd))
			output = append(output, b...)

			err = computeCheck(output)
			if err == nil {
				h.Server.Send(t.Peer, output)
			} else {
				if glog.V(1) {
					glog.Warningf("[handle] check message failed: %v", err)
				}
			}
			return nil
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
		err = computeCheck(output)
		if err == nil {
			h.Server.Send(t.Peer, output)
		} else {
			if glog.V(1) {
				glog.Warningf("[handle] check message failed: %v", err)
			}
		}
		h.Server.Send(t.Peer, output)

	} else {

		// TODO transfer message to dest id
		// maybe it needn't check pack number, because it belongs to dest mobile?

		return fmt.Errorf("[protocol] NOT IMPLEMENTED for transfer messages")
	}
	return nil

	// ???
	//inputs := strings.Split(strings.TrimSpace(string(t.Msg[1:])), "&")
	//for _, in := range inputs {
	//	args := strings.SplitN(in, "=", 2)
	//	if len(args) == 0 {
	//		return fmt.Errorf("empty argument [%v]", in)
	//	}
	//	if len(args[0]) == 0 {
	//		return fmt.Errorf("key in form cannot be empty [%v]", in)
	//	}
	//	if len(args) > 1 {
	//		t.Input[args[0]] = args[1]
	//	} else {
	//		t.Input[args[0]] = ""
	//	}
	//}
}

// TODO check pack number and other things in session here
func (h *Handler) GetAndVeryifySession(body []byte) (*uuid.UUID, error) {
	if len(body) != 16 {
		return nil, nil, fmt.Errorf("not enough bytes for session id")
	}
	sid, _ := uuid.Parse(body[0:16])
	if !gSessionList.IsExisting(sid) {
		return nil, fmt.Errorf("invalid session id %s", sid)
	}
	return sid, nil
}

func (h *Handler) onGetToken(t *Task, body []byte) ([]byte, error) {
	if len(body) != 24 {
		return nil, fmt.Errorf("onGetToken: bad body length %d", len(body))
	}

	mac := make([]byte, 8)
	copy(mac, body[:8])
	sn := make([]byte, 16)
	copy(sn, body[8:24])

	// TODO verify mac and sn by real production's information
	// if err := VerifyDevice(mac, sn); err != nil {
	//	return nil, fmt.Errorf("invalid device error: %v", err)
	// }

	sid := NewUuid()
	s := NewUdpSession(NewUdpConnection(h.Server.socket, t.Peer), mac, sn, t.Peer.String())
	err = gSessionList.AddUdpSession(sid, s)
	if err != nil {
		glog.Fatalf("[session] AddSession failed: %v", err)
	}
	output := make([]byte, 20)
	binary.LittleEndian.PutUint32(output[:4], 0)
	binary.LittleEndian.PutUint32(output[4:20], sid[:16])

	// TODO we don't need save session into redis now
	//sd, err := json.Marshal(s)
	//if err != nil {
	//	glog.Errorf("Marshal session into json failed: %v", err)
	//	return nil, err
	//}
	//err = SetDeviceSession(sid, sd)

	return output, nil
}

func (h *Handler) onRegister(t *Task, sid *uuid.UUID, body []byte) ([]byte, error) {
	if len(body) < 31 {
		return nil, fmt.Errorf("onRegister: bad body length %d", len(body))
	}
	_, err := h.GetAndVeryifySession(body[:16])
	if err != nil {
		return nil, fmt.Errorf("[session] verify session error: %v", err)
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

	//if err := VerifyToken(sn, token); err != nil {
	//	glog.Errorf("VerifyToken for sn %v and token %v failed: %v", sn, token, err)
	//	return nil, err
	//}
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
			glog.Infof("REGISTER: mac: %s, cookie: %v", mac, cookie)
			copy(output[12:76], []byte(cookie))
			// TODO 64 bytes?
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

func (h *Handler) onLogin(t *Task, sid *uuid.UUID, body []byte) ([]byte, error) {
	if len(body) != 72 {
		return nil, fmt.Errorf("onLogin: bad body length %v", len(body))
	}
	_, err := h.GetAndVeryifySession(body[:16])
	if err != nil {
		return nil, fmt.Errorf("[session] verify session error: %v", err)
	}

	mac := base64.StdEncoding.EncodeToString(body[0:8])
	cookie := body[8:72]
	t.Input["cookie"] = string(cookie)
	t.Input["mac"] = mac
	//glog.Infof("LOGIN: mac: %s, cookie: %v", mac, string(cookie))

	output := make([]byte, 12)
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
	copy(output[4:20], body[:16])
	//if 0 == status {
	//	sid := NewUuid()
	//	s := NewUdpSession(NewUdpConnection(h.Server.socket, t.Peer))
	//	err = gSessionList.AddUdpSession(sid, s)
	//	if err != nil {
	//		glog.Fatalf("[session] AddSession failed: %v", err)
	//	}
	//	s.Save()
	//}
	return output, nil
}

func (h *Handler) onRename(t *Task, sid *uuid.UUID, body []byte) ([]byte, error) {
	_, err := h.GetAndVeryifySession(body[0:16])
	if err != nil {
		return nil, fmt.Errorf("[session] verify session error: %v", err)
	}

	mac := base64.StdEncoding.EncodeToString(body[16:24])
	nameLen := body[24]
	name := body[25 : 25+nameLen]

	t.Input["name"] = string(name)
	t.Input["mac"] = mac

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
	if status != 0 {
		return output, nil
	}
	return output, nil
}

func (h *Handler) onDoBind(t *Task, sid *uuid.UUID, body []byte) ([]byte, error) {
	_, err := h.GetAndVeryifySession(body[0:16])
	if err != nil {
		return nil, fmt.Errorf("[session] verify session error: %v", err)
	}

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

func (h *Handler) onHearBeat(t *Task, sid *uuid.UUID, body []byte) ([]byte, error) {
	sid, err := h.GetAndVeryifySession(body[0:16])
	if err != nil {
		return nil, fmt.Errorf("[session] verify session error: %v", err)
	}

	id := int64(binary.LittleEndian.Uint64(body[16:24]))

	var uid uuid.UUID
	copy(uid[:], sid)
	err := gSessionList.UpdateSession(&uid, id, t.Peer)

	output := make([]byte, 20)
	copy(output[0:16], sid)
	if err != nil {
		binary.LittleEndian.PutUint32(output[16:20], 1)
	} else {
		binary.LittleEndian.PutUint32(output[16:20], 0)
	}

	return output, err
}

func (h *Handler) onSubDeviceOffline(t *Task, sid *uuid.UUID, body []byte) ([]byte, error) {
	_, err := h.GetAndVeryifySession(body[0:16])
	if err != nil {
		return nil, fmt.Errorf("[session] verify session error: %v", err)
	}

	//id := int64(binary.LittleEndian.Uint64(body[16:24]))

	// TODO broadcast offline message

	output := make([]byte, 24)
	copy(output, body)
	//if err != nil {
	//	binary.LittleEndian.PutUint32(output[16:20], 1)
	//} else {
	//	binary.LittleEndian.PutUint32(output[16:20], 0)
	//}
	return output, nil
}
