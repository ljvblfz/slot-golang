// 测试用客户端
package main

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"time"
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
	Sid         [16]byte
	Token       [16]byte
	Cookie      [64]byte
	Id          [8]byte
	Name        string
	ProduceTime time.Time
	DeviceType  uint16
	Socket      *net.UDPConn
	ServerAddr  *net.UDPAddr
}

// get token
// register
// login
// changename
// bind
// heartbeat
// offline sub
func (c *Client) Run() {
	tests := []func() (map[string]interface{}, error){
		//c.doToken,
		c.doRegister,
		c.doLogin,
		c.doRename,
		c.doDoBind,
		c.doHeartbeat,
		c.doOfflineSub,
	}
	for _, t := range tests {
		result, err := t()
		if err != nil {
			log.Printf("[%v] error: %v", err)
		}
		if result == nil {
			continue
		}
		for k, v := range result {
			log.Printf("%s=%v", k, v)
		}
		log.Println("---")
	}
}

func (c *Client) packBody(msgId uint16, body []byte) []byte {
	transFrame := make([]byte, 24)
	transFrame[0] = (transFrame[0] &^ 0x7) | 0x2

	pkgFrame := make([]byte, 24)

	dataHeader := make([]byte, 12)
	binary.LittleEndian.PutUint16(dataHeader[3:5], msgId)

	output := append(append(append(transFrame, pkgFrame...), dataHeader...), body...)

	// sum header
	// sum body
	return output
}

func (c *Client) Send(request []byte) ([]byte, error) {
	n, err := c.Socket.WriteToUDP(request, c.ServerAddr)
	if err != nil {
		return nil, err
	}
	if n != len(request) {
		return nil, fmt.Errorf("not sent all message")
	}
	response := make([]byte, 256)
	for {
		n, peer, err := c.Socket.ReadFromUDP(response)
		if peer.String() != c.ServerAddr.String() {
			continue
		}
		if n < 60 {
			return nil, fmt.Errorf("response less than header count(60 bytes)")
		}
		log.Printf("rep: len(%d) %v", n, response[:n])
		return response[60:n], err
	}
}

func (c *Client) doToken() (map[string]interface{}, error) {
	body := make([]byte, 24)
	copy(body[:8], c.MAC[:])
	copy(body[8:24], c.SN[:])

	req := c.packBody(CmdGetToken, body)
	rep, err := c.Send(req)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	m["token"] = hex.EncodeToString(rep[:24])
	copy(c.Token[:], rep[:24])
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

	req := c.packBody(CmdRegister, body)
	rep, err := c.Send(req)
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

	req := c.packBody(CmdLogin, body)
	rep, err := c.Send(req)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	m["code"] = binary.LittleEndian.Uint32(rep[:4])
	m["sid"] = hex.EncodeToString(rep[4:20])
	copy(c.Sid[:], rep[4:20])
	return m, nil
}

func (c *Client) doRename() (map[string]interface{}, error) {
	c.Name += "2"
	name := []byte(c.Name)
	body := make([]byte, 25+len(name))
	copy(body[:16], c.Sid[:])
	copy(body[16:24], c.MAC[:])
	body[24] = byte(len(name))
	copy(body[25:25+len(name)], name)

	req := c.packBody(CmdChangeName, body)
	rep, err := c.Send(req)
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

func (c *Client) doDoBind() (map[string]interface{}, error) {
	body := make([]byte, 32)
	copy(body[0:16], c.Sid[:])
	copy(body[16:24], c.MAC[:])
	copy(body[24:32], c.Id[:])

	req := c.packBody(CmdDoBind, body)
	rep, err := c.Send(req)
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
	body := make([]byte, 24)
	copy(body[0:16], c.Sid[:])
	copy(body[16:24], c.Id[:])

	req := c.packBody(CmdHeartBeat, body)
	rep, err := c.Send(req)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	m["sid"] = hex.EncodeToString(rep[:16])
	copy(c.Sid[:], rep[:16])
	m["code"] = binary.LittleEndian.Uint32(rep[16:20])
	return m, nil
}

func (c *Client) doOfflineSub() (map[string]interface{}, error) {
	body := make([]byte, 24)
	copy(body[0:16], c.Sid[:])
	copy(body[16:24], c.MAC[:])

	req := c.packBody(CmdSubDeviceOffline, body)
	rep, err := c.Send(req)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	m["sid"] = hex.EncodeToString(rep[:16])
	copy(c.Sid[:], rep[:16])
	m["mac"] = hex.EncodeToString(rep[16:24])
	return m, nil
}

func main() {
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

	log.Println("Client started")

	c := Client{
		Name:        "deviceName",
		ProduceTime: time.Now(),
		DeviceType:  0xFFFE,
		Socket:      socket,
		ServerAddr:  server,
	}
	copy(c.SN[:], []byte("AAAAAAAAAAAAAAAC")[:16])
	copy(c.MAC[:], []byte("AAAAAAAC")[:8])
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
