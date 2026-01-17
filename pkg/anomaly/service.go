package anomaly

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Service provides anomaly detection capabilities
type Service struct {
	config     *AnomalyDetectionConfig
	detectors  map[string]AnomalyDetector
	repository AnomalyRepository
	notifier   *NotificationService
	tracer     trace.Tracer
	mutex      sync.RWMutex

	// Background processing
	processingQueue chan *DetectionRequest
	stopChan        chan struct{}
	workerPool      []*DetectionWorker
}

// DetectionRequest represents a request for anomaly detection
type DetectionRequest struct {
	ID          uuid.UUID             `json:"id"`
	Input       *DetectionInput       `json:"input"`
	Context     context.Context       `json:"-"`
	Timestamp   time.Time             `json:"timestamp"`
	Priority    int                   `json:"priority"`
	CallbackURL string                `json:"callback_url,omitempty"`
	ResultChan  chan *DetectionResult `json:"-"`
}

// DetectionResult represents the result of anomaly detection
type DetectionResult struct {
	RequestID       uuid.UUID                  `json:"request_id"`
	Anomalies       []*Anomaly                 `json:"anomalies"`
	ProcessedAt     time.Time                  `json:"processed_at"`
	ProcessingTime  time.Duration              `json:"processing_time"`
	DetectorResults map[string]*DetectorResult `json:"detector_results"`
	Error           string                     `json:"error,omitempty"`
	Metadata        map[string]interface{}     `json:"metadata,omitempty"`
}

// DetectorResult represents results from a specific detector
type DetectorResult struct {
	DetectorName    string        `json:"detector_name"`
	DetectorVersion string        `json:"detector_version"`
	Anomalies       []*Anomaly    `json:"anomalies"`
	ProcessingTime  time.Duration `json:"processing_time"`
	Error           string        `json:"error,omitempty"`
	Enabled         bool          `json:"enabled"`
}

// DetectionWorker processes detection requests
type DetectionWorker struct {
	id       int
	service  *Service
	stopChan chan struct{}
}

// NotificationService handles anomaly notifications
type NotificationService struct {
	config   *NotificationConfig
	channels map[string]NotificationChannel
	mutex    sync.RWMutex
}

// NotificationChannel defines interface for notification channels
type NotificationChannel interface {
	Send(ctx context.Context, anomaly *Anomaly, config *NotificationConfig) error
	GetName() string
	IsEnabled() bool
}

// NewService creates a new anomaly detection service
func NewService(config *AnomalyDetectionConfig, repository AnomalyRepository) *Service {
	if config == nil {
		config = &AnomalyDetectionConfig{
			Enabled:                true,
			DefaultSeverity:        SeverityMedium,
			MinConfidence:          0.5,
			MaxAnomaliesToKeep:     10000,
			BaselineUpdateInterval: 24 * time.Hour,
			BatchSize:              10,
			ProcessingTimeout:      30 * time.Second,
			MaxConcurrentDetectors: 5,
			RetentionPeriod:        30 * 24 * time.Hour,
		}
	}

	service := &Service{
		config:          config,
		detectors:       make(map[string]AnomalyDetector),
		repository:      repository,
		tracer:          otel.Tracer("anomaly-service"),
		processingQueue: make(chan *DetectionRequest, config.BatchSize*10),
		stopChan:        make(chan struct{}),
	}

	// Initialize notification service
	if config.NotificationConfig != nil {
		service.notifier = NewNotificationService(config.NotificationConfig)
	}

	// Start worker pool
	service.startWorkerPool()

	return service
}

// RegisterDetector registers an anomaly detector
func (s *Service) RegisterDetector(detector AnomalyDetector) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	name := detector.GetName()
	if _, exists := s.detectors[name]; exists {
		return fmt.Errorf("detector %s already registered", name)
	}

	s.detectors[name] = detector
	log.Printf("Registered anomaly detector: %s (version: %s)", name, detector.GetVersion())

	return nil
}

// UnregisterDetector removes an anomaly detector
func (s *Service) UnregisterDetector(name string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if _, exists := s.detectors[name]; !exists {
		return fmt.Errorf("detector %s not found", name)
	}

	delete(s.detectors, name)
	log.Printf("Unregistered anomaly detector: %s", name)

	return nil
}

