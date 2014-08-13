#!/bin/bash

for x in {14..14}
do
	#scp comet.conf root@192.168.2.$x:/root/bin
	scp comet root@192.168.2.$x:/root/bin/comet_update
	ssh root@192.168.2.$x killall comet
done
