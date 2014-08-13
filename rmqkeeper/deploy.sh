#!/bin/sh

ssh root@192.168.2.225 killall beam
scp rmqkeeper run.sh root@192.168.2.225:/root/rmqkeeper
ssh root@192.168.2.225 'cd /root/rmqkeeper; nohup ./run.sh > rmqkeeper.out 2>&1 &'
