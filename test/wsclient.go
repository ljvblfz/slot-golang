package main

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"encoding/base64"
	"flag"
	"fmt"
	//"github.com/gorilla/websocket"
	"code.google.com/p/go.net/websocket"
	"github.com/golang/glog"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	RecvCount int

	// loginCount 在线数的统计
	lock       = sync.Mutex{}
	loginCount int

	// queryCount 已发送的请求数
	queryCount int64

	// recvCount 已收到的消息数
	recvCount int64

	// 已尝试建立的连接数
	connIndex int64

	// 程序启动时间
	startTime time.Time

	querysPerSec int64
	recvsPerSec  int64

	allGo int32

	// 配置项
	_Count        int
	_Host         string
	_LocalHost    string
	_ConnPerSec   int
	_SendInterval time.Duration
	_StartId      int64
	_ToId         string
	_SpecifyTo    int64
	_StatPort     string
	_SN           string
	_SentCount    int64
)

func init() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	flag.IntVar(&_Count, "n", 1, "连接数的大小，最大不能超过64511")
	flag.StringVar(&_Host, "h", "ws://127.0.0.1:1234/ws", "指定远端服务器WebSocket地址")
	flag.StringVar(&_LocalHost, "l", "127.0.0.1,192.168.2.10", "指定本地地址,逗号分隔多个")
	flag.IntVar(&_ConnPerSec, "cps", 0, "限制每秒创建连接，0为无限制")
	flag.DurationVar(&_SendInterval, "i", 30*time.Second, "设置发送的间隔(如:1s, 10us)")
	//flag.Int64Var(&_StartId, "s", 1, "设置id的初始值自动增加1")
	flag.Int64Var(&_StartId, "id", 1, "第一个客户端的id，多个客户端时自动累加")
	//flag.StringVar(&_ToId, "to_id", "", "发送到id,逗号\",\"符连接, 如: 2,3")
	flag.Int64Var(&_SpecifyTo, "to_sid", 0, "发往客户端的指定id，0或to_id中的一项")
	flag.StringVar(&_StatPort, "sh", ":30001", "设置服务器统计日志端口")
	//flag.StringVar(&_SN, "sn", "client1", "设置客户端sn")
	flag.Int64Var(&_SentCount, "sc", -1, "Sent msg count per client")
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
	//return ioutil.ReadAll(c.conn)
}

var kHeader1 [4]byte = [4]byte{1, 1, 1, 1}
var kHeader3 [12]byte = [12]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}

func packData(id int64, data []byte) []byte {
	buf := new(bytes.Buffer)
	// 协议头包含24字节的数据头，其中[4:12]是目标id，我们需要这个
	binary.Write(buf, binary.LittleEndian, kHeader1)
	binary.Write(buf, binary.LittleEndian, id)
	binary.Write(buf, binary.LittleEndian, kHeader3)
	binary.Write(buf, binary.LittleEndian, data)
	return buf.Bytes()
}

