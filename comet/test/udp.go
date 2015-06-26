// 测试用客户端
package main

import (
	//"crypto/rand"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	// "log"
	"net"
	"os"
	"os/signal"
	// "strconv"
	"runtime"
	"sync"
	"syscall"
	"time"

	"cloud-base/atomic"
	"cloud-socket/msgs"
	"github.com/fzzy/radix/redis"
	"github.com/golang/glog"
)

const (
	CmdGetToken         = uint16(0xE0)
	CmdRegister         = uint16(0xE1)
	CmdLogin            = uint16(0xE2)
	CmdChangeName       = uint16(0xE3)
	CmdDoBind           = uint16(0xE4)
	CmdHeartBeat        = uint16(0xE5)
	CmdSubDeviceOffline = uint16(0xE6)
	DeviceStatusSync    = uint16(0x30)
)

var (
	ErrAckTimeout = errors.New("ack timeout")
	ErrNoReply    = errors.New("no response")

	gRedisClient *redis.Client
	gRedisMu     *sync.Mutex

	gBenchDur time.Duration
	gDone     atomic.AtomicInt64
)

type Client struct {
	SN          [16]byte
	MAC         [8]byte
	Sid         [16]byte
	Cookie      [64]byte
	Id          [8]byte
	Name        string
	ProduceTime time.Time
	DeviceType  uint16
	Signature   [260]byte

	Sidx uint16 // 自身的包序号
	Ridx uint16 // 收取的包序号

	Socket     *net.UDPConn
	ServerAddr *net.UDPAddr
	rch        chan []byte
}

type testCase struct {
	name string
	fn   func() (map[string]interface{}, error)
}

func (c *Client) Run() {
	go c.readLoop()

	if _testbenchmark == true {
		if _testcase != "" {
			c.testBenchmark(_testcase)
		} else {
			return
		}
	} else {
		tests := []testCase{
			testCase{"doNewSession", c.doNewSession},
			/*			testCase{"doRegister", c.doRegister},
						testCase{"doLogin", c.doLogin},
						testCase{"doRename", c.doRename},
						//testCase{"doDoBind", c.doDoBind},
						testCase{"doHeartbeat", c.doHeartbeat},
						//testCase{"doOfflineSub", c.doOfflineSub},
						testCase{"doSendMessage", c.doSendMessage},
			*/}

		c.testApi(tests)
	}
}

func (c *Client) testBenchmark(name string) {

	go func() {
		t := time.NewTicker(time.Second * 5)
		last := time.Now()
		for now := range t.C {
			qps := float64(gDone.Set(0)) / now.Sub(last).Seconds()
			if qps > 0.5 {
				glog.Infof("qps: %f", qps)
				last = now
			}
		}
	}()

	t := time.NewTicker(gBenchDur)
	switch name {
	case "ns":
		tests := []testCase{
			testCase{"doNewSession", c.doNewSession},
		}
		for range t.C {
			go func() {
				c.testApi(tests)
				gDone.Inc()
			}()
		}

	case "reg":
		tests := []testCase{
			testCase{"doNewSession", c.doNewSession},
		}
		c.testApi(tests)

		tests = []testCase{
			testCase{"doRegister", c.doRegister},
		}
		for range t.C {
			go func() {
				c.testApi(tests)
				gDone.Inc()
			}()
		}
	case "login":
		tests := []testCase{
			testCase{"doNewSession", c.doNewSession},
			testCase{"doRegister", c.doRegister},
		}
		c.testApi(tests)

		tests = []testCase{
			testCase{"doLogin", c.doLogin},
		}
		for range t.C {
			go func() {
				c.testApi(tests)
				gDone.Inc()
			}()
		}
	case "rn":
		tests := []testCase{
			testCase{"doNewSession", c.doNewSession},
			testCase{"doRegister", c.doRegister},
			testCase{"doLogin", c.doLogin},
		}
		c.testApi(tests)

		tests = []testCase{
			testCase{"doRename", c.doRename},
		}
		for range t.C {
			c.testApi(tests)
			gDone.Inc()
		}
	case "send":
		tests := []testCase{
			testCase{"doNewSession", c.doNewSession},
			testCase{"doRegister", c.doRegister},
			testCase{"doLogin", c.doLogin},
			// testCase{"doRename", c.doRename},
		}
		c.testApi(tests)

		tests = []testCase{
			testCase{"doSendMessage", c.doSendMessage},
		}
		for range t.C {
			c.testApi(tests)
			gDone.Inc()
		}
	}

	// t := time.NewTicker(gBenchDur)
	// for tt := range t.C {
	// 	ret, err = c.doLogin()
	// 	if err != nil {
	// 		log.Printf("Hearbeat error: %v %v\n", ret["code"], err, tt)
	// 		continue
	// 	}
	// 	if ret["code"] == uint32(0) {
	// 		gDone.Inc()
	// 	}
	// 	//log.Printf("Heartbeat: %v", ret["code"])
	// }
}

