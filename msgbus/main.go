package main

import (
	"flag"
	"github.com/golang/glog"
	"strconv"
	"strings"
)

func main() {
	flag.Parse()
	initRedix("193.168.1.224:6379")
	notifyUserState, err := SubUserState()
	if err != nil {
		glog.Fatal(err)
	}
	go func() {
		for bytes := range notifyUserState {
			user_ori := string(bytes)
			users_def := strings.Split(user_ori, ",")
			uid, _ := strconv.ParseInt(users_def[0], 10, 64)
			host := users_def[1]
			if uid > 0 {
				GUserMap.Online(uid, host)
			} else {
				GUserMap.Offline(uid, host)
			}
		}
	}()

	handleSignal(func() {
		glog.Info("Closed Server")
	})
}
