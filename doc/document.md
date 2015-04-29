# 工作交接 - 程序

以下是我负责的所有软件开发工作内容。

## cloud-base

cloud-base是一个go语言的基础库，包含通用的工具代码库，工作包一部分为自己开发，另一部分是依赖的第三方库，但为了版本稳定，将用到第三方库的对应版本提交至这里，这些库在原官方库的地址及版本记录在这些目录下的git-version文件中（部分库的原版本已无法查到）。

* atomic，封装标准库的原子操作为类型；
* crypto，一些通用的加密解密函数，有些已不使用；
* goprocinfo，获取linux系统的CPU和内存信息；
* hlist，链表数据结构，由原作者从linux内核的链表数据结构重写为该go版本；
* procinfo，获取linux系统的CPU和内存信息，是对goprocinfo的部分封装；
* rmqkeeper，可执行程序，RabbitMQ的守护程序，RabbitMQ启动状态下注册至Zookeeper中的某节点下，RabbitMQ停止时从Zookeeper中删除；
* rmqs，对github.com/streadway/amqp的简单封装，只包括向RabbitMQ推送消息的调用；
* sentinels，对Redis客户端库github.com/fzzy/radix/sentinel的封装，以支持对sentinel集群的调用；
* status，提供对程序内部进行监控的库，包括添加、增加计数器、HTTP接口；
* util，一些零散的通用函数调用，包括提供CPU、内存进行性能统计的调用
* websocket，对官方golang.org/x/net/websocket的修改，支持对消息包的可选mask，并修改服务器不能正确回应websocket协议包心跳的BUG；
* youtube-go，由于github.com/youtube/vitess/go最新版本的接口已改变，不兼容，所以摘取其一个历史版本；
* zk，对github.com/samuel/go-zookeeper/zk的封装。

## 插座项目代理服务器

代理服务器适用于向手机客户端和自有智能设备（板子中的嵌入式客户端）提供Websocket或UDP连接的代理程序，该代理服务器负责提供代理连接、消息推送、UDP/HTTP协议转换、UDP终端在线管理功能。

架构图:
![架构图](architecture.jpeg)

### 部署文档

#### 安装Zookeeper服务器集群，得到IP地址列表ZkIPs，逗号分割的多个IP地址；

#### 安装RabbitMQ，可安装N个单独的RabbitMQ，并为每个RabbitMQ安装cloud-base/rmqkeeper守护程序，并运行：

    nohup ./rmqkeeper -h 192.168.2.225:5672 -url http://guest:guest@192.168.2.225:15672 -vh="/" -zks ZkIPs -zkroot Rabbitmq -logtostderr=true -d 1s &> rmqkeeper.log

参数：
* -h RabbitMQ服务器对外提供服务的IP:Port
* -url RabbitMQ提供监控的HTTP服务地址
* -vh RabbitMQ的虚拟主机名，使用默认的"/"
* -zks 步骤1中得到的Zookeeper服务器地址列表
* -zkroot 在RabbitMQ服务器启动时，将该服务注册至Zookeeper集群中的该节点下
* -logtostderr 将日志写至标准输出
* -d 检测RabbitMQ服务是否可用的时间间隔

#### 安装Redis服务器，得到IP地址RedisAddr，内容为IP:Port；

#### 安装msgbus服务器，可单物理服务器安装多个msgbus服务进程；

#### 安装comet服务器。

### 开发文档

代理服务器包含2个程序主要程序和1个辅助程序：

1. comet代理连接服务器，包括websocket连接的管理和代理，UDP消息的代理，并为系统提供设备的上下线事件的触发；
2. msgbus消息总线服务器，负责转发websocket消息过程中的消息路由，并对“上传类型”的消息存储至RabbitMQ；
3. rmqkeeper消息队列服务器RabbitMQ的守护进程；

#### websocket连接登录流程

![websocket登录](wsonline.jpeg)

#### websocket转发消息流程

![websocket转发](wsmsg.jpeg)

#### websocket连接退出流程

![websocket登出](wsoffline.jpeg)

#### UDP消息处理流程

![设备UDP通讯流程图](udpflowchart.jpeg)

#### 代理服务器推送消息流程

需要代理服务器推送消息至用户手机或设备时，流程图如下：

![代理服务器推送消息流程](pushmsg.jpeg)

#### RabbitMQ设备消息流程

![rmqflowchart](rmqflowchart.jpeg)

### 备忘

## 格力空调项目代理服务器

格力项目代理服务器是插座项目代理服务器的2014年6月至8月期间的一个分支，与插座项目有少量的不同：
1. 终端的ID为32位数值；
2. 手机ID为负数，设备ID为整数；
3. 支持同一ID多点登录；
4. 目前未部署RabbitMQ，但msgbus中已包含推送消息至RabbitMQ的代码；
5. 不包含UDP内容；
6. 2014年10月左右，对Comet和Msgbus做过重构，为两个程序中增加多种缓冲队列以提高吞吐量，这些修改未合并至插座项目的代理服务器；

