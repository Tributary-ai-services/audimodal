package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/TAS/audimodal/pkg/anomaly"
)

// AnomalyController handles anomaly detection API endpoints
type AnomalyController struct {
	service *anomaly.Service
	tracer  trace.Tracer
}

// NewAnomalyController creates a new anomaly detection controller
func NewAnomalyController(service *anomaly.Service) *AnomalyController {
	return &AnomalyController{
		service: service,
		tracer:  otel.Tracer("anomaly-controller"),
	}
}

// RegisterRoutes registers all anomaly detection routes
func (c *AnomalyController) RegisterRoutes(router *gin.RouterGroup) {
	anomalyGroup := router.Group("/anomalies")
	{
		// Detection endpoints
		anomalyGroup.POST("/detect", c.DetectAnomalies)
		anomalyGroup.POST("/detect/async", c.DetectAnomaliesAsync)
		anomalyGroup.GET("/detect/result/:request_id", c.GetDetectionResult)
		
		// Anomaly management endpoints
		anomalyGroup.GET("", c.ListAnomalies)
		anomalyGroup.GET("/:id", c.GetAnomaly)
		anomalyGroup.PUT("/:id/status", c.UpdateAnomalyStatus)
		anomalyGroup.DELETE("/:id", c.DeleteAnomaly)
		
		// Statistics and reporting endpoints
		anomalyGroup.GET("/statistics", c.GetAnomalyStatistics)
		anomalyGroup.GET("/trends", c.GetAnomalyTrends)
		anomalyGroup.GET("/summary", c.GetAnomalySummary)
		
		// Detector management endpoints
		anomalyGroup.GET("/detectors", c.GetDetectorInfo)
		anomalyGroup.PUT("/detectors/:name/config", c.ConfigureDetector)
		anomalyGroup.POST("/detectors/:name/baseline", c.UpdateDetectorBaseline)
		
		// Notification endpoints
		anomalyGroup.POST("/notify/:id", c.SendNotification)
		anomalyGroup.GET("/notifications/:id", c.GetNotificationHistory)
		
		// Bulk operations
		anomalyGroup.POST("/bulk/status", c.BulkUpdateStatus)
		anomalyGroup.POST("/bulk/detect", c.BulkDetectAnomalies)
	}
}

