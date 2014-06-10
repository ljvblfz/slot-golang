package main

import (
	"bitbucket.org/stevencui/rpc-cluster/log"
	stdlog "log"
	"os"
)

// -------------------------- Global Variant ---------------------------
var (
	Log           log.Logger
	GlobalChannel *ChannelList
	BuildDate     string
	BuildVersion  string
)

func init() {
	Log = log.NewLogger(os.Stdout, "", log.LOGLEVEL_DEBUG, stdlog.LstdFlags|stdlog.Lshortfile)
	GlobalChannel = NewChannelList(128)
}

// -------------------------- Main Function ---------------------------

func main() {
	Log.Infof("BuildDate[%s] Version[%s]\n", BuildDate, BuildVersion)
	InitSignal()
	StartHttp([]string{":1234", ":1235"})
	HandleSignal()
}
