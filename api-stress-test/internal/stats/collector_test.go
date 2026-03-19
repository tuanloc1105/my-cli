package stats

import (
	"sync"
	"testing"
)

func TestCollectorRecord(t *testing.T) {
	c := NewCollector(10)

	c.Record(200, 0.1, true, "")
	c.Record(200, 0.2, true, "")
	c.Record(500, 0.3, false, "server error")
	c.Record(0, 0.05, false, "connection refused")

	stat := c.GetStatistics()

	if stat.Total != 4 {
		t.Errorf("total = %d, want 4", stat.Total)
	}
	if stat.Successes != 2 {
		t.Errorf("successes = %d, want 2", stat.Successes)
	}
	if stat.Failures != 2 {
		t.Errorf("failures = %d, want 2", stat.Failures)
	}
	if stat.StatusCount[200] != 2 {
		t.Errorf("status 200 count = %d, want 2", stat.StatusCount[200])
	}
	if stat.StatusCount[500] != 1 {
		t.Errorf("status 500 count = %d, want 1", stat.StatusCount[500])
	}
	if stat.StatusCount[0] != 1 {
		t.Errorf("status 0 count = %d, want 1", stat.StatusCount[0])
	}
}

func TestCollectorMinMaxLatency(t *testing.T) {
	c := NewCollector(5)

	c.Record(200, 0.5, true, "")
	c.Record(200, 0.1, true, "")
	c.Record(200, 0.9, true, "")

	stat := c.GetStatistics()

	if stat.MinLatency != 0.1 {
		t.Errorf("min latency = %f, want 0.1", stat.MinLatency)
	}
	if stat.MaxLatency != 0.9 {
		t.Errorf("max latency = %f, want 0.9", stat.MaxLatency)
	}
}

func TestCollectorAvgLatency(t *testing.T) {
	c := NewCollector(3)

	c.Record(200, 0.1, true, "")
	c.Record(200, 0.2, true, "")
	c.Record(200, 0.3, true, "")

	stat := c.GetStatistics()

	expected := 0.2
	if diff := stat.AvgLatency - expected; diff > 0.0001 || diff < -0.0001 {
		t.Errorf("avg latency = %f, want %f", stat.AvgLatency, expected)
	}
}

func TestCollectorErrorTracking(t *testing.T) {
	c := NewCollector(10)

	for i := 0; i < 5; i++ {
		c.Record(0, 0.1, false, "connection refused")
	}
	for i := 0; i < 3; i++ {
		c.Record(0, 0.1, false, "timeout")
	}
	c.Record(0, 0.1, false, "dns error")

	stat := c.GetStatistics()

	if len(stat.TopErrors) != 3 {
		t.Fatalf("got %d top errors, want 3", len(stat.TopErrors))
	}
	if stat.TopErrors[0].Message != "connection refused" || stat.TopErrors[0].Count != 5 {
		t.Errorf("top error = %v, want {connection refused, 5}", stat.TopErrors[0])
	}
	if stat.TopErrors[1].Message != "timeout" || stat.TopErrors[1].Count != 3 {
		t.Errorf("second error = %v, want {timeout, 3}", stat.TopErrors[1])
	}
}

func TestCollectorTopErrorsMaxFive(t *testing.T) {
	c := NewCollector(20)

	errors := []string{"err1", "err2", "err3", "err4", "err5", "err6", "err7"}
	for _, e := range errors {
		c.Record(0, 0.1, false, e)
	}

	stat := c.GetStatistics()
	if len(stat.TopErrors) != 5 {
		t.Errorf("got %d top errors, want max 5", len(stat.TopErrors))
	}
}

func TestCollectorConcurrency(t *testing.T) {
	c := NewCollector(1000)
	var wg sync.WaitGroup

	numGoroutines := 10
	recordsPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < recordsPerGoroutine; j++ {
				c.Record(200, 0.1, true, "")
			}
		}()
	}

	wg.Wait()
	stat := c.GetStatistics()

	expected := int64(numGoroutines * recordsPerGoroutine)
	if stat.Total != expected {
		t.Errorf("total = %d, want %d", stat.Total, expected)
	}
}

func TestCollectorNoRecords(t *testing.T) {
	c := NewCollector(10)
	stat := c.GetStatistics()

	if stat.Total != 0 {
		t.Errorf("total = %d, want 0", stat.Total)
	}
	if stat.Successes != 0 || stat.Failures != 0 {
		t.Error("expected zero successes and failures")
	}
}

func TestPercentile(t *testing.T) {
	tests := []struct {
		name     string
		data     []float64
		p        float64
		expected float64
	}{
		{"empty", []float64{}, 0.5, 0},
		{"single", []float64{1.0}, 0.5, 1.0},
		{"p0", []float64{1, 2, 3, 4, 5}, 0.0, 1.0},
		{"p100", []float64{1, 2, 3, 4, 5}, 1.0, 5.0},
		{"p50 odd", []float64{1, 2, 3, 4, 5}, 0.5, 3.0},
		{"p50 even", []float64{1, 2, 3, 4}, 0.5, 2.5},
		{"p90", []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 0.9, 9.1},
		{"p99", []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 0.99, 9.91},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := percentile(tt.data, tt.p)
			if diff := result - tt.expected; diff > 0.001 || diff < -0.001 {
				t.Errorf("percentile(%v, %f) = %f, want %f", tt.data, tt.p, result, tt.expected)
			}
		})
	}
}
