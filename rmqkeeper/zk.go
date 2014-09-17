package main

import (
	"fmt"
	"time"

	"cloud-base/atomic"
	"github.com/golang/glog"
	"cloud-base/zk"
	zookeeper "github.com/samuel/go-zookeeper/zk"
)

var (
	zkConn		*zookeeper.Conn
	zkConnOk	atomic.AtomicBoolean
	zkReportCh	chan zookeeper.Event
	zkRoot		string
)

func init() {
	zkReportCh = make(chan zookeeper.Event, 1)
}

func onConnStatus(event zookeeper.Event) {
	switch event.Type {
	case zookeeper.EventSession:
		switch event.State {
		case zookeeper.StateHasSession:
			zkConnOk.Set(true)

		case zookeeper.StateDisconnected:
			fallthrough
		case zookeeper.StateExpired:
			zkConnOk.Set(false)
		}
	}
	zkReportCh <- event
}

func InitZK(zkAddrs []string, rootName string) {
	if len(rootName) == 0 {
		glog.Fatalf("[zk] root name for rabbitmq cannot be empty")
	}
	var (
		err   error
		conn  *zookeeper.Conn
	)

	zkRoot = "/" + rootName

	conn, err = zk.Connect(zkAddrs, 60 * time.Second, onConnStatus)
	if err != nil {
		glog.Fatal(err)
	}
	zkConn = conn

	err = zk.Create(zkConn, zkRoot)
	if err != nil {
		glog.Infof("[zk] create connection error: %v", err)
	}

	go handleEvent()
}

func handleEvent() {
	glog.Infof("Loop started")
	data := fmt.Sprintf("%s:%d,MsgTopic", gIp, gPort)

	for event := range zkReportCh {
		glog.Infof("Get zk event: %v", event)
		if event.Type != zookeeper.EventSession {
			break
		}
		if !zkConnOk.Get() || event.State != zookeeper.StateHasSession {
			continue
		}
		tpath, err := zkConn.Create(zkRoot + "/", []byte(data),
			zookeeper.FlagEphemeral|zookeeper.FlagSequence, zookeeper.WorldACL(zookeeper.PermAll))
		if err != nil {
			glog.Errorf("create comet node %s with data %s on zk failed: %v", zkRoot, data, err)
			break
		}
		if len(tpath) == 0 {
			glog.Errorf("create empty comet node %s with data %s", zkRoot, data)
			break
		}
	}
}

func CloseZK() {
	if zkConn != nil {
		zkConn.Close()
	}
}
