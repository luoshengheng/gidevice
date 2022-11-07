package perf

import (
	"testing"
	"time"
)

func Test_Performance(t *testing.T) {
	monitor := NewMonitor("")
	monitor.DumpPerformance([]string{})
	for {
		time.Sleep(time.Second)
	}
}