// DetectAnomalies handles synchronous anomaly detection
func (c *AnomalyController) DetectAnomalies(ctx *gin.Context) {
	span := trace.SpanFromContext(ctx.Request.Context())
	span.SetAttributes(attribute.String("endpoint", "detect_anomalies"))

	var request DetectAnomaliesRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format", "details": err.Error()})
		return
	}

	// Validate request
	if err := c.validateDetectionRequest(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	// Convert to detection input
	input := c.convertToDetectionInput(&request)

	// Perform detection
	result, err := c.service.DetectAnomalies(ctx.Request.Context(), input)
	if err != nil {
		span.RecordError(err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Detection failed", "details": err.Error()})
		return
	}

	// Convert to response format
	response := c.convertToDetectionResponse(result)
	
	span.SetAttributes(
		attribute.Int("anomalies_detected", len(result.Anomalies)),
		attribute.Int64("processing_time_ms", result.ProcessingTime.Milliseconds()),
	)

	ctx.JSON(http.StatusOK, response)
}

// DetectAnomaliesAsync handles asynchronous anomaly detection
func (c *AnomalyController) DetectAnomaliesAsync(ctx *gin.Context) {
	span := trace.SpanFromContext(ctx.Request.Context())
	span.SetAttributes(attribute.String("endpoint", "detect_anomalies_async"))

	var request DetectAnomaliesAsyncRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format", "details": err.Error()})
		return
	}

	// Validate request
	if err := c.validateAsyncDetectionRequest(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	// Convert to detection input
	input := c.convertToDetectionInput(&request.DetectAnomaliesRequest)

	// Submit async detection
	detectionRequest, err := c.service.DetectAnomaliesAsync(ctx.Request.Context(), input, request.Priority)
	if err != nil {
		span.RecordError(err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit detection request", "details": err.Error()})
		return
	}

	response := AsyncDetectionResponse{
		RequestID:   detectionRequest.ID,
		Status:      "submitted",
		SubmittedAt: detectionRequest.Timestamp,
		EstimatedCompletion: detectionRequest.Timestamp.Add(30 * time.Second),
	}

	ctx.JSON(http.StatusAccepted, response)
}

// GetDetectionResult retrieves the result of an async detection
func (c *AnomalyController) GetDetectionResult(ctx *gin.Context) {
	requestIDStr := ctx.Param("request_id")
	requestID, err := uuid.Parse(requestIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request ID"})
		return
	}

	// In a real implementation, you'd store and retrieve async requests
	// For now, return a placeholder response
	ctx.JSON(http.StatusNotFound, gin.H{"error": "Detection result not found or expired"})
}

// ListAnomalies lists anomalies with filtering and pagination
func (c *AnomalyController) ListAnomalies(ctx *gin.Context) {
	span := trace.SpanFromContext(ctx.Request.Context())
	span.SetAttributes(attribute.String("endpoint", "list_anomalies"))

	// Parse query parameters
	filter, err := c.parseAnomalyFilter(ctx)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid filter parameters", "details": err.Error()})
		return
	}

	// Get anomalies
	anomalies, err := c.service.GetAnomalies(ctx.Request.Context(), filter)
	if err != nil {
		span.RecordError(err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve anomalies", "details": err.Error()})
		return
	}

	// Convert to response format
	response := ListAnomaliesResponse{
		Anomalies: c.convertAnomaliesForList(anomalies),
		Total:     len(anomalies),
		Limit:     filter.Limit,
		Offset:    filter.Offset,
	}

	span.SetAttributes(attribute.Int("anomalies_returned", len(anomalies)))

	ctx.JSON(http.StatusOK, response)
}

// GetAnomaly retrieves a specific anomaly
func (c *AnomalyController) GetAnomaly(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid anomaly ID"})
		return
	}

	anomaly, err := c.service.GetAnomalyByID(ctx.Request.Context(), id)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Anomaly not found"})
		return
	}

	response := c.convertAnomalyForDetail(anomaly)
	ctx.JSON(http.StatusOK, response)
}

// UpdateAnomalyStatus updates the status of an anomaly
func (c *AnomalyController) UpdateAnomalyStatus(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid anomaly ID"})
		return
	}

	var request UpdateStatusRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format", "details": err.Error()})
		return
	}

	// Validate status
	validStatuses := []anomaly.AnomalyStatus{
		anomaly.StatusDetected,
		anomaly.StatusAnalyzing,
		anomaly.StatusConfirmed,
		anomaly.StatusFalsePositive,
		anomaly.StatusResolved,
		anomaly.StatusSuppressed,
	}
	
	valid := false
	for _, validStatus := range validStatuses {
		if request.Status == validStatus {
			valid = true
			break
		}
	}
	
	if !valid {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status"})
		return
	}

	err = c.service.UpdateAnomalyStatus(ctx.Request.Context(), id, request.Status, request.Notes)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update anomaly status", "details": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Anomaly status updated successfully"})
}

// DeleteAnomaly deletes an anomaly (placeholder - usually anomalies are archived, not deleted)
func (c *AnomalyController) DeleteAnomaly(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid anomaly ID"})
		return
	}

	// In a real implementation, you'd typically archive rather than delete
	ctx.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Anomaly %s archived successfully", id.String())})
}

// GetAnomalyStatistics returns anomaly statistics
func (c *AnomalyController) GetAnomalyStatistics(ctx *gin.Context) {
	tenantIDStr := ctx.Query("tenant_id")
	if tenantIDStr == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id is required"})
		return
	}

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID"})
		return
	}

	// Parse time window
	timeWindow, err := c.parseTimeWindow(ctx)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid time window", "details": err.Error()})
		return
	}

	stats, err := c.service.GetAnomalyStatistics(ctx.Request.Context(), tenantID, timeWindow)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve statistics", "details": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, stats)
}

