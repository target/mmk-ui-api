package service

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/target/mmk-ui-api/internal/service/rules"
)

// RulesEngineMetrics tracks metrics for the rules engine processing pipeline.
type RulesEngineMetrics struct {
	mu sync.RWMutex

	// Event processing metrics
	EventsIngested   int64 `json:"events_ingested"`
	EventsFiltered   int64 `json:"events_filtered"`
	EventsProcessed  int64 `json:"events_processed"`
	EventsSkipped    int64 `json:"events_skipped"`
	ProcessingErrors int64 `json:"processing_errors"`

	// Rules job metrics
	RulesJobsEnqueued  int64 `json:"rules_jobs_enqueued"`
	RulesJobsCompleted int64 `json:"rules_jobs_completed"`
	RulesJobsFailed    int64 `json:"rules_jobs_failed"`

	// Alert metrics
	AlertsGenerated     int64 `json:"alerts_generated"`
	UnknownDomainAlerts int64 `json:"unknown_domain_alerts"`
	IOCAlerts           int64 `json:"ioc_alerts"`

	// Cache metrics
	CacheHits   int64 `json:"cache_hits"`
	CacheMisses int64 `json:"cache_misses"`
	CacheWrites int64 `json:"cache_writes"`

	// Performance metrics
	AvgProcessingTime time.Duration `json:"avg_processing_time"`
	LastProcessedAt   time.Time     `json:"last_processed_at"`

	// Internal tracking
	totalProcessingTime time.Duration
	processingCount     int64
	sink                metricsSink
}

type metricsSink interface {
	Count(name string, value int64, tags map[string]string)
}

// NewRulesEngineMetrics creates a new metrics tracker.
func NewRulesEngineMetrics() *RulesEngineMetrics {
	return &RulesEngineMetrics{}
}

// SetSink wires a metrics sink used to emit external metrics (e.g., StatsD).
func (m *RulesEngineMetrics) SetSink(sink metricsSink) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sink = sink
}

// RecordEventIngestion records metrics for event ingestion.
func (m *RulesEngineMetrics) RecordEventIngestion(total, filtered int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.EventsIngested += int64(total)
	m.EventsFiltered += int64(filtered)
}

// RecordEventProcessing records metrics for event processing.
func (m *RulesEngineMetrics) RecordEventProcessing(processed, skipped, errors int, processingTime time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.EventsProcessed += int64(processed)
	m.EventsSkipped += int64(skipped)
	m.ProcessingErrors += int64(errors)
	m.LastProcessedAt = time.Now()

	// Update average processing time
	m.totalProcessingTime += processingTime
	m.processingCount++
	if m.processingCount > 0 {
		m.AvgProcessingTime = m.totalProcessingTime / time.Duration(m.processingCount)
	}
}

// RecordRulesJob records metrics for rules job processing.
func (m *RulesEngineMetrics) RecordRulesJob(enqueued, completed, failed int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.RulesJobsEnqueued += int64(enqueued)
	m.RulesJobsCompleted += int64(completed)
	m.RulesJobsFailed += int64(failed)
}

// RecordAlerts records metrics for alert generation.
func (m *RulesEngineMetrics) RecordAlerts(total, unknownDomain, ioc int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.AlertsGenerated += int64(total)
	m.UnknownDomainAlerts += int64(unknownDomain)
	m.IOCAlerts += int64(ioc)
}

func (m *RulesEngineMetrics) RecordCacheEvent(e rules.CacheEvent) {
	m.mu.Lock()

	switch e.Op {
	case rules.OpHit:
		if e.Ok {
			m.CacheHits++
		}
	case rules.OpMiss:
		m.CacheMisses++
	case rules.OpWrite:
		if e.Ok {
			m.CacheWrites++
		}
	}

	sink := m.sink
	m.mu.Unlock()

	emitCacheMetric(sink, e)
}

func emitCacheMetric(sink metricsSink, e rules.CacheEvent) {
	if sink == nil {
		return
	}
	tags := map[string]string{
		"cache": string(e.Name),
		"tier":  string(e.Tier),
		"op":    string(e.Op),
		"ok":    strconv.FormatBool(e.Ok),
	}
	sink.Count("cache.event", 1, tags)
}

