package main

import (
	"fmt"
	"time"

	"cloud-base/atomic"
	stat "cloud-base/goprocinfo/linux"
	"cloud-base/procinfo"
	"cloud-base/zk"
	"github.com/golang/glog"
	zookeeper "github.com/samuel/go-zookeeper/zk"
)

var (
	zkConn   *zookeeper.Conn
	zkConnOk atomic.AtomicBoolean
	//zkReportCh	chan cometStat
	zkReportCh  chan zookeeper.Event
	zkCometRoot string
)

type cometStat struct {
	Ok   bool
	Path string
	Url  string
}

func init() {
	zkReportCh = make(chan zookeeper.Event, 1)
	zkConnOk.Set(false)
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

func InitZK(zkAddrs []string, msgbusName string, cometName string) {
	if len(msgbusName) == 0 {
		glog.Fatalf("[zk] root name for msgbus cannot be empty")
	}
	if len(cometName) == 0 {
		glog.Fatalf("[zk] root name for comet cannot be empty")
	}
	var (
		nodes []string
		err   error
		conn  *zookeeper.Conn
		addr  string
		watch <-chan zookeeper.Event
	)
	conn, err = zk.Connect(zkAddrs, 60*time.Second, onConnStatus)
	if err != nil {
		glog.Fatal(err)
	}
	zkConn = conn

	zkCometRoot = "/" + cometName

	err = zk.Create(zkConn, zkCometRoot)
	if err != nil {
		glog.Infof("[zk] create connection error: %v", err)
	}
	go ReportUsage()

	zkMsgBusRoot := "/" + msgbusName
	glog.Infof("Connect zk[%v] with msgbus root [%s] OK!", zkAddrs, zkMsgBusRoot)

	var lastErr error
	for {
		nodes, watch, err = zk.GetNodesW(conn, zkMsgBusRoot)
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
		var addrs []string = make([]string, 0, len(nodes))
		for _, n := range nodes {
			addr, err = zk.GetNodeData(conn, zkMsgBusRoot+"/"+n)
			if err != nil {
				glog.Errorf("[%s] cannot get", addr)
				continue
			}
			addrs = append(addrs, addr)
		}
		for _, addr := range addrs {
			GMsgBusManager.Online(addr)
		}
		e := <-watch
		glog.Infof("zk receive an event %v", e)
		time.Sleep(time.Second)
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

	cometPath := make(map[string]cometStat)

	lastTickerTime := time.Now()
	//glog.Infof("[stat|log] loop started %v", lastTickerTime)
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
				var urls []string
				if gCometType == CometWs {
					urls = GetCometWsUrl()
				} else if gCometType == CometUdp {
					urls = append(urls, GetCometUdpUrl())
				}
				for _, u := range urls {
					data := onWriteZkData(u, 0.0, 0, 0, 0)
					tpath, err := zkConn.Create(zkCometRoot+"/", []byte(data),
						zookeeper.FlagEphemeral|zookeeper.FlagSequence, zookeeper.WorldACL(zookeeper.PermAll))
					if err != nil {
						glog.Errorf("[zk|comet] create comet node %s with data %s on zk failed: %v", zkCometRoot, data, err)
						break
					}
					if len(tpath) == 0 {
						glog.Errorf("[zk|comet] create empty comet node %s with data %s", zkCometRoot, data)
						break
					}
					cometPath[tpath] = cometStat{Ok: true, Url: u, Path: tpath}
				}

			case zookeeper.StateDisconnected:
				fallthrough
			case zookeeper.StateExpired:
				cometPath = make(map[string]cometStat)
			}

		//case s := <-zkReportCh:
		//	if len(s.Path) > 0 {
		//		cometPath[s.Path] = s
		//		//glog.Infof("[stat|log] get comet: %v", s)
		//	} else {
		//		cometPath = make(map[string]cometStat)
		//	}

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

			for path, s := range cometPath {
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
				//glog.Infof("[stat|log] write zk path: %s, data: %s", s.Path, data)
			}
		}
	}
}

func onWriteZkData(url string, cpu float64, memTotal uint64, memUsage uint64, online uint64) string {
	return fmt.Sprintf("%s,%f,%d,%d,%d", url, cpu, memTotal, memUsage, online)
}
