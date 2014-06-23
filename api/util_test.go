package main

import (
	"testing"
)

func TestDeviceKey(t *testing.T) {
	mac := "AAxDMFDB"
	var id int64 = 3
	key := GenerateDeviceKey(mac, id)
	if !VerifyDeviceKey(mac, key) {
		t.Fail()
	}

	key2 := "3|7GlzuNyd7l8AhSX9sNetz7g6x3z5kqCrHv+vB4d0Ap="
	if key != key2 {
		t.Fatalf("key not equal: %s != %s", key, key2)
	}

	if !VerifyDeviceKey(mac, key2) {
		t.Fail()
	}
}
