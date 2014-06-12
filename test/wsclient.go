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
	_SendInterval int64
	_StartId      int64
	_StatPort     string
)

func init() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	flag.IntVar(&_Count, "n", 4096, "连接数的大小，最大不能超过64511")
	flag.StringVar(&_Host, "h", "ws://127.0.0.1:1234/ws", "指定远端服务器WebSocket地址")
	flag.StringVar(&_LocalHost, "l", "127.0.0.1", "指定本地地址,不要设置端口号,端口号是自动从1024+!")
	flag.Int64Var(&_SendInterval, "i", 30, "发送数据的频率,单位秒")
	flag.Int64Var(&_StartId, "s", 1, "设置id的初始值自动增加1")
	flag.StringVar(&_StatPort, "sh", ":30001", "设置服务器统计日志端口")
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
	binary.Write(buf, binary.BigEndian, id)
	binary.Write(buf, binary.BigEndian, data)
	return buf.Bytes()
}

// func sendData(conn net.Conn, data []byte) error {
// 	_, err := conn.Write(data)
// 	return err
// }

func sendLogin(c *Connection, id int64, mac, alias string, timestamp uint32, hmac string) {
	buf := fmt.Sprintf("0|%d|%s|%s|%d|%s", id, mac, alias, timestamp, hmac)
	c.conn.Write([]byte(buf))
}

func sendData(c *Connection, id int64, data []byte) {
	new_byte := packData(id, data)
	c.conn.Write(new_byte)
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
	dd := make([]byte, 512)
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
			sendLogin(c, id, mac, alias, timestamp, hmac)

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
				for {
					sendData(c, id, dd)
					incrQueryCount()
					_, err := readData(c)
					time.Sleep(time.Duration(_SendInterval) * time.Second)
					if err != nil {
						glog.Infoln("Error Data", err)
						return
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
