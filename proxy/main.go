package main

import (
	"cloud-socket/ver"
	"flag"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/golang/glog"
)

var (
	serverAddr  string
	zkHosts     string
	gProxyRoot  string
	gCometRoot  string
	gStatusAddr string
	gCPUUsage   float64
	gMEMFree    float64
	gCount      int64
	printVer    bool
	inverval    time.Duration
)

func main() {
	if os.Getenv("GOMAXPROCS") == "" {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	glog.CopyStandardLogTo("INFO")

	flag.StringVar(&serverAddr, "hudp", "193.168.1.63:7999", "udp proxy ip and port. eg:193.168.1.63:7999")
	flag.StringVar(&zkHosts, "zks", "193.168.1.221,193.168.1.222,193.168.1.223", "设置ZK的地址,多个地址用逗号分割")
	flag.StringVar(&gProxyRoot, "zkroot", "ProxyServers", "zookeeper服务中proxy所在的根节点名")
	flag.StringVar(&gCometRoot, "zkrootUdpComet", "CometServers", "zookeeper服务中udp comet所在的根节点名")
	flag.StringVar(&gStatusAddr, "sh", ":29999", "程序状态http服务端口")
	flag.Float64Var(&gCPUUsage, "cpu", 0.9, "CPU最高使用率。如：0.9 表示90%")
	flag.Float64Var(&gMEMFree, "mem", 100, "最小空闲内存大小")
	flag.Int64Var(&gCount, "count", 6000, "最大句柄数")
	flag.DurationVar(&inverval, "inv", 10, "轮询UDP服务器状态的时间间隔，s代表秒 ns 代表纳秒 ms 代表毫秒")
	flag.BoolVar(&printVer, "ver", false, "Proxy版本")

	flag.Parse()

	if printVer {
		glog.Infof("Proxy %s, 插座后台代理服务器.\n", ver.Version)
	}

	InitStat(gStatusAddr)
	go InitZK(strings.Split(zkHosts, ","), gProxyRoot, gCometRoot)

	server := NewServer(serverAddr)
	go server.RunLoop()

	go func() {
		for {
			serviceAddr := selectUDPServer()
			glog.Infoln("===>> serviceAddr ", serviceAddr)
			time.Sleep(2 * time.Second)
		}
	}()

	handleSignal(func() {
		CloseZK()
		glog.Infof("Closed Server")
	})
}
