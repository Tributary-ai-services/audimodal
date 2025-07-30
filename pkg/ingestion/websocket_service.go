package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jscharber/eAIIngest/pkg/events"
)

// WebSocketStreamingService handles real-time document uploads and progress streaming
type WebSocketStreamingService struct {
	upgrader     websocket.Upgrader
	producer     *events.Producer
	tracer       trace.Tracer
	config       *WebSocketConfig
	connections  map[string]*WebSocketConnection
	connMutex    sync.RWMutex
}

// WebSocketConfig contains configuration for WebSocket streaming
type WebSocketConfig struct {
	// Connection settings
	ReadBufferSize    int           `yaml:"read_buffer_size"`
	WriteBufferSize   int           `yaml:"write_buffer_size"`
	HandshakeTimeout  time.Duration `yaml:"handshake_timeout"`
	
	// Message handling
	MaxMessageSize    int64         `yaml:"max_message_size"`
	ReadTimeout       time.Duration `yaml:"read_timeout"`
	WriteTimeout      time.Duration `yaml:"write_timeout"`
	PongTimeout       time.Duration `yaml:"pong_timeout"`
	PingPeriod        time.Duration `yaml:"ping_period"`
	
	// Authentication
	RequireAuth       bool          `yaml:"require_auth"`
	JWTSecret         string        `yaml:"jwt_secret"`
	
	// Rate limiting
	MaxConnections    int           `yaml:"max_connections"`
	MessageRateLimit  int           `yaml:"message_rate_limit"` // messages per minute
	
	// File upload
	MaxFileSize       int64         `yaml:"max_file_size"`
	AllowedMimeTypes  []string      `yaml:"allowed_mime_types"`
	ChunkSize         int           `yaml:"chunk_size"`
}

// DefaultWebSocketConfig returns default WebSocket configuration
func DefaultWebSocketConfig() *WebSocketConfig {
	return &WebSocketConfig{
		ReadBufferSize:   4096,
		WriteBufferSize:  4096,
		HandshakeTimeout: 10 * time.Second,
		MaxMessageSize:   10 * 1024 * 1024, // 10MB
		ReadTimeout:      60 * time.Second,
		WriteTimeout:     10 * time.Second,
		PongTimeout:      60 * time.Second,
		PingPeriod:       54 * time.Second,
		RequireAuth:      true,
		MaxConnections:   1000,
		MessageRateLimit: 100,
		MaxFileSize:      100 * 1024 * 1024, // 100MB
		AllowedMimeTypes: []string{
			"application/pdf",
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			"application/vnd.openxmlformats-officedocument.presentationml.presentation",
			"text/plain",
			"text/html",
			"application/json",
		},
		ChunkSize: 64 * 1024, // 64KB chunks
	}
}

// WebSocketConnection represents an active WebSocket connection
type WebSocketConnection struct {
	ID          string
	Conn        *websocket.Conn
	TenantID    string
	UserID      string
	Send        chan []byte
	Hub         *WebSocketStreamingService
	LastSeen    time.Time
	RateLimit   *RateLimiter
	UploadState *UploadState
}

// UploadState tracks the state of a file upload
type UploadState struct {
	FileID       string
	FileName     string
	FileSize     int64
	ContentType  string
	ChunksTotal  int
	ChunksReceived int
	ChunkData    map[int][]byte
	StartedAt    time.Time
	Metadata     map[string]string
}

// RateLimiter implements simple rate limiting
type RateLimiter struct {
	messages   []time.Time
	maxMessages int
	window      time.Duration
	mutex       sync.Mutex
}

// NewWebSocketStreamingService creates a new WebSocket streaming service
func NewWebSocketStreamingService(producer *events.Producer, config *WebSocketConfig) *WebSocketStreamingService {
	if config == nil {
		config = DefaultWebSocketConfig()
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:   config.ReadBufferSize,
		WriteBufferSize:  config.WriteBufferSize,
		HandshakeTimeout: config.HandshakeTimeout,
		CheckOrigin: func(r *http.Request) bool {
			// In production, implement proper origin checking
			return true
		},
	}

	return &WebSocketStreamingService{
		upgrader:    upgrader,
		producer:    producer,
		tracer:      otel.Tracer("websocket-streaming"),
		config:      config,
		connections: make(map[string]*WebSocketConnection),
	}
}

