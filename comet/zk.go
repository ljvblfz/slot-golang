package main

import (
	"github.com/cuixin/cloud/zk"
	"github.com/golang/glog"
	zookeeper "github.com/samuel/go-zookeeper/zk"
	"time"
)

var zkConn *zookeeper.Conn

func InitZK(zkAddrs []string, rootName string) {
	if len(rootName) == 0 {
		glog.Fatalf("[zk] root name for msgbus cannot be empty")
	}
	var (
		nodes []string
		err   error
		conn  *zookeeper.Conn
		addr  string
		watch <-chan zookeeper.Event
	)
	conn, err = zk.Connect(zkAddrs, 60 * time.Second)
	if err != nil {
		glog.Fatal(err)
	}
	glog.Infof("Connect zk[%v] with msgbus root [%s] OK!", zkAddrs, rootName)

	zkRoot := "/" + rootName
	for {
		nodes, watch, err = zk.GetNodesW(conn, zkRoot)
		if err == zookeeper.ErrNoNode || err == zookeeper.ErrNoChildrenForEphemerals {
			glog.Errorln(err)
			time.Sleep(time.Second)
			continue
		} else if err != nil {
			glog.Errorln(err)
			time.Sleep(time.Second)
			continue
		}
		var addrs []string = make([]string, 0, len(nodes))
		for _, n := range nodes {
			addr, err = zk.GetNodeData(conn, zkRoot + "/" + n)
			if err != nil {
				glog.Errorf("[%s] cannot get", addr)
				continue
				// glog.Fatal(err)
			}
			addrs = append(addrs, addr)
		}
		for _, addr := range addrs {
			GMsgBusManager.Online(addr)
		}
		e := <-watch
		glog.Infof("zk receive an event %v", e)
	}
	zkConn = conn
}

func CloseZK() {
	if zkConn != nil {
		zkConn.Close()
	}
}
