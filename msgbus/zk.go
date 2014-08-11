package main

import (
	"strings"
	"time"

	"github.com/cuixin/cloud/zk"
	"github.com/cuixin/atomic"
	"github.com/golang/glog"
	zookeeper "github.com/samuel/go-zookeeper/zk"
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

	go WatchRmq()
	return nil
}

func WatchRmq() {
	zkRmqRoot := "/" + "Rabbitmq"
	glog.Infof("Watching rabbitmq root [%s] OK!", zkRmqRoot)
	for {
		nodes, watch, err := zk.GetNodesW(zkConn, zkRmqRoot)
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
			addr, err := zk.GetNodeData(zkConn, zkRmqRoot + "/" + n)
			if err != nil {
				glog.Errorf("[%s] cannot get", addr)
				continue
			}
			addrs = append(addrs, addr)
		}
		for _, addr := range addrs {
			confs := strings.SplitN(addr, ",", 2)
			if len(confs) != 2 {
				glog.Errorf("[rmq] data in zk is not valid format(eg: url,queuename): %s", confs)
				continue
			}
			glog.Infof("[rmq] online %s", addr)
			GRmqs.Add(confs[0], confs[1])
		}
		e := <-watch
		glog.Infof("zk receive an event %v", e)
	}
}

func CloseZK() {
	if zkConn != nil {
		glog.Infoln("ZK closed")
		zkConn.Close()
	}
}
