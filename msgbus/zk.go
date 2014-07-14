package main

import (
	"github.com/cuixin/cloud/zk"
	"github.com/cuixin/atomic"
	"github.com/golang/glog"
	zookeeper "github.com/samuel/go-zookeeper/zk"
	"time"
)

var (
	zkConn			*zookeeper.Conn
	zkRoot			string
	zkListenAddr	string

	zkOk atomic.AtomicBoolean
)

func init() {
	zkOk.Set(false)
}

func watchEvent(event zookeeper.Event) {
	if !zkOk.Get() {
		return
	}
	if event.State == zookeeper.StateHasSession {
		err := zk.CreateTempW(zkConn, zkRoot, zkListenAddr, watchZK)
		if err == nil {
			glog.Infof("[zk] register msgbus node ok")
		} else {
			glog.Errorf("[zk] write msgbus node on zookeeper.StateHasSession failed, error: %v", err)
		}
	}
}

func watchZK(existed bool, event zookeeper.Event) {
	glog.Infof("Existed [%v], event [%v]", existed, event)
	watchEvent(event)
}

func InitZK(addrs []string, listenAddr string, rootName string) error {
	conn, err := zk.Connect(addrs, 60 * time.Second, watchEvent)
	if err != nil {
		return err
	}
	zkRoot = "/" + rootName
	zkConn = conn
	zkListenAddr = listenAddr

	zk.Create(conn, zkRoot)
	glog.Infof("Connect zk[%v] on msgbus root [%s] OK!", addrs, rootName)
	createErr := zk.CreateTempW(conn, zkRoot, listenAddr, watchZK)
	zkOk.Set(true)
	if createErr != nil {
		return createErr
	}
	return nil
}

func CloseZK() {
	if zkConn != nil {
		glog.Infoln("ZK closed")
		zkConn.Close()
	}
}
