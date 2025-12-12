package services

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
	"syncbot/config"
)

// ActivationLogInfo represents metadata about an activation log file
type ActivationLogInfo struct {
	Filename     string    `json:"filename"`
	EndpointName string    `json:"endpoint_name"`
	StartTime    time.Time `json:"start_time"`
	Size         int64     `json:"size"`
	Path         string    `json:"-"` // Internal use only
}

// ActivationLogService manages persistent activation log files
type ActivationLogService struct {
	mu          sync.RWMutex
	cfg         *config.Config
	logSvc      *LogService
	logsDir     string
	currentLog  *lumberjack.Logger
	currentName string // Current log filename for active process
}

// NewActivationLogService creates a new activation log service
func NewActivationLogService(cfg *config.Config, logSvc *LogService) (*ActivationLogService, error) {
	logsDir := filepath.Join(cfg.EndpointsPath, ".syncbot-logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	logSvc.Info("activation-logs", fmt.Sprintf("Activation logs directory: %s", logsDir))

	return &ActivationLogService{
		cfg:     cfg,
		logSvc:  logSvc,
		logsDir: logsDir,
	}, nil
}

// StartLog creates a new log file for an activation
func (s *ActivationLogService) StartLog(endpointName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Close any existing log
	if s.currentLog != nil {
		s.currentLog.Close()
	}

	// Generate filename: {endpoint}_{timestamp}.log
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("%s_%s.log", endpointName, timestamp)
	logPath := filepath.Join(s.logsDir, filename)

	s.currentLog = &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    10,   // 10 MB max size per file
		MaxBackups: 5,    // Keep 5 rotated files
		MaxAge:     30,   // Keep logs for 30 days
		Compress:   true, // Compress rotated files
	}
	s.currentName = filename

	s.logSvc.Info("activation-logs", fmt.Sprintf("Started log file: %s", filename))
	return nil
}

// Write writes data to the current log file
func (s *ActivationLogService) Write(data []byte) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.currentLog == nil {
		return 0, nil // No active log, silently ignore
	}
	return s.currentLog.Write(data)
}

// CloseLog closes the current log file
func (s *ActivationLogService) CloseLog() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentLog != nil {
		s.currentLog.Close()
		s.currentLog = nil
		s.logSvc.Info("activation-logs", fmt.Sprintf("Closed log file: %s", s.currentName))
		s.currentName = ""
	}
}

// GetCurrentLogName returns the current active log filename
func (s *ActivationLogService) GetCurrentLogName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentName
}

// ListLogs returns all activation logs sorted by time (newest first)
func (s *ActivationLogService) ListLogs() ([]ActivationLogInfo, error) {
	entries, err := os.ReadDir(s.logsDir)
	if err != nil {
		return nil, err
	}

	var logs []ActivationLogInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// Accept .log and .log.gz (compressed rotated files)
		name := entry.Name()
		if !strings.HasSuffix(name, ".log") && !strings.HasSuffix(name, ".log.gz") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Parse filename: {endpoint}_{timestamp}.log or {endpoint}_{timestamp}.log.gz
		baseName := strings.TrimSuffix(strings.TrimSuffix(name, ".gz"), ".log")
		parts := strings.Split(baseName, "_")

		endpointName := ""
		var startTime time.Time
		if len(parts) >= 2 {
			// Last part is timestamp, everything before is endpoint name
			endpointName = strings.Join(parts[:len(parts)-1], "_")
			startTime, _ = time.Parse("20060102-150405", parts[len(parts)-1])
		}

		logs = append(logs, ActivationLogInfo{
			Filename:     name,
			EndpointName: endpointName,
			StartTime:    startTime,
			Size:         info.Size(),
			Path:         filepath.Join(s.logsDir, name),
		})
	}

	// Sort by start time descending (newest first)
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].StartTime.After(logs[j].StartTime)
	})

	return logs, nil
}

// GetLogPath returns the full path to a log file (with security validation)
func (s *ActivationLogService) GetLogPath(filename string) (string, error) {
	// Security: validate filename to prevent path traversal
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") ||
		strings.Contains(filename, "..") {
		return "", fmt.Errorf("invalid filename")
	}
	if !strings.HasSuffix(filename, ".log") && !strings.HasSuffix(filename, ".log.gz") {
		return "", fmt.Errorf("invalid file extension")
	}

	logPath := filepath.Join(s.logsDir, filename)
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return "", fmt.Errorf("log not found")
	}

	return logPath, nil
}

// ReadLogTail reads the last N bytes of a log file (for reconnect/page load)
func (s *ActivationLogService) ReadLogTail(filename string, maxBytes int64) ([]byte, error) {
	logPath, err := s.GetLogPath(filename)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(logPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	size := info.Size()
	if size <= maxBytes {
		return io.ReadAll(file)
	}

	// Seek to read last maxBytes
	_, err = file.Seek(-maxBytes, io.SeekEnd)
	if err != nil {
		return nil, err
	}

	return io.ReadAll(file)
}

// ReadFullLog reads the entire log file
func (s *ActivationLogService) ReadFullLog(filename string) ([]byte, error) {
	logPath, err := s.GetLogPath(filename)
	if err != nil {
		return nil, err
	}

	return os.ReadFile(logPath)
}

// DeleteLog deletes a log file
func (s *ActivationLogService) DeleteLog(filename string) error {
	logPath, err := s.GetLogPath(filename)
	if err != nil {
		return err
	}

	if err := os.Remove(logPath); err != nil {
		return fmt.Errorf("failed to delete log: %w", err)
	}

	s.logSvc.Info("activation-logs", fmt.Sprintf("Deleted log: %s", filename))
	return nil
}

// GetLogsDir returns the logs directory path
func (s *ActivationLogService) GetLogsDir() string {
	return s.logsDir
}
