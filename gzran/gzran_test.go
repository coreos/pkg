package gzran

import "testing"

const (
	testData = "./testdata/tomsawyerlong.gz"
)

// basic test
func TestBasic(t *testing.T) {
	// build index from gz file
	_, err := BuildIndex(testData)
	if err != nil {
		t.Errorf("Failed to build index: %v", err)
	}
}