// DetectAnomalies performs anomaly detection on the provided input
func (s *Service) DetectAnomalies(ctx context.Context, input *DetectionInput) (*DetectionResult, error) {
	ctx, span := s.tracer.Start(ctx, "anomaly_service.detect_anomalies")
	defer span.End()

	if !s.config.Enabled {
		return &DetectionResult{
			RequestID:   uuid.New(),
			Anomalies:   []*Anomaly{},
			ProcessedAt: time.Now(),
		}, nil
	}

	span.SetAttributes(
		attribute.String("tenant_id", input.TenantID.String()),
		attribute.String("content_type", input.ContentType),
		attribute.Int("content_length", len(input.Content)),
	)

	startTime := time.Now()
	result := &DetectionResult{
		RequestID:       uuid.New(),
		Anomalies:       []*Anomaly{},
		DetectorResults: make(map[string]*DetectorResult),
		ProcessedAt:     time.Now(),
		Metadata:        make(map[string]interface{}),
	}

	// Run detectors
	detectorResults := s.runDetectors(ctx, input)
	result.DetectorResults = detectorResults

	// Collect all anomalies
	var allAnomalies []*Anomaly
	for _, detectorResult := range detectorResults {
		allAnomalies = append(allAnomalies, detectorResult.Anomalies...)
	}

	// Filter and score anomalies
	filteredAnomalies := s.filterAnomalies(allAnomalies)
	result.Anomalies = filteredAnomalies

	// Store anomalies
	for _, anomaly := range filteredAnomalies {
		if err := s.repository.Create(ctx, anomaly); err != nil {
			log.Printf("Failed to store anomaly %s: %v", anomaly.ID, err)
		}
	}

	// Send notifications
	if s.notifier != nil {
		go s.sendNotifications(ctx, filteredAnomalies)
	}

	result.ProcessingTime = time.Since(startTime)

	span.SetAttributes(
		attribute.Int("anomalies_detected", len(filteredAnomalies)),
		attribute.Int64("processing_time_ms", result.ProcessingTime.Milliseconds()),
	)

	return result, nil
}

// DetectAnomaliesAsync performs asynchronous anomaly detection
func (s *Service) DetectAnomaliesAsync(ctx context.Context, input *DetectionInput, priority int) (*DetectionRequest, error) {
	request := &DetectionRequest{
		ID:         uuid.New(),
		Input:      input,
		Context:    ctx,
		Timestamp:  time.Now(),
		Priority:   priority,
		ResultChan: make(chan *DetectionResult, 1),
	}

	select {
	case s.processingQueue <- request:
		return request, nil
	default:
		return nil, fmt.Errorf("processing queue is full")
	}
}

// GetDetectionResult gets the result of an async detection request
func (s *Service) GetDetectionResult(request *DetectionRequest, timeout time.Duration) (*DetectionResult, error) {
	select {
	case result := <-request.ResultChan:
		return result, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("detection request timed out")
	}
}

// GetAnomalies retrieves anomalies with filtering
func (s *Service) GetAnomalies(ctx context.Context, filter *AnomalyFilter) ([]*Anomaly, error) {
	return s.repository.List(ctx, filter)
}

// GetAnomalyByID retrieves a specific anomaly
func (s *Service) GetAnomalyByID(ctx context.Context, id uuid.UUID) (*Anomaly, error) {
	return s.repository.GetByID(ctx, id)
}

// UpdateAnomalyStatus updates the status of an anomaly
func (s *Service) UpdateAnomalyStatus(ctx context.Context, id uuid.UUID, status AnomalyStatus, notes string) error {
	return s.repository.UpdateStatus(ctx, id, status, notes)
}

// GetAnomalyStatistics returns anomaly statistics
func (s *Service) GetAnomalyStatistics(ctx context.Context, tenantID uuid.UUID, timeWindow *TimeWindow) (*AnomalyStatistics, error) {
	return s.repository.GetStatistics(ctx, tenantID, timeWindow)
}

// UpdateBaseline updates the baseline for a specific detector
func (s *Service) UpdateBaseline(ctx context.Context, detectorName string, baseline *BaselineData) error {
	s.mutex.RLock()
	detector, exists := s.detectors[detectorName]
	s.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("detector %s not found", detectorName)
	}

	return detector.UpdateBaseline(ctx, baseline)
}