// get token
// register
// login
// changename
// bind
// heartbeat
// offline sub
func (c *Client) testApi(tests []testCase) {
	for _, t := range tests {
		if t.name != "doHeartbeat" {
			if _testbenchmark != true {
				glog.Infof("--- %s ---", t.name)
			}
			result, err := t.fn()
			if err != nil {
				glog.Infof("[%v] error: %v", t.name, err)
			}
			if result == nil {
				continue
			}
			if _testbenchmark != true {
				for k, v := range result {
					glog.Infof("%s=%v", k, v)
				}
			}
			time.Sleep(time.Millisecond)
		} else {
			// go func() {
			// 	for {
			glog.Infof("--- %s ---", t.name)
			result, err := t.fn()
			if err != nil {
				glog.Infof("[%v] error: %v", t.name, err)
			}
			if result == nil {
				continue
			}
			for k, v := range result {
				glog.Infof("%s=%v", k, v)
			}
			time.Sleep(time.Second)
			// 	}
			// }()
		}
	}
	if _testbenchmark != true {
		glog.Infof("--- All done ---%v", c.MAC[:])
	}
}

func SetDeviceId(deviceMac string, deviceId int32) error {
	gRedisMu.Lock()
	reply := gRedisClient.Cmd("hset", "DeviceMacToDeviceId", deviceMac, deviceId)
	err := reply.Err
	gRedisMu.Unlock()
	return err
}

func (c *Client) packBody(msgId uint16, otherBody []byte) []byte {
	frameHeader := make([]byte, 24)
	frameHeader[0] = (frameHeader[0] &^ 0x7) | 0x2
	c.Sidx++
	binary.LittleEndian.PutUint16(frameHeader[2:4], c.Sidx)

	dataHeader := make([]byte, 12)
	binary.LittleEndian.PutUint16(dataHeader[4:6], msgId)
	binary.LittleEndian.PutUint16(dataHeader[6:8], uint16(len(otherBody)))

	output := append(frameHeader, dataHeader...)
	output = append(output, otherBody...)
	// log.Printf("[len]otherBody: [%v]%v", len(otherBody), otherBody)

	// sum header
	output[24+8] = msgs.ChecksumHeader(output, 24+8)
	output[24+9] = msgs.ChecksumHeader(output[24+10:], 2+len(otherBody))
	// TODO sum body
	return output
}

func (c *Client) Query(request []byte) ([]byte, error) {
	// log.Printf("request header: %v, body: %v", request[0:24+12], request[24+12:])
	// log.Printf("[len]request message: [%v]%v", len(request[:]), request[:])
	for retry := 0; retry < 5; retry++ {
		/*		idx := binary.LittleEndian.Uint16(request[2:4])
		 */n, err := c.Socket.WriteToUDP(request, c.ServerAddr)
		if err != nil {
			return nil, err
		}
		if n != len(request) {
			return nil, fmt.Errorf("not sent all message")
		}
		response, err := c.read()
		if err == ErrAckTimeout {
			c.Sidx++
			binary.LittleEndian.PutUint16(request[2:4], c.Sidx)
			time.Sleep(time.Second * 3)
			continue
		}
		twoHeaderLen := 24 + 12
		// ignore twoHeaderLen header bytes
		if n < twoHeaderLen {
			return nil, fmt.Errorf("response less than header count(twoHeaderLen bytes)")
		}
		bodyLen := binary.LittleEndian.Uint16(response[24+6 : 24+6+2])
		/*		nidx := binary.LittleEndian.Uint16(response[2:4])
				if nidx != idx {
					return nil, fmt.Errorf("wrong seqNum in ACK message %v != %v\n", nidx, idx)
				}*/
		// log.Printf("%d + %d(%v) = %d ?", twoHeaderLen, bodyLen, response[24+6:24+6+2], len(response))
		// log.Printf("response header: %v, body: %v", response[:twoHeaderLen], response[twoHeaderLen:])
		return response[twoHeaderLen : twoHeaderLen+int(bodyLen)], err
	}
	return nil, ErrNoReply
}

