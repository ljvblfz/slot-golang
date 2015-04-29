#!/bin/sh

if [ -z $1 ]; then
	echo "Usage: $0 <new-version>

<new-version> can be find in cloud-socket/ver/ver.go
"
		
	exit
fi

Ver=$1
Pwd=$(cd "$(dirname "$0")";pwd)
PackageName=cloudsocket-"$Ver".tar.gz

mkdir -p cloudsocket/comet
mkdir -p cloudsocket/msgbus
mkdir -p cloudsocket/rmqkeeper

echo "Building comet..."
cd $Pwd/comet
go build
cp comet run.sh comet.conf $Pwd/cloudsocket/comet/

echo "Building msgbus..."
cd $Pwd/msgbus
go build
cp msgbus run.sh msgbus.conf $Pwd/cloudsocket/msgbus/

echo "Building rmqkeeper..."
cd $Pwd/../cloud-base/rmqkeeper
go build
cp rmqkeeper run.sh rmqkeeper.conf $Pwd/cloudsocket/rmqkeeper

echo "Packing to $PackageName..."
cd $Pwd
tar zcf $PackageName cloudsocket

echo "All done"
