// Package stats provides thread-safe statistics collection and calculation
// for HTTP stress test results, including latency percentiles.
package stats

import (
	"sort"
	"sync"
)

// Collector collects and calculates statistics for stress test results.
// It is thread-safe and designed to handle concurrent result recording.
// The collector maintains latency data for percentile calculations and
// tracks success/failure counts and HTTP status code distribution.
type Collector struct {
	mu            sync.Mutex    // Protects all fields from concurrent access
	successes     int64         // Count of successful requests (2xx status)
	failures      int64         // Count of failed requests
	latencies     []float64     // All recorded latencies (for percentile calculation)
	statusCount   map[int]int   // Distribution of HTTP status codes
	minLatency    float64       // Minimum observed latency
	maxLatency    float64       // Maximum observed latency
	firstLatency  bool          // Flag to initialize min/max on first record
}

// NewCollector creates a new statistics collector with pre-allocated capacity.
// The initialCapacity parameter helps optimize memory allocation by reserving
// space for the expected number of latency records.
func NewCollector(initialCapacity int) *Collector {
	return &Collector{
		latencies:   make([]float64, 0, initialCapacity),
		statusCount: make(map[int]int),
		firstLatency: true,
	}
}

// Record adds a request result to the collector in a thread-safe manner.
// It updates success/failure counts, latency tracking, and status code distribution.
func (c *Collector) Record(statusCode int, elapsed float64, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.latencies = append(c.latencies, elapsed)
	// Track HTTP status code distribution (status code 0 indicates request errors)
	if statusCode != 0 {
		c.statusCount[statusCode]++
	} else {
		c.statusCount[0]++
	}

	// Track min/max latency in real-time
	if c.firstLatency {
		c.minLatency = elapsed
		c.maxLatency = elapsed
		c.firstLatency = false
	} else {
		if elapsed < c.minLatency {
			c.minLatency = elapsed
		}
		if elapsed > c.maxLatency {
			c.maxLatency = elapsed
		}
	}

	if ok {
		c.successes++
	} else {
		c.failures++
	}
}

// Statistics holds the calculated final statistics from a stress test run.
// All latency values are in seconds.
type Statistics struct {
	Successes      int64             // Total successful requests
	Failures       int64             // Total failed requests
	Total          int               // Total requests processed
	StatusCount    map[int]int       // Distribution of HTTP status codes
	MinLatency     float64           // Minimum latency in seconds
	MaxLatency     float64           // Maximum latency in seconds
	AvgLatency     float64           // Average latency in seconds
	P50Latency     float64           // 50th percentile (median) latency in seconds
	P90Latency     float64           // 90th percentile latency in seconds
	P99Latency     float64           // 99th percentile latency in seconds
}

// GetStatistics calculates and returns final statistics from all collected results.
// It sorts latencies, calculates percentiles using linear interpolation,
// and creates a thread-safe copy of the status code distribution.
// This operation should be called after all results have been recorded.
func (c *Collector) GetStatistics() Statistics {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.latencies) == 0 {
		return Statistics{
			StatusCount: c.statusCount,
		}
	}

	// Sort latencies for percentile calculation (create copy to avoid modifying original)
	latencies := make([]float64, len(c.latencies))
	copy(latencies, c.latencies)
	sort.Float64s(latencies)

	// Calculate average
	avgLatency := 0.0
	for _, l := range latencies {
		avgLatency += l
	}
	avgLatency /= float64(len(latencies))

	// Calculate percentiles using linear interpolation for accuracy
	p50 := percentile(latencies, 0.50) // Median
	p90 := percentile(latencies, 0.90) // 90th percentile
	p99 := percentile(latencies, 0.99) // 99th percentile

	// Create a copy of statusCount for thread safety
	statusCountCopy := make(map[int]int)
	for k, v := range c.statusCount {
		statusCountCopy[k] = v
	}

	return Statistics{
		Successes:   c.successes,
		Failures:    c.failures,
		Total:       len(c.latencies),
		StatusCount: statusCountCopy,
		MinLatency:  c.minLatency,
		MaxLatency:  c.maxLatency,
		AvgLatency:  avgLatency,
		P50Latency:  p50,
		P90Latency:  p90,
		P99Latency:  p99,
	}
}

// percentile calculates percentile using linear interpolation method.
// This approach provides more accurate percentile values than simple array indexing.
// The method uses the standard percentile formula: position = (N-1) * p,
// where N is the number of elements and p is the percentile (0.0 to 1.0).
// Linear interpolation between adjacent values provides smooth percentile estimates.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}

	n := float64(len(sorted))
	// Calculate position using standard percentile formula: (N-1) * p
	position := (n - 1) * p
	lower := int(position)
	upper := lower + 1

	// Handle edge case where upper index would be out of bounds
	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}

	// Perform linear interpolation between lower and upper values
	weight := position - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}