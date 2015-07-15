package main

import (
	"fmt"
	"strconv"
	"strings"
)

func selectUDPServer() (addr string) {
	addr = gQueue.next()
	return
}
