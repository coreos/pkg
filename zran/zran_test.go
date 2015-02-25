package zran

import (
	"io"
	"os"
	"testing"
)

const (
	testData = "./testdata/tomsawyer.gz"
	testAns  = "./testdata/ans"
)

type testRead struct {
	off int64
	l   int64
}

var tests = []testRead{
	testRead{0, 10},
	testRead{4, 17},
	testRead{1 << 20, 1 << 20},
	testRead{1 << 20 * 2, 500},
	testRead{1 << 20 * 3, 500},
	testRead{1 << 20 * 4, 500},
	testRead{1 << 20 * 5, 1 << 21},
}

// basic test
func TestBasic(t *testing.T) {
	// build index from gz file
	idx, err := BuildIndex(testData)
	if err != nil {
		t.Errorf("Failed to build index: %v", err)
	}

	for i := range tests {
		// get desired answer to check against
		f, err := os.Open(testAns)
		if err != nil {
			t.Errorf("Error opening answer file: %v", err)
		}
		f.Seek(tests[i].off, 0)
		ans := make([]byte, tests[i].l)
		f.Read(ans)
		f.Close()

		// extract and compare to desired answer
		b, err := Extract(testData, idx, tests[i].off, tests[i].l)
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
}
