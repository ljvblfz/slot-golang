#!/bin/bash

for x in {13..13}
do
	scp msgbus root@192.168.2.$x:/root/bin/msgbus_update
	ssh root@192.168.2.$x killall msgbus
done
