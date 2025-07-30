package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// SyncJobManager handles persistence and retrieval of sync jobs
type SyncJobManager struct {
	db     *sql.DB
	config *OrchestratorConfig
	tracer trace.Tracer
}

// NewSyncJobManager creates a new sync job manager
func NewSyncJobManager(config *OrchestratorConfig) *SyncJobManager {
	return &SyncJobManager{
		config: config,
		tracer: otel.Tracer("sync-job-manager"),
	}
}

// SetDB sets the database connection for the job manager
func (jm *SyncJobManager) SetDB(db *sql.DB) {
	jm.db = db
}

// InitializeSchema creates the necessary database tables
func (jm *SyncJobManager) InitializeSchema(ctx context.Context) error {
	ctx, span := jm.tracer.Start(ctx, "job_manager.initialize_schema")
	defer span.End()

	// Create sync_jobs table
	createSyncJobsTable := `
	CREATE TABLE IF NOT EXISTS sync_jobs (
		id UUID PRIMARY KEY,
		data_source_id UUID NOT NULL,
		connector_type VARCHAR(100) NOT NULL,
		sync_type VARCHAR(50) NOT NULL,
		sync_direction VARCHAR(50) NOT NULL,
		sync_options JSONB NOT NULL,
		state VARCHAR(50) NOT NULL,
		start_time TIMESTAMP WITH TIME ZONE NOT NULL,
		end_time TIMESTAMP WITH TIME ZONE,
		duration INTERVAL,
		last_activity TIMESTAMP WITH TIME ZONE NOT NULL,
		progress JSONB,
		metrics JSONB,
		error_message TEXT,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		INDEX(data_source_id),
		INDEX(connector_type),
		INDEX(state),
		INDEX(start_time),
		INDEX(created_at)
	)`

	if _, err := jm.db.ExecContext(ctx, createSyncJobsTable); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to create sync_jobs table: %w", err)
	}

	// Create sync_job_files table for tracking individual file operations
	createSyncJobFilesTable := `
	CREATE TABLE IF NOT EXISTS sync_job_files (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		job_id UUID NOT NULL REFERENCES sync_jobs(id) ON DELETE CASCADE,
		file_path VARCHAR(2048) NOT NULL,
		file_size BIGINT,
		operation VARCHAR(50) NOT NULL, -- created, updated, deleted, skipped, failed
		status VARCHAR(50) NOT NULL,
		error_message TEXT,
		start_time TIMESTAMP WITH TIME ZONE NOT NULL,
		end_time TIMESTAMP WITH TIME ZONE,
		duration INTERVAL,
		bytes_transferred BIGINT DEFAULT 0,
		checksum VARCHAR(128),
		metadata JSONB,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		INDEX(job_id),
		INDEX(operation),
		INDEX(status),
		INDEX(start_time),
		INDEX(file_path)
	)`

	if _, err := jm.db.ExecContext(ctx, createSyncJobFilesTable); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to create sync_job_files table: %w", err)
	}

	// Create sync_schedules table
	createSyncSchedulesTable := `
	CREATE TABLE IF NOT EXISTS sync_schedules (
		id UUID PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		description TEXT,
		data_source_id UUID NOT NULL,
		connector_type VARCHAR(100) NOT NULL,
		sync_options JSONB NOT NULL,
		schedule_type VARCHAR(50) NOT NULL,
		schedule_config JSONB NOT NULL,
		active BOOLEAN DEFAULT true,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		last_run TIMESTAMP WITH TIME ZONE,
		next_run TIMESTAMP WITH TIME ZONE,
		run_count INTEGER DEFAULT 0,
		max_runs INTEGER,
		INDEX(data_source_id),
		INDEX(connector_type),
		INDEX(active),
		INDEX(next_run)
	)`

	if _, err := jm.db.ExecContext(ctx, createSyncSchedulesTable); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to create sync_schedules table: %w", err)
	}

	// Create sync_conflicts table
	createSyncConflictsTable := `
	CREATE TABLE IF NOT EXISTS sync_conflicts (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		job_id UUID NOT NULL REFERENCES sync_jobs(id) ON DELETE CASCADE,
		file_path VARCHAR(2048) NOT NULL,
		conflict_type VARCHAR(50) NOT NULL, -- modified_both, deleted_modified, etc.
		local_version JSONB,
		remote_version JSONB,
		resolution_strategy VARCHAR(50),
		resolved_at TIMESTAMP WITH TIME ZONE,
		resolved_by VARCHAR(255),
		resolution_result JSONB,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		INDEX(job_id),
		INDEX(conflict_type),
		INDEX(resolved_at),
		INDEX(created_at)
	)`

	if _, err := jm.db.ExecContext(ctx, createSyncConflictsTable); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to create sync_conflicts table: %w", err)
	}

	return nil
}

