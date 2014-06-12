package main

import (
	"log"
	"github.com/golang/glog"
)

var (
	gSessionList *SessionList
)

func main() {
	gSessionList = InitSessionList()

	StartHttp([]string{":1234", ":1235"})
	glog.Infoln("Server starting...")
	handleSignal(func() {
		glog.Info("Closed Server")
		log.Println("Closed Server")
	})
}
