package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/golang/glog"
)

const (
	kPidFile = ""
)

var (
	gIp		string
	gPort	int
)

func main() {
	// get config
	flag.StringVar(&gIp, "h", "", "rabbitmq的服务ip")
	flag.IntVar(&gPort, "p", 5672, "rabbitmq的服务端口")
	zks := flag.String("zks", "", "zookeeper地址, 可用\",\"分隔的多个ip")
	zkRoot := flag.String("zkroot", "rabbitmq", "将rabbitmq注册到zookeeper的该节点下")

	flag.Parse()

	cmd := exec.Command("/bin/sh", "-c", "ulimit -S -c 0 >/dev/null 2>&1 ; /usr/sbin/rabbitmq-server")

	cmd.Env = append(os.Environ(),
		fmt.Sprintf("RABBITMQ_NODE_IP_ADDRESS=%s", gIp),
		fmt.Sprintf("RABBITMQ_NODE_PORT=%d", gPort),
	)
	if err := cmd.Start(); err != nil {
		glog.Errorf("start rabbitmq error: %v", err)
		return
	}
	pid := cmd.Process.Pid
	InitZK(strings.Split(*zks, ","), *zkRoot)

	quitCh := make(chan struct{})
	go func() {
		defer close(quitCh)

		glog.Infof("Waiting on process pid: %d", pid)
		err := cmd.Wait()
		if err != nil {
			glog.Errorf("Wait rabbitmq process %d error: %v", pid, err)
			return
		}
		glog.Info("Rabbitmq quit:", cmd.ProcessState)
	}()

	// quit would clear zk automatically
	handleSignal(quitCh, func() {
		glog.Info("Closed on signal")
	})
	CloseZK()
}

func handleSignal(quit <-chan struct{}, closeF func()) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGSTOP, syscall.SIGTERM)

	for  {
		select {
		case sig := <-c:
			switch sig {
			case syscall.SIGINT, syscall.SIGSTOP:
				closeF()
				return
			case syscall.SIGTERM:
				glog.Info("catch sigterm, ignore")
				return
			}

		case <-quit:
			return
		}
	}
}
