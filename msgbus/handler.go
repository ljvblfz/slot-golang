package main

import (
	"github.com/golang/glog"
)

func MainHandle(msg []byte) {
	glog.Info(msg)
}

func HandleClose(host string) {
	glog.Infof("server closed")
}
