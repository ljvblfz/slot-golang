package main

import (
	"cloud-base/atomic"
	stat "cloud-base/goprocinfo/linux"
	"cloud-base/procinfo"
	"cloud-base/zk"
	"fmt"
	"github.com/golang/glog"
	zookeeper "github.com/samuel/go-zookeeper/zk"
	"strconv"
	"strings"
	"time"
)

var (
	zkConn      *zookeeper.Conn
	zkReportCh  chan zookeeper.Event
	zkConnOk    atomic.AtomicBoolean
	zkProxyRoot string
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

func InitZK(zkAddrs []string, proxyName string, cometName string) {
	if len(proxyName) == 0 {
		glog.Fatalf("[zk] root name for proxy cannot be empty")
	}
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

	zkProxyRoot = "/" + proxyName
	zkCometRoot = "/" + cometName
	err = zk.Create(zkConn, zkProxyRoot)
	if err != nil {
		glog.Infof("[zk] create connection error: %v", err)
	}
	go ReportUsage()

	// timing update the cache of the udp comet stat on zookeeper
	go func() {
		tick := time.NewTicker(inverval)
		for {
			<-tick.C
			getUdpCometStat()
		}
	}()
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

		getUdpCometStat()

		e := <-watch
		glog.Infof("zk receive an event %v", e)
		time.Sleep(time.Second)
	}
}

func getUdpCometStat() {

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
}

func CloseZK() {
	if zkConn != nil {
		zkConn.Close()
	}
}

func ReportUsage() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	cpuChan := procinfo.NewWatcher(time.Second)
	defer close(cpuChan)

	var cpuUsage float64

	proxyPath := make(map[string]proxyStat)

	lastTickerTime := time.Now()

	for {
		select {
		case event := <-zkReportCh:
			glog.Infof("[stat|log] get zk event: %v", event)
			if event.Type != zookeeper.EventSession {
				break
			}
			switch event.State {
			case zookeeper.StateHasSession:
				if !zkConnOk.Get() {
					break
				}
				var urls []string = []string{}
				urls = append(urls, serverAddr)

				for _, u := range urls {
					data := onWriteZkData(u, 0.0, 0, 0, 0)
					tpath, err := zkConn.Create(zkProxyRoot+"/", []byte(data),
						zookeeper.FlagEphemeral|zookeeper.FlagSequence, zookeeper.WorldACL(zookeeper.PermAll))
					if err != nil {
						glog.Errorf("[zk|comet] create comet node %s with data %s on zk failed: %v", zkProxyRoot, data, err)
						break
					}
					if len(tpath) == 0 {
						glog.Errorf("[zk|comet] create empty comet node %s with data %s", zkProxyRoot, data)
						break
					}
					proxyPath[tpath] = proxyStat{Ok: true, Url: u, Path: tpath}
				}

			case zookeeper.StateDisconnected:
				fallthrough
			case zookeeper.StateExpired:
				proxyPath = make(map[string]proxyStat)
			}

		case cpu, ok := <-cpuChan:
			if !ok {
				cpuUsage = 0.0
				glog.Errorf("[stat|usage] can't get cpu usage from |cpuChan|")
				break
			}
			if cpu.Error != nil {
				cpuUsage = 0.0
				glog.Errorf("[stat|usage] error on get cpu info: %v", cpu.Error)
				break
			}
			cpuUsage = cpu.Usage
			//glog.Infof("[stat|log] get cpu: %f", cpu.Usage)

		case t := <-ticker.C:
			if t.Sub(lastTickerTime) > time.Second*3 {
				glog.Warningf("[stat|ticker] ticker happened too late, %v after last time", t.Sub(lastTickerTime))
			}
			lastTickerTime = t
			var memTotal uint64
			var memUsage uint64
			meminfo, err := stat.ReadMemInfo("/proc/meminfo")
			if err == nil {
				memTotal = meminfo["MemTotal"]
				memUsage = memTotal - meminfo["MemFree"]
			} else {
				glog.Errorf("[stat|usage] get meminfo from /proc/meminfo failed: %v", err)
			}
			onlineCount := statGetConnOnline()

			for path, s := range proxyPath {
				if !s.Ok {
					continue
				}
				if !zkConnOk.Get() {
					glog.Warning("[zk] write zk but conn was broken")
					continue
				}
				data := onWriteZkData(s.Url, cpuUsage, memTotal, memUsage, onlineCount)
				_, err := zkConn.Set(path, []byte(data), -1)
				if err != nil {
					glog.Errorf("[zk|comet] set zk node [%s] with comet's status [%s] failed: %v", s.Path, data, err)
				}
			}
		}
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

func onWriteZkData(url string, cpu float64, memTotal uint64, memUsage uint64, online uint64) string {
	return fmt.Sprintf("%s,%f,%d,%d,%d", url, cpu, memTotal, memUsage, online)
}
