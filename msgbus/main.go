package main

import (
	"flag"
	"fmt"
	"github.com/golang/glog"
	"strconv"
	"strings"
)

func main() {

	rh := flag.String("rh", "193.168.1.224:6379", "Redis地址")
	lPort := flag.Int("port", 9923, "设置MsgBus监听服务器端口地址")
	flag.Parse()
	initRedix(*rh)
	notifyUserState, err := SubUserState()
	if err != nil {
		glog.Fatal(err)
	}
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

	local := NewServer(fmt.Sprintf(":%d", *lPort))
	local.Start()
	handleSignal(func() {
		glog.Info("Closed Server")
		local.Stop()
	})
}