// StoreJobResult persists a completed sync job to the database
func (jm *SyncJobManager) StoreJobResult(ctx context.Context, job *SyncJob) error {
	ctx, span := jm.tracer.Start(ctx, "job_manager.store_job_result")
	defer span.End()

	span.SetAttributes(
		attribute.String("job_id", job.ID.String()),
		attribute.String("state", string(job.Status.State)),
	)

	if jm.db == nil {
		return fmt.Errorf("database not configured")
	}

	// Serialize options and progress
	optionsJSON, err := json.Marshal(job.Options)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to marshal sync options: %w", err)
	}

	progressJSON, err := json.Marshal(job.Status.Progress)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to marshal progress: %w", err)
	}

	metricsJSON, err := json.Marshal(job.Status.Metrics)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to marshal metrics: %w", err)
	}

	// Insert or update sync job
	query := `
		INSERT INTO sync_jobs (
			id, data_source_id, connector_type, sync_type, sync_direction,
			sync_options, state, start_time, end_time, duration,
			last_activity, progress, metrics, error_message, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW())
		ON CONFLICT (id) DO UPDATE SET
			state = EXCLUDED.state,
			end_time = EXCLUDED.end_time,
			duration = EXCLUDED.duration,
			last_activity = EXCLUDED.last_activity,
			progress = EXCLUDED.progress,
			metrics = EXCLUDED.metrics,
			error_message = EXCLUDED.error_message,
			updated_at = NOW()
	`

	var duration *time.Duration
	if job.Status.EndTime != nil {
		d := job.Status.EndTime.Sub(job.Status.StartTime)
		duration = &d
	}

	_, err = jm.db.ExecContext(ctx, query,
		job.ID,
		job.DataSourceID,
		job.ConnectorType,
		job.Options.SyncType,
		job.Options.Direction,
		optionsJSON,
		job.Status.State,
		job.Status.StartTime,
		job.Status.EndTime,
		duration,
		job.Status.LastActivity,
		progressJSON,
		metricsJSON,
		job.Status.Error,
	)

	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to store sync job: %w", err)
	}

	return nil
}

// GetJobStatus retrieves the status of a sync job
func (jm *SyncJobManager) GetJobStatus(ctx context.Context, jobID uuid.UUID) (*SyncJobStatus, error) {
	ctx, span := jm.tracer.Start(ctx, "job_manager.get_job_status")
	defer span.End()

	span.SetAttributes(attribute.String("job_id", jobID.String()))

	if jm.db == nil {
		return nil, fmt.Errorf("database not configured")
	}

	query := `
		SELECT id, state, start_time, end_time, duration, last_activity,
		       progress, metrics, error_message
		FROM sync_jobs
		WHERE id = $1
	`

	var status SyncJobStatus
	var progressJSON, metricsJSON []byte
	var duration *time.Duration

	err := jm.db.QueryRowContext(ctx, query, jobID).Scan(
		&status.JobID,
		&status.State,
		&status.StartTime,
		&status.EndTime,
		&duration,
		&status.LastActivity,
		&progressJSON,
		&metricsJSON,
		&status.Error,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrSyncJobNotFound
		}
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get sync job status: %w", err)
	}

	if duration != nil {
		status.Duration = *duration
	}

	// Unmarshal progress
	if len(progressJSON) > 0 {
		var progress SyncProgress
		if err := json.Unmarshal(progressJSON, &progress); err == nil {
			status.Progress = &progress
		}
	}

	// Unmarshal metrics
	if len(metricsJSON) > 0 {
		var metrics SyncMetrics
		if err := json.Unmarshal(metricsJSON, &metrics); err == nil {
			status.Metrics = &metrics
		}
	}

	return &status, nil
}

