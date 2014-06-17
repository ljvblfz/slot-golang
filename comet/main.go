package main

import (
	"flag"
	"github.com/golang/glog"
	"strings"
)

var (
	gSessionList *SessionList
)

var (
	kHostName string
)

func main() {
	rh := flag.String("rh", "193.168.1.224:6379", "Redis地址")
	lHost := flag.String("ports", ":1234,:1235", "监听的websocket地址")
	flag.StringVar(&kHostName, "lip", "", "comet服务器本地地址")
	flag.Parse()

	if len(kHostName) == 0 {
		glog.Fatalf("必须指定本机IP")
	}

	initRedix(*rh)
	go InitZK([]string{"193.168.1.221", "193.168.1.222", "193.168.1.223"})

	gSessionList = InitSessionList()
	hostList := strings.Split(*lHost, ",")
	StartHttp(hostList)

	handleSignal(func() {
		CloseZK()
		glog.Info("Closed Server")
	})
}
