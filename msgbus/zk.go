package main

import (
	"github.com/cuixin/cloud/zk"
	"github.com/golang/glog"
	zookeeper "github.com/samuel/go-zookeeper/zk"
	"time"
)

var (
	zkConn			*zookeeper.Conn
	zkRoot			string
	zkListenAddr	string
)

func watchZK(existed bool, event zookeeper.Event) {
	glog.Infof("Existed [%v], event [%v]", existed, event)
	if event.State == zookeeper.StateHasSession {
		err := zk.CreateTempW(zkConn, zkRoot, zkListenAddr, watchZK)
		if err == nil {
			glog.Infof("[zk] register msgbus node ok")
		} else {
			glog.Errorf("[zk] write msgbus node on zookeeper.StateHasSession failed, error: %v", err)
		}
	}
}

func InitZK(addrs []string, listenAddr string, rootName string) error {
	conn, err := zk.Connect(addrs, 60 * time.Second)
	if err != nil {
		return err
	}
	zkRoot = "/" + rootName
	zk.Create(conn, zkRoot)
	glog.Infof("Connect zk[%v] on msgbus root [%s] OK!", addrs, rootName)
	createErr := zk.CreateTempW(conn, zkRoot, listenAddr, watchZK)
	if createErr != nil {
		return createErr
	}
	zkConn = conn
	zkListenAddr = listenAddr
	return nil
}

func CloseZK() {
	if zkConn != nil {
		glog.Infoln("ZK closed")
		zkConn.Close()
	}
}
