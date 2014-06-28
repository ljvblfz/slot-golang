package main

import (
	"cloud/status"
)

const (
	kConnTotal	= "ConnTotal"
	kConnOnline	= "ConnOnline"

	kUpStreamIn		= "UpStreamIn"
	kUpStreamOut	= "UpStreamOut"

	kDownStreamIn	= "DownStreamIn"
	kDownStreamOut	= "DownStreamOut"
)

func InitStat(addr string) {
	status.AppStat.Add(kConnTotal)
	status.AppStat.Add(kConnOnline)
	status.AppStat.Add(kUpStreamIn)
	status.AppStat.Add(kUpStreamOut)
	status.AppStat.Add(kDownStreamIn)
	status.AppStat.Add(kDownStreamOut)

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
