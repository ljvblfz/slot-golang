package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	//"net/url"
	"strings"

	"github.com/golang/glog"
)

const (
	CmdRegister = iota
	CmdLogin
	CmdBind
	CmdMax

	UrlRegister = "/api/device/register"
	UrlLogin    = "/api/device/login"
	UrlBind     = "/api/bind/in"
)

var (
	AckOk          = []byte{0}
	AckHTTPError   = []byte{1}
	AckServerError = []byte{2}
)

type Task struct {
	Peer    *net.UDPAddr
	Msg     []byte
	Url     string
	CmdType int8
	Input   map[string]string
	Output  map[string]string
}

// 返回需要回复给设备的消息
func (t *Task) Do() []byte {
	if len(t.Input) == 0 {
		glog.Warning("input is empty")
	}
	reqs := ""
	for k, v := range t.Input {
		reqs = fmt.Sprintf("%s&%s=%s", reqs, k, v)
	}
	//req := bytes.NewReader([]byte(url.QueryEscape(strings.TrimLeft(reqs, "&"))))
	req := bytes.NewReader([]byte(strings.TrimLeft(reqs, "&")))
	rep, err := http.Post(t.Url, "application/x-www-form-urlencoded;charset=utf-8", req)
	// TODO response
	if err != nil {
		glog.Errorf("[task] process task %v failed on server, response: %v, error: %v", t, rep, err)
		return AckServerError
	}

	if rep.StatusCode != 200 {
		glog.Errorf("[task] process task [%#v] failed on http code: %v", t, rep.StatusCode)
		return AckHTTPError
	}

	buf := bytes.Buffer{}
	buf.Write(AckOk)
	_, err = io.Copy(&buf, rep.Body)
	if err != nil {
		glog.Warningf("[task] copy from body to buffer failed, buffer: %v, err: %v", buf.Bytes(), err)
	}
	rep.Body.Close()
	return buf.Bytes()
	//body, err := url.QueryUnescape(string(buf.Bytes()))
	//if err != nil {
	//	glog.Warningf("[task] unescaped data %s failed: %v", string(buf.Bytes()), err)
	//	return AckHTTPError
	//}
	//return []byte(body)
}

//type DeviceRegisterTask struct {
//	url string
//}
//
//func (t *DeviceRegisterTask) Do() error {
//}
//
//type DeviceLoginTask struct {
//	url string
//}
//
//func (t *DeviceLoginTask) Do() error {
//}
//
//type DeviceBindTask struct {
//	url string
//}
//
//func (t *DeviceBindTask) Do() error {
//}