func (c *Client) Query1(request []byte) ([]byte, error) {
	// log.Printf("request header: %v, body: %v", request[0:24+12], request[24+12:])
	glog.V(4).Infof("[len]request message: [%v]%v", len(request[:]), request[:])
	for retry := 0; retry < 5; retry++ {
		// idx := binary.LittleEndian.Uint16(request[2:4])
		// log.Printf("idx: %v", idx)
		n, err := c.Socket.WriteToUDP(request, c.ServerAddr)
		if err != nil {
			return nil, err
		}
		if n != len(request) {
			return nil, fmt.Errorf("not sent all message")
		}
		response, err := c.read()
		if err == ErrAckTimeout {
			c.Sidx++
			binary.LittleEndian.PutUint16(request[2:4], c.Sidx)
			time.Sleep(time.Second * 3)
			continue
		}
		twoHeaderLen := 24 + 24
		// ignore twoHeaderLen header bytes
		if n < twoHeaderLen {
			return nil, fmt.Errorf("response less than header count(twoHeaderLen bytes)")
		}
		// bodyLen := binary.LittleEndian.Uint16(response[24+6 : 24+6+2])
		// nidx := binary.LittleEndian.Uint16(response[2:4])
		// if nidx != idx {
		// 	return nil, fmt.Errorf("wrong seqNum in ACK message %v != %v\n", nidx, idx)
		// }
		// log.Printf("%d + %d(%v) = %d ?", twoHeaderLen, bodyLen, response[24+6:24+6+2], len(response))
		glog.V(4).Infof("[%v]response header: %v, body: %v", len(response[:]), response[:twoHeaderLen], response[twoHeaderLen:])
		return response[twoHeaderLen:], err
	}
	return nil, ErrNoReply
}

func (c *Client) doNewSession() (map[string]interface{}, error) {
	body := make([]byte, 24)
	copy(body[:8], c.MAC[:])
	copy(body[8:24], c.SN[:])

	req := c.packBody(CmdGetToken, body)
	glog.Infof("req: %v\n", req)
	rep, err := c.Query(req)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	ok := binary.LittleEndian.Uint32(rep[:4])
	m["code"] = ok
	m["sid"] = hex.EncodeToString(rep[4:20])
	m["platformKey"] = hex.EncodeToString(rep[20:28])
	copy(c.Sid[:], rep[4:20])
	if ok != 0 {
		glog.Errorf("doNewSession failed: %v", m)
	}
	return m, nil
}

func (c *Client) doRegister() (map[string]interface{}, error) {
	name := []byte(c.Name)
	body := make([]byte, 309+len(name))
	copy(body, c.Sid[:])
	binary.LittleEndian.PutUint16(body[16:18], c.DeviceType)
	binary.LittleEndian.PutUint16(body[18:20], 0)
	binary.LittleEndian.PutUint32(body[20:24], uint32(int32(c.ProduceTime.Unix())))
	copy(body[24:32], c.MAC[:])
	copy(body[32:48], c.SN[:])
	copy(body[48:308], c.Signature[:])
	body[308] = byte(len(name))
	// copy(body[31:31+len(name)], name) //wrong line

	req := c.packBody(CmdRegister, body)
	rep, err := c.Query(req)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	m["code"] = binary.LittleEndian.Uint32(rep[:4])
	sid2 := rep[4:20]
	m["id"] = int64(binary.LittleEndian.Uint64(rep[20:28]))
	copy(c.Id[:], rep[20:28])
	mac := string(c.MAC[:])
	did := int32(binary.LittleEndian.Uint64(c.Id[:]))
	err = SetDeviceId(mac, did)
	if err != nil {
		glog.Errorf("SetDeviceId failed, Mac: %s, error: %v", mac, err)
	}
	m["cookie"] = string(rep[28:92])
	copy(c.Cookie[:64], rep[28:92])
	if bytes.Compare(c.Sid[:], sid2) != 0 {
		glog.Errorf("wrong session id response: %v != %v", c.Sid, sid2)
	}
	return m, nil
}

