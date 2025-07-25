package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// MetricsRegistry manages application metrics
type MetricsRegistry struct {
	mu      sync.RWMutex
	enabled bool
	metrics map[string]Metric
	labels  map[string]string
}

// Metric represents a single metric
type Metric interface {
	GetName() string
	GetType() string
	GetValue() interface{}
	GetHelp() string
	GetLabels() map[string]string
}

// Counter represents a monotonically increasing metric
type Counter struct {
	name   string
	help   string
	value  int64
	labels map[string]string
}

// Gauge represents a metric that can go up and down
type Gauge struct {
	name   string
	help   string
	value  int64
	labels map[string]string
}

// Histogram represents a metric with buckets for distribution
type Histogram struct {
	name        string
	help        string
	buckets     []float64
	counts      []int64
	sum         int64
	totalCount  int64
	labels      map[string]string
	mu          sync.RWMutex
}

// Timer measures duration
type Timer struct {
	name      string
	help      string
	durations []time.Duration
	mu        sync.RWMutex
	labels    map[string]string
}

// Global metrics registry
var defaultRegistry *MetricsRegistry
var once sync.Once

// GetRegistry returns the default metrics registry
func GetRegistry() *MetricsRegistry {
	once.Do(func() {
		defaultRegistry = NewRegistry()
	})
	return defaultRegistry
}

// NewRegistry creates a new metrics registry
func NewRegistry() *MetricsRegistry {
	return &MetricsRegistry{
		enabled: true,
		metrics: make(map[string]Metric),
		labels:  make(map[string]string),
	}
}

// SetEnabled enables or disables metrics collection
func (r *MetricsRegistry) SetEnabled(enabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enabled = enabled
}

// IsEnabled returns whether metrics collection is enabled
func (r *MetricsRegistry) IsEnabled() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.enabled
}

// AddGlobalLabel adds a label that will be applied to all metrics
func (r *MetricsRegistry) AddGlobalLabel(key, value string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.labels[key] = value
}

