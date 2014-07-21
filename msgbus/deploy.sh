#!/bin/bash

for x in {28..28}
do
	ssh root@192.168.2.$x killall msgbusgl
	scp msgbus root@192.168.2.$x:/root/bin_gl/msgbusgl
	ssh root@192.168.2.$x 'cd /root/bin_gl && ./msgbus_run.sh'
done