func (c *Client) doLogin() (map[string]interface{}, error) {
	body := make([]byte, 88)
	copy(body, c.Sid[:])
	copy(body[16:24], c.MAC[:])
	copy(body[24:88], c.Cookie[:])
	req := c.packBody(CmdLogin, body)

	rep, err := c.Query(req)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	m["code"] = binary.LittleEndian.Uint32(rep[:4])
	sid2 := rep[4:20]
	if bytes.Compare(c.Sid[:], sid2) != 0 {
		glog.Errorf("wrong session id response: %v != %v", c.Sid, sid2)
	}
	return m, nil
}

func (c *Client) doRename() (map[string]interface{}, error) {
	c.Name += "2"
	name := []byte(c.Name)
	body := make([]byte, 25+len(name))
	copy(body, c.Sid[:])
	copy(body[16:24], c.MAC[:])
	body[24] = byte(len(name))
	copy(body[25:25+len(name)], name)

	//log.Printf("SID: %v", c.Sid)
	req := c.packBody(CmdChangeName, body)
	rep, err := c.Query(req)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	m["code"] = binary.LittleEndian.Uint32(rep[:4])
	sid2 := rep[4:20]
	if bytes.Compare(c.Sid[:], sid2) != 0 {
		glog.Errorf("wrong session id response: %v != %v", c.Sid, sid2)
	}
	m["GUID"] = hex.EncodeToString(rep[4:20])
	m["mac"] = hex.EncodeToString(rep[20:28])
	return m, nil
}

//func (c *Client) doDoBind() (map[string]interface{}, error) {
//	body := make([]byte, 32)
//	copy(body[0:16], c.Sid[:])
//	copy(body[16:24], c.MAC[:])
//	copy(body[24:32], c.Id[:])
//
//	req := c.packBody(CmdDoBind, c.Sid, body)
//	rep, err := c.Query(req)
//	if err != nil {
//		return nil, err
//	}
//	m := make(map[string]interface{})
//	m["sid"] = hex.EncodeToString(rep[:16])
//	copy(c.Sid[:], rep[:16])
//	m["mac"] = hex.EncodeToString(rep[16:24])
//	m["code"] = binary.LittleEndian.Uint32(rep[24:28])
//	return m, nil
//}

func (c *Client) doHeartbeat() (map[string]interface{}, error) {
	body := make([]byte, 24)
	copy(body, c.Sid[:])
	copy(body[16:24], c.Id[:])

	req := c.packBody(CmdHeartBeat, body)
	rep, err := c.Query(req)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	m["code"] = binary.LittleEndian.Uint32(rep[:4])
	sid2 := rep[4:20]
	if bytes.Compare(c.Sid[:], sid2) != 0 {
		glog.Errorf("wrong session id response: %v != %v", c.Sid, sid2)
	}
	return m, nil
}

func (c *Client) doSendMessage() (map[string]interface{}, error) {
	otherBody := []byte("p")
	frameHeader := make([]byte, 24)
	frameHeader[0] = (frameHeader[0] &^ 0x7) | 0x3
	c.Sidx++
	binary.LittleEndian.PutUint16(frameHeader[2:4], c.Sidx)
	binary.LittleEndian.PutUint32(frameHeader[4:8], uint32(time.Now().Unix()))

	binary.LittleEndian.PutUint64(frameHeader[8:16], uint64(16))
	copy(frameHeader[16:24], c.Id[:])

	// gUid := make([]byte, 16)
	// copy(gUid[:], c.Sid)
	req := append(frameHeader, c.Sid[:]...)

	dataHeader := make([]byte, 12)
	binary.LittleEndian.PutUint16(dataHeader[4:6], DeviceStatusSync)
	binary.LittleEndian.PutUint16(dataHeader[6:8], uint16(len(otherBody)))

	req = append(req, frameHeader...)
	req = append(req, dataHeader...)
	req = append(req, otherBody...)
	// log.Printf("[len]otherBody: [%v]%v", len(otherBody), otherBody)

	// sum header
	// req[24+8] = msgs.ChecksumHeader(req, 24+8)
	// req[24+9] = msgs.ChecksumHeader(req[24+10:], 2+len(otherBody))
	rep, err := c.Query1(req)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	m["data"] = string(binary.LittleEndian.Uint32(rep[:4]))
	// sid2 := rep[4:20]
	// if bytes.Compare(c.Sid[:], sid2) != 0 {
	// 	log.Printf("wrong session id response: %v != %v", c.Sid, sid2)
	// }
	return m, nil
}

