package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/golang/glog"
)

type Handler struct {
	Server *Server

	kApiUrls    map[uint16]string
	workerCount int
	msgQueue    chan *Task
}

func NewHandler(workerCount int, httpUrl string) *Handler {
	urls := make(map[uint16]string)
	urls[CmdRegister] = httpUrl + UrlRegister
	urls[CmdLogin] = httpUrl + UrlLogin
	urls[CmdBind] = httpUrl + UrlBind
	urls[CmdChangeName] = httpUrl + UrlChangeName

	h := &Handler{
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
		res, err := h.handle(t)
		if err != nil {
			if glog.V(1) {
				glog.Errorf("[handler] handle msg (len[%d] %v) error: %v", len(t.Msg), t.Msg, err)
			} else {
				glog.Errorf("[handler] handle msg (len[%d] %v) error: %v", len(t.Msg), t.Msg[:5], err)
			}
		}

		if len(res) == 0 {
			glog.Error("[handler] should not reponse with empty")
		} else {
			// TODO compute check in header
			// computeCheck(res)

			h.Server.Send(t.Peer, res)
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

func (h *Handler) handle(t *Task) ([]byte, error) {
	mlen := len(t.Msg)
	if mlen < 24 {
		return nil, fmt.Errorf("[protocol] invalid message length for device proxy")
	}
	// check opcode
	op := (0x7 & t.Msg[0])
	if op != 0x2 {
		return nil, fmt.Errorf("[protocol] invalid message protocol")
	}

	// check xor
	if mlen < 24+24+12 {
		return nil, fmt.Errorf("[protocol] invalid message length for protocol")
	}
	// discard msg if found checking error
	err := checkHeader(t.Msg)
	if err != nil {
		return nil, err
	}
	err = checkData(t.Msg)
	if err != nil {
		return nil, err
	}

	// parse data(udp)
	// 52 = 24 + 24 + 4
	c := binary.LittleEndian.Uint16(t.Msg[52:54])

	// 60 = 24 + 24 + 12
	bodyIndex := 60
	body := t.Msg[:bodyIndex]

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
		res, err = h.onRegister(t, body)

	case CmdLogin:
		t.CmdType = c
		t.Url = h.kApiUrls[c]
		res, err = h.onLogin(t, body)

	case CmdChangeName:
		t.CmdType = c
		t.Url = h.kApiUrls[c]
		res, err = h.onChangeName(t, body)

	case CmdBind:
		t.CmdType = c
		t.Url = h.kApiUrls[c]
		res, err = h.onBind(t, body)

	case CmdHeartBeat:
		t.CmdType = c
		res, err = h.onHearBeat(t, body)

	case CmdSubDeviceOffline:
		t.CmdType = c
		res, err = h.onSubDeviceOffline(t, body)

	default:
		glog.Warningf("invalid command type %v", t.Msg[0])
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, uint32(DAckBadCmd))
		output = append(output, b...)
		return output, nil
	}
	if err != nil {
		if glog.V(1) {
			glog.Errorf("[handle] cmd: %v, error: %v", c, err)
		}
	}
	if res != nil {
		output = append(output, res...)
	}
	return output, nil

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

func (h *Handler) onGetToken(t *Task, body []byte) ([]byte, error) {
	if len(body) != 16 {
		glog.Errorf("onGetToken: bad sn length")
		return nil, fmt.Errorf("bad sn length")
	}
	sn := body
	token, err := NewToken(sn)
	if err != nil {
		glog.Errorf("NewToken for sn %v error: %v", sn, err)
		return nil, err
	}
	return token, nil
}

func (h *Handler) onRegister(t *Task, body []byte) ([]byte, error) {
	token := body[0:16]
	dv := body[16:18]
	//createTime := body[18:22]
	sn := body[22:38]
	nameLen := body[38]
	name := body[39 : 39+nameLen]
	// TODO
	//mac :=

	if err := VerifyToken(sn, token); err != nil {
		glog.Errorf("VerifyToken for sn %v and token %v failed: %v", sn, token, err)
		return nil, err
	}
	//t.Input["mac"] = fmt.Sprintf("%x", mac)
	t.Input["dv"] = fmt.Sprintf("%x", dv)
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
			if status != 0 {
				return output, nil
			}
		}
	}
	if c, ok := rep["cookie"]; ok {
		if cookie, ok := c.(string); ok {
			// TODO 64 bytes?
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

func (h *Handler) onLogin(t *Task, body []byte) ([]byte, error) {
	// TODO check length
	var sn []byte //:= body[22:38]
	token := body[0:16]
	if err := VerifyToken(sn, token); err != nil {
		glog.Errorf("VerifyToken for sn %v and token %v failed: %v", sn, token, err)
		return nil, err
	}
	cookie := body[16:80]
	pid := body[80:88]
	//t.Input["mac"] = fmt.Sprintf("%x", mac)
	t.Input["cookie"] = string(cookie)
	t.Input["pid"] = fmt.Sprintf("%d", pid)

	output := make([]byte, 12)
	httpStatus, rep, err := t.DoHTTPTask()
	if err != nil {
		binary.LittleEndian.PutUint32(output[0:4], uint32(httpStatus))
		return output, err
	}

	if s, ok := rep["status"]; ok {
		if status, ok := s.(float64); ok {
			binary.LittleEndian.PutUint32(output[0:4], uint32(int32(status)))
			if status != 0 {
				return output, nil
			}
		}
	}
	if c, ok := rep["proxyKey"]; ok {
		if cookie, ok := c.(string); ok {
			// TODO 64 bytes?
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

func (h *Handler) onChangeName(t *Task, body []byte) ([]byte, error) {
	return nil, nil
}

func (h *Handler) onBind(t *Task, body []byte) ([]byte, error) {
	return nil, nil
}

func (h *Handler) onHearBeat(t *Task, body []byte) ([]byte, error) {
	return nil, nil
}

func (h *Handler) onSubDeviceOffline(t *Task, body []byte) ([]byte, error) {
	return nil, nil
}
