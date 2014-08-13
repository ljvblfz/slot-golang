#!/bin/bash

for x in {13..13}
do
	ssh root@192.168.2.$x killall msgbus
	scp msgbus root@192.168.2.$x:/root/bin/msgbus
	ssh root@192.168.2.$x 'cd /root/bin && ./msgbus_run.sh'
done
