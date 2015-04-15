package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"strings"

	"cloud-socket/ver"
	"github.com/golang/glog"
)

var (
	gSessionList *SessionList
	gLocalAddr   string
	gStatusAddr  string
	gMsgbusRoot  string
	gCometRoot   string
)

func main() {

	if os.Getenv("GOMAXPROCS") == "" {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}
	cType := flag.String("type", "ws", "comet服务类型，可选:1)ws, 2)udp, 3)ws,udp")

	addr := flag.String("hudp", ":7999", "UDP监听地址")
	apiUrl := flag.String("hurl", "", "HTTP服务器根URL(eg: http://127.0.0.1:8080)")
	serveUdpAddr := flag.String("hhttp", ":8081", "UDP服务器提供HTTP服务的地址")

	rh := flag.String("rh", "193.168.1.224:6379", "Redis服务器地址")
	lHost := flag.String("ports", ":1234,:1235", "监听的websocket地址")
	zkHosts := flag.String("zks", "193.168.1.221,193.168.1.222,193.168.1.223", "设置ZK的地址,多个地址用逗号分割")
	flag.StringVar(&gLocalAddr, "lip", "", "comet服务器本地地址")
	flag.StringVar(&gStatusAddr, "sh", ":29999", "程序状态http服务端口")
	flag.StringVar(&gMsgbusRoot, "zkroot", "MsgBusServers", "zookeeper服务中msgbus所在的根节点名")
	flag.StringVar(&gCometRoot, "zkrootc", "CometServers", "zookeeper服务中comet所在的根节点名")
	flag.IntVar(&gUdpTimeout, "uto", gUdpTimeout, "客户端UDP端口失效时长（秒)")
	printVer := flag.Bool("ver", false, "Comet版本")
	flag.Parse()

	if *printVer {
		fmt.Printf("Comet %s, 插座后台代理服务器.\n", ver.Version)
		return
	}

	defer glog.Flush()

	glog.CopyStandardLogTo("INFO")

	InitStat(gStatusAddr)
	InitRedix(*rh)

	if err := ClearRedis(gLocalAddr); err != nil {
		glog.Fatalf("ClearRedis before starting failed: %v", err)
	}

	go InitZK(strings.Split(*zkHosts, ","), gMsgbusRoot, gCometRoot)

	gSessionList = InitSessionList()

	types := strings.Split(*cType, ",")
	for _, t := range types {
		switch t {
		case "ws":
			if len(gLocalAddr) == 0 {
				glog.Fatalf("必须指定本机IP")
			}
			StartHttp(strings.Split(*lHost, ","))

		case "udp":
			if _, e := url.Parse(*apiUrl); len(*apiUrl) == 0 || e != nil {
				glog.Fatalf("Invalid argument of '-hurl': %s, error: %v", *apiUrl, e)
			}

			handler := NewHandler(*apiUrl, *serveUdpAddr)
			server := NewUdpServer(*addr, handler)
			handler.Server = server
			go server.RunLoop()

		default:
			glog.Fatalf("undifined argument for \"-type\"")
		}
	}

	handleSignal(func() {
		CloseZK()
		glog.Info("Closed Server")
	})
}
