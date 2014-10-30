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
		case syscall.SIGINT, syscall.SIGTERM:
			closeF()
			return
		}
	}
}
