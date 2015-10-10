package main

import (
	"cloud-base/atomic"
	//	stat "cloud-base/goprocinfo/linux"
	//	"cloud-base/procinfo"
	"cloud-base/zk"
	//	"fmt"
	"github.com/golang/glog"
	zookeeper "github.com/samuel/go-zookeeper/zk"
	"strconv"
	"strings"
	"time"
)

var (
	zkConn     *zookeeper.Conn
	zkReportCh chan zookeeper.Event
	zkConnOk   atomic.AtomicBoolean
	//	zkProxyRoot string
	zkCometRoot string
	zkNodes     []string
	gQueue      *Queue // the data get from zookeeper
)

type (
	proxyStat struct {
		Ok   bool
		Path string
		Url  string
	}
)

func init() {
	gQueue = NewQueue()
	zkReportCh = make(chan zookeeper.Event, 1)
	zkConnOk.Set(false)
}

func InitZK(zkAddrs []string, cometName string) {
	if len(cometName) == 0 {
		glog.Fatalf("[zk] root name for comet cannot be empty")
	}
	var (
		err  error
		conn *zookeeper.Conn
	)
	conn, err = zk.Connect(zkAddrs, 60*time.Second, onConnStatus)
	if err != nil {
		glog.Fatal(err)
	}
	zkConn = conn

	zkCometRoot = "/" + cometName

	// timing update the cache of the udp comet stat on zookeeper
	//	go func() {
	//		tick := time.NewTicker(inverval)
	//		for {
	//			<-tick.C
	//			updatingCometQue()
	//		}
	//	}()
	UdpCometStat(zkAddrs)
}

func UdpCometStat(zkAddrs []string) {
	glog.Infof("Connect zk[%v] with udp comet root [%s] OK!", zkAddrs, zkCometRoot)
	var (
		lastErr, err error
		watch        <-chan zookeeper.Event
	)
	for {
		zkNodes, watch, err = zk.GetNodesW(zkConn, zkCometRoot)
		if err != lastErr {
			if err != nil {
				glog.Errorln(err)
			}
			lastErr = err
		}
		if err == zookeeper.ErrNoNode || err == zookeeper.ErrNoChildrenForEphemerals {
			time.Sleep(time.Second)
			continue
		} else if err != nil {
			time.Sleep(time.Second)
			continue
		}

		updatingCometQue()

		e := <-watch
		glog.Infof("zk receive an event %v", e)
		time.Sleep(time.Second)
	}
}

func updatingCometQue() {
	var preRetrain []interface{}
	for _, childzode := range zkNodes {
		zdata, err := zk.GetNodeData(zkConn, zkCometRoot+"/"+childzode)
		if err != nil {
			glog.Errorf("[%s] cannot get", zdata)
			continue
		}

		items := strings.Split(zdata, ",")
		if len(items) == 5 {
			cpuUsage, errT := strconv.ParseFloat(items[1], 64)
			if errT != nil {
				glog.Errorf("get cpu usage err:%v", errT)
			}
			menTotal, errT := strconv.ParseFloat(items[2], 64)
			if errT != nil {
				glog.Errorf("get total memory err:%v", errT)
			}
			memUsage, errT := strconv.ParseFloat(items[3], 64)
			if errT != nil {
				glog.Errorf("get  memory usage err:%v", errT)
			}
			onlineCount, errT := strconv.ParseInt(items[4], 10, 64)
			if errT != nil {
				glog.Errorf("get  online count err:%v", errT)
			}
			preRetrain = append(preRetrain, items[0])
			if cpuUsage < gCPUUsage && menTotal-memUsage > gMEMFree && onlineCount < gCount {
				if !gQueue.contains(items[0]) {
					gQueue.Put(items[0]) //如果满足条件并且队列中没有，则入队
				}
			} else {
				if gQueue.contains(items[0]) {
					gQueue.Remove(items[0]) //如果不满足条件并且队列有，则移除
				}
				glog.Errorf("no suitable server can use %v", zdata)
			}
		}
	}
	//如果znode中没有（说明此udpcomet节点失败），而队列里有则移除
	gQueue.Retrain(preRetrain)
}

func CloseZK() {
	if zkConn != nil {
		zkConn.Close()
	}
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
