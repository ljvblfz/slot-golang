package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	//"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"cloud-socket/ver"
	"github.com/golang/glog"
)

const (
	kPidFile = ""
)

var (
	//gZkIp   string
	gAddr         string
	gAPIUrl       string
	gVhost        string
	gHeartbeatDur time.Duration
)

type ApiResult struct {
	Status string `json:"status"`
}

func main() {
	// get config
	//flag.StringVar(&gZkIp, "zh", "", "rabbitmq注册到zookeeper中的ip")
	flag.StringVar(&gAddr, "h", "127.0.0.1:5672", "rabbitmq的监听ip")
	flag.StringVar(&gAPIUrl, "url", "http://guest:guest@127.0.0.1:15672", "rabbitmq的API URL地址")
	flag.StringVar(&gVhost, "vh", "/", "测试rabbitmq的心跳API使用的vhost名")
	flag.DurationVar(&gHeartbeatDur, "d", 10*time.Second, "测试rabbitmq的心跳间隔")
	zks := flag.String("zks", "", "zookeeper地址, 可用\",\"分隔的多个ip")
	zkRoot := flag.String("zkroot", "rabbitmq", "将rabbitmq注册到zookeeper的该节点下")
	printVer := flag.Bool("ver", false, "Comet版本")

	flag.Parse()

	if *printVer {
		fmt.Printf("Comet %s, 插座后台代理服务器.\n", ver.Version)
		return
	}

	defer glog.Flush()

	// new code
	InitZK(strings.Split(*zks, ","), *zkRoot)
	u, err := url.Parse(gAPIUrl)
	if err != nil {
		glog.Fatalf("Parse API URL (%s) failed: %v", gAPIUrl, err)
		return
	}
	u.Path = "/api/aliveness-test/"
	apiUrl := u.String() + url.QueryEscape(gVhost)
	opaque := u.Path + url.QueryEscape(gVhost)

	onlinedLast := false

	glog.Infof("Start hearbeat of %s", apiUrl)
	for {
		r, err := http.NewRequest("GET", apiUrl, nil)
		if err != nil {
			glog.Fatal("NewRequest from url %s failed: %v", apiUrl, err)
		}
		r.URL.Opaque = opaque
		response, err := http.DefaultClient.Do(r)
		onlinedNow := false
		if err == nil {
			buf, err := ioutil.ReadAll(response.Body)
			if err == nil {
				ret := ApiResult{}
				err = json.Unmarshal(buf, &ret)
				if err == nil {
					onlinedNow = (ret.Status == "ok")
				} else {
					glog.Errorf("Unmarshal json failed: %v, body: %s", err, string(buf))
				}
			} else {
				glog.Errorf("Read HTTP body failed: %v", err)
			}
			err = response.Body.Close()
			if err != nil {
				glog.Errorf("Close body error: %v", err)
			}
		} else {
			glog.Errorf("Get aliveness test failed: %v", err)
		}

		if onlinedLast != onlinedNow {
			onlinedLast = onlinedNow
			if onlinedNow {
				OnlineRmq()
				glog.Infof("onlined")
			} else {
				OfflineRmq()
				glog.Infof("offlined")
			}
		}

		time.Sleep(gHeartbeatDur)
	}

	// old code
	//cmd := exec.Command("/bin/sh", "-c", "ulimit -S -c 0 >/dev/null 2>&1 ; /usr/sbin/rabbitmq-server")

	//cmd.Env = append(os.Environ(),
	//	fmt.Sprintf("RABBITMQ_NODE_IP_ADDRESS=%s", gIp),
	//	fmt.Sprintf("RABBITMQ_NODE_PORT=%d", gPort),
	//)
	//if err := cmd.Start(); err != nil {
	//	glog.Errorf("start rabbitmq error: %v", err)
	//	return
	//}
	//pid := cmd.Process.Pid

	//// Sleep before register rmq to zk to wait rmq listening on its server socket
	//time.Sleep(3 * time.Second)

	//InitZK(strings.Split(*zks, ","), *zkRoot)

	//quitCh := make(chan struct{})
	//go func() {
	//	defer close(quitCh)

	//	glog.Infof("Waiting on process pid: %d", pid)
	//	err := cmd.Wait()
	//	if err != nil {
	//		glog.Errorf("Wait rabbitmq process %d error: %v", pid, err)
	//		return
	//	}
	//	glog.Info("Rabbitmq quit:", cmd.ProcessState)
	//}()

	//// quit would clear zk automatically
	//handleSignal(quitCh, func() {
	//	glog.Info("Closed on signal")
	//})
	//CloseZK()
}

func handleSignal(quit <-chan struct{}, closeF func()) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGSTOP, syscall.SIGTERM)

	for {
		select {
		case sig := <-c:
			switch sig {
			case syscall.SIGINT, syscall.SIGTERM:
				closeF()
				return
			}

		case <-quit:
			return
		}
	}
}
