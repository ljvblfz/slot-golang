package main

import (
	//"fmt"

	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"strconv"
	"time"

	"github.com/golang/glog"
)

const (
	kUrlPrefix = "/api"

	// Json key: API返回值中的json内容的key名
	JKStatus		= "status"
	JKStatusMsg		= "statusMsg"
	JKUserId		= "userId"
	JKCookie		= "cookie"
	JKKey			= "wbKey"
	JKUrlOrigin		= "urlOrigin"
	JKWebsocketAddr = "websocketAddr"
	JKMac			= "mac"
	JKSn			= "sn"
	JKEmail			= "email"
	JKPasswd		= "passwd"
	JKPhone			= "phone"
	JKBindUsers		= "bindedUsers"
	JKBindDevices	= "bindedDevices"
)

// 初始化api的url处理
func InitRouter() {
	http.HandleFunc(kUrlPrefix + "/user/register", apiUserRegister)
	http.HandleFunc(kUrlPrefix + "/user/login", apiUserLogin)
	http.HandleFunc(kUrlPrefix + "/user/devices", apiUserDevices)
	http.HandleFunc(kUrlPrefix + "/device/register", apiDeviceRegister)
	http.HandleFunc(kUrlPrefix + "/device/login", apiDeviceLogin)
	http.HandleFunc(kUrlPrefix + "/bind/in", apiBindIn)
	http.HandleFunc(kUrlPrefix + "/bind/out", apiBindOut)
}

// POST /api/user/register
func apiUserRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	email := r.FormValue(JKEmail)
	passwd := r.FormValue(JKPasswd)
	phone := r.FormValue(JKPhone)

	u, apiErr := RegisterUser(email, passwd, phone)

	data := make(map[string]interface{})

	if apiErr == nil {
		w.WriteHeader(http.StatusOK)
		data[JKStatus] = ErrOk
		data[JKUserId] = u.Id

		cookie, e := Encode(u.Id, *gCookieExpire)
		if e != nil {
			cookie = []byte("error")
		}
		data[JKCookie] = hex.EncodeToString(cookie)
		data[JKKey] = GenerateKey(u.Id, time.Now().UnixNano(), *gCookieExpire)
	} else {
		w.WriteHeader(http.StatusOK)
		data[JKStatus] = apiErr.Code

		// TODO(yy) 这个详细的描述是否可返回给客户端，需要再考虑
		if apiErr.Msg != nil {
			data[JKStatusMsg] = apiErr.Msg.Error()
		}
	}

	data[JKUrlOrigin] = calcReferUrl(r)
	data[JKWebsocketAddr] = *gWebsocketAddr

	body, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(body)
	writeCommonResp(w)
}

// POST /api/user/login
func apiUserLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	email := r.FormValue(JKEmail)
	passwd := r.FormValue(JKPasswd)

	u, apiErr := LoginUser(email, passwd)

	data := make(map[string]interface{})

	if apiErr == nil {
		w.WriteHeader(http.StatusOK)
		data[JKStatus] = ErrOk
		data[JKUserId] = u.Id

		cookie, e := Encode(u.Id, *gCookieExpire)
		if e != nil {
			cookie = []byte("error")
		}
		data[JKCookie] = hex.EncodeToString(cookie)
		data[JKKey] = GenerateKey(u.Id, time.Now().UnixNano(), *gCookieExpire)
		data[JKBindDevices] = u.BindedDevices
	} else {
		w.WriteHeader(http.StatusOK)
		data[JKStatus] = apiErr.Code

		// TODO(yy) 这个详细的描述是否可返回给客户端，需要再考虑
		if apiErr.Msg != nil {
			data[JKStatusMsg] = apiErr.Msg.Error()
		}
	}

	data[JKUrlOrigin] = calcReferUrl(r)
	data[JKWebsocketAddr] = *gWebsocketAddr

	body, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(body)
	writeCommonResp(w)
}

// POST /api/user/devices
func apiUserDevices(w http.ResponseWriter, r *http.Request) {

	if r.Method != "POST" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	data := make(map[string]interface{})

	idUser, err := strconv.ParseInt(r.FormValue(JKUserId), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		data[JKStatus] = ErrInvalidId
		data[JKStatusMsg] = "userid is wrong"
	} else {

		cookie := r.FormValue(JKCookie)

		cookieBuf, err := hex.DecodeString(cookie)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		verifyId, _, err := Decode(cookieBuf)
		if err == nil && verifyId == idUser {
			deviceIds, apiErr := GetUserDevices(idUser)

			if apiErr == nil {
				w.WriteHeader(http.StatusOK)
				data[JKStatus] = ErrOk
				data[JKBindDevices] = deviceIds
			} else {
				w.WriteHeader(http.StatusOK)
				data[JKStatus] = apiErr.Code

				// TODO(yy) 这个详细的描述是否可返回给客户端，需要再考虑
				if apiErr.Msg != nil {
					data[JKStatusMsg] = apiErr.Msg.Error()
				}
			}
		} else {
			w.WriteHeader(http.StatusOK)
			data[JKStatus] = ErrInvalidId
			data[JKStatusMsg] = "user is not valid"
		}
	}

	data[JKUrlOrigin] = calcReferUrl(r)

	body, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(body)
	writeCommonResp(w)
}

