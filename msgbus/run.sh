#!/bin/bash

Pwd=$(cd "$(dirname "$0")";pwd)

if [ -z $1 ]; then
	source $Pwd/msgbus.conf
else
	source $1
fi

$Pwd/msgbus $args