// GetDetectorInfo returns information about registered detectors
func (s *Service) GetDetectorInfo(ctx context.Context) map[string]interface{} {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	info := make(map[string]interface{})
	for name, detector := range s.detectors {
		info[name] = map[string]interface{}{
			"name":            detector.GetName(),
			"version":         detector.GetVersion(),
			"enabled":         detector.IsEnabled(),
			"supported_types": detector.GetSupportedTypes(),
		}
	}

	return info
}

// ConfigureDetector updates the configuration of a specific detector
func (s *Service) ConfigureDetector(ctx context.Context, detectorName string, config map[string]interface{}) error {
	s.mutex.RLock()
	detector, exists := s.detectors[detectorName]
	s.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("detector %s not found", detectorName)
	}

	return detector.Configure(config)
}

// Internal methods

func (s *Service) runDetectors(ctx context.Context, input *DetectionInput) map[string]*DetectorResult {
	s.mutex.RLock()
	detectors := make(map[string]AnomalyDetector)
	for name, detector := range s.detectors {
		if detector.IsEnabled() {
			detectors[name] = detector
		}
	}
	s.mutex.RUnlock()

	results := make(map[string]*DetectorResult)
	resultsChan := make(chan *DetectorResult, len(detectors))

	// Run detectors concurrently
	semaphore := make(chan struct{}, s.config.MaxConcurrentDetectors)

	for name, detector := range detectors {
		go func(detectorName string, det AnomalyDetector) {
			semaphore <- struct{}{}        // Acquire semaphore
			defer func() { <-semaphore }() // Release semaphore

			result := s.runSingleDetector(ctx, detectorName, det, input)
			resultsChan <- result
		}(name, detector)
	}

	// Collect results
	for i := 0; i < len(detectors); i++ {
		result := <-resultsChan
		results[result.DetectorName] = result
	}

	return results
}

func (s *Service) runSingleDetector(ctx context.Context, name string, detector AnomalyDetector, input *DetectionInput) *DetectorResult {
	startTime := time.Now()

	ctx, span := s.tracer.Start(ctx, fmt.Sprintf("detector.%s", name))
	defer span.End()

	result := &DetectorResult{
		DetectorName:    name,
		DetectorVersion: detector.GetVersion(),
		Enabled:         detector.IsEnabled(),
		ProcessingTime:  0,
		Anomalies:       []*Anomaly{},
	}

	// Add timeout context
	detectorCtx, cancel := context.WithTimeout(ctx, s.config.ProcessingTimeout)
	defer cancel()

	anomalies, err := detector.DetectAnomalies(detectorCtx, input)
	if err != nil {
		result.Error = err.Error()
		span.RecordError(err)
		log.Printf("Detector %s failed: %v", name, err)
	} else {
		result.Anomalies = anomalies
	}

	result.ProcessingTime = time.Since(startTime)

	span.SetAttributes(
		attribute.String("detector_name", name),
		attribute.String("detector_version", detector.GetVersion()),
		attribute.Int("anomalies_found", len(result.Anomalies)),
		attribute.Int64("processing_time_ms", result.ProcessingTime.Milliseconds()),
		attribute.Bool("has_error", err != nil),
	)

	return result
}

func (s *Service) filterAnomalies(anomalies []*Anomaly) []*Anomaly {
	var filtered []*Anomaly

	for _, anomaly := range anomalies {
		// Filter by minimum confidence
		if anomaly.Confidence < s.config.MinConfidence {
			continue
		}

		// Apply severity thresholds if configured
		if s.config.SeverityThresholds != nil {
			if threshold, exists := s.config.SeverityThresholds[anomaly.Severity]; exists {
				if anomaly.Score < threshold {
					continue
				}
			}
		}

		// Apply type-specific thresholds if configured
		if s.config.Thresholds != nil {
			if threshold, exists := s.config.Thresholds[anomaly.Type]; exists {
				if anomaly.Score < threshold {
					continue
				}
			}
		}

		filtered = append(filtered, anomaly)
	}

	return filtered
}

func (s *Service) sendNotifications(ctx context.Context, anomalies []*Anomaly) {
	if s.notifier == nil {
		return
	}

	for _, anomaly := range anomalies {
		// Apply notification rules
		if s.shouldNotify(anomaly) {
			if err := s.notifier.SendNotification(ctx, anomaly); err != nil {
				log.Printf("Failed to send notification for anomaly %s: %v", anomaly.ID, err)
			}
		}
	}
}

