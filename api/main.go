package main

import (
	"flag"
	"net/http"

	"github.com/golang/glog"
)

var (
	hostAddr = flag.String("h", ":8080", "服务器监听地址")
	redisAddr = flag.String("r", "127.0.0.1:6379", "Redis服务器地址")
	redisPwd = flag.String("rp", "", "Redis服务器密码")
	gCookieExpire = flag.Int64("e", 300, "cookie过期时间")
	gWebsocketAddr = flag.String("wh", "ws://127.0.0.1/ws", "websocket地址")
)

func main() {

	flag.Parse()

	InitRedis(*redisAddr, *redisPwd)
	InitRouter()

	glog.Infoln("Start server on", *hostAddr)
	err := http.ListenAndServe(*hostAddr, nil)
	if err != nil {
		glog.Fatalf("Listen on %s failed: %v\n", *hostAddr, err)
	}
}
