package logger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// LogLevel represents the severity of a log entry
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// LogEntry represents a structured log entry
type LogEntry struct {
	ID             string                 `json:"id"`
	Timestamp      time.Time              `json:"timestamp"`
	Level          string                 `json:"level"`
	Method         string                 `json:"method,omitempty"`
	Path           string                 `json:"path,omitempty"`
	Query          string                 `json:"query,omitempty"`
	StatusCode     int                    `json:"statusCode,omitempty"`
	Duration       time.Duration          `json:"duration,omitempty"`
	ClientIP       string                 `json:"clientIP,omitempty"`
	UserAgent      string                 `json:"userAgent,omitempty"`
	RequestHeaders map[string]string      `json:"requestHeaders,omitempty"`
	RequestBody    string                 `json:"requestBody,omitempty"`
	ResponseBody   string                 `json:"responseBody,omitempty"`
	Error          string                 `json:"error,omitempty"`
	Message        string                 `json:"message,omitempty"`
	Context        map[string]interface{} `json:"context,omitempty"`
	Bucket         string                 `json:"bucket,omitempty"`
	Key            string                 `json:"key,omitempty"`
	Operation      string                 `json:"operation,omitempty"`
}

// Logger is the main logger struct
type Logger struct {
	mu            sync.RWMutex
	entries       []LogEntry
	maxEntries    int
	currentLevel  LogLevel
	logFile       *os.File
	logDir        string
	rotationSize  int64 // Size in bytes for rotation
	currentSize   int64
	enableConsole bool
	enableFile    bool
}

var (
	instance *Logger
	once     sync.Once
)

// GetInstance returns the singleton logger instance
func GetInstance() *Logger {
	once.Do(func() {
		instance = &Logger{
			entries:       make([]LogEntry, 0),
			maxEntries:    10000,
			currentLevel:  INFO,
			rotationSize:  100 * 1024 * 1024, // 100MB
			enableConsole: true,
			enableFile:    false, // Default to false, will be set by server initialization
			logDir:        "",    // Default to empty, will be set by server initialization
		}
		// Don't initialize log file here - let the server configuration decide
	})
	return instance
}

// initLogFile initializes the log file
func (l *Logger) initLogFile() {
	if !l.enableFile {
		return
	}

	// Create logs directory if it doesn't exist
	if err := os.MkdirAll(l.logDir, 0755); err != nil {
		fmt.Printf("Failed to create log directory: %v\n", err)
		return
	}

	filename := filepath.Join(l.logDir, fmt.Sprintf("s3pit_%s.log", time.Now().Format("2006-01-02")))
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Failed to open log file: %v\n", err)
		return
	}

	l.logFile = file

	// Get current file size
	info, err := file.Stat()
	if err == nil {
		l.currentSize = info.Size()
	}
}

// rotateLogFile rotates the log file if it exceeds the size limit
func (l *Logger) rotateLogFile() {
	if l.logFile == nil || !l.enableFile {
		return
	}

	l.logFile.Close()

	// Rename current file with timestamp
	oldPath := l.logFile.Name()
	newPath := fmt.Sprintf("%s.%s", oldPath, time.Now().Format("20060102_150405"))
	_ = os.Rename(oldPath, newPath)

	// Create new log file
	l.initLogFile()

	// Clean up old log files (keep last 10)
	l.cleanupOldLogs()
}

// cleanupOldLogs removes old log files, keeping only the most recent ones
func (l *Logger) cleanupOldLogs() {
	files, err := filepath.Glob(filepath.Join(l.logDir, "s3pit_*.log.*"))
	if err != nil {
		return
	}

	if len(files) > 10 {
		// Sort files by modification time and remove oldest
		for i := 0; i < len(files)-10; i++ {
			os.Remove(files[i])
		}
	}
}

// SetLevel sets the minimum log level
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.currentLevel = level
}

// SetMaxEntries sets the maximum number of entries to keep in memory
func (l *Logger) SetMaxEntries(max int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.maxEntries = max
}

// SetLogDir sets the directory for log files
func (l *Logger) SetLogDir(dir string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logDir = dir
	if l.logFile != nil {
		l.logFile.Close()
	}
	l.initLogFile()
}

