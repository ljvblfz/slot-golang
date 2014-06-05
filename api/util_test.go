package main

import (
	"testing"
)

func TestDeviceKey(t *testing.T) {
	mac := "918101"
	var id int64 = 125491310
	key := GenerateDeviceKey(mac, id)
	if !VerifyDeviceKey(mac, key) {
		t.Fail()
	}
}
