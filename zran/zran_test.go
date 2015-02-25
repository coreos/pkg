package zran

import (
	"io"
	"os"
	"testing"
)

const (
	testData = "./testdata/tomsawyer.gz"
	testAns  = "./testdata/ans"
	offset   = 1<<20 + 4
	length   = 17
)

// basic test
func TestBasic(t *testing.T) {
	// build index from gz file
	idx, err := BuildIndex(testData)
	if err != nil {
		t.Errorf("Failed to build index: %v", err)
	}

	// get desired answer to check against
	f, err := os.Open(testAns)
	if err != nil {
		t.Errorf("Error opening answer file: %v", err)
	}
	f.Seek(offset, 0)
	ans := make([]byte, length)
	f.Read(ans)
	f.Close()

	// extract and compare to desired answer
	b, err := Extract(testData, idx, offset, length)
	if err != nil && err != io.EOF {
		t.Errorf("Extract error: %s\n", err)
	}
	if err == nil || err == io.EOF {
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
			return
		}
		t.Errorf("Extract returned: %s, Expected: %s\n", b, ans)
	}
}
