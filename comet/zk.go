package main

import (
	"github.com/cuixin/cloud/zk"
	"github.com/golang/glog"
	zookeeper "github.com/samuel/go-zookeeper/zk"
	"time"
)

var zkConn *zookeeper.Conn

func InitZK(zkAddrs []string) {
	var (
		nodes []string
		err   error
		conn  *zookeeper.Conn
		addr  string
		event <-chan zookeeper.Event
	)
	conn, err = zk.Connect(zkAddrs, time.Second)
	if err != nil {
		glog.Fatal(err)
	}
	glog.Infof("Connect zk[%v] OK!", zkAddrs)
	for {
		nodes, event, err = zk.GetNodesW(conn, "/MsgBusServers")
		if err != nil {
			glog.Errorln(err)
			time.Sleep(time.Second)
			continue
		}
		for _, n := range nodes {
			addr, err = zk.GetNodeData(conn, "/MsgBusServers/"+n)
			if err != nil {
				glog.Fatal(err)
			}
			GMsgBusManager.Online(addr)
		}

		for e := range event {
			glog.Infof("Got Event %v", e)
		}
	}
	zkConn = conn
}

func CloseZK() {
	if zkConn != nil {
		zkConn.Close()
	}
}
