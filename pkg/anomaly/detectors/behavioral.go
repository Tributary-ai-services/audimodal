package detectors

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jscharber/eAIIngest/pkg/anomaly"
)

// BehavioralDetector implements behavioral anomaly detection
type BehavioralDetector struct {
	name         string
	version      string
	enabled      bool
	config       *BehavioralConfig
	baselines    map[string]*anomaly.BaselineData
	userProfiles map[uuid.UUID]*UserProfile
	sessionData  map[string]*SessionTracker
}

// BehavioralConfig contains configuration for behavioral detection
type BehavioralConfig struct {
	AccessPatternThreshold  float64       `json:"access_pattern_threshold"`
	VelocityThreshold       float64       `json:"velocity_threshold"`
	SessionTimeoutThreshold time.Duration `json:"session_timeout_threshold"`
	BurstDetectionWindow    time.Duration `json:"burst_detection_window"`
	BurstThreshold          int           `json:"burst_threshold"`
	LocationChangeThreshold int           `json:"location_change_threshold"`
	OffHoursThreshold       float64       `json:"off_hours_threshold"`
	UserProfileMinSamples   int           `json:"user_profile_min_samples"`
	AnomalousSequenceLength int           `json:"anomalous_sequence_length"`
	DeviceFingerprinting    bool          `json:"device_fingerprinting"`
	EnableUserProfiling     bool          `json:"enable_user_profiling"`
	EnableGeolocationCheck  bool          `json:"enable_geolocation_check"`
	EnableTimeBasedCheck    bool          `json:"enable_time_based_check"`
	SuspiciousFileTypes     []string      `json:"suspicious_file_types"`
	LargeFileThresholdMB    int64         `json:"large_file_threshold_mb"`
}

