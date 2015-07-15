package main

import (
)

func selectUDPServer() (addr string) {
	addr = gQueue.next().(string)
	return
}