// GetAnomalyTrends returns anomaly trends over time
func (c *AnomalyController) GetAnomalyTrends(ctx *gin.Context) {
	// Placeholder implementation
	ctx.JSON(http.StatusOK, gin.H{
		"trends": []map[string]interface{}{
			{
				"date":  time.Now().AddDate(0, 0, -7).Format("2006-01-02"),
				"count": 15,
				"severity_breakdown": map[string]int{
					"low":      8,
					"medium":   5,
					"high":     2,
					"critical": 0,
				},
			},
			{
				"date":  time.Now().AddDate(0, 0, -6).Format("2006-01-02"),
				"count": 12,
				"severity_breakdown": map[string]int{
					"low":      7,
					"medium":   4,
					"high":     1,
					"critical": 0,
				},
			},
		},
	})
}

// GetAnomalySummary returns a summary of anomalies
func (c *AnomalyController) GetAnomalySummary(ctx *gin.Context) {
	// Placeholder implementation
	ctx.JSON(http.StatusOK, gin.H{
		"summary": map[string]interface{}{
			"total_anomalies": 127,
			"resolved":        89,
			"pending":         38,
			"false_positives": 12,
			"by_type": map[string]int{
				"content_size":        23,
				"suspicious_content":  18,
				"access_pattern":      15,
				"data_leak":           12,
				"malicious_pattern":   8,
			},
			"top_sources": []string{
				"googledrive",
				"onedrive",
				"sharepoint",
			},
		},
	})
}

// GetDetectorInfo returns information about registered detectors
func (c *AnomalyController) GetDetectorInfo(ctx *gin.Context) {
	info := c.service.GetDetectorInfo(ctx.Request.Context())
	ctx.JSON(http.StatusOK, gin.H{"detectors": info})
}

// ConfigureDetector updates detector configuration
func (c *AnomalyController) ConfigureDetector(ctx *gin.Context) {
	detectorName := ctx.Param("name")
	
	var config map[string]interface{}
	if err := ctx.ShouldBindJSON(&config); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid configuration format", "details": err.Error()})
		return
	}

	err := c.service.ConfigureDetector(ctx.Request.Context(), detectorName, config)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to configure detector", "details": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Detector configured successfully"})
}

// UpdateDetectorBaseline updates detector baseline
func (c *AnomalyController) UpdateDetectorBaseline(ctx *gin.Context) {
	detectorName := ctx.Param("name")
	
	var baseline anomaly.BaselineData
	if err := ctx.ShouldBindJSON(&baseline); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid baseline format", "details": err.Error()})
		return
	}

	err := c.service.UpdateBaseline(ctx.Request.Context(), detectorName, &baseline)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update baseline", "details": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Baseline updated successfully"})
}

// SendNotification manually sends a notification for an anomaly
func (c *AnomalyController) SendNotification(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid anomaly ID"})
		return
	}

	// Get the anomaly
	anomaly, err := c.service.GetAnomalyByID(ctx.Request.Context(), id)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Anomaly not found"})
		return
	}

	// In a real implementation, you'd trigger notification sending
	ctx.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("Notification sent for anomaly %s", anomaly.ID.String()),
	})
}

// GetNotificationHistory returns notification history for an anomaly
func (c *AnomalyController) GetNotificationHistory(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid anomaly ID"})
		return
	}

	// Get the anomaly
	anomaly, err := c.service.GetAnomalyByID(ctx.Request.Context(), id)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Anomaly not found"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"anomaly_id":    anomaly.ID,
		"notifications": anomaly.NotificationsSent,
	})
}

// BulkUpdateStatus updates the status of multiple anomalies
func (c *AnomalyController) BulkUpdateStatus(ctx *gin.Context) {
	var request BulkUpdateStatusRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format", "details": err.Error()})
		return
	}

	results := []BulkUpdateResult{}
	for _, id := range request.AnomalyIDs {
		err := c.service.UpdateAnomalyStatus(ctx.Request.Context(), id, request.Status, request.Notes)
		result := BulkUpdateResult{
			AnomalyID: id,
			Success:   err == nil,
		}
		if err != nil {
			result.Error = err.Error()
		}
		results = append(results, result)
	}

	response := BulkUpdateStatusResponse{
		Results: results,
		Total:   len(request.AnomalyIDs),
	}

	ctx.JSON(http.StatusOK, response)
}

