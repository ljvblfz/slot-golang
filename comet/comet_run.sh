#!/bin/sh

#GODEBUG='gctrace=1'

redisIp=192.168.2.14
redisPort=6379
cometIp=193.168.1.103

while :
do
	GOMAXPROC=2 ./comet -logtostderr -rh=$redisIp:$redisPort -drh=192.168.2.14:6379 -lip=$cometIp -zks=192.168.2.221,192.168.2.222,192.168.2.223 -zkroot="MsgBusServersGeLi000000" -v=2 -sh=":29998"
	redis-cli -h $redisIp -port $redisPort -c del Host:$cometIp
	sleep 3
done
