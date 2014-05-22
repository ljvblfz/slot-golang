#NameSrv设计说明
---

##NameSrv解决了什么问题

NameServer目的是为了保存用户或者设备的在线状态，以及用户或者设备所在服务器的IP,当然还可以做一些在线状态的查询

---

##NameSrv的一些概念

1. Name Server实际的状态存储依赖Redis
2. 本身Name Server是可以无限扩展的，但根据Redis的能力，其实集群中，3台在50万QPS基本满足
3. Name Server不光存储了设备的在线信息，还存储了用户的在线信息，如果数量众多可以将其分离
4. Name Server只是负责写入Redis
5. 关于是否做Sharding，如果并发请求太高，会考虑加入Sharding，或者一开始单机上做虚拟Sharding
6. Master-Slave可以通过Redis Sentinel来做
7. NameSrv如果压力过大，可以考虑多台负载均衡

##NameSrv的设计

1. TCP协议，长连接，心跳检测
2. 连接后的第一条协议，表明自己的身份，以及自己服务器开放的端口等等。
3. 某台Comet服务器掉线后，应当清除所有该服务器在线用户或者设备的状态（这个过程应该采用lua脚本来做)
4. Comet跟NameServer之间心跳超时，则NameServer踢下线改Comet，并删除所有该Comet服务器的在线用户状态
5. 某个用户登陆到Comet后，通知NameServer，NameServer写入Redis
6. NameServer只负责Redis的写入
7. KeepAlive保证NameServer的HA
8. 设备是不允许多点登陆的，所以设备的状态表只有一个
9. 协议设计可以参考Memcached或者Redis, 文本化
10. 支持多个端口监听

##NameSrv的Redis数据结构
#### 用户状态

* 用户所在服务器表: User:$UID -> {$SID, $IP}
* 用户服务器状态表: User:$IP -> [$SID]

#### 设备状态

* 设备所在服务器表: Device -> {$UID, $IP}

##NameSrv的监控和查询信息

1. 多少次用户上线
2. 多少次用户下线
3. 当前所有用户在线数量（当NameSrv只有单台的时候，可以写入内存，当多台时应该查询Redis)
4. 某台机器的用户在线数量
5. Comet掉线的次数
6. 当前连接了多少台服务器
7. 机器状态和版本和配置文件信息
8. gc的状态和routine信息