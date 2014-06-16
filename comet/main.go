package main

import (
	"flag"
	"github.com/golang/glog"
	"strings"
)

var (
	gSessionList *SessionList
)

func main() {
	rh := flag.String("rh", "193.168.1.224:6379", "Redis地址")
	// msgbusAddr := flag.String("msgbus", "localhost:9923", "MsgBus地址")
	lHost := flag.String("ports", ":1234,:1235", "监听的websocket地址")
	flag.Parse()
	initRedix(*rh)
	go InitZK([]string{"193.168.1.221", "193.168.1.222", "193.168.1.223"})

	gSessionList = InitSessionList()
	hostList := strings.Split(*lHost, ",")
	StartHttp(hostList)
	glog.Infoln("Server starting...")

	handleSignal(func() {
		glog.Info("Closed Server")
	})
}
