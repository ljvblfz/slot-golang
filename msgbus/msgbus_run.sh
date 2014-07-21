#!/bin/sh

#GODEBUG='gctrace=1'
GOMAXPROCS=2 ./msgbus -logtostderr=true -rh=193.168.1.103:6379 -addr=193.168.1.103:9923 -zks="192.168.2.221,192.168.2.222,192.168.2.223" -zkroot="MsgBusServersGeLi" -v=2