// GetSyncHistory retrieves historical sync operations with filtering
func (jm *SyncJobManager) GetSyncHistory(ctx context.Context, filters *SyncHistoryFilters) (*SyncHistoryResult, error) {
	ctx, span := jm.tracer.Start(ctx, "job_manager.get_sync_history")
	defer span.End()

	if jm.db == nil {
		return nil, fmt.Errorf("database not configured")
	}

	// Build query with filters
	whereClause := "WHERE 1=1"
	args := make([]interface{}, 0)
	argIndex := 1

	if filters.DataSourceID != uuid.Nil {
		whereClause += fmt.Sprintf(" AND data_source_id = $%d", argIndex)
		args = append(args, filters.DataSourceID)
		argIndex++
	}

	if filters.ConnectorType != "" {
		whereClause += fmt.Sprintf(" AND connector_type = $%d", argIndex)
		args = append(args, filters.ConnectorType)
		argIndex++
	}

	if filters.StartTime != nil {
		whereClause += fmt.Sprintf(" AND start_time >= $%d", argIndex)
		args = append(args, *filters.StartTime)
		argIndex++
	}

	if filters.EndTime != nil {
		whereClause += fmt.Sprintf(" AND start_time <= $%d", argIndex)
		args = append(args, *filters.EndTime)
		argIndex++
	}

	if len(filters.States) > 0 {
		placeholders := make([]string, len(filters.States))
		for i, state := range filters.States {
			placeholders[i] = fmt.Sprintf("$%d", argIndex)
			args = append(args, state)
			argIndex++
		}
		whereClause += fmt.Sprintf(" AND state IN (%s)", fmt.Sprintf("%v", placeholders))
	}

	// Get total count
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM sync_jobs %s", whereClause)
	var totalCount int
	err := jm.db.QueryRowContext(ctx, countQuery, args...).Scan(&totalCount)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get sync history count: %w", err)
	}

	// Get jobs with pagination
	limit := filters.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := filters.Offset
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(`
		SELECT id, data_source_id, connector_type, sync_type, sync_direction,
		       state, start_time, end_time, duration, last_activity,
		       progress, metrics, error_message
		FROM sync_jobs
		%s
		ORDER BY start_time DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIndex, argIndex+1)

	args = append(args, limit, offset)

	rows, err := jm.db.QueryContext(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get sync history: %w", err)
	}
	defer rows.Close()

	var jobs []*SyncJobStatus
	for rows.Next() {
		var status SyncJobStatus
		var dataSourceID uuid.UUID
		var connectorType string
		var syncType SyncType
		var syncDirection SyncDirection
		var progressJSON, metricsJSON []byte
		var duration *time.Duration

		err := rows.Scan(
			&status.JobID,
			&dataSourceID,
			&connectorType,
			&syncType,
			&syncDirection,
			&status.State,
			&status.StartTime,
			&status.EndTime,
			&duration,
			&status.LastActivity,
			&progressJSON,
			&metricsJSON,
			&status.Error,
		)

		if err != nil {
			span.RecordError(err)
			continue
		}

		if duration != nil {
			status.Duration = *duration
		}

		// Unmarshal progress
		if len(progressJSON) > 0 {
			var progress SyncProgress
			if err := json.Unmarshal(progressJSON, &progress); err == nil {
				status.Progress = &progress
			}
		}

		// Unmarshal metrics
		if len(metricsJSON) > 0 {
			var metrics SyncMetrics
			if err := json.Unmarshal(metricsJSON, &metrics); err == nil {
				status.Metrics = &metrics
			}
		}

		jobs = append(jobs, &status)
	}

	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to iterate sync history rows: %w", err)
	}

	hasMore := offset+len(jobs) < totalCount

	span.SetAttributes(
		attribute.Int("total_count", totalCount),
		attribute.Int("returned_count", len(jobs)),
		attribute.Bool("has_more", hasMore),
	)

	return &SyncHistoryResult{
		Jobs:       jobs,
		TotalCount: totalCount,
		HasMore:    hasMore,
	}, nil
}

// StoreFileOperation records a file operation within a sync job
func (jm *SyncJobManager) StoreFileOperation(ctx context.Context, jobID uuid.UUID, operation *FileOperation) error {
	ctx, span := jm.tracer.Start(ctx, "job_manager.store_file_operation")
	defer span.End()

	span.SetAttributes(
		attribute.String("job_id", jobID.String()),
		attribute.String("file_path", operation.FilePath),
		attribute.String("operation", operation.Operation),
	)

	if jm.db == nil {
		return fmt.Errorf("database not configured")
	}

	metadataJSON, err := json.Marshal(operation.Metadata)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to marshal file operation metadata: %w", err)
	}

	var duration *time.Duration
	if operation.EndTime != nil {
		d := operation.EndTime.Sub(operation.StartTime)
		duration = &d
	}

	query := `
		INSERT INTO sync_job_files (
			job_id, file_path, file_size, operation, status,
			error_message, start_time, end_time, duration,
			bytes_transferred, checksum, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	_, err = jm.db.ExecContext(ctx, query,
		jobID,
		operation.FilePath,
		operation.FileSize,
		operation.Operation,
		operation.Status,
		operation.ErrorMessage,
		operation.StartTime,
		operation.EndTime,
		duration,
		operation.BytesTransferred,
		operation.Checksum,
		metadataJSON,
	)

	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to store file operation: %w", err)
	}

	return nil
}

