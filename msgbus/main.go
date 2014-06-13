package main

import (
	"flag"
	"github.com/golang/glog"
	"strconv"
	"strings"
)

func main() {

	rh := flag.String("rh", "193.168.1.224:6379", "Redis地址")
	lhost := flag.String("addr", "localhost:9923", "设置MsgBus监听服务器端口地址")
	flag.Parse()
	initRedix(*rh)

	if notifyUserState, err := SubUserState(); err != nil {
		glog.Fatal(err)
	} else {
		go func() {
			for bytes := range notifyUserState {
				user_ori := string(bytes)
				users_def := strings.Split(user_ori, ",")
				uid, _ := strconv.ParseInt(users_def[0], 10, 64)
				host := users_def[1]
				if uid > 0 {
					GUserMap.Online(uid, host)
				} else {
					GUserMap.Offline(uid, host)
				}
			}
		}()
	}

	local := NewServer(*lhost)
	local.Start()

	if err := InitZK([]string{"193.168.1.221", "193.168.1.222", "193.168.1.223"},
		local.addr); err != nil {
		glog.Fatal(err)
	}
	handleSignal(func() {
		glog.Info("Closed Server")
		local.Stop()
	})
}
