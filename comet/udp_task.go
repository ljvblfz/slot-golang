package main

import (
	"fmt"
	"sync"
)

var (
	gReqs   = make(map[bindingReq]chan error, 128)
	gReqsMu = sync.Mutex{}
)

type bindingReq struct {
	deviceId int64
	userId   int64
}

func AppendBindingRequest(c chan error, deviceId int64, userId int64) error {
	gReqsMu.Lock()
	defer gReqsMu.Unlock()

	req := bindingReq{
		deviceId: deviceId,
		userId:   userId,
	}
	gReqs[req] = c
	return nil
}

//func AckRequest(deviceId

func FinishBindingRequest(deviceId int64, userId int64) error {
	gReqsMu.Lock()
	defer gReqsMu.Unlock()

	req := bindingReq{
		deviceId: deviceId,
		userId:   userId,
	}
	c, ok := gReqs[req]
	if !ok {
		err := fmt.Errorf("no such request")
		c <- err
		return err
	}
	c <- nil
	return nil
}
