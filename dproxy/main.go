package main

import (
	"flag"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/golang/glog"
)

func main() {

	if os.Getenv("GOMAXPROCS") == "" {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	addr := flag.String("h", ":7999", "UDP监听地址")
	handlerCount := flag.Int("hc", 1024, "处理消息的线程数")
	httpUrl := flag.String("hurl", "", "HTTP服务器根URL(eg: http://127.0.0.1:8080)")

	flag.Parse()
	defer glog.Flush()

	glog.CopyStandardLogTo("INFO")

	if _, e := url.Parse(*httpUrl); len(*httpUrl) == 0 || e != nil {
		glog.Fatalf("Invalid argument of '-hurl': %s, error: %v", *httpUrl, e)
	}

	handler := NewHandler(*handlerCount, *httpUrl)
	server := NewServer(*addr, handler)
	go server.Start()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)

	for sig := range c {
		switch sig {
		case syscall.SIGINT, syscall.SIGTERM:
			glog.Info("Server stopped")
			return
		}
	}
}
