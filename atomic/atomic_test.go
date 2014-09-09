package atomic

import (
	"testing"
)

func TestIncInt64(t *testing.T) {
	var aint AtomicInt64 = 0
	if aint.Inc() != 1 {
		t.Fatal("Error in TestIncInt64")
	}
}

func TestDecInt64(t *testing.T) {
	var aint AtomicInt64 = 1
	if aint.Dec() != 0 {
		t.Fatal("Error in TestDecInt64")
	}
}

func TestGetAndSetInt64(t *testing.T) {
	var aint AtomicInt64 = 0
	aint.Set(33)
	if aint != 33 {
		t.Fatal("Error in TestGetAndSetInt64__1")
	}
	if aint.Get() != 33 {
		t.Fatal("Error in TestGetAndSetInt64")
	}
}

func TestCompareAndSwapInt64(t *testing.T) {
	var aint AtomicInt64 = 1
	if !aint.CompareAndSwap(1, 33) && aint.Get() != 33 {
		t.Fatal("Error in TestCompareAndSwapInt64")
	}
}

func TestUincUint64(t *testing.T) {
	var aint AtomicUint64 = 0
	if aint.Inc() != 1 {
		t.Fatal("Uint64 TestUincUint64")
	}
}

func TestUdecUint64(t *testing.T) {
	var aint AtomicUint64 = 2
	if aint.Dec() != 1 {
		t.Fatal("Uint64 TestUdecUint64")
	}
}

func TestGetAndSetUint64(t *testing.T) {
	var aint AtomicUint64 = 0
	aint.Set(33)
	if aint != 33 {
		t.Fatal("Error in TestGetAndSetUint64__1")
	}
	if aint.Get() != 33 {
		t.Fatal("Error in TestGetAndSetUint64")
	}
}

func TestCompareAndSwapUint64(t *testing.T) {
	var aint AtomicUint64 = 1
	if !aint.CompareAndSwap(1, 33) && aint.Get() != 33 {
		t.Fatal("Error in TestCompareAndSwapUint64")
	}
}

func TestGetAndSetBool(t *testing.T) {
	var abool AtomicBoolean = 1
	if !abool.Get() {
		t.Fatal("AtomicBool TestGetAndSetBool_1")
	}
	abool.Set(false)
	if abool.Get() {
		t.Fatal("AtomicBool TestGetAndSetBool")
	}
}
