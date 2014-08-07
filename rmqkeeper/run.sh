#!/bin/bash

echo "Stop old rabbitmq..."
service rabbitmq-server stop

echo "Starting new rabbitmq..."
./rmqkeeper -h 192.168.2.225 -p 5672 -zks 192.168.2.221 -zkroot Rabbitmq -logtostderr=true
