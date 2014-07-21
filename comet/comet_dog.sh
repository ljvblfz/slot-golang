#!/bin/sh

onlineRedisIp=192.168.2.27
onlineRedisPort=6379
cometIp=192.168.2.15
cometName=cometgl
Pwd=$(cd "$(dirname "$0")";pwd)

#向redis广播comet服务器整体下线的频道名
offlineHostKey=PubKey

while :
do
	GOMAXPROC=2 $Pwd/$cometName -alsologtostderr=false -log_dir=log -ports=":1234" -rh=$onlineRedisIp:$onlineRedisPort -drh=192.168.2.29:6379 -lip=$cometIp -zks=192.168.2.221,192.168.2.222,192.168.2.223 -zkroot="MsgBusServersGeLi_qa" -v=2 -sh=":29989"
	$Pwd/redis-cli -h $onlineRedisIp -p $onlineRedisPort del Host:$cometIp
	$Pwd/redis-cli -h $onlineRedisIp -p $onlineRedisPort publish $offlineHostKey 0\|$cometIp\|0
	if [ -f $Pwd/${cometName}_update ];then
		mv $Pwd/${cometName} ${cometName}_`date +%Y%m%d-%H%M%S`
		mv $Pwd/${cometName}_update $Pwd/${cometName}
	fi
	sleep 3
done