// EnableFileLogging enables or disables file logging
func (l *Logger) EnableFileLogging(enable bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.enableFile = enable
	if enable && l.logFile == nil {
		l.initLogFile()
	} else if !enable && l.logFile != nil {
		l.logFile.Close()
		l.logFile = nil
	}
}

// EnableConsoleLogging enables or disables console logging
func (l *Logger) EnableConsoleLogging(enable bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.enableConsole = enable
}

// generateID generates a unique ID for a log entry
func generateID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid())
}

// Log adds a new log entry
func (l *Logger) Log(level LogLevel, message string, context map[string]interface{}) {
	if level < l.currentLevel {
		return
	}

	entry := LogEntry{
		ID:        generateID(),
		Timestamp: time.Now(),
		Level:     level.String(),
		Message:   message,
		Context:   context,
	}

	l.addEntry(entry)
}

// LogRequest logs an HTTP request with detailed information
func (l *Logger) LogRequest(c *gin.Context, start time.Time, responseBody []byte) {
	duration := time.Since(start)

	// Extract request headers
	headers := make(map[string]string)
	for key, values := range c.Request.Header {
		if len(values) > 0 && !isSensitiveHeader(key) {
			headers[key] = values[0]
		}
	}

	// Extract request body (if not too large)
	var requestBody string
	if c.Request.Body != nil && c.Request.ContentLength > 0 && c.Request.ContentLength < 10*1024 { // 10KB limit
		bodyBytes, _ := io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		if len(bodyBytes) > 0 {
			requestBody = string(bodyBytes)
		}
	}

	// Extract response body (if not too large and not binary)
	var respBody string
	if len(responseBody) > 0 && len(responseBody) < 10*1024 && isTextContent(c.Writer.Header().Get("Content-Type")) {
		respBody = string(responseBody)
	}

	// Determine operation type
	operation := determineOperation(c)

	entry := LogEntry{
		ID:             generateID(),
		Timestamp:      start,
		Level:          INFO.String(),
		Method:         c.Request.Method,
		Path:           c.Request.URL.Path,
		Query:          c.Request.URL.RawQuery,
		StatusCode:     c.Writer.Status(),
		Duration:       duration,
		ClientIP:       c.ClientIP(),
		UserAgent:      c.Request.UserAgent(),
		RequestHeaders: headers,
		RequestBody:    requestBody,
		ResponseBody:   respBody,
		Bucket:         c.Param("bucket"),
		Key:            c.Param("key"),
		Operation:      operation,
	}

	// Add error if present
	if len(c.Errors) > 0 {
		entry.Level = ERROR.String()
		errMsgs, _ := json.Marshal(c.Errors)
		entry.Error = string(errMsgs)
	} else if c.Writer.Status() >= 400 {
		entry.Level = WARN.String()
	}

	l.addEntry(entry)
}

// addEntry adds an entry to the logger
func (l *Logger) addEntry(entry LogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Add to memory buffer
	if l.maxEntries > 0 && len(l.entries) >= l.maxEntries {
		l.entries = l.entries[1:]
	}
	l.entries = append(l.entries, entry)

	// Write to console
	if l.enableConsole {
		l.writeToConsole(entry)
	}

	// Write to file
	if l.enableFile && l.logFile != nil {
		l.writeToFile(entry)
	}
}

// writeToConsole writes a log entry to the console
func (l *Logger) writeToConsole(entry LogEntry) {
	var color string
	switch entry.Level {
	case "DEBUG":
		color = "\033[36m" // Cyan
	case "INFO":
		color = "\033[32m" // Green
	case "WARN":
		color = "\033[33m" // Yellow
	case "ERROR":
		color = "\033[31m" // Red
	default:
		color = "\033[0m" // Reset
	}

	if entry.Method != "" {
		// HTTP request log
		fmt.Printf("%s[%s]%s %s %s %s %d %v %s\n",
			color,
			entry.Level,
			"\033[0m",
			entry.Timestamp.Format("2006-01-02 15:04:05"),
			entry.Method,
			entry.Path,
			entry.StatusCode,
			entry.Duration,
			entry.ClientIP,
		)
	} else {
		// General log
		fmt.Printf("%s[%s]%s %s %s\n",
			color,
			entry.Level,
			"\033[0m",
			entry.Timestamp.Format("2006-01-02 15:04:05"),
			entry.Message,
		)
	}
}

