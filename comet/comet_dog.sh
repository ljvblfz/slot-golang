#!/bin/sh

Pwd=$(cd "$(dirname "$0")";pwd)

while :
do
	source $Pwd/comet.conf
	[[ -d $Pwd/old ]] || mkdir -p $Pwd/old

	$Pwd/redis-cli -h $onlineRedisIp -p $onlineRedisPort del Host:$cometIp
	$Pwd/redis-cli -h $onlineRedisIp -p $onlineRedisPort publish $offlineHostKey 0\|$cometIp\|0
	if [ -f $Pwd/${cometName}_update ];then
		mv $Pwd/${cometName} $Pwd/old/${cometName}_`date +%Y%m%d-%H%M%S`
		mv $Pwd/${cometName}_update $Pwd/${cometName}
	fi
	GOMAXPROC=2 $Pwd/$cometName -alsologtostderr=true -log_dir=log -ports="$Ports" -rh=$onlineRedisIp:$onlineRedisPort -lip=$cometIp -zks=$Zks -zkroot="$zkRoot" -zkrootc=$zkRootc -v=$GlogV -sh="$StatusServer"
	sleep 3
done

