#!/bin/bash

Pwd=$(cd "$(dirname "$0")";pwd)

while :
do
	source $Pwd/msgbus.conf

	echo "Checking redis $onlineRedisIp:$onlineRedisPort ..."
	while :
	do
		$Pwd/redis-cli -h $onlineRedisIp -p $onlineRedisPort PING > /dev/null 2>&1 && break
		sleep 3
	done
	echo "Redis ok"

	[[ -d $Pwd/old ]] || mkdir -p $Pwd/old
	if [ -f $Pwd/${msgbusName}_update ];then
		mv $Pwd/${msgbusName} $Pwd/old/${msgbusName}_`date +%Y%m%d-%H%M%S`
		mv $Pwd/${msgbusName}_update $Pwd/${msgbusName}
		echo "Updated program"
	fi
	GOMAXPROCS=2 $Pwd/$msgbusName -alsologtostderr=$GlogToStderr -rh=$onlineRedisIp:$onlineRedisPort -addr=$msgbusIp:$msgbusPort -zks=$Zks -zkroot=$zkRoot -v=$GlogV
	sleep 3
done

