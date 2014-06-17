package main

import (
	"github.com/cuixin/cloud/zk"
	"github.com/golang/glog"
	zookeeper "github.com/samuel/go-zookeeper/zk"
	"time"
)

var zkConn *zookeeper.Conn

func watchZK(existed bool, event zookeeper.Event) {
	glog.Infof("Existed [%v], event [%v]", existed, event)
}

func InitZK(addrs []string, listenAddr string) error {
	conn, err := zk.Connect(addrs, time.Second)
	if err != nil {
		return err
	}
	zk.Create(conn, "/MsgBusServers")
	glog.Infof("Connect zk[%v] OK!", addrs)
	createErr := zk.CreateTempW(conn, "/MsgBusServers", listenAddr, watchZK)
	if createErr != nil {
		return createErr
	}
	zkConn = conn
	return nil
}

func CloseZK() {
	if zkConn != nil {
		glog.Infoln("ZK closed")
		zkConn.Close()
	}
}
