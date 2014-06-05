package main

import (
	"fmt"
	"math/rand"
	"testing"
)

type Data struct {
	id		int64
	expire	int64
}

func TestEncodeAndDecode(t *testing.T) {

	testData := []Data{
		{1, 0},
		{1241, 1541},
		{99999999999, 9223372036854775807},
	}
	for i := 0; i < 1000; i++ {
		id := rand.Int63() + 1
		testData = append(testData, Data{int64(id), rand.Int63()})
	}

	for _, v := range testData {
		idSrc := v.id
		expireSrc := v.expire

		cond := fmt.Sprintf("id=%s, expire=%d", idSrc, expireSrc)

		res, err := Encode(idSrc, expireSrc)
		if err != nil {
			t.Fatalf("Encode failed: %v, cond: %s", err, cond)
		}

		idTest, expireTest, err := Decode(res)
		if err != nil {
			t.Fatalf("Decode failed: %v, cond: %s", err, cond)
		}

		if idSrc != idTest || expireSrc != expireTest {
			t.Errorf("Encode and Decode are not matched: %s", cond)
		}
	}
}