// GetFileOperations retrieves file operations for a sync job
func (jm *SyncJobManager) GetFileOperations(ctx context.Context, jobID uuid.UUID, filters *FileOperationFilters) ([]*FileOperation, error) {
	ctx, span := jm.tracer.Start(ctx, "job_manager.get_file_operations")
	defer span.End()

	span.SetAttributes(attribute.String("job_id", jobID.String()))

	if jm.db == nil {
		return nil, fmt.Errorf("database not configured")
	}

	// Build query with filters
	whereClause := "WHERE job_id = $1"
	args := []interface{}{jobID}
	argIndex := 2

	if filters != nil {
		if filters.Operation != "" {
			whereClause += fmt.Sprintf(" AND operation = $%d", argIndex)
			args = append(args, filters.Operation)
			argIndex++
		}

		if filters.Status != "" {
			whereClause += fmt.Sprintf(" AND status = $%d", argIndex)
			args = append(args, filters.Status)
			argIndex++
		}

		if filters.FilePathPattern != "" {
			whereClause += fmt.Sprintf(" AND file_path LIKE $%d", argIndex)
			args = append(args, "%"+filters.FilePathPattern+"%")
			argIndex++
		}
	}

	query := fmt.Sprintf(`
		SELECT id, file_path, file_size, operation, status,
		       error_message, start_time, end_time, duration,
		       bytes_transferred, checksum, metadata
		FROM sync_job_files
		%s
		ORDER BY start_time DESC
	`, whereClause)

	rows, err := jm.db.QueryContext(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get file operations: %w", err)
	}
	defer rows.Close()

	var operations []*FileOperation
	for rows.Next() {
		var op FileOperation
		var metadataJSON []byte
		var duration *time.Duration
		var id uuid.UUID

		err := rows.Scan(
			&id,
			&op.FilePath,
			&op.FileSize,
			&op.Operation,
			&op.Status,
			&op.ErrorMessage,
			&op.StartTime,
			&op.EndTime,
			&duration,
			&op.BytesTransferred,
			&op.Checksum,
			&metadataJSON,
		)

		if err != nil {
			span.RecordError(err)
			continue
		}

		if duration != nil {
			op.Duration = *duration
		}

		// Unmarshal metadata
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &op.Metadata); err == nil {
				// Successfully unmarshaled
			}
		}

		operations = append(operations, &op)
	}

	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to iterate file operations rows: %w", err)
	}

	span.SetAttributes(attribute.Int("operations_count", len(operations)))

	return operations, nil
}

// CleanupExpiredJobs removes old sync jobs based on retention policy
func (jm *SyncJobManager) CleanupExpiredJobs(ctx context.Context) error {
	ctx, span := jm.tracer.Start(ctx, "job_manager.cleanup_expired_jobs")
	defer span.End()

	if jm.db == nil {
		return fmt.Errorf("database not configured")
	}

	cutoffTime := time.Now().Add(-jm.config.JobRetentionDuration)

	// Delete old sync jobs (cascade will delete related records)
	query := `DELETE FROM sync_jobs WHERE created_at < $1`
	result, err := jm.db.ExecContext(ctx, query, cutoffTime)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to cleanup expired jobs: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	span.SetAttributes(attribute.Int64("deleted_jobs", rowsAffected))

	return nil
}

// Shutdown gracefully shuts down the job manager
func (jm *SyncJobManager) Shutdown(ctx context.Context) error {
	if jm.db != nil {
		return jm.db.Close()
	}
	return nil
}

// FileOperation represents a file operation within a sync job
type FileOperation struct {
	FilePath          string                 `json:"file_path"`
	FileSize          *int64                 `json:"file_size,omitempty"`
	Operation         string                 `json:"operation"` // created, updated, deleted, skipped, failed
	Status            string                 `json:"status"`    // pending, in_progress, completed, failed
	ErrorMessage      string                 `json:"error_message,omitempty"`
	StartTime         time.Time              `json:"start_time"`
	EndTime           *time.Time             `json:"end_time,omitempty"`
	Duration          time.Duration          `json:"duration"`
	BytesTransferred  int64                  `json:"bytes_transferred"`
	Checksum          string                 `json:"checksum,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
}

// FileOperationFilters contains filters for querying file operations
type FileOperationFilters struct {
	Operation       string `json:"operation,omitempty"`
	Status          string `json:"status,omitempty"`
	FilePathPattern string `json:"file_path_pattern,omitempty"`
}