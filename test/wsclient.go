package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	//"github.com/gorilla/websocket"
	"code.google.com/p/go.net/websocket"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	//"net/url"
	"os"
	"os/signal"
	//"strings"
	"sync"
	"sync/atomic"
	"time"
	"github.com/golang/glog"
)

var (
	RecvCount int

	// loginCount 在线数的统计
	lock = sync.Mutex{}
	loginCount int

	// queryCount 请求数的计数字段
	queryCount int64

	// 程序启动时间
	startTime time.Time

	// 配置项
	_Count        int
	_Host         string
	_LocalHost    string
	_SendInterval int
	_StartId      int64
	_ToId         string
	_StatPort     string
	_SN           string
)

func init() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	flag.IntVar(&_Count, "n", 1, "(暂时无效)连接数的大小，最大不能超过64511")
	flag.StringVar(&_Host, "h", "ws://127.0.0.1:1234/ws", "指定远端服务器WebSocket地址")
	flag.StringVar(&_LocalHost, "l", "127.0.0.1", "指定本地地址,不要设置端口号,端口号是自动从1024+!")
	flag.IntVar(&_SendInterval, "i", 30, "发送数据的频率,单位秒")
	//flag.Int64Var(&_StartId, "s", 1, "设置id的初始值自动增加1")
	flag.Int64Var(&_StartId, "id", 1, "本客户端")
	flag.StringVar(&_ToId, "to_id", "", "发送到id,&符连接, 如: 2&3")
	flag.StringVar(&_StatPort, "sh", ":30001", "设置服务器统计日志端口")
	flag.StringVar(&_SN, "sn", "client1", "设置客户端sn")
}

type Connection struct {
	conn     *websocket.Conn
	isClosed bool
}

func (this *Connection) Close() {
	if !this.isClosed {
		this.isClosed = true
		this.conn.Close()
	}
}

func readData(c *Connection) ([]byte, error) {
	r, e := c.conn.NewFrameReader()
	if e != nil {
		return nil, e
	}
	return ioutil.ReadAll(r)
}

func packData(id int64, data []byte) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, id)
	binary.Write(buf, binary.LittleEndian, data)
	return buf.Bytes()
}

// func sendData(conn net.Conn, data []byte) error {
// 	_, err := conn.Write(data)
// 	return err
// }

func sendLogin(c *Connection, id int64, mac, alias string, timestamp uint32, bindedIds []int64, hmac string) {
	var ids string
	//for k, v := range bindedIds {
	//	if k != 0 {
	//		ids += "&"
	//	}
	//	ids += fmt.Sprintf("%d", v)
	//}
	ids = _ToId
	buf := fmt.Sprintf("0|%d|%s|%s|%d|%s|%s", id, mac, alias, timestamp, ids, hmac)
	c.conn.Write([]byte(buf))
}

func sendData(c *Connection, id int64, data []byte) {
	new_byte := packData(id, data)
	c.conn.Write(new_byte)
	glog.Infof("[msg] [%d] sent msg: %s\n", id, string(data))
}

// 状态统计

func incLoginCount() int {
	lock.Lock()
	defer lock.Unlock()
	loginCount++
	return loginCount
}

func decLoginCount() int {
	lock.Lock()
	defer lock.Unlock()
	loginCount--
	return loginCount
}

func getLoginCount() int {
	lock.Lock()
	n := loginCount
	lock.Unlock()
	return n
}

func incrQueryCount() {
	atomic.AddInt64(&queryCount, 1)
}

func getQueryCount(reset bool) int64 {
	if reset {
		return atomic.SwapInt64(&queryCount, 0)
	} else {
		return atomic.LoadInt64(&queryCount)
	}
}

// 状态信息