// NewCounter creates and registers a new counter
func (r *MetricsRegistry) NewCounter(name, help string, labels map[string]string) *Counter {
	if !r.IsEnabled() {
		return &Counter{name: name, help: help, labels: labels}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if labels == nil {
		labels = make(map[string]string)
	}

	// Add global labels
	for k, v := range r.labels {
		if _, exists := labels[k]; !exists {
			labels[k] = v
		}
	}

	counter := &Counter{
		name:   name,
		help:   help,
		labels: labels,
	}

	r.metrics[name] = counter
	return counter
}

// NewGauge creates and registers a new gauge
func (r *MetricsRegistry) NewGauge(name, help string, labels map[string]string) *Gauge {
	if !r.IsEnabled() {
		return &Gauge{name: name, help: help, labels: labels}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if labels == nil {
		labels = make(map[string]string)
	}

	// Add global labels
	for k, v := range r.labels {
		if _, exists := labels[k]; !exists {
			labels[k] = v
		}
	}

	gauge := &Gauge{
		name:   name,
		help:   help,
		labels: labels,
	}

	r.metrics[name] = gauge
	return gauge
}

// NewHistogram creates and registers a new histogram
func (r *MetricsRegistry) NewHistogram(name, help string, buckets []float64, labels map[string]string) *Histogram {
	if !r.IsEnabled() {
		return &Histogram{name: name, help: help, buckets: buckets, labels: labels}
	}

	if buckets == nil {
		buckets = []float64{0.001, 0.01, 0.1, 0.5, 1.0, 2.5, 5.0, 10.0}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if labels == nil {
		labels = make(map[string]string)
	}

	// Add global labels
	for k, v := range r.labels {
		if _, exists := labels[k]; !exists {
			labels[k] = v
		}
	}

	histogram := &Histogram{
		name:    name,
		help:    help,
		buckets: buckets,
		counts:  make([]int64, len(buckets)),
		labels:  labels,
	}

	r.metrics[name] = histogram
	return histogram
}

// NewTimer creates and registers a new timer
func (r *MetricsRegistry) NewTimer(name, help string, labels map[string]string) *Timer {
	if !r.IsEnabled() {
		return &Timer{name: name, help: help, labels: labels}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if labels == nil {
		labels = make(map[string]string)
	}

	// Add global labels
	for k, v := range r.labels {
		if _, exists := labels[k]; !exists {
			labels[k] = v
		}
	}

	timer := &Timer{
		name:   name,
		help:   help,
		labels: labels,
	}

	r.metrics[name] = timer
	return timer
}

// GetMetrics returns all registered metrics
func (r *MetricsRegistry) GetMetrics() map[string]Metric {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]Metric)
	for k, v := range r.metrics {
		result[k] = v
	}
	return result
}

// Counter methods

// Inc increments the counter by 1
func (c *Counter) Inc() {
	atomic.AddInt64(&c.value, 1)
}

// Add adds the given value to the counter
func (c *Counter) Add(value int64) {
	if value < 0 {
		return // Counters cannot decrease
	}
	atomic.AddInt64(&c.value, value)
}

// GetName returns the metric name
func (c *Counter) GetName() string {
	return c.name
}

// GetType returns the metric type
func (c *Counter) GetType() string {
	return "counter"
}

// GetValue returns the current value
func (c *Counter) GetValue() interface{} {
	return atomic.LoadInt64(&c.value)
}

// GetHelp returns the help text
func (c *Counter) GetHelp() string {
	return c.help
}

// GetLabels returns the labels
func (c *Counter) GetLabels() map[string]string {
	return c.labels
}

// Gauge methods

// Set sets the gauge to a specific value
func (g *Gauge) Set(value int64) {
	atomic.StoreInt64(&g.value, value)
}

// Inc increments the gauge by 1
func (g *Gauge) Inc() {
	atomic.AddInt64(&g.value, 1)
}

// Dec decrements the gauge by 1
func (g *Gauge) Dec() {
	atomic.AddInt64(&g.value, -1)
}

// Add adds the given value to the gauge
func (g *Gauge) Add(value int64) {
	atomic.AddInt64(&g.value, value)
}

// GetName returns the metric name
func (g *Gauge) GetName() string {
	return g.name
}

// GetType returns the metric type
func (g *Gauge) GetType() string {
	return "gauge"
}

// GetValue returns the current value
func (g *Gauge) GetValue() interface{} {
	return atomic.LoadInt64(&g.value)
}

// GetHelp returns the help text
func (g *Gauge) GetHelp() string {
	return g.help
}

// GetLabels returns the labels
func (g *Gauge) GetLabels() map[string]string {
	return g.labels
}

// Histogram methods

// Observe adds an observation to the histogram
func (h *Histogram) Observe(value float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	atomic.AddInt64(&h.totalCount, 1)
	atomic.AddInt64(&h.sum, int64(value*1000)) // Store as milliseconds

	for i, bucket := range h.buckets {
		if value <= bucket {
			atomic.AddInt64(&h.counts[i], 1)
		}
	}
}

// GetName returns the metric name
func (h *Histogram) GetName() string {
	return h.name
}

// GetType returns the metric type
func (h *Histogram) GetType() string {
	return "histogram"
}

// GetValue returns the histogram data
func (h *Histogram) GetValue() interface{} {
	h.mu.RLock()
	defer h.mu.RUnlock()

	counts := make([]int64, len(h.counts))
	for i, count := range h.counts {
		counts[i] = atomic.LoadInt64(&count)
	}

	return map[string]interface{}{
		"buckets":     h.buckets,
		"counts":      counts,
		"sum":         atomic.LoadInt64(&h.sum),
		"total_count": atomic.LoadInt64(&h.totalCount),
	}
}

// GetHelp returns the help text
func (h *Histogram) GetHelp() string {
	return h.help
}

// GetLabels returns the labels
func (h *Histogram) GetLabels() map[string]string {
	return h.labels
}

// Timer methods

// Start starts timing and returns a function to stop timing
func (t *Timer) Start() func() {
	start := time.Now()
	return func() {
		t.Record(time.Since(start))
	}
}

// Record records a duration
func (t *Timer) Record(duration time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.durations = append(t.durations, duration)
	
	// Keep only last 1000 measurements to avoid memory issues
	if len(t.durations) > 1000 {
		t.durations = t.durations[len(t.durations)-1000:]
	}
}

// GetName returns the metric name
func (t *Timer) GetName() string {
	return t.name
}

// GetType returns the metric type
func (t *Timer) GetType() string {
	return "timer"
}

// GetValue returns timer statistics
func (t *Timer) GetValue() interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(t.durations) == 0 {
		return map[string]interface{}{
			"count": 0,
			"min":   0,
			"max":   0,
			"avg":   0,
		}
	}

	var total time.Duration
	min := t.durations[0]
	max := t.durations[0]

	for _, d := range t.durations {
		total += d
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
	}

	avg := total / time.Duration(len(t.durations))

	return map[string]interface{}{
		"count": len(t.durations),
		"min":   min.Nanoseconds() / 1e6, // Convert to milliseconds
		"max":   max.Nanoseconds() / 1e6,
		"avg":   avg.Nanoseconds() / 1e6,
	}
}

// GetHelp returns the help text
func (t *Timer) GetHelp() string {
	return t.help
}

// GetLabels returns the labels
func (t *Timer) GetLabels() map[string]string {
	return t.labels
}

// System metrics collection

// SystemMetrics represents system-level metrics
type SystemMetrics struct {
	registry *MetricsRegistry
	
	// Runtime metrics
	goroutines    *Gauge
	gcPauseTotal  *Counter
	gcPauseLast   *Gauge
	memAlloc      *Gauge
	memSys        *Gauge
	heapAlloc     *Gauge
	heapSys       *Gauge
	heapInuse     *Gauge
	stackInuse    *Gauge
	
	// Process metrics
	openFDs       *Gauge
	maxFDs        *Gauge
	
	stopCh        chan struct{}
	running       bool
	mu            sync.Mutex
}

// NewSystemMetrics creates a new system metrics collector
func NewSystemMetrics(registry *MetricsRegistry) *SystemMetrics {
	if registry == nil {
		registry = GetRegistry()
	}

	return &SystemMetrics{
		registry:      registry,
		goroutines:    registry.NewGauge("go_goroutines", "Number of goroutines", nil),
		gcPauseTotal:  registry.NewCounter("go_gc_pause_total_ns", "Total GC pause time in nanoseconds", nil),
		gcPauseLast:   registry.NewGauge("go_gc_pause_last_ns", "Last GC pause time in nanoseconds", nil),
		memAlloc:      registry.NewGauge("go_memstats_alloc_bytes", "Number of bytes allocated and still in use", nil),
		memSys:        registry.NewGauge("go_memstats_sys_bytes", "Number of bytes obtained from system", nil),
		heapAlloc:     registry.NewGauge("go_memstats_heap_alloc_bytes", "Number of heap bytes allocated and still in use", nil),
		heapSys:       registry.NewGauge("go_memstats_heap_sys_bytes", "Number of heap bytes obtained from system", nil),
		heapInuse:     registry.NewGauge("go_memstats_heap_inuse_bytes", "Number of heap bytes that are in use", nil),
		stackInuse:    registry.NewGauge("go_memstats_stack_inuse_bytes", "Number of stack bytes that are in use", nil),
		stopCh:        make(chan struct{}),
	}
}

// Start begins collecting system metrics
func (sm *SystemMetrics) Start(ctx context.Context, interval time.Duration) {
	sm.mu.Lock()
	if sm.running {
		sm.mu.Unlock()
		return
	}
	sm.running = true
	sm.mu.Unlock()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-sm.stopCh:
			return
		case <-ticker.C:
			sm.collect()
		}
	}
}

