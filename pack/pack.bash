#!/bin/bash

if [ -z $1 ]; then
	echo "Usage: $0 new-version"
	exit
fi

Pwd=$(cd "$(dirname "$0")";pwd)
Ver=$1
PackDir=(comet msgbus rmqkeeper)
DirSize=${#PackDir[@]}
GitBranch=master

mkdir -p src
cd src

if [ -d cloud ]; then
	echo "Pull repository"
	cd cloud
	git pull
else
	echo "Clone new repository"
	git clone git@192.168.0.231:serverside/cloud.git
	cd cloud
fi

for((i=0;i<DirSize;i++))
do
	[[ -d $Pwd/${PackDir[i]} ]] || mkdir $Pwd/${PackDir[i]}
done

git checkout $GitBranch

# modify version
mkdir -p $Pwd/src/cloud/ver
echo -e "package ver\n\nvar Version = \"$Ver\"" > $Pwd/src/cloud/ver/ver.go
git add $Pwd/src/cloud/ver/ver.go
git commit -m "modify version to $Ver"

echo "Packing branch $GitBranch"

echo "Type in log for v$Ver:"
TagName=socket-v$Ver
git tag -a $TagName
git push origin $TagName

for((i=0;i<DirSize;i++))
do
	echo "Building ${PackDir[i]} ..."
	cd $Pwd/src/cloud/${PackDir[i]}
	GOPATH=$Pwd:$GOPATH go build -a && cp ${PackDir[i]} $Pwd/${PackDir[i]}
done

echo "Zipping $Ver"
AllDir=""
for((i=0;i<DirSize;i++))
do
	AllDir="$AllDir ${PackDir[i]}"
done

cd $PwdDir
[[ -d pack ]] || mkdir pack
zip -rq pack/powersocket-v"$Ver"-`date +%Y%m%d`.zip $AllDir