func sendLogin(c *Connection, id int64, timestamp uint32, timeout uint32) {
	bs := md5.Sum([]byte(fmt.Sprintf("%d|%d|%d|BlackCrystalWb14527", id, timestamp, timeout)))
	md5Value := base64.StdEncoding.EncodeToString(bs[0:])
	buf := fmt.Sprintf("%d|%d|%d|%s", id, timestamp, timeout, md5Value)
	glog.Infof("[login] uid [%d] login: %s", id, buf)
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

func incrRecvCount() {
	atomic.AddInt64(&recvCount, 1)
}

func getRecvCount(reset bool) int64 {
	if reset {
		return atomic.SwapInt64(&recvCount, 0)
	} else {
		return atomic.LoadInt64(&recvCount)
	}
}

// record 用于统计的消息格式
type Record struct {
	IdFrom   int64     `json:"IdFrom"`
	IdTo     int64     `json:"IdTo"`
	TimeSent time.Time `json:"TimeSent"`
	//TimeRecv	time.Time	`json:"TimeRecv"`
	Duration time.Duration `json:"Dura"`
}

// 状态信息

type statusInfo struct {
	AppStartTime  time.Time // 应用启动时间
	Connections   int       // 当前连接数
	Querys        int64     // 当前已收到的总请求数
	QuerysPerSec  int64     `json:"QueryPerSec"`
	Recvs         int64     // 当前已收到的总消息树
	RecvsPerSec   int64     `json:"RecvsPerSec"`
	ConnConfig    int64     `json:"ConnConfigCount"`
	ConnStepOn    int64     `json:"ConnBuilding"`
	Communicating bool      `json:"Communicating"`
}

func updateQPS() {

	ticker := time.NewTicker(time.Second)

	lastTime := time.Now()
	lastQuerys := int64(0)
	lastRecvs := int64(0)

	for t := range ticker.C {
		d := t.Sub(lastTime).Seconds()

		qc := getQueryCount(false)
		atomic.StoreInt64(&querysPerSec, int64(float64(qc-lastQuerys) / d))
		lastQuerys = qc

		rc := getRecvCount(false)
		atomic.StoreInt64(&recvsPerSec, int64(float64(rc-lastRecvs) / d))
		lastRecvs = rc

		lastTime = t
	}
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method Not Allowed", 405)
		return
	}
	stat := statusInfo{
		AppStartTime:  startTime,
		Connections:   getLoginCount(),
		Querys:        getQueryCount(false),
		QuerysPerSec:  atomic.LoadInt64(&querysPerSec),
		Recvs:         getRecvCount(false),
		RecvsPerSec:   atomic.LoadInt64(&recvsPerSec),
		ConnConfig:    int64(_Count),
		ConnStepOn:    int64(atomic.LoadInt64(&connIndex)),
		Communicating: 1 == atomic.LoadInt32(&allGo),
	}

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

func dialWebsocket(url, protocol, origin string, localAddr *net.TCPAddr) (ws *websocket.Conn, err error) {
	config, err := websocket.NewConfig(url, origin)
	if err != nil {
		return nil, err
	}
	if protocol != "" {
		config.Protocol = []string{protocol}
	}

	var client net.Conn
	if config.Location == nil {
		return nil, &websocket.DialError{config, websocket.ErrBadWebSocketLocation}
	}
	if config.Origin == nil {
		return nil, &websocket.DialError{config, websocket.ErrBadWebSocketOrigin}
	}

	//glog.Infof("location: %v, host: %v, url: %s", config.Location, config.Location.Host, url)
	raddr, err := net.ResolveTCPAddr("tcp", config.Location.Host)
	if err != nil {
		goto Error
	}

	switch config.Location.Scheme {
	case "ws":
		client, err = net.DialTCP("tcp", localAddr, raddr)

	//case "wss":
	//	client, err = tls.Dial("tcp", config.Location.Host, config.TlsConfig)

	default:
		err = websocket.ErrBadScheme
	}
	if err != nil {
		goto Error
	}

	ws, err = websocket.NewClient(config, client, true)
	if err != nil {
		goto Error
	}
	return

Error:
	return nil, &websocket.DialError{config, err}
}

func main() {
	flag.Parse()

	go updateQPS()

	//if _Count == 1 && len(_ToId) == 0 {
	//	glog.Fatalf("\"-to_id\" can't be empty")
	//}

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

	localHosts := strings.Split(_LocalHost, ",")
	var localAddr = make([]net.TCPAddr, len(localHosts))
	for i := 0; i < len(localHosts); i++ {
		localAddr[i].IP = net.ParseIP(localHosts[i])
	}

	wg := sync.WaitGroup{}

	sysc := make(chan os.Signal, 1)
	signal.Notify(sysc, os.Interrupt, os.Kill)
	for i := 0; i < _Count; i++ {

		wg.Add(1)

		go func(num int) {
			la := &localAddr[num%len(localAddr)]
			//la.Port = 1024 + num
			// log.Println(localAddr, localAddr.Network())

			// new code
			ws, err := dialWebsocket(_Host, websocket.SupportedProtocolVersion, "http://localhost",
				la)
			// old code
			//ws, err := websocket.Dial(_Host, websocket.SupportedProtocolVersion, "http://localhost")

			if err != nil {
				glog.Infof("websocket error [%s] %v", _Host, err)
				wg.Done()
				atomic.AddInt64(&connIndex, 1)
				return
			}
			atomic.AddInt64(&connIndex, 1)
			defer ws.Close()

			c := &Connection{conn: ws, isClosed: false}
			// log.Println("Local Addr", conn.LocalAddr())

			id := _StartId + int64(num)
			timestamp := uint32(time.Now().Unix())

			toId := _SpecifyTo
			//if id < 0 {
			//	toId = 0
			//}

			sendLogin(c, id, timestamp, 10)

			ack, err := readData(c)
			if err != nil {
				glog.Infoln("Error read login", err)
				wg.Done()
				return
			}
			wg.Done()
			if ack[0] == 0 {
				// log.Println(num, ack[0])
				incLoginCount()
				defer decLoginCount()

				// writer
				//msgChan := make(chan []byte)
				quitChan := make(chan struct{})
				defer close(quitChan)
				go func() {
					for {
						select {
						case <-time.After(10 * time.Second):
							c.conn.Write([]byte("p"))
							if glog.V(2) {
								glog.Infof("[ping] uid %d", id)
							}
						case <-quitChan:
							return
						}
					}
				}()
				go func() {
					sentIndex := int64(0)
					index := 0
					re := Record{IdFrom: id, IdTo: toId}

					//whenPing := time.Now()
					for {
						select {
						case <-time.After(_SendInterval):
							if 1 == atomic.LoadInt32(&allGo) {
								if _SentCount < 0 || sentIndex < _SentCount {
									incrQueryCount()
									sentIndex++
									re.TimeSent = time.Now()
									data, _ := json.Marshal(&re)
									sendData(c, toId, data)
									index++
									if glog.V(2) {
										glog.Infof("[record] %s >>>\n", string(data))
									}
								}
							}
						case <-quitChan:
							return
						}
					}
				}()
				// reader
				rc := 0
				record := Record{}
				for {
					msgReceived, err := readData(c)
					if err != nil {
						glog.Infoln("[websocket] Recv Data", err)
						return
					}
					strMsg := string(msgReceived)
					if strMsg == "P" {
						//go func() {
						//	time.After(10 * time.Second)
						//	msgChan <- []byte("p")
						//}()
						if glog.V(2) {
							glog.Infof("[pong] uid %d", id)
						}
					} else {
						rc++
						if len(msgReceived) <= 24 {
							glog.Errorf("[websocket] [uid: %d] read bad data, less than 25, %d bytes, %v(%s)", id, len(msgReceived), msgReceived, strMsg)
							break
						}
						incrRecvCount()
						err = json.Unmarshal(msgReceived[24:], &record)
						if err != nil {
							glog.Fatalln("Json error:", err)
						}
						record.Duration = time.Now().Sub(record.TimeSent)
						if glog.V(1) {
							glog.Infof("[record] %s\n", fmt.Sprintf("%d,%d,%v,%d", record.IdFrom, record.IdTo, record.TimeSent, record.Duration.Nanoseconds()))
						}
					}
				}
			} else {
				glog.Infoln(num, "login failed", ack)
			}

		}(i)
		if i > 0 && _ConnPerSec > 0 && ((i % _ConnPerSec) == (_ConnPerSec - 1)) {
			time.Sleep(time.Second)
		}
	}
	wg.Wait()

	//fmt.Printf("已建立连接%d/%d连接，按回车开始发送数据:\n", getLoginCount(), _Count)
	//any := ""
	//fmt.Scanln(&any)

	atomic.StoreInt32(&allGo, 1)
	// Block until a signal is received.
	s := <-sysc
	glog.Infoln("收到进程信号:", s)

	n := getLoginCount()
	querys := getQueryCount(false)
	recvs := getRecvCount(false)
	glog.Infof("当前连接数: %d, 发送数: %d, 接收数: %d\n", n, querys, recvs)
}
