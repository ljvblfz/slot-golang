package main

import (
	"github.com/golang/glog"
	uuid "github.com/nu7hatch/gouuid"
)

var (
//kBalabala = []byte{'h', 'a', 'd', 't', 'o', 'b', 'e', '8'}
)

func NewUuid() *uuid.UUID {
	id, _ := uuid.NewV4()
	return id
}

// TransId 转换ID到对应的所有ID,板子ID原样转换为自身ID,用户ID转换为所有手机ID
func TransId(id int64) []int64 {
	if id < 0 {
		return []int64{id}
	}
	if id%int64(kUseridUnit) != 0 {
		glog.Errorf("[bind|id] wrong id %d, invalid user id, it should be multiples of %d", id, kUseridUnit)
		return nil
	}
	var allIds []int64
	for i := 1; i < 16; i++ {
		allIds = append(allIds, id+int64(i))
	}
	return allIds
}
