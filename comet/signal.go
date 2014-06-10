package main

import (
	"os"
	"os/signal"
	"syscall"
)

var osCh = make(chan os.Signal, 1)

func InitSignal() /*chan os.Signal*/ {
	signal.Notify(osCh, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT, syscall.SIGSTOP)
}

func HandleSignal() {
	for {
		s := <-osCh
		Log.Infof("get a signal %v\n", s)
		switch s {
		case syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGSTOP, syscall.SIGINT:
			return
		case syscall.SIGHUP:
			// TODO reload configuration
			return
		default:
			return
		}
	}
}
