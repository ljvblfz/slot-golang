package main

import (
	"flag"
	"log"
	"github.com/golang/glog"
	"strings"
)

var (
	gSessionList *SessionList
	gLocalAddr   string
	gStatusAddr  string
	gMsgbusRoot  string
	gCometRoot   string
)

func main() {
	rh := flag.String("rh", "193.168.1.224:6379", "Redis服务器地址")
	lHost := flag.String("ports", ":1234,:1235", "监听的websocket地址")
	zkHosts := flag.String("zks", "193.168.1.221,193.168.1.222,193.168.1.223", "设置ZK的地址,多个地址用逗号分割")
	flag.StringVar(&gLocalAddr, "lip", "", "comet服务器本地地址")
	flag.StringVar(&gStatusAddr, "sh", ":29999", "程序状态http服务端口")
	flag.StringVar(&gMsgbusRoot, "zkroot", "MsgBusServers", "zookeeper服务中msgbus所在的根节点名")
	flag.StringVar(&gCometRoot, "zkrootc", "CometServers", "zookeeper服务中comet所在的根节点名")
	flag.Parse()

	defer glog.Flush()

	log.SetFlags(log.Flags() | log.Llongfile)

	InitStat(gStatusAddr)

	if len(gLocalAddr) == 0 {
		glog.Fatalf("必须指定本机IP")
	}

	initRedix(*rh)

	go InitZK(strings.Split(*zkHosts, ","), gMsgbusRoot, gCometRoot)

	gSessionList = InitSessionList()
	StartHttp(strings.Split(*lHost, ","))

	handleSignal(func() {
		CloseZK()
		glog.Info("Closed Server")
	})
}
