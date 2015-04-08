package main

import (
	"fmt"
	"sync"
	"time"

	"cloud-base/atomic"
	"cloud-base/zk"
	"github.com/golang/glog"
	zookeeper "github.com/samuel/go-zookeeper/zk"
)

var (
	zkConn     *zookeeper.Conn
	zkConnOk   atomic.AtomicBoolean
	zkReportCh chan zookeeper.Event
	zkRoot     string

	gPath string
	gMu   sync.Mutex

	gData []byte
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
		err  error
		conn *zookeeper.Conn
	)

	gData = []byte(fmt.Sprintf("%s,MsgTopic", gAddr))

	zkRoot = "/" + rootName

	conn, err = zk.Connect(zkAddrs, 60*time.Second, onConnStatus)
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
	glog.Infof("Start watching zookeeper")

	for event := range zkReportCh {
		glog.Infof("Get zk event: %v", event)
		if event.Type != zookeeper.EventSession {
			break
		}
		if !zkConnOk.Get() || event.State != zookeeper.StateHasSession {
			continue
		}

		gMu.Lock()
		tp := gPath
		gMu.Unlock()
		if len(tp) == 0 {
			continue
		}

		OnlineRmq()
		//gMu.Lock()

		//if gPath == "" {
		//	gMu.Unlock()
		//	continue
		//}
		//tpath, err := createNode()
		//if err != nil {
		//	gMu.Unlock()
		//	glog.Error(err)
		//	break
		//}
		//gPath = tpath
		//gMu.Unlock()
	}
}

func createNode() (string, error) {
	tpath, err := zkConn.Create(zkRoot+"/", gData, zookeeper.FlagEphemeral|zookeeper.FlagSequence, zookeeper.WorldACL(zookeeper.PermAll))
	if err != nil {
		return "", fmt.Errorf("create comet node %s with data %s on zk failed: %v", zkRoot, gData, err)
	}
	return tpath, nil
}

func OnlineRmq() {
	gMu.Lock()
	tpath, err := createNode()
	if err != nil {
		gMu.Unlock()
		glog.Error(err)
		return
	}
	gPath = tpath
	gMu.Unlock()
}

func OfflineRmq() {
	gMu.Lock()
	if len(gPath) != 0 {
		err := zkConn.Delete(gPath, -1)
		if err != nil {
			glog.Errorf("Delete node from zookeeper failed: %v", err)
		}
		gPath = ""
	}
	gMu.Unlock()
}

func CloseZK() {
	if zkConn != nil {
		zkConn.Close()
	}
}