// Stop stops collecting system metrics
func (sm *SystemMetrics) Stop() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	if sm.running {
		close(sm.stopCh)
		sm.running = false
	}
}

// collect gathers system metrics
func (sm *SystemMetrics) collect() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Runtime metrics
	sm.goroutines.Set(int64(runtime.NumGoroutine()))
	sm.gcPauseTotal.Add(int64(m.PauseTotalNs))
	if len(m.PauseNs) > 0 {
		sm.gcPauseLast.Set(int64(m.PauseNs[(m.NumGC+255)%256]))
	}
	
	// Memory metrics
	sm.memAlloc.Set(int64(m.Alloc))
	sm.memSys.Set(int64(m.Sys))
	sm.heapAlloc.Set(int64(m.HeapAlloc))
	sm.heapSys.Set(int64(m.HeapSys))
	sm.heapInuse.Set(int64(m.HeapInuse))
	sm.stackInuse.Set(int64(m.StackInuse))
}

// HTTP Metrics Handler

// HTTPMetricsHandler provides an HTTP endpoint for metrics
func HTTPMetricsHandler(registry *MetricsRegistry) http.HandlerFunc {
	if registry == nil {
		registry = GetRegistry()
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		metrics := registry.GetMetrics()
		
		// Build metrics response
		response := map[string]interface{}{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"metrics":   make(map[string]interface{}),
		}

		for name, metric := range metrics {
			response["metrics"].(map[string]interface{})[name] = map[string]interface{}{
				"name":   metric.GetName(),
				"type":   metric.GetType(),
				"help":   metric.GetHelp(),
				"value":  metric.GetValue(),
				"labels": metric.GetLabels(),
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(response); err != nil {
			http.Error(w, fmt.Sprintf("Failed to encode metrics: %v", err), http.StatusInternalServerError)
		}
	}
}

// Convenience functions for global registry

// NewCounter creates a counter in the global registry
func NewCounter(name, help string, labels map[string]string) *Counter {
	return GetRegistry().NewCounter(name, help, labels)
}

// NewGauge creates a gauge in the global registry
func NewGauge(name, help string, labels map[string]string) *Gauge {
	return GetRegistry().NewGauge(name, help, labels)
}

// NewHistogram creates a histogram in the global registry
func NewHistogram(name, help string, buckets []float64, labels map[string]string) *Histogram {
	return GetRegistry().NewHistogram(name, help, buckets, labels)
}

// NewTimer creates a timer in the global registry
func NewTimer(name, help string, labels map[string]string) *Timer {
	return GetRegistry().NewTimer(name, help, labels)
}

// SetEnabled enables or disables metrics in the global registry
func SetEnabled(enabled bool) {
	GetRegistry().SetEnabled(enabled)
}

// AddGlobalLabel adds a global label to the default registry
func AddGlobalLabel(key, value string) {
	GetRegistry().AddGlobalLabel(key, value)
}