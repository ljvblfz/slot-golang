// 测试用客户端
package main

import (
	"fmt"
	"net"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Usage: %v ip:port\n", os.Args[0])
		return
	}
	server, err := net.ResolveUDPAddr("udp", os.Args[1])
	if err != nil {
		fmt.Printf("Resolve server addr %s failed: %v\n", os.Args[1], err)
		return
	}
	con, err := net.DialUDP("udp", nil, server)
	if err != nil {
		fmt.Printf("Connect server failed: %v\n", err)
		return
	}

	fmt.Println("Connect ok, we can send message now(eg: <cmd> <body>)")

	buf := make([]byte, 65536)
	cmd := byte(0)
	in := ""
	inBuf := make([]byte, 0, 256)
	for {
		inBuf = inBuf[:0]
		fmt.Print("send: ")
		nin, err := fmt.Scanf("%d %v\n", &cmd, &in)
		//fmt.Printf("args: %d, cmd: %v, msg: %v\n", nin, cmd, in)
		if len(in) == 0 {
			continue
		}
		inBuf = append(inBuf, cmd)
		inBuf = append(inBuf, []byte(in)...)

		fmt.Printf("sending...")
		_, err = con.Write(inBuf)
		if err != nil {
			if op, ok := err.(*net.OpError); ok && op.Temporary() {
				fmt.Printf("error: sending failed on %v", err)
				continue
			}
			fmt.Printf("fatal: sending failed on %v", err)
			break
		}
		fmt.Printf("done\n")

		n, r, err := con.ReadFromUDP(buf)
		if err != nil {
			if op, ok := err.(*net.OpError); ok && op.Temporary() {
				fmt.Printf("error: sending failed on %v", err)
				continue
			}
			fmt.Printf("fatal: sending failed on %v", err)
			break
		}
		if n == 0 {
			fmt.Printf("error: read empty data from %v", r)
			continue
		}
		fmt.Printf("read: code: %d, msg: %s\n", buf[0], string(buf[1:n]))

	}
}
