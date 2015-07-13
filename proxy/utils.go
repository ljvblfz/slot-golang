package main

import (
	"fmt"
	"strconv"
	"strings"
)

func selectUDPServer() (addr string, err error) {
	for _, itemsStr := range zkData {
		items := strings.Split(itemsStr, ",")
		if len(items) == 5 {
			cpuUsage, errT := strconv.ParseFloat(items[1], 64)
			if errT != nil {
				err = fmt.Errorf("get cpu usage err:%v", errT)
				return
			}
			menTotal, errT := strconv.ParseFloat(items[2], 64)
			if errT != nil {
				err = fmt.Errorf("get total memory err:%v", errT)
				return
			}
			memUsage, errT := strconv.ParseFloat(items[3], 64)
			if errT != nil {
				err = fmt.Errorf("get  memory usage err:%v", errT)
				return
			}
			onlineCount, errT := strconv.ParseInt(items[4], 10, 64)
			if errT != nil {
				err = fmt.Errorf("get  online count err:%v", errT)
				return
			}
			if cpuUsage < gCPUUsage && menTotal-memUsage > gMEMFree && onlineCount < gCount {
				addr = items[0]
			} else {
				err = fmt.Errorf("no suitable server can use")
			}
			return
		}
	}
	return
}