// writeToFile writes a log entry to the file
func (l *Logger) writeToFile(entry LogEntry) {
	if l.logFile == nil {
		return
	}

	jsonData, err := json.Marshal(entry)
	if err != nil {
		return
	}

	jsonData = append(jsonData, '\n')
	n, err := l.logFile.Write(jsonData)
	if err != nil {
		fmt.Printf("Failed to write to log file: %v\n", err)
		return
	}

	l.currentSize += int64(n)

	// Check if rotation is needed
	if l.currentSize >= l.rotationSize {
		l.rotateLogFile()
	}
}

// GetEntries returns log entries with optional filtering
func (l *Logger) GetEntries(limit int, level string, operation string, startTime, endTime *time.Time) []LogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var filtered []LogEntry
	for _, entry := range l.entries {
		// Filter by level
		if level != "" && entry.Level != level {
			continue
		}

		// Filter by operation
		if operation != "" && entry.Operation != operation {
			continue
		}

		// Filter by time range
		if startTime != nil && entry.Timestamp.Before(*startTime) {
			continue
		}
		if endTime != nil && entry.Timestamp.After(*endTime) {
			continue
		}

		filtered = append(filtered, entry)
	}

	// Apply limit
	if limit > 0 && len(filtered) > limit {
		return filtered[len(filtered)-limit:]
	}

	return filtered
}

// Clear clears all log entries from memory
func (l *Logger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = make([]LogEntry, 0)
}

// Debug logs a debug message
func Debug(message string, context ...map[string]interface{}) {
	ctx := make(map[string]interface{})
	if len(context) > 0 {
		ctx = context[0]
	}
	GetInstance().Log(DEBUG, message, ctx)
}

// Info logs an info message
func Info(message string, context ...map[string]interface{}) {
	ctx := make(map[string]interface{})
	if len(context) > 0 {
		ctx = context[0]
	}
	GetInstance().Log(INFO, message, ctx)
}

// Warn logs a warning message
func Warn(message string, context ...map[string]interface{}) {
	ctx := make(map[string]interface{})
	if len(context) > 0 {
		ctx = context[0]
	}
	GetInstance().Log(WARN, message, ctx)
}

// Error logs an error message
func Error(message string, context ...map[string]interface{}) {
	ctx := make(map[string]interface{})
	if len(context) > 0 {
		ctx = context[0]
	}
	GetInstance().Log(ERROR, message, ctx)
}

// Helper functions

func isSensitiveHeader(key string) bool {
	sensitive := []string{"Authorization", "X-Amz-Security-Token", "Cookie", "Set-Cookie"}
	for _, s := range sensitive {
		if key == s {
			return true
		}
	}
	return false
}

func isTextContent(contentType string) bool {
	textTypes := []string{"text/", "application/json", "application/xml"}
	for _, t := range textTypes {
		if len(contentType) >= len(t) && contentType[:len(t)] == t {
			return true
		}
	}
	return false
}

func determineOperation(c *gin.Context) string {
	method := c.Request.Method
	path := c.Request.URL.Path
	query := c.Request.URL.Query()

	// Check for specific S3 operations
	if method == "GET" && path == "/" {
		return "ListBuckets"
	}
	if query.Get("uploads") != "" {
		if method == "POST" {
			return "InitiateMultipartUpload"
		}
	}
	if query.Get("uploadId") != "" {
		if method == "POST" {
			return "CompleteMultipartUpload"
		}
		if method == "DELETE" {
			return "AbortMultipartUpload"
		}
		if method == "PUT" {
			return "UploadPart"
		}
	}
	if c.GetHeader("x-amz-copy-source") != "" {
		return "CopyObject"
	}
	if query.Get("cors") != "" {
		return method + "BucketCors"
	}
	if query.Get("policy") != "" {
		return method + "BucketPolicy"
	}

	// Standard operations
	bucket := c.Param("bucket")
	key := c.Param("key")

	if bucket != "" && key != "" {
		switch method {
		case "GET":
			return "GetObject"
		case "PUT":
			return "PutObject"
		case "DELETE":
			return "DeleteObject"
		case "HEAD":
			return "HeadObject"
		}
	} else if bucket != "" {
		switch method {
		case "GET":
			return "ListObjects"
		case "PUT":
			return "CreateBucket"
		case "DELETE":
			return "DeleteBucket"
		case "HEAD":
			return "HeadBucket"
		}
	}

	return method
}
