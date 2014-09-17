package main

import (
	"fmt"
	"cloud-base/hlist"
	"sync"
	"testing"
)

func TestSession(t *testing.T) {
	g := InitSessionList()
	lens := 10
	els := make([]*hlist.Element, 0, lens)
	wg := sync.WaitGroup{}
	wg.Add(lens)
	for i := 0; i < lens; i++ {
		go func(id int) {
			s1 := NewSession(int64(id), "a", "1111", nil, nil)
			e1 := g.AddSession(s1)
			els = append(els, e1)
			wg.Done()
		}(i)
	}

	wg.Wait()

	wg.Add(lens)
	for i := 0; i < lens; i++ {
		go func(id int) {
			g.RemoveSession(els[id])
			fmt.Println("remove:", els[id])
			wg.Done()
		}(i)
	}
	wg.Wait()
	fmt.Printf("%#v\n", g.kv)
}
