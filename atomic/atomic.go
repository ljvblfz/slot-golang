package atomic

import (
	"sync/atomic"
)

type AtomicInt64 int64

func (this *AtomicInt64) Inc() int64 {
	return atomic.AddInt64((*int64)(this), 1)
}

func (this *AtomicInt64) Dec() int64 {
	return atomic.AddInt64((*int64)(this), -1)
}

func (this *AtomicInt64) Set(val int64) int64 {
	return atomic.SwapInt64((*int64)(this), val)
}

func (this *AtomicInt64) Get() int64 {
	return atomic.LoadInt64((*int64)(this))
}

func (this *AtomicInt64) CompareAndSwap(old, newv int64) bool {
	return atomic.CompareAndSwapInt64((*int64)(this), old, newv)
}

type AtomicUint64 uint64

func (this *AtomicUint64) Inc() uint64 {
	return atomic.AddUint64((*uint64)(this), 1)
}

func (this *AtomicUint64) Dec() uint64 {
	return atomic.AddUint64((*uint64)(this), ^(uint64)(0))
}

func (this *AtomicUint64) Set(val uint64) uint64 {
	return atomic.SwapUint64((*uint64)(this), val)
}

func (this *AtomicUint64) Get() uint64 {
	return atomic.LoadUint64((*uint64)(this))
}

func (this *AtomicUint64) CompareAndSwap(old, newv uint64) bool {
	return atomic.CompareAndSwapUint64((*uint64)(this), old, newv)
}

type AtomicBoolean int32

func (this *AtomicBoolean) Set(val bool) bool {
	var b int32 = 0
	if val {
		b = 1
	}
	return atomic.SwapInt32((*int32)(this), b) != 0
}

func (this *AtomicBoolean) Get() bool {
	return atomic.LoadInt32((*int32)(this)) == 1
}

func (this *AtomicBoolean) CompareAndSwap(old, newv bool) bool {
	var oldInt, newInt int32 = 0, 0
	if old {
		oldInt = 1
	}
	if newv {
		newInt = 1
	}
	return atomic.CompareAndSwapInt32((*int32)(this), oldInt, newInt)
}
