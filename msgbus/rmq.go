package main

import (
	"fmt"
	"strconv"
	"sync"

	"cloud/hlist"

	"github.com/golang/glog"
	"github.com/streadway/amqp"
)

var (
	GRmqs = &Rmqs{
		servers:	hlist.New(),
		curr:		nil,
		mu:			sync.Mutex{},
	}
)

type Rmqs struct {
	servers	*hlist.Hlist
	curr	*hlist.Element
	mu		sync.Mutex
}

func (this *Rmqs) Add(addr string, exchangeName string) {
	this.mu.Lock()
	defer this.mu.Unlock()

	for e := this.servers.Front(); e != nil; e = e.Next() {
		s, _ := e.Value.(*rmqServer)
		if s.addr == addr {
			glog.Warningf("[rmq] repeat online server %s", addr)
			return
		}
	}

	url := fmt.Sprintf("amqp://%s/", addr)
	conn, err := amqp.Dial(url)
	if err != nil {
		glog.Errorf("[rmq] dial to server %s failed: %v", addr, err)
		return
	}
	channel, err := conn.Channel()
	if err != nil {
		conn.Close()
		glog.Errorf("[rmq] get channel from rabbitmq server %s failed: %v", addr, err)
		return
	}

	err = channel.ExchangeDeclare(
		exchangeName,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		glog.Errorf("[rmq] declare exchange %s failed from rabbitmq server %s failed: %v", exchangeName, addr, err)
		channel.Close()
		conn.Close()
		return
	}

	this.servers.PushFront(&rmqServer{
		conn: conn,
		channel: channel,
		addr: addr,
		exchangeName: exchangeName,
	})

	if this.curr == nil {
		this.curr = this.servers.Front()
	}
}

// remove s but not close
func (this *Rmqs) Remove(s *rmqServer) {
	this.mu.Lock()
	defer this.mu.Unlock()

	for e := this.servers.Front(); e != nil; e = e.Next() {
		if srv, ok := e.Value.(*rmqServer); !ok {
			glog.Error("Invalid type in rmqserver's hlist")
			return
		} else {
			if srv == s {
				glog.Infof("[%s] removed ok", s.addr)
				this.servers.Remove(e)
				break
			}
		}
	}
}

func (this *Rmqs) Push(msg []byte, serviceId int) {
	this.mu.Lock()
	defer this.mu.Unlock()

	if this.curr != nil {
		c := this.curr.Value.(*rmqServer)
		err := c.channel.Publish(
			c.exchangeName,
			strconv.Itoa(serviceId),
			false,
			false,
			amqp.Publishing{
				ContentType:	"text/plain",
				Body:			msg,
			},
		)
		if err != nil {
			glog.Errorf("[rmq|publish] error on publish msg, error: %v, msg: (%v)", err, msg)
			// 试验性的错误处理，还不确认这个能正确处理服务器关闭的情况
			if err == amqp.ErrClosed {
				go func(server *rmqServer) {
					this.Remove(server)
					server.Close()
					glog.Errorf("[rmq|close] close server %v on error: %v", server, err)
				}(c)
			}
		}
		next := this.curr.Next()
		if next != nil {
			this.curr = next
		} else {
			this.curr = this.servers.Front()
		}
	} else {
		glog.Errorf("[rmq] curr == nil, list: %v", this.servers)
	}
}

type rmqServer struct {
	conn			*amqp.Connection
	channel			*amqp.Channel
	addr			string
	exchangeName	string
}

func (s *rmqServer) Close() {
	defer s.conn.Close()
	defer s.channel.Close()
	s.addr = ""
}
