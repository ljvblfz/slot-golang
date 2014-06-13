package main

import (
	"github.com/cuixin/cloud/zk"
	"github.com/golang/glog"
	zookeeper "github.com/samuel/go-zookeeper/zk"
	"time"
)

func ServerTrigger(existed bool, event zookeeper.Event) {
	glog.Infof("Existed [%v], event [%v]", existed, event)
}

func InitZK(zkAddrs []string) error {
	conn, err := zk.Connect(zkAddrs, time.Second)
	if err != nil {
		return err
	}
	glog.Infof("Connect zk[%v] OK!", zkAddrs)
	nodes, event, nerr := zk.GetNodesW(conn, "/MsgBusServers")
	if nerr != nil {
		return nerr
	}
	for _, n := range nodes {
		addr, err := zk.GetNodeData(conn, "/MsgBusServers/"+n)
		if err != nil {
			return err
		}
		gMsgBusServer = NewMsgBusServer(addr)
		if gMsgBusServer.Dail() == nil {
			go gMsgBusServer.Reciver()
		}
	}
	for e := range event {
		glog.Info(e)
	}
	return nil
}
