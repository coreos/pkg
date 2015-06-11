package flagutil

import "testing"

func TestIPv4FlagSetInvalidArgument(t *testing.T) {
	tests := []string{
		"",
		"foo",
		"::",
		"127.0.0.1:4328",
	}

	for i, tt := range tests {
		var f IPv4Flag
		if err := f.Set(tt); err == nil {
			t.Errorf("case %d: expected non-nil error", i)
		}
	}
}

func TestIPv4FlagSetValidArgument(t *testing.T) {
	tests := []string{
		"127.0.0.1",
		"0.0.0.0",
	}

	for i, tt := range tests {
		var f IPv4Flag
		if err := f.Set(tt); err != nil {
			t.Errorf("case %d: err=%v", i, err)
		}
	}
}
