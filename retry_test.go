package gtcp_test

import (
	"testing"
	"time"

	"github.com/cat2neat/gtcp"
)

func TestExponentialRetry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		expected time.Duration
	}{
		{expected: time.Millisecond},
		{expected: 2 * time.Millisecond},
		{expected: 4 * time.Millisecond},
		{expected: 8 * time.Millisecond},
		{expected: 16 * time.Millisecond},
		{expected: 20 * time.Millisecond},
		{expected: 20 * time.Millisecond},
	}
	var retry gtcp.Retry = gtcp.ExponentialRetry{
		InitialDelay: time.Millisecond,
		MaxDelay:     20 * time.Millisecond,
	}
	for i, test := range tests {
		ret := retry.Backoff(uint64(i))
		if test.expected != ret {
			t.Errorf("gtcp_test: ExponentialRetry.Backoff expected: %v actual: %v\n",
				test.expected, ret)
		}
	}
}
