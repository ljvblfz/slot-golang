package main

import (
	"cloud/status"
	"time"
)

const (
	kConnTotal	= "ConnTotal"
	kConnOnline	= "ConnOnline"

	kUpStreamIn		= "UpStreamIn"
	kUpStreamOut	= "UpStreamOut"
	kUpStreamOutPS1s	= "UpStreamOutPerSecond1s"
	kUpStreamOutPS1m	= "UpStreamOutPerSecond1m"
	kUpStreamOutPS5m	= "UpStreamOutPerSecond5m"

	kDownStreamIn		= "DownStreamIn"
	kDownStreamOut		= "DownStreamOut"
	kDownStreamOutPS1s	= "DownStreamOutPerSecond1s"
	kDownStreamOutPS1m	= "DownStreamOutPerSecond1m"
	kDownStreamOutPS5m	= "DownStreamOutPerSecond5m"
)

func InitStat(addr string) {
	status.AppStat.Add(kConnTotal)
	status.AppStat.Add(kConnOnline)

	status.AppStat.Add(kUpStreamIn)
	status.AppStat.Add(kUpStreamOut)
	status.AppStat.Add(kUpStreamOutPS1s)
	status.AppStat.Add(kUpStreamOutPS1m)
	status.AppStat.Add(kUpStreamOutPS5m)

	status.AppStat.Add(kDownStreamIn)
	status.AppStat.Add(kDownStreamOut)
	status.AppStat.Add(kDownStreamOutPS1s)
	status.AppStat.Add(kDownStreamOutPS1m)
	status.AppStat.Add(kDownStreamOutPS5m)

	go statUpdatePerSecond()

	status.InitStat(addr)
}

func statIncConnTotal() {
	status.AppStat.Inc(kConnTotal)
}

func statIncConnOnline() {
	status.AppStat.Inc(kConnOnline)
}

func statDecConnOnline() {
	status.AppStat.Dec(kConnOnline)
}

func statIncUpStreamIn() {
	status.AppStat.Inc(kUpStreamIn)
}

func statIncUpStreamOut() {
	status.AppStat.Inc(kUpStreamOut)
}

func statIncDownStreamIn() {
	status.AppStat.Inc(kDownStreamIn)
}

func statIncDownStreamOut() {
	status.AppStat.Inc(kDownStreamOut)
}

func statUpdatePerSecond() {

	ticker := time.Tick(time.Second)

	var n1s uint64 = 0

	lastU1s := status.AppStat.Get(kUpStreamOut)
	lastU1m := lastU1s
	lastU5m := lastU1s

	lastD1s := status.AppStat.Get(kDownStreamOut)
	lastD1m := lastD1s
	lastD5m := lastD1s

	for {
		<-ticker
		n1s++

		// upstream
		uTotal := status.AppStat.Get(kUpStreamOut)

		status.AppStat.Set(kUpStreamOutPS1s, uTotal - lastU1s)
		lastU1s = uTotal

		if n1s % 60 == 0 {
			status.AppStat.Set(kUpStreamOutPS1m, (uTotal - lastU1m) / 60)
			lastU1m = uTotal
		}

		if n1s % 300 == 0 {
			status.AppStat.Set(kUpStreamOutPS5m, (uTotal - lastU5m) / 300)
			lastU5m = uTotal
		}

		// downstream
		dTotal := status.AppStat.Get(kDownStreamOut)

		status.AppStat.Set(kDownStreamOutPS1s, dTotal - lastD1s)
		lastD1s = dTotal

		if n1s % 60 == 0 {
			status.AppStat.Set(kDownStreamOutPS1m, (dTotal - lastD1m) / 60)
			lastD1m = dTotal
		}

		if n1s % 300 == 0 {
			status.AppStat.Set(kDownStreamOutPS5m, (dTotal - lastD5m) / 300)
			lastD5m = dTotal
		}
	}
}
