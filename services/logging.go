package services

import (
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

const maxLogEntries = 1000

type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Component string    `json:"component,omitempty"`
}

type LogService struct {
	mu      sync.RWMutex
	entries []LogEntry
	Logger  zerolog.Logger
}

func NewLogService() *LogService {
	ls := &LogService{
		entries: make([]LogEntry, 0, maxLogEntries),
	}

	// Create console writer
	consoleWriter := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "15:04:05"}

	// Create logger with console output
	ls.Logger = zerolog.New(consoleWriter).With().Timestamp().Logger()

	return ls
}

// Log logs a message and stores it in the buffer
func (ls *LogService) Log(level, component, message string) {
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
		Component: component,
	}

	ls.mu.Lock()
	if len(ls.entries) >= maxLogEntries {
		ls.entries = ls.entries[1:]
	}
	ls.entries = append(ls.entries, entry)
	ls.mu.Unlock()

	// Also log to console
	switch level {
	case "error":
		ls.Logger.Error().Str("component", component).Msg(message)
	case "warn":
		ls.Logger.Warn().Str("component", component).Msg(message)
	case "debug":
		ls.Logger.Debug().Str("component", component).Msg(message)
	default:
		ls.Logger.Info().Str("component", component).Msg(message)
	}
}

func (ls *LogService) Info(component, message string) {
	ls.Log("info", component, message)
}

func (ls *LogService) Error(component, message string) {
	ls.Log("error", component, message)
}

func (ls *LogService) Warn(component, message string) {
	ls.Log("warn", component, message)
}

func (ls *LogService) Debug(component, message string) {
	ls.Log("debug", component, message)
}

func (ls *LogService) GetEntries(level string, search string, limit int) []LogEntry {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	if limit <= 0 || limit > len(ls.entries) {
		limit = len(ls.entries)
	}

	var filtered []LogEntry
	for i := len(ls.entries) - 1; i >= 0 && len(filtered) < limit; i-- {
		entry := ls.entries[i]

		// Filter by level
		if level != "" && level != "all" && entry.Level != level {
			continue
		}

		// Filter by search term
		if search != "" {
			if !containsIgnoreCase(entry.Message, search) &&
				!containsIgnoreCase(entry.Component, search) {
				continue
			}
		}

		filtered = append(filtered, entry)
	}

	return filtered
}

func (ls *LogService) Clear() {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.entries = make([]LogEntry, 0, maxLogEntries)
}

func (ls *LogService) Count() int {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return len(ls.entries)
}

func containsIgnoreCase(s, substr string) bool {
	if s == "" || substr == "" {
		return false
	}
	sLower := toLower(s)
	substrLower := toLower(substr)
	return contains(sLower, substrLower)
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func contains(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