// POST /api/device/register
func apiDeviceRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	mac := r.FormValue(JKMac)
	sn := r.FormValue(JKSn)

	data := make(map[string]interface{})

	if VerifySn(sn) {
		d, apiErr := RegisterDevice(mac, sn)

		if apiErr == nil {
			w.WriteHeader(http.StatusOK)
			data[JKStatus] = ErrOk
			data[JKCookie] = d.Key
			data[JKKey] = GenerateKey(d.Id, time.Now().UnixNano(), *gCookieExpire)
		} else {
			w.WriteHeader(http.StatusOK)
			data[JKStatus] = apiErr.Code

			// TODO(yy) 这个详细的描述是否可返回给客户端，需要再考虑
			if apiErr.Msg != nil {
				data[JKStatusMsg] = apiErr.Msg.Error()
			}
		}
	} else {
		w.WriteHeader(http.StatusOK)
		data[JKStatus] = ErrInvalidSn
		data[JKStatusMsg] = "SN is not valid"
	}
	data[JKUrlOrigin] = calcReferUrl(r)
	data[JKWebsocketAddr] = *gWebsocketAddr

	body, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(body)
	writeCommonResp(w)
}

// POST /api/device/login
func apiDeviceLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	mac := r.FormValue(JKMac)
	cookie := r.FormValue(JKCookie)

	data := make(map[string]interface{})

	if VerifyDeviceKey(mac, cookie) {
		d, apiErr := LoginDevice(mac)

		if apiErr == nil {
			w.WriteHeader(http.StatusOK)
			data[JKStatus] = ErrOk
			data[JKKey] = GenerateKey(d.Id, time.Now().UnixNano(), *gCookieExpire)
			data[JKBindUsers] = d.BindedUsers
		} else {
			w.WriteHeader(http.StatusOK)
			data[JKStatus] = apiErr.Code

			// TODO(yy) 这个详细的描述是否可返回给客户端，需要再考虑
			if apiErr.Msg != nil {
				data[JKStatusMsg] = apiErr.Msg.Error()
			}
		}
	} else {
		w.WriteHeader(http.StatusOK)
		data[JKStatus] = ErrInvalidMac
		data[JKStatusMsg] = "MAC is not valid"
	}

	data[JKUrlOrigin] = calcReferUrl(r)
	data[JKWebsocketAddr] = *gWebsocketAddr

	body, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(body)
	writeCommonResp(w)
}

// POST /api/bind/in
func apiBindIn(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	mac := r.FormValue(JKMac)
	cookie := r.FormValue(JKCookie)
	uid, e := strconv.ParseInt(r.FormValue(JKUserId), 10, 64)
	if e != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	data := make(map[string]interface{})

	if VerifyDeviceKey(mac, cookie) {
		dId := extractDeviceId(cookie)
		if dId <= 0 {
			glog.Errorf("VerifyDeviceKey passed, but extraceDeviceId failed, mac=%s, cookie=%s", mac, cookie)
		}

		apiErr := BindDevice(dId, uid)

		if apiErr == nil {
			w.WriteHeader(http.StatusOK)
			data[JKStatus] = ErrOk
		} else {
			w.WriteHeader(http.StatusOK)
			data[JKStatus] = apiErr.Code

			// TODO(yy) 这个详细的描述是否可返回给客户端，需要再考虑
			if apiErr.Msg != nil {
				data[JKStatusMsg] = apiErr.Msg.Error()
			}
		}
	} else {
		w.WriteHeader(http.StatusOK)
		data[JKStatus] = ErrInvalidMac
		data[JKStatusMsg] = "MAC is not valid"
	}

	data[JKUrlOrigin] = calcReferUrl(r)

	body, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(body)
	writeCommonResp(w)
}

// POST /api/bind/out
func apiBindOut(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	mac := r.FormValue(JKMac)
	cookie := r.FormValue(JKCookie)
	uid, e := strconv.ParseInt(r.FormValue(JKUserId), 10, 64)
	if e != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	data := make(map[string]interface{})

	if VerifyDeviceKey(mac, cookie) {
		dId := extractDeviceId(cookie)
		if dId <= 0 {
			glog.Errorf("VerifyDeviceKey passed, but extraceDeviceId failed, mac=%s, cookie=%s", mac, cookie)
		}

		apiErr := UnbindDevice(dId, uid)

		if apiErr == nil {
			w.WriteHeader(http.StatusOK)
			data[JKStatus] = ErrOk
		} else {
			w.WriteHeader(http.StatusOK)
			data[JKStatus] = apiErr.Code

			// TODO(yy) 这个详细的描述是否可返回给客户端，需要再考虑
			if apiErr.Msg != nil {
				data[JKStatusMsg] = apiErr.Msg.Error()
			}
		}
	} else {
		w.WriteHeader(http.StatusOK)
		data[JKStatus] = ErrInvalidMac
		data[JKStatusMsg] = "MAC is not valid"
	}

	data[JKUrlOrigin] = calcReferUrl(r)

	body, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(body)
	writeCommonResp(w)
}

// utils

func calcReferUrl(r *http.Request) string {
	url := r.URL.RequestURI()
	n := strings.Index(url, kUrlPrefix)
	if n >= 0 {
		url = url[n+len(kUrlPrefix):]
	}
	return url
}
