// 测试用客户端
package main

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"time"

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

type Client struct {
	SN          [16]byte
	MAC         [8]byte
	Sid         []byte
	Cookie      [64]byte
	Id          [8]byte
	Name        string
	ProduceTime time.Time
	DeviceType  uint16
	Socket      *net.UDPConn
	ServerAddr  *net.UDPAddr
	rch         chan []byte
}

type testCase struct {
	name string
	fn   func() (map[string]interface{}, error)
}

// get token
// register
// login
// changename
// bind
// heartbeat
// offline sub
func (c *Client) Run() {
	go c.readLoop()

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
			log.Printf("[%v] error: %v", err)
		}
		if result == nil {
			continue
		}
		for k, v := range result {
			log.Printf("%s=%v", k, v)
		}
		time.Sleep(time.Millisecond)
		//log.Println("---")
	}
}

func (c *Client) packBody(msgId uint16, sid []byte, body []byte) []byte {
	transFrame := make([]byte, 24)
	transFrame[0] = (transFrame[0] &^ 0x7) | 0x2

	dataHeader := make([]byte, 10)
	binary.LittleEndian.PutUint16(dataHeader[4:6], msgId)
	binary.LittleEndian.PutUint16(dataHeader[6:8], uint16(len(body)))

	output := append(transFrame, dataHeader...)
	output = append(output, sid...)
	output = append(output, body...)

	// sum header
	output[24+8] = msgs.ChecksumHeader(output, 24+8)
	// TODO sum body
	return output
}

func (c *Client) Query(request []byte) ([]byte, error) {
	n, err := c.Socket.WriteToUDP(request, c.ServerAddr)
	if err != nil {
		return nil, err
	}
	if n != len(request) {
		return nil, fmt.Errorf("not sent all message")
	}
	response := c.read()
	// ignore 50 header bytes
	if n < 50 {
		return nil, fmt.Errorf("response less than header count(50 bytes)")
	}
	//log.Printf("rep: len(%d) %v", n, response[:n])
	return response[50:n], err
}

func (c *Client) doNewSession() (map[string]interface{}, error) {
	body := make([]byte, 24)
	copy(body[:8], c.MAC[:])
	copy(body[8:24], c.SN[:])

	req := c.packBody(CmdGetToken, c.Sid, body)
	rep, err := c.Query(req)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	ok := binary.LittleEndian.Uint32(rep[:4])
	m["code"] = ok
	m["sid"] = hex.EncodeToString(rep[4:20])
	copy(c.Sid[:], rep[4:20])
	if ok != 0 {
		log.Fatalf("doNewSession failed: %v", m)
	}
	return m, nil
}

func (c *Client) doRegister() (map[string]interface{}, error) {
	name := []byte(c.Name)
	body := make([]byte, 31+len(name))
	binary.LittleEndian.PutUint16(body[:2], c.DeviceType)
	copy(body[2:10], c.MAC[:])
	binary.LittleEndian.PutUint32(body[10:14], uint32(int32(c.ProduceTime.Unix())))
	copy(body[14:30], c.SN[:])
	body[30] = byte(len(name))
	copy(body[31:31+len(name)], name)

	req := c.packBody(CmdRegister, c.Sid, body)
	rep, err := c.Query(req)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	m["code"] = binary.LittleEndian.Uint32(rep[:4])
	m["id"] = int64(binary.LittleEndian.Uint64(rep[4:12]))
	copy(c.Id[:], rep[4:12])
	m["cookie"] = string(rep[12:76])
	copy(c.Cookie[:64], rep[12:76])
	return m, nil
}

func (c *Client) doLogin() (map[string]interface{}, error) {
	body := make([]byte, 72)
	copy(body[0:8], c.MAC[:])
	copy(body[8:72], c.Cookie[:])

	req := c.packBody(CmdLogin, c.Sid, body)
	rep, err := c.Query(req)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	m["code"] = binary.LittleEndian.Uint32(rep[:4])
	return m, nil
}

func (c *Client) doRename() (map[string]interface{}, error) {
	c.Name += "2"
	name := []byte(c.Name)
	body := make([]byte, 25+len(name))
	copy(body[:8], c.MAC[:])
	body[8] = byte(len(name))
	copy(body[9:9+len(name)], name)

	//log.Printf("SID: %v", c.Sid)
	req := c.packBody(CmdChangeName, c.Sid, body)
	rep, err := c.Query(req)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	m["code"] = binary.LittleEndian.Uint32(rep[:4])
	m["mac"] = hex.EncodeToString(rep[4:12])
	return m, nil
}

