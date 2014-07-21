#!/bin/bash

for x in {15,16,17,18}
do
	scp comet root@192.168.2.$x:/root/bin_gl/cometgl_update
	ssh root@192.168.2.$x killall cometgl
done
