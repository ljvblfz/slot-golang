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
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloud-base/atomic"
	"cloud-socket/msgs"
)

const (
	CmdGetToken         = uint16(0xE0)
	CmdRegister         = uint16(0xE1)
	CmdLogin            = uint16(0xE2)
	CmdChangeName       = uint16(0xE3)
	CmdDoBind           = uint16(0xE4)
	CmdHeartBeat        = uint16(0xE5)
	CmdSubDeviceOffline = uint16(0xE6)
)

var (
	ErrAckTimeout = errors.New("ack timeout")
	ErrNoReply    = errors.New("no response")

	gBenchDur time.Duration
	gDone     atomic.AtomicInt64
)

type Client struct {
	SN          [16]byte
	MAC         [8]byte
	Sid         []byte
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

	//c.testBenchmark()
	c.testApi()
}

func (c *Client) testBenchmark() {
	ret, err := c.doNewSession()
	if err != nil || ret["code"] != uint32(0) {
		log.Printf("NewSession return %v, error: %v", ret["code"], err)
		return
	}
	ret, err = c.doRegister()
	if (ret["code"] != uint32(0) && ret["code"] != uint32(1)) || err != nil {
		log.Printf("Register return %v, error: %v", ret["code"], err)
		return
	}

	go func() {
		t := time.NewTicker(time.Second * 3)
		last := time.Now()
		for now := range t.C {
			log.Printf("qps: %f", float64(gDone.Set(0))/now.Sub(last).Seconds())
			last = now
		}
	}()
	t := time.NewTicker(gBenchDur)
	for range t.C {
		ret, err = c.doHeartbeat()
		if err != nil {
			log.Printf("Hearbeat error: %v", ret["code"], err)
			continue
		}
		if ret["code"] == uint32(0) {
			gDone.Inc()
		}
		//log.Printf("Heartbeat: %v", ret["code"])
	}
}

// get token
// register
// login
// changename
// bind
// heartbeat
// offline sub
func (c *Client) testApi() {
	tests := []testCase{
		testCase{"doNewSession", c.doNewSession},
		testCase{"doRegister", c.doRegister},
		testCase{"doLogin", c.doLogin},
		testCase{"doRename", c.doRename},
		//testCase{"doDoBind", c.doDoBind},
		testCase{"doHeartbeat", c.doHeartbeat},
		//testCase{"doOfflineSub", c.doOfflineSub},
	}
	for _, t := range tests {
		result, err := t.fn()
		log.Printf("--- %s ---", t.name)
		if err != nil {
			log.Printf("[%v] error: %v", t.name, err)
		}
		if result == nil {
			continue
		}
		for k, v := range result {
			log.Printf("%s=%v", k, v)
		}
		time.Sleep(time.Millisecond)
	}
	log.Println("--- All done ---")
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

	// sum header
	output[24+8] = msgs.ChecksumHeader(output, 24+8)
	output[24+9] = msgs.ChecksumHeader(output[24+10:], 2+len(otherBody))
	// TODO sum body
	return output
}

func (c *Client) Query(request []byte) ([]byte, error) {
	for retry := 0; retry < 5; retry++ {
		idx := binary.LittleEndian.Uint16(request[2:4])
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
		twoHeaderLen := 24 + 12
		// ignore twoHeaderLen header bytes
		if n < twoHeaderLen {
			return nil, fmt.Errorf("response less than header count(twoHeaderLen bytes)")
		}
		bodyLen := binary.LittleEndian.Uint16(response[24+6 : 24+6+2])
		nidx := binary.LittleEndian.Uint16(response[2:4])
		if nidx != idx {
			return nil, fmt.Errorf("wrong seqNum in ACK message")
		}
		log.Printf("%d + %d(%v) = %d ?", twoHeaderLen, bodyLen, response[24+6:24+6+2], len(response))
		log.Printf("header: %v, body: %v", response[:twoHeaderLen], response[twoHeaderLen:])
		return response[twoHeaderLen : twoHeaderLen+int(bodyLen)], err
	}
	return nil, ErrNoReply
}

func (c *Client) doNewSession() (map[string]interface{}, error) {
	body := make([]byte, 24)
	copy(body[:8], c.MAC[:])
	copy(body[8:24], c.SN[:])

	req := c.packBody(CmdGetToken, body)
	rep, err := c.Query(req)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	ok := binary.LittleEndian.Uint32(rep[:4])
	m["code"] = ok
	m["sid"] = hex.EncodeToString(rep[4:20])
	m["platformKey"] = rep[20:28]
	copy(c.Sid[:], rep[4:20])
	if ok != 0 {
		log.Fatalf("doNewSession failed: %v", m)
	}
	return m, nil
}

