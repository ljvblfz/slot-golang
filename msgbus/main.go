package main

import (
	"cloud-socket/ver"
	"flag"
	"fmt"
	"github.com/golang/glog"
	"strconv"
	"strings"
)

func main() {

	rh := flag.String("rh", "193.168.1.224:6379", "Redis地址")
	addr := flag.String("addr", "localhost:9923", "设置MsgBus监听服务器端口地址")
	zks := flag.String("zks", "193.168.1.221,193.168.1.222,193.168.1.223", "设置ZK服务器地址列表")
	zkRootName := flag.String("zkroot", "MsgBusServers", "msgbus注册到zookeeper服务中的根节点名")
	zkRootRmq := flag.String("zkrootr", "Rabbitmq", "rabbitmq注册到zookeeper服务中的根节点名")
	statusAddr := flag.String("sh", ":29998", "程序状态http服务端口")
	printVer := flag.Bool("ver", false, "Comet版本")
	flag.Parse()

	if *printVer {
		fmt.Printf("Comet %s, 插座后台代理服务器.\n", ver.Version)
		return
	}

	defer glog.Flush()

	InitStat(*statusAddr)

	InitUserMap()
	InitModel(*rh)

	// 先sub用户登录信息,再载入已有的用户列表,避免在载入和sub之间的空隙遗漏信息
	if notifyUserState, err := SubUserState(); err != nil {
		glog.Fatal(err)
	} else {
		go handleOnlineEvent(notifyUserState)
	}
	err := LoadUsers()
	if err != nil {
		glog.Fatal(err)
	}

	local := NewServer(*addr)
	local.Start()

	if err := InitZK(strings.Split(*zks, ","), local.addr, *zkRootName, *zkRootRmq); err != nil {
		glog.Fatal(err)
	}
	handleSignal(func() {
		CloseZK()
		glog.Info("MsgBusServer Closed")
		local.Stop()
	})
}

func handleOnlineEvent(notifyUserState <-chan []byte) {
	for bytes := range notifyUserState {
		user_ori := string(bytes)
		users_def := strings.Split(user_ori, "|")
		uid, _ := strconv.ParseInt(users_def[0], 10, 64)
		host := users_def[1]
		isOnline := users_def[2]
		if isOnline == "1" {
			GUserMap.Online(uid, host)
			if glog.V(3) {
				glog.Infof("[bus:online] user %d on comet %s", uid, host)
			}
		} else {
			if uid == 0 {
				GUserMap.OfflineHost(host)
			} else {
				GUserMap.Offline(uid, host)
			}
			if glog.V(3) {
				glog.Infof("[bus:offline] user %d on comet %s", uid, host)
			}
		}
	}
}