// UserProfile contains behavioral profile information for a user
type UserProfile struct {
	UserID             uuid.UUID          `json:"user_id"`
	TypicalAccessHours []int              `json:"typical_access_hours"`
	TypicalLocations   []string           `json:"typical_locations"`
	TypicalDevices     []string           `json:"typical_devices"`
	AvgSessionDuration time.Duration      `json:"avg_session_duration"`
	TypicalFileTypes   map[string]int     `json:"typical_file_types"`
	AvgFilesPerSession float64            `json:"avg_files_per_session"`
	AccessVelocity     *VelocityProfile   `json:"access_velocity"`
	LastSeen           time.Time          `json:"last_seen"`
	TotalSessions      int64              `json:"total_sessions"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
	BehaviorSignatures map[string]float64 `json:"behavior_signatures"`
}

// VelocityProfile tracks user access velocity patterns
type VelocityProfile struct {
	AvgRequestsPerMinute float64       `json:"avg_requests_per_minute"`
	MaxRequestsPerMinute float64       `json:"max_requests_per_minute"`
	TypicalBurstSize     int           `json:"typical_burst_size"`
	TypicalPauseTime     time.Duration `json:"typical_pause_time"`
}

// SessionTracker tracks current session behavior
type SessionTracker struct {
	SessionID         string             `json:"session_id"`
	UserID            uuid.UUID          `json:"user_id"`
	StartTime         time.Time          `json:"start_time"`
	LastActivity      time.Time          `json:"last_activity"`
	RequestCount      int                `json:"request_count"`
	FileAccessCount   int                `json:"file_access_count"`
	Locations         []string           `json:"locations"`
	DeviceFingerprint string             `json:"device_fingerprint"`
	AccessedFiles     []FileAccess       `json:"accessed_files"`
	SuspiciousActions []SuspiciousAction `json:"suspicious_actions"`
}

// FileAccess represents a file access event
type FileAccess struct {
	DocumentID uuid.UUID `json:"document_id"`
	AccessTime time.Time `json:"access_time"`
	AccessType string    `json:"access_type"` // view, download, share, etc.
	FileSize   int64     `json:"file_size"`
	FileType   string    `json:"file_type"`
	Location   string    `json:"location"`
	Success    bool      `json:"success"`
}

// SuspiciousAction represents a suspicious action during a session
type SuspiciousAction struct {
	Action    string                  `json:"action"`
	Timestamp time.Time               `json:"timestamp"`
	Severity  anomaly.AnomalySeverity `json:"severity"`
	Details   map[string]interface{}  `json:"details"`
}

// NewBehavioralDetector creates a new behavioral anomaly detector
func NewBehavioralDetector() *BehavioralDetector {
	return &BehavioralDetector{
		name:    "behavioral_detector",
		version: "1.0.0",
		enabled: true,
		config: &BehavioralConfig{
			AccessPatternThreshold:  0.3,
			VelocityThreshold:       2.0,
			SessionTimeoutThreshold: 30 * time.Minute,
			BurstDetectionWindow:    5 * time.Minute,
			BurstThreshold:          20,
			LocationChangeThreshold: 3,
			OffHoursThreshold:       0.1,
			UserProfileMinSamples:   10,
			AnomalousSequenceLength: 5,
			DeviceFingerprinting:    true,
			EnableUserProfiling:     true,
			EnableGeolocationCheck:  true,
			EnableTimeBasedCheck:    true,
			SuspiciousFileTypes:     []string{".exe", ".bat", ".sh", ".ps1", ".vbs"},
			LargeFileThresholdMB:    100,
		},
		baselines:    make(map[string]*anomaly.BaselineData),
		userProfiles: make(map[uuid.UUID]*UserProfile),
		sessionData:  make(map[string]*SessionTracker),
	}
}

func (d *BehavioralDetector) GetName() string {
	return d.name
}

func (d *BehavioralDetector) GetVersion() string {
	return d.version
}

func (d *BehavioralDetector) GetSupportedTypes() []anomaly.AnomalyType {
	return []anomaly.AnomalyType{
		anomaly.AnomalyTypeUploadPattern,
		anomaly.AnomalyTypeAccessPattern,
		anomaly.AnomalyTypeUsagePattern,
		anomaly.AnomalyTypeFrequency,
		anomaly.AnomalyTypeUnauthorizedAccess,
		anomaly.AnomalyTypePrivilegeEscalation,
	}
}

func (d *BehavioralDetector) IsEnabled() bool {
	return d.enabled
}

func (d *BehavioralDetector) Configure(config map[string]interface{}) error {
	if enabled, ok := config["enabled"].(bool); ok {
		d.enabled = enabled
	}
	if threshold, ok := config["access_pattern_threshold"].(float64); ok {
		d.config.AccessPatternThreshold = threshold
	}
	if threshold, ok := config["velocity_threshold"].(float64); ok {
		d.config.VelocityThreshold = threshold
	}
	return nil
}

func (d *BehavioralDetector) DetectAnomalies(ctx context.Context, input *anomaly.DetectionInput) ([]*anomaly.Anomaly, error) {
	var anomalies []*anomaly.Anomaly

	// Update session tracking
	sessionID := d.getSessionID(input)
	d.updateSessionTracker(sessionID, input)

	// Get user profile
	var userProfile *UserProfile
	if input.UserID != nil && d.config.EnableUserProfiling {
		userProfile = d.getUserProfile(*input.UserID)
	}

	// Detect access pattern anomalies
	if input.AccessPattern != nil {
		accessAnomalies := d.detectAccessPatternAnomalies(ctx, input, userProfile)
		anomalies = append(anomalies, accessAnomalies...)
	}

	// Detect velocity anomalies
	velocityAnomalies := d.detectVelocityAnomalies(ctx, input, sessionID, userProfile)
	anomalies = append(anomalies, velocityAnomalies...)

	// Detect time-based anomalies
	if d.config.EnableTimeBasedCheck {
		timeAnomalies := d.detectTimeBasedAnomalies(ctx, input, userProfile)
		anomalies = append(anomalies, timeAnomalies...)
	}

	// Detect location-based anomalies
	if d.config.EnableGeolocationCheck {
		locationAnomalies := d.detectLocationAnomalies(ctx, input, userProfile)
		anomalies = append(anomalies, locationAnomalies...)
	}

	// Detect session anomalies
	sessionAnomalies := d.detectSessionAnomalies(ctx, input, sessionID)
	anomalies = append(anomalies, sessionAnomalies...)

	// Detect file access anomalies
	fileAnomalies := d.detectFileAccessAnomalies(ctx, input, userProfile)
	anomalies = append(anomalies, fileAnomalies...)

	// Update user profile if applicable
	if input.UserID != nil && d.config.EnableUserProfiling {
		d.updateUserProfile(*input.UserID, input)
	}

	return anomalies, nil
}

func (d *BehavioralDetector) detectAccessPatternAnomalies(ctx context.Context, input *anomaly.DetectionInput, profile *UserProfile) []*anomaly.Anomaly {
	var anomalies []*anomaly.Anomaly

	if input.AccessPattern == nil {
		return anomalies
	}

	pattern := input.AccessPattern

	// Unusual access frequency
	if profile != nil && profile.AvgFilesPerSession > 0 {
		// Calculate current session file access rate
		sessionTracker := d.getSessionTracker(d.getSessionID(input))
		if sessionTracker != nil {
			currentRate := float64(sessionTracker.FileAccessCount)
			expectedRate := profile.AvgFilesPerSession

			deviation := math.Abs(currentRate-expectedRate) / expectedRate

			if deviation > d.config.AccessPatternThreshold {
				severity := anomaly.SeverityLow
				if deviation > 0.5 {
					severity = anomaly.SeverityMedium
				}
				if deviation > 1.0 {
					severity = anomaly.SeverityHigh
				}

				anomaly := &anomaly.Anomaly{
					ID:              uuid.New(),
					Type:            anomaly.AnomalyTypeAccessPattern,
					Severity:        severity,
					Status:          anomaly.StatusDetected,
					Title:           "Unusual File Access Rate",
					Description:     fmt.Sprintf("User accessing files at unusual rate: current=%.1f, expected=%.1f (deviation: %.1f%%)", currentRate, expectedRate, deviation*100),
					DetectedAt:      time.Now(),
					UpdatedAt:       time.Now(),
					TenantID:        input.TenantID,
					DataSourceID:    input.DataSourceID,
					DocumentID:      input.DocumentID,
					UserID:          input.UserID,
					Score:           math.Min(deviation, 1.0),
					Confidence:      0.75,
					Threshold:       d.config.AccessPatternThreshold,
					DetectorName:    d.name,
					DetectorVersion: d.version,
					Baseline: map[string]interface{}{
						"expected_access_rate": expectedRate,
					},
					Detected: map[string]interface{}{
						"current_access_rate": currentRate,
						"deviation":           deviation,
					},
					Metadata: map[string]interface{}{
						"detection_method": "access_rate_deviation",
						"session_id":       d.getSessionID(input),
					},
				}
				anomalies = append(anomalies, anomaly)
			}
		}
	}

	// Unusual access method distribution
	totalAccess := pattern.ViewCount + pattern.DownloadCount + pattern.ShareCount
	if totalAccess > 0 {
		downloadRatio := float64(pattern.DownloadCount) / float64(totalAccess)
		shareRatio := float64(pattern.ShareCount) / float64(totalAccess)

		// High download ratio might indicate data exfiltration
		if downloadRatio > 0.8 && totalAccess > 5 {
			anomaly := &anomaly.Anomaly{
				ID:              uuid.New(),
				Type:            anomaly.AnomalyTypeAccessPattern,
				Severity:        anomaly.SeverityMedium,
				Status:          anomaly.StatusDetected,
				Title:           "High Download Activity",
				Description:     fmt.Sprintf("Unusually high download ratio: %.1f%% of accesses are downloads", downloadRatio*100),
				DetectedAt:      time.Now(),
				UpdatedAt:       time.Now(),
				TenantID:        input.TenantID,
				DataSourceID:    input.DataSourceID,
				DocumentID:      input.DocumentID,
				UserID:          input.UserID,
				Score:           downloadRatio,
				Confidence:      0.8,
				Threshold:       0.8,
				DetectorName:    d.name,
				DetectorVersion: d.version,
				Detected: map[string]interface{}{
					"download_ratio": downloadRatio,
					"download_count": pattern.DownloadCount,
					"total_access":   totalAccess,
				},
				Metadata: map[string]interface{}{
					"detection_method": "download_ratio",
				},
			}
			anomalies = append(anomalies, anomaly)
		}

		// Unusual sharing activity
		if shareRatio > 0.5 && totalAccess > 3 {
			anomaly := &anomaly.Anomaly{
				ID:              uuid.New(),
				Type:            anomaly.AnomalyTypeAccessPattern,
				Severity:        anomaly.SeverityMedium,
				Status:          anomaly.StatusDetected,
				Title:           "High Sharing Activity",
				Description:     fmt.Sprintf("Unusually high sharing ratio: %.1f%% of accesses involve sharing", shareRatio*100),
				DetectedAt:      time.Now(),
				UpdatedAt:       time.Now(),
				TenantID:        input.TenantID,
				DataSourceID:    input.DataSourceID,
				DocumentID:      input.DocumentID,
				UserID:          input.UserID,
				Score:           shareRatio,
				Confidence:      0.75,
				Threshold:       0.5,
				DetectorName:    d.name,
				DetectorVersion: d.version,
				Detected: map[string]interface{}{
					"share_ratio":  shareRatio,
					"share_count":  pattern.ShareCount,
					"total_access": totalAccess,
				},
				Metadata: map[string]interface{}{
					"detection_method": "share_ratio",
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	return anomalies
}

func (d *BehavioralDetector) detectVelocityAnomalies(ctx context.Context, input *anomaly.DetectionInput, sessionID string, profile *UserProfile) []*anomaly.Anomaly {
	var anomalies []*anomaly.Anomaly

	sessionTracker := d.getSessionTracker(sessionID)
	if sessionTracker == nil {
		return anomalies
	}

	// Calculate current velocity
	sessionDuration := time.Since(sessionTracker.StartTime)
	if sessionDuration < time.Minute {
		return anomalies // Too early to detect velocity anomalies
	}

	requestsPerMinute := float64(sessionTracker.RequestCount) / sessionDuration.Minutes()

	// Compare with user profile
	if profile != nil && profile.AccessVelocity != nil {
		expectedVelocity := profile.AccessVelocity.AvgRequestsPerMinute

		if expectedVelocity > 0 {
			velocityRatio := requestsPerMinute / expectedVelocity

			if velocityRatio > d.config.VelocityThreshold {
				severity := anomaly.SeverityMedium
				if velocityRatio > 3.0 {
					severity = anomaly.SeverityHigh
				}
				if velocityRatio > 5.0 {
					severity = anomaly.SeverityCritical
				}

				anomaly := &anomaly.Anomaly{
					ID:              uuid.New(),
					Type:            anomaly.AnomalyTypeUsagePattern,
					Severity:        severity,
					Status:          anomaly.StatusDetected,
					Title:           "Unusual Access Velocity",
					Description:     fmt.Sprintf("User access velocity (%.1f req/min) is %.1fx higher than typical (%.1f req/min)", requestsPerMinute, velocityRatio, expectedVelocity),
					DetectedAt:      time.Now(),
					UpdatedAt:       time.Now(),
					TenantID:        input.TenantID,
					UserID:          input.UserID,
					Score:           math.Min(velocityRatio/5.0, 1.0),
					Confidence:      0.85,
					Threshold:       d.config.VelocityThreshold,
					DetectorName:    d.name,
					DetectorVersion: d.version,
					Baseline: map[string]interface{}{
						"expected_velocity": expectedVelocity,
					},
					Detected: map[string]interface{}{
						"current_velocity": requestsPerMinute,
						"velocity_ratio":   velocityRatio,
						"session_duration": sessionDuration.String(),
					},
					Metadata: map[string]interface{}{
						"detection_method": "velocity_comparison",
						"session_id":       sessionID,
					},
				}
				anomalies = append(anomalies, anomaly)
			}
		}
	}

	// Detect burst activity
	burstAnomalies := d.detectBurstActivity(ctx, input, sessionTracker)
	anomalies = append(anomalies, burstAnomalies...)

	return anomalies
}

func (d *BehavioralDetector) detectBurstActivity(ctx context.Context, input *anomaly.DetectionInput, sessionTracker *SessionTracker) []*anomaly.Anomaly {
	var anomalies []*anomaly.Anomaly

	// Count recent requests within burst detection window
	now := time.Now()
	recentRequests := 0

	for _, access := range sessionTracker.AccessedFiles {
		if now.Sub(access.AccessTime) <= d.config.BurstDetectionWindow {
			recentRequests++
		}
	}

	if recentRequests > d.config.BurstThreshold {
		severity := anomaly.SeverityMedium
		if recentRequests > d.config.BurstThreshold*2 {
			severity = anomaly.SeverityHigh
		}

		anomaly := &anomaly.Anomaly{
			ID:              uuid.New(),
			Type:            anomaly.AnomalyTypeFrequency,
			Severity:        severity,
			Status:          anomaly.StatusDetected,
			Title:           "Burst Activity Detected",
			Description:     fmt.Sprintf("User made %d requests in the last %s (threshold: %d)", recentRequests, d.config.BurstDetectionWindow.String(), d.config.BurstThreshold),
			DetectedAt:      time.Now(),
			UpdatedAt:       time.Now(),
			TenantID:        input.TenantID,
			UserID:          input.UserID,
			Score:           float64(recentRequests) / float64(d.config.BurstThreshold*2),
			Confidence:      0.9,
			Threshold:       float64(d.config.BurstThreshold),
			DetectorName:    d.name,
			DetectorVersion: d.version,
			Detected: map[string]interface{}{
				"recent_requests":  recentRequests,
				"detection_window": d.config.BurstDetectionWindow.String(),
				"burst_threshold":  d.config.BurstThreshold,
			},
			Metadata: map[string]interface{}{
				"detection_method": "burst_detection",
				"session_id":       sessionTracker.SessionID,
			},
		}
		anomalies = append(anomalies, anomaly)
	}

	return anomalies
}

func (d *BehavioralDetector) detectTimeBasedAnomalies(ctx context.Context, input *anomaly.DetectionInput, profile *UserProfile) []*anomaly.Anomaly {
	var anomalies []*anomaly.Anomaly

	currentHour := input.Timestamp.Hour()

	// Check if user is accessing during unusual hours
	if profile != nil && len(profile.TypicalAccessHours) > 0 {
		isTypicalHour := false
		for _, hour := range profile.TypicalAccessHours {
			if hour == currentHour {
				isTypicalHour = true
				break
			}
		}

		if !isTypicalHour {
			// Calculate how far this is from typical hours
			minDistance := 24
			for _, hour := range profile.TypicalAccessHours {
				distance := int(math.Abs(float64(currentHour - hour)))
				if distance > 12 {
					distance = 24 - distance // Circular distance
				}
				if distance < minDistance {
					minDistance = distance
				}
			}

			severity := anomaly.SeverityLow
			if minDistance > 4 {
				severity = anomaly.SeverityMedium
			}
			if minDistance > 8 {
				severity = anomaly.SeverityHigh
			}

			anomaly := &anomaly.Anomaly{
				ID:              uuid.New(),
				Type:            anomaly.AnomalyTypeUsagePattern,
				Severity:        severity,
				Status:          anomaly.StatusDetected,
				Title:           "Off-Hours Access",
				Description:     fmt.Sprintf("User accessing system at unusual hour (%d:00), %d hours from typical access times", currentHour, minDistance),
				DetectedAt:      time.Now(),
				UpdatedAt:       time.Now(),
				TenantID:        input.TenantID,
				UserID:          input.UserID,
				Score:           float64(minDistance) / 12.0,
				Confidence:      0.7,
				DetectorName:    d.name,
				DetectorVersion: d.version,
				Baseline: map[string]interface{}{
					"typical_access_hours": profile.TypicalAccessHours,
				},
				Detected: map[string]interface{}{
					"current_hour":       currentHour,
					"hours_from_typical": minDistance,
				},
				Metadata: map[string]interface{}{
					"detection_method": "off_hours_access",
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	// Weekend access anomaly (if user typically doesn't work weekends)
	if input.Timestamp.Weekday() == time.Saturday || input.Timestamp.Weekday() == time.Sunday {
		if profile != nil && profile.TotalSessions > 20 {
			// This is a simplified check - in production, you'd track weekend vs weekday patterns
			anomaly := &anomaly.Anomaly{
				ID:              uuid.New(),
				Type:            anomaly.AnomalyTypeUsagePattern,
				Severity:        anomaly.SeverityLow,
				Status:          anomaly.StatusDetected,
				Title:           "Weekend Access",
				Description:     fmt.Sprintf("User accessing system on %s", input.Timestamp.Weekday().String()),
				DetectedAt:      time.Now(),
				UpdatedAt:       time.Now(),
				TenantID:        input.TenantID,
				UserID:          input.UserID,
				Score:           0.3,
				Confidence:      0.6,
				DetectorName:    d.name,
				DetectorVersion: d.version,
				Detected: map[string]interface{}{
					"day_of_week": input.Timestamp.Weekday().String(),
				},
				Metadata: map[string]interface{}{
					"detection_method": "weekend_access",
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	return anomalies
}

func (d *BehavioralDetector) detectLocationAnomalies(ctx context.Context, input *anomaly.DetectionInput, profile *UserProfile) []*anomaly.Anomaly {
	var anomalies []*anomaly.Anomaly

	if input.AccessPattern == nil || len(input.AccessPattern.Locations) == 0 {
		return anomalies
	}

	currentLocation := input.AccessPattern.Locations[0] // Take first location

	if profile != nil && len(profile.TypicalLocations) > 0 {
		isTypicalLocation := false
		for _, location := range profile.TypicalLocations {
			if location == currentLocation {
				isTypicalLocation = true
				break
			}
		}

		if !isTypicalLocation {
			severity := anomaly.SeverityMedium
			if len(profile.TypicalLocations) > 3 {
				severity = anomaly.SeverityHigh // More established patterns make deviation more suspicious
			}

			anomaly := &anomaly.Anomaly{
				ID:              uuid.New(),
				Type:            anomaly.AnomalyTypeUnauthorizedAccess,
				Severity:        severity,
				Status:          anomaly.StatusDetected,
				Title:           "Access from New Location",
				Description:     fmt.Sprintf("User accessing from unusual location: %s", currentLocation),
				DetectedAt:      time.Now(),
				UpdatedAt:       time.Now(),
				TenantID:        input.TenantID,
				UserID:          input.UserID,
				Score:           0.8,
				Confidence:      0.75,
				DetectorName:    d.name,
				DetectorVersion: d.version,
				Baseline: map[string]interface{}{
					"typical_locations": profile.TypicalLocations,
				},
				Detected: map[string]interface{}{
					"current_location": currentLocation,
				},
				Metadata: map[string]interface{}{
					"detection_method": "location_deviation",
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	// Check for rapid location changes (possible account compromise)
	sessionTracker := d.getSessionTracker(d.getSessionID(input))
	if sessionTracker != nil && len(sessionTracker.Locations) > 1 {
		uniqueLocations := make(map[string]bool)
		for _, loc := range sessionTracker.Locations {
			uniqueLocations[loc] = true
		}

		if len(uniqueLocations) > d.config.LocationChangeThreshold {
			anomaly := &anomaly.Anomaly{
				ID:              uuid.New(),
				Type:            anomaly.AnomalyTypeUnauthorizedAccess,
				Severity:        anomaly.SeverityHigh,
				Status:          anomaly.StatusDetected,
				Title:           "Multiple Location Access",
				Description:     fmt.Sprintf("User accessed from %d different locations in single session", len(uniqueLocations)),
				DetectedAt:      time.Now(),
				UpdatedAt:       time.Now(),
				TenantID:        input.TenantID,
				UserID:          input.UserID,
				Score:           float64(len(uniqueLocations)) / 10.0,
				Confidence:      0.9,
				Threshold:       float64(d.config.LocationChangeThreshold),
				DetectorName:    d.name,
				DetectorVersion: d.version,
				Detected: map[string]interface{}{
					"unique_locations": len(uniqueLocations),
					"locations":        sessionTracker.Locations,
				},
				Metadata: map[string]interface{}{
					"detection_method": "multiple_locations",
					"session_id":       sessionTracker.SessionID,
				},
			}
			anomalies = append(anomalies, anomaly)
		}
	}

	return anomalies
}

func (d *BehavioralDetector) detectSessionAnomalies(ctx context.Context, input *anomaly.DetectionInput, sessionID string) []*anomaly.Anomaly {
	var anomalies []*anomaly.Anomaly

	sessionTracker := d.getSessionTracker(sessionID)
	if sessionTracker == nil {
		return anomalies
	}

	// Detect unusually long sessions
	sessionDuration := time.Since(sessionTracker.StartTime)
	if sessionDuration > d.config.SessionTimeoutThreshold*3 {
		severity := anomaly.SeverityLow
		if sessionDuration > d.config.SessionTimeoutThreshold*6 {
			severity = anomaly.SeverityMedium
		}

		anomaly := &anomaly.Anomaly{
			ID:              uuid.New(),
			Type:            anomaly.AnomalyTypeUsagePattern,
			Severity:        severity,
			Status:          anomaly.StatusDetected,
			Title:           "Extended Session Duration",
			Description:     fmt.Sprintf("Session duration (%s) exceeds normal patterns", sessionDuration.String()),
			DetectedAt:      time.Now(),
			UpdatedAt:       time.Now(),
			TenantID:        input.TenantID,
			UserID:          input.UserID,
			Score:           math.Min(sessionDuration.Hours()/24.0, 1.0),
			Confidence:      0.7,
			DetectorName:    d.name,
			DetectorVersion: d.version,
			Detected: map[string]interface{}{
				"session_duration": sessionDuration.String(),
			},
			Metadata: map[string]interface{}{
				"detection_method": "extended_session",
				"session_id":       sessionID,
			},
		}
		anomalies = append(anomalies, anomaly)
	}

	return anomalies
}

func (d *BehavioralDetector) detectFileAccessAnomalies(ctx context.Context, input *anomaly.DetectionInput, profile *UserProfile) []*anomaly.Anomaly {
	var anomalies []*anomaly.Anomaly

	// Check for suspicious file types
	if input.FileName != "" {
		for _, suspiciousType := range d.config.SuspiciousFileTypes {
			if strings.HasSuffix(strings.ToLower(input.FileName), suspiciousType) {
				anomaly := &anomaly.Anomaly{
					ID:              uuid.New(),
					Type:            anomaly.AnomalyTypeSuspiciousContent,
					Severity:        anomaly.SeverityHigh,
					Status:          anomaly.StatusDetected,
					Title:           "Suspicious File Type Access",
					Description:     fmt.Sprintf("User accessed file with suspicious extension: %s", suspiciousType),
					DetectedAt:      time.Now(),
					UpdatedAt:       time.Now(),
					TenantID:        input.TenantID,
					DataSourceID:    input.DataSourceID,
					DocumentID:      input.DocumentID,
					UserID:          input.UserID,
					Score:           0.9,
					Confidence:      0.85,
					DetectorName:    d.name,
					DetectorVersion: d.version,
					Detected: map[string]interface{}{
						"file_name": input.FileName,
						"file_type": suspiciousType,
					},
					Metadata: map[string]interface{}{
						"detection_method": "suspicious_file_type",
					},
				}
				anomalies = append(anomalies, anomaly)
				break
			}
		}
	}

	// Check for large file access
	if input.FileSize > d.config.LargeFileThresholdMB*1024*1024 {
		anomaly := &anomaly.Anomaly{
			ID:              uuid.New(),
			Type:            anomaly.AnomalyTypeAccessPattern,
			Severity:        anomaly.SeverityMedium,
			Status:          anomaly.StatusDetected,
			Title:           "Large File Access",
			Description:     fmt.Sprintf("User accessed unusually large file: %.2f MB", float64(input.FileSize)/(1024*1024)),
			DetectedAt:      time.Now(),
			UpdatedAt:       time.Now(),
			TenantID:        input.TenantID,
			DataSourceID:    input.DataSourceID,
			DocumentID:      input.DocumentID,
			UserID:          input.UserID,
			Score:           math.Min(float64(input.FileSize)/(float64(d.config.LargeFileThresholdMB)*1024*1024*10), 1.0),
			Confidence:      0.6,
			Threshold:       float64(d.config.LargeFileThresholdMB),
			DetectorName:    d.name,
			DetectorVersion: d.version,
			Detected: map[string]interface{}{
				"file_size_mb": float64(input.FileSize) / (1024 * 1024),
				"file_name":    input.FileName,
			},
			Metadata: map[string]interface{}{
				"detection_method": "large_file_access",
			},
		}
		anomalies = append(anomalies, anomaly)
	}

	return anomalies
}

func (d *BehavioralDetector) UpdateBaseline(ctx context.Context, data *anomaly.BaselineData) error {
	key := d.getBaselineKey(data.TenantID, data.DataType)
	data.UpdatedAt = time.Now()
	d.baselines[key] = data
	return nil
}

func (d *BehavioralDetector) GetBaseline(ctx context.Context) (*anomaly.BaselineData, error) {
	for _, baseline := range d.baselines {
		return baseline, nil
	}
	return nil, fmt.Errorf("no baseline data available")
}

// Helper methods

func (d *BehavioralDetector) getBaselineKey(tenantID uuid.UUID, dataType string) string {
	return fmt.Sprintf("%s:%s", tenantID.String(), dataType)
}

func (d *BehavioralDetector) getSessionID(input *anomaly.DetectionInput) string {
	// In a real implementation, this would be extracted from the request context
	// For now, use a combination of user ID and timestamp (rounded to hour)
	hour := input.Timestamp.Truncate(time.Hour)
	if input.UserID != nil {
		return fmt.Sprintf("%s:%s", input.UserID.String(), hour.Format("2006010215"))
	}
	return fmt.Sprintf("anonymous:%s", hour.Format("2006010215"))
}

func (d *BehavioralDetector) getSessionTracker(sessionID string) *SessionTracker {
	return d.sessionData[sessionID]
}

func (d *BehavioralDetector) updateSessionTracker(sessionID string, input *anomaly.DetectionInput) {
	tracker, exists := d.sessionData[sessionID]
	if !exists {
		tracker = &SessionTracker{
			SessionID:       sessionID,
			StartTime:       input.Timestamp,
			LastActivity:    input.Timestamp,
			RequestCount:    0,
			FileAccessCount: 0,
			Locations:       []string{},
			AccessedFiles:   []FileAccess{},
		}
		if input.UserID != nil {
			tracker.UserID = *input.UserID
		}
		d.sessionData[sessionID] = tracker
	}

	// Update tracker
	tracker.LastActivity = input.Timestamp
	tracker.RequestCount++

	if input.DocumentID != nil {
		tracker.FileAccessCount++
		fileAccess := FileAccess{
			DocumentID: *input.DocumentID,
			AccessTime: input.Timestamp,
			AccessType: "view", // Default, could be extracted from context
			FileSize:   input.FileSize,
			FileType:   input.ContentType,
			Success:    true,
		}
		tracker.AccessedFiles = append(tracker.AccessedFiles, fileAccess)
	}

	// Update locations
	if input.AccessPattern != nil && len(input.AccessPattern.Locations) > 0 {
		location := input.AccessPattern.Locations[0]
		if len(tracker.Locations) == 0 || tracker.Locations[len(tracker.Locations)-1] != location {
			tracker.Locations = append(tracker.Locations, location)
		}
	}
}

func (d *BehavioralDetector) getUserProfile(userID uuid.UUID) *UserProfile {
	return d.userProfiles[userID]
}

func (d *BehavioralDetector) updateUserProfile(userID uuid.UUID, input *anomaly.DetectionInput) {
	profile, exists := d.userProfiles[userID]
	if !exists {
		profile = &UserProfile{
			UserID:             userID,
			TypicalAccessHours: []int{},
			TypicalLocations:   []string{},
			TypicalDevices:     []string{},
			TypicalFileTypes:   make(map[string]int),
			CreatedAt:          time.Now(),
			BehaviorSignatures: make(map[string]float64),
		}
		d.userProfiles[userID] = profile
	}

	// Update profile data
	profile.UpdatedAt = time.Now()
	profile.LastSeen = input.Timestamp

	// Update typical access hours
	currentHour := input.Timestamp.Hour()
	hasHour := false
	for _, hour := range profile.TypicalAccessHours {
		if hour == currentHour {
			hasHour = true
			break
		}
	}
	if !hasHour && len(profile.TypicalAccessHours) < 24 {
		profile.TypicalAccessHours = append(profile.TypicalAccessHours, currentHour)
		sort.Ints(profile.TypicalAccessHours)
	}

	// Update locations
	if input.AccessPattern != nil && len(input.AccessPattern.Locations) > 0 {
		location := input.AccessPattern.Locations[0]
		hasLocation := false
		for _, loc := range profile.TypicalLocations {
			if loc == location {
				hasLocation = true
				break
			}
		}
		if !hasLocation && len(profile.TypicalLocations) < 10 {
			profile.TypicalLocations = append(profile.TypicalLocations, location)
		}
	}

	// Update file types
	if input.ContentType != "" {
		profile.TypicalFileTypes[input.ContentType]++
	}
}
