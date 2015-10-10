package main

import (
	"cloud-socket/ver"
	"encoding/binary"
	"flag"
	"github.com/golang/glog"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
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
	gServer     Server
	gIPS        map[string]string
	gStrIPS     string
)

func chooseAUDPServer() (output []byte,addr string) {
	output = make([]byte, 37)
	addr = gQueue.next().(string)
	glog.Infoln(addr, gIPS[addr])
	glog.Infoln(gIPS)
	addr = strings.Replace(addr, addr, gIPS[addr], 1)
	adr := strings.Split(addr, ":")
	if len(adr) == 2 {
		port, _ := strconv.Atoi(adr[1])
		binary.LittleEndian.PutUint32(output[0:4], uint32(port))
		output[36] = byte(len(adr[0]))
		output = append(output, []byte(adr[0])...)
	}
	return
}

func main() {
	if os.Getenv("GOMAXPROCS") == "" {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}
	defer glog.Flush()
	glog.CopyStandardLogTo("INFO")

	flag.StringVar(&serverAddr, "serverAddr", "193.168.1.63:7999", "udp proxy ip and port. eg:193.168.1.63:7999")
	flag.StringVar(&zkHosts, "zks", "193.168.1.221,193.168.1.222,193.168.1.223", "设置ZK的地址,多个地址用逗号分割")
	flag.StringVar(&gCometRoot, "zkrootUdpComet", "CometServersUdp", "zookeeper服务中udp comet所在的根节点名")
	flag.Float64Var(&gCPUUsage, "cpu", 0.9, "CPU最高使用率。如：0.9 表示90%")
	flag.Float64Var(&gMEMFree, "mem", 100, "最小空闲内存大小")
	flag.Int64Var(&gCount, "count", 6000, "最大句柄数")
	flag.DurationVar(&inverval, "inv", 10, "轮询UDP服务器状态的时间间隔，s代表秒 ns 代表纳秒 ms 代表毫秒")
	flag.BoolVar(&printVer, "ver", false, "Proxy版本")
	flag.StringVar(&gStrIPS, "ipmap", "193.168.0.60:7999=188.168.0.60:7999|193.168.0.61:7999=188.168.0.60:7999", "代理服务器ip映射表")
	flag.Parse()

	if printVer {
		glog.Infof("Proxy %s, 插座后台代理服务器.\n", ver.Version)
	}
	glog.Infoln("gStrIPS", gStrIPS)
	ips := strings.Split(gStrIPS, "|")
	gIPS = make(map[string]string)
	for _, ip := range ips {
		kv := strings.Split(ip, "=")
		gIPS[kv[0]] = kv[1]
	}
	glog.Infoln("gIPS", gIPS)

	go InitZK(strings.Split(zkHosts, ","), gCometRoot)

	gServer = NewServer(serverAddr)
	go gServer.RunLoop()

	handleSignal(func() {
		CloseZK()
		glog.Infof("Closed Server")
	})
}
