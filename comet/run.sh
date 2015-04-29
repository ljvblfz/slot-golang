#!/bin/bash

Pwd=$(cd "$(dirname "$0")";pwd)

if [ -z $1 ]; then
	source $Pwd/comet.conf
else
	source $1
fi

$Pwd/comet $args