type statusInfo struct {
	AppStartTime	time.Time	// 应用启动时间
	Connections		int			// 当前连接数
	Querys			int64		// 当前已受到的总请求数
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method Not Allowed", 405)
		return
	}
	stat := statusInfo{startTime, getLoginCount(), getQueryCount(false)}

	body, err := json.MarshalIndent(stat, "", "\t")

	if err != nil {
		glog.Errorf("Failed to Marshal, content: %v, error: %v\n", stat, err)
		w.WriteHeader(500)
		return
	}

	w.Header().Add("Content-Type", "application/json;charset=UTF-8")
	if _, err := w.Write(body); err != nil {
		glog.Errorf("w.Write(\"%s\") error(%v)\n", string(body), err)
	}
}

// runStat 在statPort端口上运行http服务，报告连接等状态信息
func runStat(statPort string) {
	server := http.NewServeMux()
	server.HandleFunc("/stat", statusHandler)
	if err := http.ListenAndServe(statPort, server); err != nil {
		glog.Errorln("failed to start status server:", err)
	} else {
		glog.Infoln("Start status server:", statPort)
	}
}

func main() {
	flag.Parse()

	startTime = time.Now()

	//u, err := url.Parse(_Host)
	//if err != nil {
	//	glog.Fatal(err)
	//}
	//remoteAddr, err := net.ResolveTCPAddr("tcp", u.Host)
	//if err != nil {
	//	glog.Fatalln("解析远端地址出错", err)
	//}

	go runStat(_StatPort)

	var localAddr net.TCPAddr
	localAddr.IP = net.ParseIP(_LocalHost)

	sysc := make(chan os.Signal, 1)
	signal.Notify(sysc, os.Interrupt, os.Kill)
	_Count = 1
	for i := 0; i < _Count; i++ {
		go func(num int) {
			//localAddr.Port = 1024 + num
			// log.Println(localAddr, localAddr.Network())
			ws, err := websocket.Dial(_Host, websocket.SupportedProtocolVersion, "http://localhost:1234")
			if err != nil {
				glog.Infof("websocket error [%s] %v", _Host, err)
				return
			}
			defer ws.Close()

			c := &Connection{conn: ws, isClosed: false}
			// log.Println("Local Addr", conn.LocalAddr())

			id := _StartId + int64(num)
			mac := fmt.Sprintf("mac%d", id)
			alias := fmt.Sprintf("alias%d", id)
			timestamp := uint32(time.Now().Unix())
			hmac := "whatever"
			sendLogin(c, id, mac, alias, timestamp, []int64{1, 2}, hmac)

			incrQueryCount()
			ack, err := readData(c)
			if err != nil {
				glog.Infoln("Error read login", err)
				return
			}
			if ack[0] == 0 {
				// log.Println(num, ack[0])
				incLoginCount()
				defer decLoginCount()

				// writer
				msgChan := make(chan []byte)
				quitChan := make(chan struct{})
				defer close(quitChan)
				go func() {
					index := 0
					for {
						select {
						case <-time.After(10 * time.Second):
							incrQueryCount()
							c.conn.Write([]byte("p"))
						case <-time.After(time.Duration(_SendInterval) * time.Second):
							incrQueryCount()
							sendData(c, id+1, []byte(fmt.Sprintf("%s(%d->%d) %d", _SN, id, id+1, index)))
							index++
						case msg, ok := <-msgChan:
							if !ok {
								return
							}
							incrQueryCount()
							if string(msg) == "p" {
								c.conn.Write(msg)
							} else {
								sendData(c, id, msg)
							}
						case <-quitChan:
							return
						}
					}
				}()
				// reader
				rc := 0
				for {
					msgReceived, err := readData(c)
					if err != nil {
						glog.Infoln("Error Data", err)
						return
					}
					strMsg := string(msgReceived)
					if strMsg == "P" {
						//go func() {
						//	time.After(10 * time.Second)
						//	msgChan <- []byte("p")
						//}()
					} else {
						rc++
						glog.Infof("[msg] %d receive: %s, index: %d\n", id, strMsg, rc)
					}
				}
			} else {
				glog.Infoln(num, "login failed", ack)
			}

		}(i)
	}
	// Block until a signal is received.
	s := <-sysc
	glog.Infoln("收到进程信号:", s)

	n := getLoginCount()
	querys := getQueryCount(false)
	glog.Infof("当前连接数: %d, 请求数: %d\n", n, querys)
}
