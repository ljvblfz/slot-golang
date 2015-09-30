package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	//"io"
	"net"
	"net/http"
	//"net/url"
	"strings"

	//"github.com/golang/glog"
)

const (
	CmdSess             = uint16(0xC1)
	CmdRegister         = uint16(0xC2)
	CmdLogin            = uint16(0xC3)
	CmdHeartBeat        = uint16(0xC4)
	CmdLoginout         = uint16(0xC5)

	UrlRegister   = "/api/device/register"
	UrlLogin      = "/api/device/login"
)

var (
	DAckOk          int32 = 0
	DAckHTTPError   int32 = 500
	DAckServerError int32 = 503

//	DAckBadCmd      int32 = 1002
)

type UdpMsg struct {
	Peer    *net.UDPAddr
	Msg     []byte
	Url     string
	CmdType uint16
	Input   map[string]string
	Output  map[string]string
}

// 返回需要回复给设备的消息
func (t *UdpMsg) DoHTTPTask() (status int32, response map[string]interface{}, error error) {
	reqs := ""
	for k, v := range t.Input {
		reqs = fmt.Sprintf("%s&%s=%s", reqs, k, v)
	}
	req := bytes.NewReader([]byte(strings.TrimLeft(reqs, "&")))
	rep, err := http.Post(t.Url, "application/x-www-form-urlencoded;charset=utf-8", req)
	if err != nil {
		return DAckServerError, nil, fmt.Errorf("[POST TO HTTP] input:[%#v] failed on server,\nresponse:%v,\nerror: %v", t, rep, err)
	}
	defer rep.Body.Close()

	if rep.StatusCode != 200 {
		return int32(rep.StatusCode), nil, fmt.Errorf("[POST TO HTTP] input:[%#v] failed,\nhttp code: %v", t, rep.StatusCode)
	}

	d := json.NewDecoder(rep.Body)
	response = make(map[string]interface{})
	err = d.Decode(&response)
	if err != nil {
		return int32(rep.StatusCode), nil, fmt.Errorf("[POST TO HTTP] %v", err)
	}

	return int32(rep.StatusCode), response, nil

	//buf := bytes.Buffer{}
	//_, err = io.Copy(&buf, rep.Body)
	//if err != nil {
	//	glog.Warningf("[task] copy from body to buffer failed, buffer: %v, err: %v", buf.Bytes(), err)
	//}
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