func (c *Client) doRegister() (map[string]interface{}, error) {
	name := []byte(c.Name)
	body := make([]byte, 309+len(name))
	copy(body, c.Sid)
	binary.LittleEndian.PutUint16(body[16:18], c.DeviceType)
	binary.LittleEndian.PutUint16(body[18:20], 0)
	binary.LittleEndian.PutUint32(body[20:24], uint32(int32(c.ProduceTime.Unix())))
	copy(body[24:32], c.MAC[:])
	copy(body[32:48], c.SN[:])
	copy(body[48:308], c.Signature[:])
	body[308] = byte(len(name))
	copy(body[31:31+len(name)], name)

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
	m["cookie"] = string(rep[28:92])
	copy(c.Cookie[:64], rep[28:92])
	if bytes.Compare(c.Sid, sid2) != 0 {
		log.Printf("wrong session id response: %v != %v", c.Sid, sid2)
	}
	return m, nil
}

func (c *Client) doLogin() (map[string]interface{}, error) {
	body := make([]byte, 88)
	copy(body, c.Sid)
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
	if bytes.Compare(c.Sid, sid2) != 0 {
		log.Printf("wrong session id response: %v != %v", c.Sid, sid2)
	}
	return m, nil
}

func (c *Client) doRename() (map[string]interface{}, error) {
	c.Name += "2"
	name := []byte(c.Name)
	body := make([]byte, 25+len(name))
	copy(body, c.Sid)
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
	if bytes.Compare(c.Sid, sid2) != 0 {
		log.Printf("wrong session id response: %v != %v", c.Sid, sid2)
	}
	m["mac"] = hex.EncodeToString(rep[4:12])
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
	copy(body, c.Sid)
	copy(body[16:24], c.Id[:])

	req := c.packBody(CmdHeartBeat, body)
	rep, err := c.Query(req)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	m["code"] = binary.LittleEndian.Uint32(rep[:4])
	sid2 := rep[4:20]
	if bytes.Compare(c.Sid, sid2) != 0 {
		log.Printf("wrong session id response: %v != %v", c.Sid, sid2)
	}
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

func (c *Client) read() ([]byte, error) {
	select {
	case <-time.After(3 * time.Second):
		return nil, ErrAckTimeout
	case b := <-c.rch:
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
	}
}

func main() {
	flag.DurationVar(&gBenchDur, "d", time.Millisecond, "duration for bench")

	var (
		localAddr  = flag.String("l", "", "local udp address, eg: ip:port")
		serverAddr = flag.String("h", "", "server udp address, eg: ip:port")
		concurent  = flag.Int("c", 1, "concurent client number")
	)
	flag.Parse()

	log.SetFlags(log.Flags() | log.Lshortfile)
	if len(*localAddr) == 0 || len(*serverAddr) == 0 {
		flag.Usage()
		return
	}
	server, err := net.ResolveUDPAddr("udp", *serverAddr)
	if err != nil {
		fmt.Printf("Resolve server addr %s failed: %v\n", *serverAddr, err)
		return
	}
	local, err := net.ResolveUDPAddr("udp", *localAddr)
	if err != nil {
		fmt.Printf("Resolve local addr %s failed: %v\n", *localAddr, err)
		return
	}

	firstPort := local.Port
	// 跳过监听失败的端口
	for p, n := 0, 0; n < *concurent; p++ {
		local.Port = firstPort + p
		socket, err := net.ListenUDP("udp", local)
		if err != nil {
			fmt.Printf("Listen on local addr %v failed: %v\n", local, err)
			continue
		}
		n++
		c := Client{
			Name:        "deviceName",
			ProduceTime: time.Now(),
			DeviceType:  0xFFFE,
			Sid:         make([]byte, 16),
			Socket:      socket,
			ServerAddr:  server,
			rch:         make(chan []byte, 128),
		}
		binary.LittleEndian.PutUint32(c.SN[11:15], uint32(n))
		binary.LittleEndian.PutUint32(c.MAC[3:7], uint32(n))
		log.Printf("Client started, mac: %v, sn: %v", hex.EncodeToString(c.MAC[:]), hex.EncodeToString(c.SN[:]))
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
