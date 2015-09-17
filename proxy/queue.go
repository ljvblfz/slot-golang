package main

import (
	"cloud-base/atomic"
	"sync"
)

type IQueue interface {
	take() (ele interface{})
}

type Node struct {
	prev  *Node
	next  *Node
	value interface{}
}

type Queue struct {
	head       *Node
	tail       *Node
	count      atomic.AtomicUint64
	lock       *sync.Mutex
	notEmpty   *sync.Cond
	notFull    *sync.Cond
	capacity   uint64
	cursor     *Node
	cursorlock *sync.Mutex
}

func NewQueue() *Queue {
	q := new(Queue)
	q.lock = new(sync.Mutex)
	q.cursorlock = &sync.Mutex{}
	q.notEmpty = sync.NewCond(q.lock)
	q.notFull = sync.NewCond(q.lock)
	q.count.Set(0)
	q.capacity = 1<<64 - 1
	q.head = nil
	q.tail = nil
	return q
}

func (this *Queue) Put(v interface{}) {
	node := new(Node)
	node.value = v
	this.lock.Lock()
	defer this.lock.Unlock()
	for this.linkLast(node) == false {
		this.notEmpty.Wait()
	}
}
func (this *Queue) linkLast(node *Node) bool {
	if this.count.Get() >= this.capacity {
		return false
	}
	l := this.tail
	node.prev = l
	this.tail = node
	if this.head == nil {
		this.head = node
	} else {
		l.next = node
	}
	this.count.Inc()
	this.notEmpty.Signal()
	return true
}
func (this *Queue) Poll() (v interface{}) {
	return this.pollFirst()
}
func (this *Queue) pollFirst() (v interface{}) {
	this.lock.Lock()
	defer this.lock.Unlock()
	return this.unlinkFirst()
}
func (this *Queue) unlinkFirst() (v interface{}) {
	f := this.head
	if f == nil {
		return nil
	}
	n := f.next
	item := f.value
	f.value = nil
	f.next = f // help GC
	this.head = n
	if n == nil {
		this.tail = nil
	} else {
		n.prev = nil
	}
	this.count.Dec()
	this.notFull.Signal()
	return item

}
func (this *Queue) Remove(v interface{}) bool {
	if v == nil {
		return false
	}
	this.lock.Lock()
	defer this.lock.Unlock()
	for p := this.head; p != nil; p = p.next {
		if v == p.value {
			this.unlink(p)
			return true
		}
	}
	return false
}
func (this *Queue) unlink(x *Node) {

	// assert lock.isHeldByCurrentThread();
	p := x.prev
	n := x.next
	if p == nil {
		this.unlinkFirst()
	} else if n == nil {
		this.unlinkLast()
	} else {
		p.next = n
		n.prev = p
		x.value = nil
		// Don't mess with x's links.  They may still be in use by
		// an iterator.
		this.count.Dec()
		this.notFull.Signal()
	}

}

func (this *Queue) unlinkLast() interface{} {
	l := this.tail
	if l == nil {
		return nil
	}
	p := l.prev
	item := l.value
	l.value = nil
	l.prev = l // help GC
	this.tail = p
	if p == nil {
		this.head = nil
	} else {

		p.next = nil
	}
	this.count.Dec()
	this.notFull.Signal()
	return item
}
func (this *Queue) contains(o interface{}) bool {
	if o == nil {
		return false
	}
	this.lock.Lock()
	defer this.lock.Unlock()
	for p := this.head; p != nil; p = p.next {
		if o == p.value {
			return true
		}
	}
	return false
}
func (this *Queue) next() interface{} {
	this.cursorlock.Lock()
	defer this.cursorlock.Unlock()
	var o interface{}
	if this.cursor == nil {
		this.cursor = this.head
	}
	if this.cursor != nil {
		o = this.cursor.value
	}
	this.cursor = this.cursor.next
	return o
}
