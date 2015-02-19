package zran

import (
	"fmt"
	"testing"
)

const testData = "./testdata/tomsawyer.gz"

// Mock test demonstrating intended use of library, doesn't check correctness
func TestBasic(t *testing.T) {
	// build index from gz file
	idx, err := BuildIndex(testData)
	// read bytes 100-200 and print
	b, err := Extract(testData, idx, 100, 100)
	if err != nil {
		t.Errorf("Recieved error %s and read %s bytes", err, len(b))
	} else {
		fmt.Println(string(b))
	}
}
