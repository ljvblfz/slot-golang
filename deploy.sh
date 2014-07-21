#!/bin/bash

for y in {28,30}
do
	echo "Stopping msgbus 192.168.2.$y ..."
	ssh root@192.168.2.$y 'killall msgbusgl'
	scp msgbus/msgbus root@192.168.2.$y:/root/bin_gl/msgbusgl
done

for x in {15,16,17,18,24,25,26,27}
do
	echo "Updating comet 192.168.2.$x ..."
	ssh root@192.168.2.$x 'kill -9 `pgrep cometgl` `pgrep comet_dog`'
	redis-cli -h 192.168.2.27 del Host:192.168.2.$x
	redis-cli -h 192.168.2.224 del Host:192.168.2.$x
	scp comet/comet root@192.168.2.$x:/root/bin_gl/cometgl_update
	scp comet/comet_dog.sh root@192.168.2.$x:/root/bin_gl
	echo "Restarting comet 192.168.2.$x ..."
	ssh root@192.168.2.$x 'cd /root/bin_gl && ./comet_run.sh'
done

for y in {28,30}
do
	echo "Restarting msgbus 192.168.2.$y ..."
	ssh root@192.168.2.$y 'cd /root/bin_gl && ./msgbus_run.sh'
done
