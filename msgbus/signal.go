package main

import (
	"github.com/golang/glog"
	"os"
	"os/signal"
	"syscall"
)

type closeFunc func()

func handleSignal(closeF closeFunc) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)

	for sig := range c {
		switch sig {
		case syscall.SIGINT:
			closeF()
		// case SIG_STATUS:
		// 	glog.Infoln("catch sigstatus, ignore")
		case syscall.SIGTERM:
			glog.Info("catch sigterm, ignore")
		}
		return
	}

}
