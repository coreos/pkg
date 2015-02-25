package zran

import (
	"fmt"
	"io"
	"os"
	"testing"
)

const (
	testData = "./testdata/tomsawyer.gz"
	testAns  = "./testdata/ans"
	offset   = 4 //change to 1<<20 to see error when reading from 2nd access point
	length   = 17
)

// basic test
func TestBasic(t *testing.T) {
	// build index from gz file
	idx, err := BuildIndex(testData)
	if err != nil {
		t.Errorf("Failed to build index: %v", err)
	}

	// get desired answer to check with
	f, err := os.Open(testAns)
	f.Seek(offset, 0)
	ans := make([]byte, length)
	f.Read(ans)
	f.Close()

	// extract and compare to desired answer
	b, err := Extract(testData, idx, offset, length, 0)
	if err == nil || err == io.EOF {
		//s += fmt.Sprintln("Success at", i, "\n", string(b))
		var fail bool
		if len(b) != len(ans) {
			fail = true
		} else {
			for i := range b {
				if b[i] != ans[i] {
					fail = true
				}

			}
		}
		if !fail {
			fmt.Printf("Success, read bytes: %s\n", b)
			return
		}

	}
	t.Errorf("Got: %s, Expected: %s", b, ans)
}