// HandleWebSocket handles WebSocket upgrade requests
func (ws *WebSocketStreamingService) HandleWebSocket(w http.ResponseWriter, r *http.Request, tenantID, userID string) {
	ctx, span := ws.tracer.Start(r.Context(), "websocket_connection")
	defer span.End()

	span.SetAttributes(
		attribute.String("tenant.id", tenantID),
		attribute.String("user.id", userID),
	)

	// Check connection limits
	ws.connMutex.RLock()
	if len(ws.connections) >= ws.config.MaxConnections {
		ws.connMutex.RUnlock()
		http.Error(w, "Too many connections", http.StatusTooManyRequests)
		return
	}
	ws.connMutex.RUnlock()

	// Upgrade connection
	conn, err := ws.upgrader.Upgrade(w, r, nil)
	if err != nil {
		span.RecordError(err)
		http.Error(w, "Failed to upgrade connection", http.StatusBadRequest)
		return
	}

	// Create connection object
	connID := uuid.New().String()
	wsConn := &WebSocketConnection{
		ID:       connID,
		Conn:     conn,
		TenantID: tenantID,
		UserID:   userID,
		Send:     make(chan []byte, 256),
		Hub:      ws,
		LastSeen: time.Now(),
		RateLimit: &RateLimiter{
			maxMessages: ws.config.MessageRateLimit,
			window:      time.Minute,
		},
	}

	// Register connection
	ws.connMutex.Lock()
	ws.connections[connID] = wsConn
	ws.connMutex.Unlock()

	// Start goroutines for reading and writing
	go wsConn.readPump()
	go wsConn.writePump()
}

// readPump handles reading messages from the WebSocket
func (c *WebSocketConnection) readPump() {
	defer func() {
		c.Hub.unregisterConnection(c.ID)
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(c.Hub.config.MaxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(c.Hub.config.PongTimeout))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(c.Hub.config.PongTimeout))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				fmt.Printf("WebSocket error: %v\n", err)
			}
			break
		}

		// Rate limiting
		if !c.RateLimit.Allow() {
			continue
		}

		c.LastSeen = time.Now()
		c.handleMessage(message)
	}
}

// writePump handles writing messages to the WebSocket
func (c *WebSocketConnection) writePump() {
	ticker := time.NewTicker(c.Hub.config.PingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(c.Hub.config.WriteTimeout))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued messages
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(c.Hub.config.WriteTimeout))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// WebSocketMessage represents a WebSocket message
type WebSocketMessage struct {
	Type     string                 `json:"type"`
	ID       string                 `json:"id,omitempty"`
	Data     map[string]interface{} `json:"data,omitempty"`
	Metadata map[string]string      `json:"metadata,omitempty"`
}

// handleMessage processes incoming WebSocket messages
func (c *WebSocketConnection) handleMessage(message []byte) {
	ctx, span := c.Hub.tracer.Start(context.Background(), "handle_websocket_message")
	defer span.End()

	var msg WebSocketMessage
	if err := json.Unmarshal(message, &msg); err != nil {
		span.RecordError(err)
		c.sendError("Invalid message format")
		return
	}

	span.SetAttributes(
		attribute.String("message.type", msg.Type),
		attribute.String("message.id", msg.ID),
	)

	switch msg.Type {
	case "file_upload_start":
		c.handleFileUploadStart(ctx, &msg)
	case "file_chunk":
		c.handleFileChunk(ctx, &msg)
	case "file_upload_complete":
		c.handleFileUploadComplete(ctx, &msg)
	case "processing_status_request":
		c.handleProcessingStatusRequest(ctx, &msg)
	case "ping":
		c.sendMessage(&WebSocketMessage{Type: "pong", ID: msg.ID})
	default:
		c.sendError(fmt.Sprintf("Unknown message type: %s", msg.Type))
	}
}

