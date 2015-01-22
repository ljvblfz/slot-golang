package main

import (
	"testing"
)

func TestToken(t *testing.T) {
	sn := [][]byte{
		[]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		[]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		[]byte{1, 2, 3, 4, 5, 6, 7, 8, 0, 0, 0, 0, 0, 0, 0, 0},
		[]byte{0, 0, 0, 0, 0, 0, 0, 0, 9, 10, 11, 12, 13, 14, 15, 16},
		[]byte{213, 128, 129, 121, 82, 1, 8, 190, 218, 217, 91, 7, 9, 91, 5, 15},
		[]byte{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255},
	}
	for _, s := range sn {
		token, err := NewToken(s)
		if err != nil {
			t.Fatalf("NewToken for sn %v error: %v", s, err)
		}
		err = VerifyToken(s, token)
		if err != nil {
			t.Errorf("NewToken for sn %v error: %v", s, err)
		}
	}
}
