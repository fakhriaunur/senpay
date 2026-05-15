package telemetry

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MetricLabel represents a key-value label pair for metrics.
type MetricLabel struct {
	Name  string
	Value string
}

// metricKey uniquely identifies a metric by its name and labels.
type metricKey struct {
	name   string
	labels string // sorted "key=value,key=value"
}

// counterMetric holds a single counter value.
type counterMetric struct {
	key    metricKey
	value  int64
	help   string
}

// histogramMetric holds duration observations.
type histogramMetric struct {
	key        metricKey
	count      int64
	sum        float64
	buckets    []float64
	bucketVals []int64
	help       string
}

// Metrics collects Prometheus-compatible metrics.
type Metrics struct {
	mu           sync.RWMutex
	counters     map[metricKey]*counterMetric
	histograms   map[metricKey]*histogramMetric
	requestCount atomic.Int64
	errorCount   atomic.Int64
}

// NewMetrics creates a new Metrics collector.
func NewMetrics() *Metrics {
	return &Metrics{
		counters:   make(map[metricKey]*counterMetric),
		histograms: make(map[metricKey]*histogramMetric),
	}
}

// RecordRequest increments the request counter and records duration.
func (m *Metrics) RecordRequest(method, path string, statusCode int, duration time.Duration) {
	m.requestCount.Add(1)
	if statusCode >= 500 {
		m.errorCount.Add(1)
	}

	labels := []MetricLabel{
		{Name: "method", Value: method},
		{Name: "path", Value: path},
		{Name: "status", Value: fmt.Sprintf("%d", statusCode)},
	}

	m.IncrementCounter("senpay_request_count", "Total HTTP requests", labels...)
	m.ObserveHistogram("senpay_request_duration_seconds", "HTTP request duration in seconds",
		[]float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
		duration.Seconds(),
		labels...,
	)

	if statusCode >= 500 {
		m.IncrementCounter("senpay_error_count", "Total HTTP errors", labels...)
	}
}

// IncrementCounter increments a counter metric.
func (m *Metrics) IncrementCounter(name, help string, labels ...MetricLabel) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := metricKey{name: name, labels: labelsKey(labels)}
	if c, ok := m.counters[key]; ok {
		c.value++
	} else {
		m.counters[key] = &counterMetric{
			key:   key,
			value: 1,
			help:  help,
		}
	}
}

// ObserveHistogram records a duration observation into histogram buckets.
func (m *Metrics) ObserveHistogram(name, help string, buckets []float64, value float64, labels ...MetricLabel) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := metricKey{name: name, labels: labelsKey(labels)}
	h, ok := m.histograms[key]
	if !ok {
		h = &histogramMetric{
			key:        key,
			buckets:    buckets,
			bucketVals: make([]int64, len(buckets)+1),
			help:       help,
		}
		m.histograms[key] = h
	}

	h.count++
	h.sum += value

	// Place value into buckets
	for i, b := range buckets {
		if value <= b {
			h.bucketVals[i]++
		}
	}
	// +Inf bucket
	h.bucketVals[len(buckets)]++
}

// MetricsHandler returns an http.Handler that serves metrics in Prometheus text format.
func (m *Metrics) MetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.WriteHeader(http.StatusOK)

		m.mu.RLock()
		defer m.mu.RUnlock()

		var b strings.Builder
		m.writeCounters(&b)
		m.writeHistograms(&b)
		fmt.Fprint(w, b.String())
	})
}

func (m *Metrics) writeCounters(b *strings.Builder) {
	// Collect and sort keys for deterministic output
	type kv struct {
		key   metricKey
		value int64
		help  string
	}
	var items []kv
	for _, c := range m.counters {
		items = append(items, kv{key: c.key, value: c.value, help: c.help})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].key.name < items[j].key.name ||
			(items[i].key.name == items[j].key.name && items[i].key.labels < items[j].key.labels)
	})

	for _, item := range items {
		fmt.Fprintf(b, "# HELP %s %s\n", item.key.name, item.help)
		fmt.Fprintf(b, "# TYPE %s counter\n", item.key.name)
		if item.key.labels != "" {
			fmt.Fprintf(b, "%s{%s} %d\n", item.key.name, item.key.labels, item.value)
		} else {
			fmt.Fprintf(b, "%s %d\n", item.key.name, item.value)
		}
	}
}

func (m *Metrics) writeHistograms(b *strings.Builder) {
	type kv struct {
		key   metricKey
		h     *histogramMetric
	}
	var items []kv
	for _, h := range m.histograms {
		items = append(items, kv{key: h.key, h: h})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].key.name < items[j].key.name ||
			(items[i].key.name == items[j].key.name && items[i].key.labels < items[j].key.labels)
	})

	for _, item := range items {
		h := item.h
		fmt.Fprintf(b, "# HELP %s %s\n", h.key.name, h.help)
		fmt.Fprintf(b, "# TYPE %s histogram\n", h.key.name)

		labelsStr := ""
		if h.key.labels != "" {
			labelsStr = "," + h.key.labels
		}

		for i, bucket := range h.buckets {
			fmt.Fprintf(b, "%s_bucket{le=%q%s} %d\n", h.key.name,
				fmt.Sprintf("%.3f", bucket), labelsStr, h.bucketVals[i])
		}
		fmt.Fprintf(b, "%s_bucket{le=\"+Inf\"%s} %d\n", h.key.name, labelsStr, h.bucketVals[len(h.buckets)])
		fmt.Fprintf(b, "%s_count%s %d\n", h.key.name, labelsStr, h.count)
		fmt.Fprintf(b, "%s_sum%s %.9f\n", h.key.name, labelsStr, h.sum)
	}
}

// labelsKey creates a sorted string key from label pairs.
func labelsKey(labels []MetricLabel) string {
	if len(labels) == 0 {
		return ""
	}
	sorted := make([]MetricLabel, len(labels))
	copy(sorted, labels)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})
	var parts []string
	for _, l := range sorted {
		parts = append(parts, fmt.Sprintf("%s=%q", l.Name, l.Value))
	}
	return strings.Join(parts, ",")
}
