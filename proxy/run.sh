#!/bin/bash

Pwd=$(cd "$(dirname "$0")";pwd)

if [ -z $1 ]; then
	source $Pwd/proxy.conf
else
	source $1
fi

$Pwd/proxy $args
