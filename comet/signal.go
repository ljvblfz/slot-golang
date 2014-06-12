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
	signal.Notify(c, SIG_STOP, SIG_STATUS, syscall.SIGTERM)

	for sig := range c {
		switch sig {
		case SIG_STOP:
			closeF()
		case SIG_STATUS:
			glog.Infoln("catch sigstatus, ignore")
		case syscall.SIGTERM:
			glog.Info("catch sigterm, ignore")
		}
	}
}
