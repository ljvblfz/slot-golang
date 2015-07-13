package main

import (
	"cloud-base/status"
	"encoding/json"
	"net/http"
	"time"
)

const (
	kUpStreamIn     = "UpStreamIn"
	kUpStreamInPS1s = "UpStreamInPerSecond1s"
	kUpStreamInPS1m = "UpStreamInPerSecond1m"
	kUpStreamInPS5m = "UpStreamInPerSecond5m"

	kDownStreamOut     = "DownStreamOut"
	kDownStreamOutBad  = "DownStreamOutBad"
	kDownStreamOutPS1s = "DownStreamOutPerSecond1s"
	kDownStreamOutPS1m = "DownStreamOutPerSecond1m"
	kDownStreamOutPS5m = "DownStreamOutPerSecond5m"

	kMsgToRmq = "MsgsToMq"

	kCometCount = "CometCount"
	kRmqCount   = "RmqCount"
)

func InitStat(addr string) {
	status.AppStat.Add(kUpStreamIn)
	status.AppStat.Add(kUpStreamInPS1s)
	status.AppStat.Add(kUpStreamInPS1m)
	status.AppStat.Add(kUpStreamInPS5m)

	//status.AppStat.Add(kDownStreamIn)
	status.AppStat.Add(kDownStreamOut)
	status.AppStat.Add(kDownStreamOutBad)
	status.AppStat.Add(kDownStreamOutPS1s)
	status.AppStat.Add(kDownStreamOutPS1m)
	status.AppStat.Add(kDownStreamOutPS5m)

	status.AppStat.Add(kCometCount)
	status.AppStat.Add(kRmqCount)

	status.AppStat.Add(kMsgToRmq)

	go statUpdatePerSecond()

	mux := http.NewServeMux()
	mux.HandleFunc("/usermap", handleUsermap)
	status.InitStat(addr, mux)
}

func handleUsermap(resp http.ResponseWriter, req *http.Request) {
	usermap := GUserMap.GetAll()
	data, err := json.MarshalIndent(usermap, "", "\t")
	if err != nil {
		resp.WriteHeader(500)
		return
	}
	resp.Header().Add("Content-Type", "application/json")
	resp.Write(data)
}

func statIncUpStreamIn() {
	status.AppStat.Inc(kUpStreamIn)
}

func statIncDownStreamOut() {
	status.AppStat.Inc(kDownStreamOut)
}

func statIncDownStreamOutBad() {
	status.AppStat.Inc(kDownStreamOutBad)
}

func statIncCometConns() {
	status.AppStat.Inc(kCometCount)
}

func statDecCometConns() {
	status.AppStat.Dec(kCometCount)
}

func statIncRmqCount() {
	status.AppStat.Inc(kRmqCount)
}

func statDecRmqCount() {
	status.AppStat.Dec(kRmqCount)
}

func statIncMsgToRmq() {
	status.AppStat.Inc(kMsgToRmq)
}

func statUpdatePerSecond() {

	ticker := time.Tick(time.Second)

	var n1s uint64 = 0

	lastU1s := status.AppStat.Get(kUpStreamIn)
	lastU1m := lastU1s
	lastU5m := lastU1s

	lastD1s := status.AppStat.Get(kDownStreamOut)
	lastD1m := lastD1s
	lastD5m := lastD1s

	for {
		<-ticker
		n1s++

		// upstream
		uTotal := status.AppStat.Get(kUpStreamIn)

		status.AppStat.Set(kUpStreamInPS1s, uTotal-lastU1s)
		lastU1s = uTotal

		if n1s%60 == 0 {
			status.AppStat.Set(kUpStreamInPS1m, (uTotal-lastU1m)/60)
			lastU1m = uTotal
		}

		if n1s%300 == 0 {
			status.AppStat.Set(kUpStreamInPS5m, (uTotal-lastU5m)/300)
			lastU5m = uTotal
		}

		// downstream
		dTotal := status.AppStat.Get(kDownStreamOut)

		status.AppStat.Set(kDownStreamOutPS1s, dTotal-lastD1s)
		lastD1s = dTotal

		if n1s%60 == 0 {
			status.AppStat.Set(kDownStreamOutPS1m, (dTotal-lastD1m)/60)
			lastD1m = dTotal
		}

		if n1s%300 == 0 {
			status.AppStat.Set(kDownStreamOutPS5m, (dTotal-lastD5m)/300)
			lastD5m = dTotal
		}
	}
}