func (c *Client) doDoBind() (map[string]interface{}, error) {
	body := make([]byte, 32)
	copy(body[0:16], c.Sid[:])
	copy(body[16:24], c.MAC[:])
	copy(body[24:32], c.Id[:])

	req := c.packBody(CmdDoBind, c.Sid, body)
	rep, err := c.Query(req)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	m["sid"] = hex.EncodeToString(rep[:16])
	copy(c.Sid[:], rep[:16])
	m["mac"] = hex.EncodeToString(rep[16:24])
	m["code"] = binary.LittleEndian.Uint32(rep[24:28])
	return m, nil
}

func (c *Client) doHeartbeat() (map[string]interface{}, error) {
	body := make([]byte, 8)
	copy(body[:8], c.Id[:8])

	req := c.packBody(CmdHeartBeat, c.Sid, body)
	rep, err := c.Query(req)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	m["code"] = binary.LittleEndian.Uint32(rep[:4])
	return m, nil
}

func (c *Client) doOfflineSub() (map[string]interface{}, error) {
	body := make([]byte, 24)
	copy(body[0:16], c.Sid[:])
	copy(body[16:24], c.MAC[:])

	req := c.packBody(CmdSubDeviceOffline, c.Sid, body)
	rep, err := c.Query(req)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	m["sid"] = hex.EncodeToString(rep[:16])
	copy(c.Sid[:], rep[:16])
	m["mac"] = hex.EncodeToString(rep[16:24])
	return m, nil
}

func (c *Client) read() []byte {
	return <-c.rch
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
	log.SetFlags(log.Flags() | log.Lshortfile)
	if len(os.Args) < 2 {
		fmt.Printf("Usage: %v server-ip:server-port local-ip:local-port\n", os.Args[0])
		return
	}
	server, err := net.ResolveUDPAddr("udp", os.Args[1])
	if err != nil {
		fmt.Printf("Resolve server addr %s failed: %v\n", os.Args[1], err)
		return
	}
	local, err := net.ResolveUDPAddr("udp", os.Args[2])
	if err != nil {
		fmt.Printf("Resolve local addr %s failed: %v\n", os.Args[2], err)
		return
	}
	socket, err := net.ListenUDP("udp", local)
	if err != nil {
		fmt.Printf("Listen on local addr %v failed: %v\n", local, err)
		return
	}

	c := Client{
		Name:        "deviceName",
		ProduceTime: time.Now(),
		DeviceType:  0xFFFE,
		Sid:         make([]byte, 16),
		Socket:      socket,
		ServerAddr:  server,
		rch:         make(chan []byte, 128),
	}
	n, err := rand.Read(c.SN[:])
	if n != 16 || err != nil {
		fmt.Println("crypto/rand on SN failed:", n, err)
		return
	}
	n, err = rand.Read(c.MAC[:])
	if n != 8 || err != nil {
		fmt.Println("crypto/rand on MAC failed:", n, err)
		return
	}
	log.Printf("Client started, mac: %v, sn: %v", hex.EncodeToString(c.MAC[:]), hex.EncodeToString(c.SN[:]))
	c.Run()

	//fmt.Println("Connect ok, we can send message now(eg: <cmd> <body>)")

	//buf := make([]byte, 65536)
	//cmd := byte(0)
	//in := ""
	//inBuf := make([]byte, 0, 256)
	//for {
	//	inBuf = inBuf[:0]
	//	fmt.Print("send: ")
	//	nin, err := fmt.Scanf("%d %v\n", &cmd, &in)
	//	//fmt.Printf("args: %d, cmd: %v, msg: %v\n", nin, cmd, in)
	//	if len(in) == 0 {
	//		continue
	//	}
	//	inBuf = append(inBuf, cmd)
	//	inBuf = append(inBuf, []byte(in)...)

	//	fmt.Printf("sending...")
	//	_, err = socket.Write(inBuf)
	//	if err != nil {
	//		if op, ok := err.(*net.OpError); ok && op.Temporary() {
	//			fmt.Printf("error: sending failed on %v", err)
	//			continue
	//		}
	//		fmt.Printf("fatal: sending failed on %v", err)
	//		break
	//	}
	//	fmt.Printf("done\n")

	//	n, r, err := socket.ReadFromUDP(buf)
	//	if err != nil {
	//		if op, ok := err.(*net.OpError); ok && op.Temporary() {
	//			fmt.Printf("error: sending failed on %v", err)
	//			continue
	//		}
	//		fmt.Printf("fatal: sending failed on %v", err)
	//		break
	//	}
	//	if n == 0 {
	//		fmt.Printf("error: read empty data from %v", r)
	//		continue
	//	}
	//	fmt.Printf("read: code: %d, msg: %s\n", buf[0], string(buf[1:n]))
	//}
}