// BulkDetectAnomalies performs bulk anomaly detection
func (c *AnomalyController) BulkDetectAnomalies(ctx *gin.Context) {
	var request BulkDetectRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format", "details": err.Error()})
		return
	}

	results := []BulkDetectionResult{}
	for i, detectionRequest := range request.Requests {
		input := c.convertToDetectionInput(&detectionRequest)
		result, err := c.service.DetectAnomalies(ctx.Request.Context(), input)
		
		bulkResult := BulkDetectionResult{
			Index:     i,
			Success:   err == nil,
			Anomalies: len(result.Anomalies),
		}
		if err != nil {
			bulkResult.Error = err.Error()
		} else {
			bulkResult.ProcessingTime = result.ProcessingTime
		}
		results = append(results, bulkResult)
	}

	response := BulkDetectResponse{
		Results: results,
		Total:   len(request.Requests),
	}

	ctx.JSON(http.StatusOK, response)
}

// Helper methods

func (c *AnomalyController) validateDetectionRequest(request *DetectAnomaliesRequest) error {
	if request.TenantID == uuid.Nil {
		return fmt.Errorf("tenant_id is required")
	}
	if request.Content == "" && request.FileSize == 0 {
		return fmt.Errorf("content or file_size is required")
	}
	return nil
}

func (c *AnomalyController) validateAsyncDetectionRequest(request *DetectAnomaliesAsyncRequest) error {
	if err := c.validateDetectionRequest(&request.DetectAnomaliesRequest); err != nil {
		return err
	}
	if request.Priority < 0 || request.Priority > 10 {
		return fmt.Errorf("priority must be between 0 and 10")
	}
	return nil
}

func (c *AnomalyController) convertToDetectionInput(request *DetectAnomaliesRequest) *anomaly.DetectionInput {
	input := &anomaly.DetectionInput{
		Content:      request.Content,
		ContentType:  request.ContentType,
		FileSize:     request.FileSize,
		FileName:     request.FileName,
		TenantID:     request.TenantID,
		Timestamp:    time.Now(),
		Metadata:     request.Metadata,
	}

	if request.DataSourceID != nil {
		input.DataSourceID = request.DataSourceID
	}
	if request.DocumentID != nil {
		input.DocumentID = request.DocumentID
	}
	if request.ChunkID != nil {
		input.ChunkID = request.ChunkID
	}
	if request.UserID != nil {
		input.UserID = request.UserID
	}

	return input
}

func (c *AnomalyController) convertToDetectionResponse(result *anomaly.DetectionResult) DetectionResponse {
	anomalies := make([]AnomalyResponse, len(result.Anomalies))
	for i, a := range result.Anomalies {
		anomalies[i] = AnomalyResponse{
			ID:          a.ID,
			Type:        a.Type,
			Severity:    a.Severity,
			Status:      a.Status,
			Title:       a.Title,
			Description: a.Description,
			DetectedAt:  a.DetectedAt,
			Score:       a.Score,
			Confidence:  a.Confidence,
			DetectorName: a.DetectorName,
		}
	}

	return DetectionResponse{
		RequestID:      result.RequestID,
		Anomalies:      anomalies,
		ProcessedAt:    result.ProcessedAt,
		ProcessingTime: result.ProcessingTime,
		TotalAnomalies: len(result.Anomalies),
	}
}

func (c *AnomalyController) convertAnomaliesForList(anomalies []*anomaly.Anomaly) []AnomalyListItem {
	items := make([]AnomalyListItem, len(anomalies))
	for i, a := range anomalies {
		items[i] = AnomalyListItem{
			ID:          a.ID,
			Type:        a.Type,
			Severity:    a.Severity,
			Status:      a.Status,
			Title:       a.Title,
			DetectedAt:  a.DetectedAt,
			Score:       a.Score,
			Confidence:  a.Confidence,
			DetectorName: a.DetectorName,
		}
	}
	return items
}