func (s *Service) shouldNotify(anomaly *Anomaly) bool {
	config := s.config.NotificationConfig
	if config == nil {
		return false
	}

	// Check if notifications are suppressed for this anomaly
	if anomaly.SuppressNotifications {
		return false
	}

	// Check severity-based notification rules
	if recipients, exists := config.Recipients[anomaly.Severity]; exists && len(recipients) > 0 {
		return true
	}

	// Check if there are any general recipients
	if recipients, exists := config.Recipients[SeverityLow]; exists && len(recipients) > 0 {
		return true
	}

	return false
}

func (s *Service) startWorkerPool() {
	workerCount := s.config.MaxConcurrentDetectors
	if workerCount <= 0 {
		workerCount = 3
	}

	s.workerPool = make([]*DetectionWorker, workerCount)

	for i := 0; i < workerCount; i++ {
		worker := &DetectionWorker{
			id:       i,
			service:  s,
			stopChan: make(chan struct{}),
		}
		s.workerPool[i] = worker
		go worker.start()
	}

	log.Printf("Started anomaly detection worker pool with %d workers", workerCount)
}

func (s *Service) Shutdown(ctx context.Context) error {
	log.Println("Shutting down anomaly detection service...")

	// Stop all workers
	for _, worker := range s.workerPool {
		close(worker.stopChan)
	}

	// Close processing queue
	close(s.stopChan)

	// Wait for workers to finish (with timeout)
	time.Sleep(5 * time.Second)

	log.Println("Anomaly detection service shut down complete")
	return nil
}

// DetectionWorker implementation

func (w *DetectionWorker) start() {
	log.Printf("Starting anomaly detection worker %d", w.id)

	for {
		select {
		case <-w.stopChan:
			log.Printf("Stopping anomaly detection worker %d", w.id)
			return
		case request := <-w.service.processingQueue:
			w.processRequest(request)
		}
	}
}

func (w *DetectionWorker) processRequest(request *DetectionRequest) {
	startTime := time.Now()

	result, err := w.service.DetectAnomalies(request.Context, request.Input)
	if err != nil {
		result = &DetectionResult{
			RequestID:   request.ID,
			Error:       err.Error(),
			ProcessedAt: time.Now(),
		}
	}

	result.RequestID = request.ID
	result.ProcessingTime = time.Since(startTime)

	// Send result back
	select {
	case request.ResultChan <- result:
		// Success
	default:
		// Channel might be closed or full
		log.Printf("Failed to send result for request %s", request.ID)
	}
}

// NewNotificationService creates a new notification service
func NewNotificationService(config *NotificationConfig) *NotificationService {
	service := &NotificationService{
		config:   config,
		channels: make(map[string]NotificationChannel),
	}

	// Initialize notification channels based on config
	for _, channelName := range config.Channels {
		switch channelName {
		case "email":
			// Initialize email channel (would integrate with email service)
		case "slack":
			// Initialize Slack channel (would integrate with Slack API)
		case "webhook":
			// Initialize webhook channel
		}
	}

	return service
}

func (ns *NotificationService) SendNotification(ctx context.Context, anomaly *Anomaly) error {
	if ns.config == nil {
		return fmt.Errorf("notification config not available")
	}

	// Check rate limiting
	if ns.config.RateLimiting != nil {
		// Implement rate limiting logic here
		// This would check against a rate limiter (Redis, in-memory, etc.)
	}

	// Send to configured channels
	for _, channelName := range ns.config.Channels {
		if channel, exists := ns.channels[channelName]; exists && channel.IsEnabled() {
			go func(ch NotificationChannel) {
				if err := ch.Send(ctx, anomaly, ns.config); err != nil {
					log.Printf("Failed to send notification via %s: %v", ch.GetName(), err)
				}
			}(channel)
		}
	}

	// Record notification
	anomaly.NotificationsSent = append(anomaly.NotificationsSent, NotificationRecord{
		Channel:    strings.Join(ns.config.Channels, ","),
		Recipients: ns.getRecipients(anomaly.Severity),
		SentAt:     time.Now(),
		Status:     "sent",
	})

	return nil
}

func (ns *NotificationService) getRecipients(severity AnomalySeverity) []string {
	if ns.config == nil || ns.config.Recipients == nil {
		return []string{}
	}

	if recipients, exists := ns.config.Recipients[severity]; exists {
		return recipients
	}

	// Fallback to default recipients
	if recipients, exists := ns.config.Recipients[SeverityLow]; exists {
		return recipients
	}

	return []string{}
}
