package main

import (
	"flag"
	"log"
	"github.com/golang/glog"
)

var (
	gSessionList *SessionList
)

func main() {
	rh := flag.String("rh", "193.168.1.224:6379", "Redis地址")
	flag.Parse()

	gSessionList = InitSessionList()
	initRedix(*rh)

	StartHttp([]string{":1234", ":1235"})
	glog.Infoln("Server starting...")
	handleSignal(func() {
		glog.Info("Closed Server")
		log.Println("Closed Server")
	})
}
