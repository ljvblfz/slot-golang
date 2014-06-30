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
	zks := flag.String("zks", "193.168.1.221,193.168.1.222,193.168.1.223", "设置ZK服务器地址列表")
	statusAddr := flag.String("sh", ":29998", "程序状态http服务端口")
	flag.Parse()

	InitStat(*statusAddr)

	InitUserMap()
	err := InitModel(*rh)
	if err != nil {
		glog.Fatal(err)
	}

	if notifyUserState, err := SubUserState(); err != nil {
		glog.Fatal(err)
	} else {
		go func() {
			for bytes := range notifyUserState {
				user_ori := string(bytes)
				users_def := strings.Split(user_ori, "|")
				uid, _ := strconv.ParseInt(users_def[0], 10, 64)
				host := users_def[1]
				isOnline := users_def[2]
				if isOnline == "1" {
					GUserMap.Online(uid, host)
					if glog.V(1) {
						glog.Infof("[online] user %d on %s", uid, host)
					}
				} else {
					GUserMap.Offline(uid, host)
					if glog.V(1) {
						glog.Infof("[offline] user %d on %s", uid, host)
					}
				}
			}
		}()
	}

	local := NewServer(*lhost)
	local.Start()

	if err := InitZK(strings.Split(*zks, ","),
		local.addr); err != nil {
		glog.Fatal(err)
	}
	handleSignal(func() {
		CloseZK()
		glog.Info("Closed Server")
		local.Stop()
	})
}
