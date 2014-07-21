#!/bin/bash

for x in {15,16,17,18,24,25,26,27}
do
	#scp stop.sh restart.sh root@192.168.2.$x:/root/bin_gl
	#ssh root@192.168.2.$x 'cd /root/bin_gl && chmod +x stop.sh && chmod +x restart.sh'
	ssh root@192.168.2.$x 'cd /root/bin_gl && ./restart.sh'
done