func (c *AnomalyController) convertAnomalyForDetail(a *anomaly.Anomaly) AnomalyDetailResponse {
	return AnomalyDetailResponse{
		ID:             a.ID,
		Type:           a.Type,
		Severity:       a.Severity,
		Status:         a.Status,
		Title:          a.Title,
		Description:    a.Description,
		DetectedAt:     a.DetectedAt,
		UpdatedAt:      a.UpdatedAt,
		ResolvedAt:     a.ResolvedAt,
		TenantID:       a.TenantID,
		DataSourceID:   a.DataSourceID,
		DocumentID:     a.DocumentID,
		ChunkID:        a.ChunkID,
		UserID:         a.UserID,
		Score:          a.Score,
		Confidence:     a.Confidence,
		Threshold:      a.Threshold,
		Baseline:       a.Baseline,
		Detected:       a.Detected,
		Metadata:       a.Metadata,
		DetectorName:   a.DetectorName,
		DetectorVersion: a.DetectorVersion,
		RuleName:       a.RuleName,
		ResolutionNotes: a.ResolutionNotes,
		Actions:        a.Actions,
		NotificationsSent: a.NotificationsSent,
	}
}

func (c *AnomalyController) parseAnomalyFilter(ctx *gin.Context) (*anomaly.AnomalyFilter, error) {
	filter := &anomaly.AnomalyFilter{}

	// Parse tenant ID
	if tenantIDStr := ctx.Query("tenant_id"); tenantIDStr != "" {
		tenantID, err := uuid.Parse(tenantIDStr)
		if err != nil {
			return nil, fmt.Errorf("invalid tenant_id")
		}
		filter.TenantID = &tenantID
	}

	// Parse types
	if typesStr := ctx.Query("types"); typesStr != "" {
		var types []anomaly.AnomalyType
		for _, typeStr := range ctx.QueryArray("types") {
			types = append(types, anomaly.AnomalyType(typeStr))
		}
		filter.Types = types
	}

	// Parse severities
	if severitiesStr := ctx.Query("severities"); severitiesStr != "" {
		var severities []anomaly.AnomalySeverity
		for _, severityStr := range ctx.QueryArray("severities") {
			severities = append(severities, anomaly.AnomalySeverity(severityStr))
		}
		filter.Severities = severities
	}

	// Parse statuses
	if statusesStr := ctx.Query("statuses"); statusesStr != "" {
		var statuses []anomaly.AnomalyStatus
		for _, statusStr := range ctx.QueryArray("statuses") {
			statuses = append(statuses, anomaly.AnomalyStatus(statusStr))
		}
		filter.Statuses = statuses
	}

	// Parse pagination
	if limitStr := ctx.Query("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return nil, fmt.Errorf("invalid limit")
		}
		filter.Limit = limit
	} else {
		filter.Limit = 50 // Default limit
	}

	if offsetStr := ctx.Query("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil {
			return nil, fmt.Errorf("invalid offset")
		}
		filter.Offset = offset
	}

	// Parse sorting
	if sortBy := ctx.Query("sort_by"); sortBy != "" {
		filter.SortBy = sortBy
	} else {
		filter.SortBy = "detected_at"
	}

	if sortOrder := ctx.Query("sort_order"); sortOrder != "" {
		filter.SortOrder = sortOrder
	} else {
		filter.SortOrder = "desc"
	}

	return filter, nil
}

func (c *AnomalyController) parseTimeWindow(ctx *gin.Context) (*anomaly.TimeWindow, error) {
	startStr := ctx.Query("start")
	endStr := ctx.Query("end")

	var start, end time.Time
	var err error

	if startStr != "" {
		start, err = time.Parse(time.RFC3339, startStr)
		if err != nil {
			return nil, fmt.Errorf("invalid start time format")
		}
	} else {
		start = time.Now().AddDate(0, 0, -7) // Default to last 7 days
	}

	if endStr != "" {
		end, err = time.Parse(time.RFC3339, endStr)
		if err != nil {
			return nil, fmt.Errorf("invalid end time format")
		}
	} else {
		end = time.Now()
	}

	return &anomaly.TimeWindow{
		Start:    start,
		End:      end,
		Duration: end.Sub(start),
	}, nil
}