// GetSnapshot returns a snapshot of current metrics.
func (m *RulesEngineMetrics) GetSnapshot() RulesEngineMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Create a copy without the mutex to avoid copylocks issue
	return RulesEngineMetrics{
		EventsIngested:      m.EventsIngested,
		EventsFiltered:      m.EventsFiltered,
		EventsProcessed:     m.EventsProcessed,
		EventsSkipped:       m.EventsSkipped,
		ProcessingErrors:    m.ProcessingErrors,
		RulesJobsEnqueued:   m.RulesJobsEnqueued,
		RulesJobsCompleted:  m.RulesJobsCompleted,
		RulesJobsFailed:     m.RulesJobsFailed,
		AlertsGenerated:     m.AlertsGenerated,
		UnknownDomainAlerts: m.UnknownDomainAlerts,
		IOCAlerts:           m.IOCAlerts,
		CacheHits:           m.CacheHits,
		CacheMisses:         m.CacheMisses,
		CacheWrites:         m.CacheWrites,
		AvgProcessingTime:   m.AvgProcessingTime,
		LastProcessedAt:     m.LastProcessedAt,
		totalProcessingTime: m.totalProcessingTime,
		processingCount:     m.processingCount,
	}
}

// Reset resets all metrics to zero without replacing the mutex (which is unsafe).
func (m *RulesEngineMetrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Zero all exported counters and internal tracking fields explicitly
	m.EventsIngested = 0
	m.EventsFiltered = 0
	m.EventsProcessed = 0
	m.EventsSkipped = 0
	m.ProcessingErrors = 0
	m.RulesJobsEnqueued = 0
	m.RulesJobsCompleted = 0
	m.RulesJobsFailed = 0
	m.AlertsGenerated = 0
	m.UnknownDomainAlerts = 0
	m.IOCAlerts = 0
	m.CacheHits = 0
	m.CacheMisses = 0
	m.CacheWrites = 0
	m.AvgProcessingTime = 0
	m.LastProcessedAt = time.Time{}
	m.totalProcessingTime = 0
	m.processingCount = 0
}

// GetCacheHitRatio returns the cache hit ratio as a percentage.
func (m *RulesEngineMetrics) GetCacheHitRatio() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	total := m.CacheHits + m.CacheMisses
	if total == 0 {
		return 0.0
	}
	return float64(m.CacheHits) / float64(total) * 100.0
}

// GetProcessingRate returns events processed per second over the last period.
func (m *RulesEngineMetrics) GetProcessingRate() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.LastProcessedAt.IsZero() || m.AvgProcessingTime == 0 {
		return 0.0
	}

	return float64(time.Second) / float64(m.AvgProcessingTime)
}

// RulesEngineMetricsService provides a service for collecting and exposing rules engine metrics.
type RulesEngineMetricsService struct {
	metrics *RulesEngineMetrics
}

// NewRulesEngineMetricsService creates a new metrics service.
func NewRulesEngineMetricsService() *RulesEngineMetricsService {
	return &RulesEngineMetricsService{
		metrics: NewRulesEngineMetrics(),
	}
}

// SetSink wires a metrics sink into the underlying metrics tracker.
func (s *RulesEngineMetricsService) SetSink(sink metricsSink) {
	s.metrics.SetSink(sink)
}

// GetMetrics returns the current metrics.
func (s *RulesEngineMetricsService) GetMetrics() *RulesEngineMetrics {
	return s.metrics
}

// GetHealthStatus returns a simple health status based on metrics.
func (s *RulesEngineMetricsService) GetHealthStatus() map[string]interface{} {
	snapshot := s.metrics.GetSnapshot()

	status := map[string]interface{}{
		"status": "healthy",
		"metrics": map[string]interface{}{
			"events_processed":    snapshot.EventsProcessed,
			"alerts_generated":    snapshot.AlertsGenerated,
			"cache_hit_ratio":     s.metrics.GetCacheHitRatio(),
			"processing_rate":     s.metrics.GetProcessingRate(),
			"last_processed_at":   snapshot.LastProcessedAt,
			"avg_processing_time": snapshot.AvgProcessingTime,
		},
	}

	// Determine health status based on metrics
	if snapshot.ProcessingErrors > 0 && snapshot.EventsProcessed > 0 {
		errorRate := float64(snapshot.ProcessingErrors) / float64(snapshot.EventsProcessed) * 100.0
		if errorRate > 10.0 { // More than 10% error rate
			status["status"] = "degraded"
			status["error_rate"] = errorRate
		}
	}

	// Check if processing is stalled
	if !snapshot.LastProcessedAt.IsZero() && time.Since(snapshot.LastProcessedAt) > 5*time.Minute {
		status["status"] = "stalled"
		status["last_activity"] = time.Since(snapshot.LastProcessedAt).String()
	}

	return status
}

// StartMetricsCollection starts a background goroutine to collect periodic metrics.
// It returns immediately; the collection loop stops when ctx is done.
func (s *RulesEngineMetricsService) StartMetricsCollection(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Periodic metrics collection could be implemented here
				// For now, metrics are collected on-demand
			}
		}
	}()
}