// handleFileUploadStart initiates a file upload session
func (c *WebSocketConnection) handleFileUploadStart(ctx context.Context, msg *WebSocketMessage) {
	fileName, _ := msg.Data["file_name"].(string)
	fileSize, _ := msg.Data["file_size"].(float64)
	contentType, _ := msg.Data["content_type"].(string)
	chunkSize, _ := msg.Data["chunk_size"].(float64)

	if fileName == "" || fileSize <= 0 {
		c.sendError("Invalid file upload parameters")
		return
	}

	// Validate file size
	if int64(fileSize) > c.Hub.config.MaxFileSize {
		c.sendError("File too large")
		return
	}

	// Validate content type
	if !c.isAllowedContentType(contentType) {
		c.sendError("File type not allowed")
		return
	}

	// Create upload state
	fileID := uuid.New().String()
	chunksTotal := int((int64(fileSize) + int64(chunkSize) - 1) / int64(chunkSize))

	c.UploadState = &UploadState{
		FileID:       fileID,
		FileName:     fileName,
		FileSize:     int64(fileSize),
		ContentType:  contentType,
		ChunksTotal:  chunksTotal,
		ChunkData:    make(map[int][]byte),
		StartedAt:    time.Now(),
		Metadata:     msg.Metadata,
	}

	// Send confirmation
	c.sendMessage(&WebSocketMessage{
		Type: "file_upload_started",
		ID:   msg.ID,
		Data: map[string]interface{}{
			"file_id":      fileID,
			"chunks_total": chunksTotal,
		},
	})
}

// handleFileChunk processes a file chunk
func (c *WebSocketConnection) handleFileChunk(ctx context.Context, msg *WebSocketMessage) {
	if c.UploadState == nil {
		c.sendError("No active upload session")
		return
	}

	chunkIndex, _ := msg.Data["chunk_index"].(float64)
	chunkData, _ := msg.Data["chunk_data"].(string)

	// Decode base64 chunk data
	// In production, you might use binary frames instead
	// decodedData, err := base64.StdEncoding.DecodeString(chunkData)
	// For simplicity, assume chunkData is already decoded
	decodedData := []byte(chunkData)

	c.UploadState.ChunkData[int(chunkIndex)] = decodedData
	c.UploadState.ChunksReceived++

	// Send progress update
	progress := float64(c.UploadState.ChunksReceived) / float64(c.UploadState.ChunksTotal) * 100
	c.sendMessage(&WebSocketMessage{
		Type: "upload_progress",
		Data: map[string]interface{}{
			"file_id":         c.UploadState.FileID,
			"chunks_received": c.UploadState.ChunksReceived,
			"chunks_total":    c.UploadState.ChunksTotal,
			"progress":        progress,
		},
	})
}

// handleFileUploadComplete completes the file upload
func (c *WebSocketConnection) handleFileUploadComplete(ctx context.Context, msg *WebSocketMessage) {
	if c.UploadState == nil {
		c.sendError("No active upload session")
		return
	}

	// Verify all chunks received
	if c.UploadState.ChunksReceived != c.UploadState.ChunksTotal {
		c.sendError("Not all chunks received")
		return
	}

	// Reconstruct file from chunks
	fileData := make([]byte, 0, c.UploadState.FileSize)
	for i := 0; i < c.UploadState.ChunksTotal; i++ {
		if chunk, exists := c.UploadState.ChunkData[i]; exists {
			fileData = append(fileData, chunk...)
		} else {
			c.sendError(fmt.Sprintf("Missing chunk %d", i))
			return
		}
	}

	// TODO: Save file to storage and emit file discovered event
	fileURL := fmt.Sprintf("temp://uploads/%s", c.UploadState.FileID)

	// Emit file discovered event
	discoveryEvent := events.NewFileDiscoveredEvent("websocket-upload", c.TenantID, events.FileDiscoveredData{
		URL:          fileURL,
		SourceID:     "websocket",
		DiscoveredAt: time.Now(),
		Size:         c.UploadState.FileSize,
		ContentType:  c.UploadState.ContentType,
		Metadata:     c.UploadState.Metadata,
		Priority:     "high", // Real-time uploads get high priority
	})

	if err := c.Hub.producer.PublishEvent(ctx, discoveryEvent); err != nil {
		c.sendError("Failed to initiate processing")
		return
	}

	// Send completion confirmation
	c.sendMessage(&WebSocketMessage{
		Type: "file_upload_complete",
		ID:   msg.ID,
		Data: map[string]interface{}{
			"file_id":      c.UploadState.FileID,
			"file_url":     fileURL,
			"processing":   true,
		},
	})

	// Clear upload state
	c.UploadState = nil
}

