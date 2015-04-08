# Rmqkeeper

RabbitMQ守护程序，通过轮询调用RabbitMQ的心跳测试HTTP接口,检查其是否在线，支持远程检查。

## 安装方法

进入rmqkeeper目录编译:
go build

查看启动参数说明:
./rmqkeeper -help

运行启动命令:
nohup ./rmqkeeper -h 192.168.2.225:5672 -url http://guest:guest@192.168.2.225:15672 -vh="/" -zks 192.168.2.221 -zkroot Rabbitmq -logtostderr=true -d 1s > rmqkeeper.log 2>&1 &