//func (c *Client) doOfflineSub() (map[string]interface{}, error) {
//	body := make([]byte, 24)
//	copy(body, c.Sid)
//	copy(body[0:16], c.Sid[:])
//	copy(body[16:24], c.MAC[:])
//
//	req := c.packBody(CmdSubDeviceOffline, c.Sid, body)
//	rep, err := c.Query(req)
//	if err != nil {
//		return nil, err
//	}
//	m := make(map[string]interface{})
//	m["sid"] = hex.EncodeToString(rep[:16])
//	copy(c.Sid[:], rep[:16])
//	m["mac"] = hex.EncodeToString(rep[16:24])
//	return m, nil
//}

func (c *Client) send() {

}

func (c *Client) read() ([]byte, error) {
	select {
	case <-time.After(3 * time.Second):
		return nil, ErrAckTimeout
	case b := <-c.rch:
		// log.Printf("[msg|in|read] [%v]%v\n", len(b), b)
		return b, nil
	}

}

func (c *Client) readLoop() {
	for {
		buf := make([]byte, 128)
		n, peer, err := c.Socket.ReadFromUDP(buf)
		if err != nil {
			continue
		}
		if peer.String() != c.ServerAddr.String() {
			continue
		}
		c.rch <- buf[:n]
		// log.Printf("[msg|in|readloop] [%v]%v\n", len(buf[:n]), buf[:n])

	}
}

var (
	_testbenchmark bool
	_testcase      string
)

func init() {
	flag.BoolVar(&_testbenchmark, "tb", false, "test benchmark")
	flag.StringVar(&_testcase, "tc", "", "newsession=ns,register=reg,login=login,rename=rn")

}

func main() {
	flag.DurationVar(&gBenchDur, "d", time.Millisecond, "duration for bench")
	_StartId := flag.Int("s", 1, "设置MAC的初始值自动增加1")

	redisAddr := flag.String("ra", "127.0.0.1:6379", "查找目标MAC设备ID的Redis服务器地址")
	runtime.GOMAXPROCS(runtime.NumCPU())

	var (
		localAddr  = flag.String("l", "", "local udp address, eg: ip:port")
		serverAddr = flag.String("h", "", "server udp address, eg: ip:port")
		concurent  = flag.Int("c", 1, "concurent client number")
	)
	flag.Parse()

	var err error
	gRedisClient, err = redis.Dial("tcp", *redisAddr)
	if err != nil {
		glog.Errorf("Dial to redis failed: %v", *redisAddr)
		return
	}

	gRedisMu = &sync.Mutex{}
	defer gRedisClient.Close()

	glog.Flush()
	// log.SetFlags(log.Flags() | log.Lshortfile)
	if len(*localAddr) == 0 || len(*serverAddr) == 0 {
		flag.Usage()
		return
	}
	server, err := net.ResolveUDPAddr("udp", *serverAddr)
	if err != nil {
		glog.Errorf("Resolve server addr %s failed: %v\n", *serverAddr, err)
		return
	}
	local, err := net.ResolveUDPAddr("udp", *localAddr)
	if err != nil {
		glog.Errorf("Resolve local addr %s failed: %v\n", *localAddr, err)
		return
	}

	firstPort := local.Port
	// 跳过监听失败的端口
	for p, n := 0, 0; n < *concurent; p++ {
		local.Port = firstPort + p
		socket, err := net.ListenUDP("udp", local)
		if err != nil {
			glog.Errorf("Listen on local addr %v failed: %v\n", local, err)
			continue
		}
		n++
		c := Client{
			Name:        "dn",
			ProduceTime: time.Now(),
			DeviceType:  0xFFFE,
			// Sid:         make([]byte, 16),
			Socket:     socket,
			ServerAddr: server,
			rch:        make(chan []byte, 128),
		}
		id := n + *_StartId
		binary.LittleEndian.PutUint32(c.SN[:], uint32(id))
		// str1, _ := strconv.ParseUint("28D244663552", 16, 64)
		binary.LittleEndian.PutUint32(c.MAC[:], uint32(id))
		// binary.LittleEndian.PutUint64(c.MAC[:], str1)
		glog.V(1).Infof("Client started, mac: %v, sn: %v", hex.EncodeToString(c.MAC[:]), hex.EncodeToString(c.SN[:]))
		go c.Run()
	}
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)

	for sig := range c {
		switch sig {
		case syscall.SIGINT, syscall.SIGTERM:
			return
		}
	}
}