// handleProcessingStatusRequest handles requests for processing status
func (c *WebSocketConnection) handleProcessingStatusRequest(ctx context.Context, msg *WebSocketMessage) {
	fileID, _ := msg.Data["file_id"].(string)
	
	// TODO: Query actual processing status
	// For now, return a mock status
	c.sendMessage(&WebSocketMessage{
		Type: "processing_status",
		ID:   msg.ID,
		Data: map[string]interface{}{
			"file_id":            fileID,
			"status":            "processing",
			"progress":          75.0,
			"chunks_created":    10,
			"embeddings_created": 8,
			"estimated_completion": time.Now().Add(2 * time.Minute),
		},
	})
}

// sendMessage sends a message to the WebSocket client
func (c *WebSocketConnection) sendMessage(msg *WebSocketMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	select {
	case c.Send <- data:
	default:
		close(c.Send)
	}
}

// sendError sends an error message to the WebSocket client
func (c *WebSocketConnection) sendError(errorMsg string) {
	c.sendMessage(&WebSocketMessage{
		Type: "error",
		Data: map[string]interface{}{
			"message": errorMsg,
		},
	})
}

// isAllowedContentType checks if a content type is allowed
func (c *WebSocketConnection) isAllowedContentType(contentType string) bool {
	for _, allowed := range c.Hub.config.AllowedMimeTypes {
		if contentType == allowed {
			return true
		}
	}
	return false
}

// Allow implements rate limiting
func (rl *RateLimiter) Allow() bool {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()
	// Remove old messages outside the window
	cutoff := now.Add(-rl.window)
	for i := 0; i < len(rl.messages); i++ {
		if rl.messages[i].After(cutoff) {
			rl.messages = rl.messages[i:]
			break
		}
	}

	if len(rl.messages) >= rl.maxMessages {
		return false
	}

	rl.messages = append(rl.messages, now)
	return true
}

// unregisterConnection removes a connection from the registry
func (ws *WebSocketStreamingService) unregisterConnection(connID string) {
	ws.connMutex.Lock()
	defer ws.connMutex.Unlock()

	if conn, exists := ws.connections[connID]; exists {
		close(conn.Send)
		delete(ws.connections, connID)
	}
}

// BroadcastProcessingUpdate broadcasts a processing update to all relevant connections
func (ws *WebSocketStreamingService) BroadcastProcessingUpdate(tenantID, fileID string, update map[string]interface{}) {
	message := &WebSocketMessage{
		Type: "processing_update",
		Data: update,
	}

	data, err := json.Marshal(message)
	if err != nil {
		return
	}

	ws.connMutex.RLock()
	defer ws.connMutex.RUnlock()

	for _, conn := range ws.connections {
		if conn.TenantID == tenantID {
			select {
			case conn.Send <- data:
			default:
				// Connection is slow or closed, skip
			}
		}
	}
}

// GetMetrics returns WebSocket service metrics
func (ws *WebSocketStreamingService) GetMetrics() map[string]interface{} {
	ws.connMutex.RLock()
	defer ws.connMutex.RUnlock()

	activeUploads := 0
	for _, conn := range ws.connections {
		if conn.UploadState != nil {
			activeUploads++
		}
	}

	return map[string]interface{}{
		"active_connections": len(ws.connections),
		"active_uploads":     activeUploads,
		"max_connections":    ws.config.MaxConnections,
	}
}

// HealthCheck performs a health check on the WebSocket service
func (ws *WebSocketStreamingService) HealthCheck(ctx context.Context) error {
	// Check producer health
	if err := ws.producer.HealthCheck(ctx); err != nil {
		return fmt.Errorf("producer health check failed: %w", err)
	}

	// Check if we're within connection limits
	ws.connMutex.RLock()
	connectionCount := len(ws.connections)
	ws.connMutex.RUnlock()

	if connectionCount > ws.config.MaxConnections {
		return fmt.Errorf("too many active connections: %d", connectionCount)
	}

	return nil
}