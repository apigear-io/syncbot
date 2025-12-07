package services

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"

	"syncbot/config"
)

var (
	ErrTerminalSessionNotFound = errors.New("terminal session not found")
)

// RingBuffer is a simple circular buffer for storing recent terminal output
type RingBuffer struct {
	mu   sync.Mutex
	data []byte
	size int
}

// NewRingBuffer creates a new ring buffer with the given size
func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		data: make([]byte, 0, size),
		size: size,
	}
}

// Write appends data to the buffer, discarding old data if necessary
func (r *RingBuffer) Write(p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data = append(r.data, p...)
	if len(r.data) > r.size {
		r.data = r.data[len(r.data)-r.size:]
	}
}

// Bytes returns a copy of the buffered data
func (r *RingBuffer) Bytes() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := make([]byte, len(r.data))
	copy(result, r.data)
	return result
}

// TerminalSession represents an active PTY session
type TerminalSession struct {
	ID           string
	Username     string
	PTY          *os.File
	Cmd          *exec.Cmd
	Cols         uint16
	Rows         uint16
	CreatedAt    time.Time
	LastActive   time.Time
	Done         chan struct{}
	OutputBuffer *RingBuffer // Buffer recent output for reconnection
	Connected    bool        // Whether a WebSocket is currently connected
}

// TerminalService manages PTY sessions
type TerminalService struct {
	mu       sync.RWMutex
	sessions map[string]*TerminalSession
	cfg      *config.Config
	authSvc  *AuthService
	logSvc   *LogService

	idleTimeout time.Duration
}

// NewTerminalService creates a new terminal service
func NewTerminalService(cfg *config.Config, authSvc *AuthService, logSvc *LogService) *TerminalService {
	svc := &TerminalService{
		sessions:    make(map[string]*TerminalSession),
		cfg:         cfg,
		authSvc:     authSvc,
		logSvc:      logSvc,
		idleTimeout: 15 * time.Minute,
	}

	// Start idle session checker
	go svc.checkIdleSessions()

	return svc
}

// CreateSession creates a new PTY session
func (t *TerminalService) CreateSession(sessionID, username string, cols, rows uint16) (*TerminalSession, error) {
	shell := t.cfg.Terminal.Shell
	if shell == "" {
		shell = "/bin/bash"
	}

	// Create command with color support
	// Use -l for login shell to source profiles, and set up colors
	cmd := exec.Command(shell, "-l")
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"SYNCBOT_USER="+username,
		"CLICOLOR=1",              // macOS: enable colors
		"CLICOLOR_FORCE=1",        // force colors even if not tty detected
		"LSCOLORS=GxFxCxDxBxegedabagaced", // macOS ls colors
		"LS_COLORS=di=1;36:ln=1;35:so=1;32:pi=1;33:ex=1;31:bd=34;46:cd=34;43:su=30;41:sg=30;46:tw=30;42:ow=34;43", // GNU ls colors
	)

	// Start with PTY
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: cols,
		Rows: rows,
	})
	if err != nil {
		return nil, err
	}

	now := time.Now()
	session := &TerminalSession{
		ID:           sessionID,
		Username:     username,
		PTY:          ptmx,
		Cmd:          cmd,
		Cols:         cols,
		Rows:         rows,
		CreatedAt:    now,
		LastActive:   now,
		Done:         make(chan struct{}),
		OutputBuffer: NewRingBuffer(16 * 1024), // 16KB buffer
		Connected:    true,
	}

	t.mu.Lock()
	t.sessions[sessionID] = session
	t.mu.Unlock()

	t.logSvc.Info("terminal", "Terminal session started for user: "+username)

	// Wait for command to exit in background
	go func() {
		cmd.Wait()
		close(session.Done)
		t.CloseSession(sessionID)
	}()

	return session, nil
}

// GetSession retrieves a terminal session by ID
func (t *TerminalService) GetSession(sessionID string) (*TerminalSession, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	session, ok := t.sessions[sessionID]
	if !ok {
		return nil, ErrTerminalSessionNotFound
	}

	return session, nil
}

// GetOrCreateSession returns an existing session or creates a new one
func (t *TerminalService) GetOrCreateSession(sessionID, username string, cols, rows uint16) (*TerminalSession, bool, error) {
	t.mu.RLock()
	session, exists := t.sessions[sessionID]
	t.mu.RUnlock()

	if exists {
		// Check if the process is still alive
		select {
		case <-session.Done:
			// Process exited, need new session
			t.CloseSession(sessionID)
		default:
			// Process still alive, reconnect
			session.Connected = true
			session.LastActive = time.Now()
			return session, true, nil // true = reconnected
		}
	}

	// Create new session
	newSession, err := t.CreateSession(sessionID, username, cols, rows)
	if err != nil {
		return nil, false, err
	}
	return newSession, false, nil // false = new session
}

// DisconnectSession marks a session as disconnected but keeps it alive
func (t *TerminalService) DisconnectSession(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if session, ok := t.sessions[sessionID]; ok {
		session.Connected = false
		session.LastActive = time.Now()
		t.logSvc.Debug("terminal", "WebSocket disconnected, session kept alive: "+sessionID[:8]+"...")
	}
}

// UpdateActivity updates the last active time for a session
func (t *TerminalService) UpdateActivity(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if session, ok := t.sessions[sessionID]; ok {
		session.LastActive = time.Now()
	}
}

// ResizeSession resizes the PTY window
func (t *TerminalService) ResizeSession(sessionID string, cols, rows uint16) error {
	t.mu.Lock()
	session, ok := t.sessions[sessionID]
	t.mu.Unlock()

	if !ok {
		return ErrTerminalSessionNotFound
	}

	session.Cols = cols
	session.Rows = rows

	return pty.Setsize(session.PTY, &pty.Winsize{
		Cols: cols,
		Rows: rows,
	})
}

// CloseSession closes a terminal session and cleans up resources
func (t *TerminalService) CloseSession(sessionID string) {
	t.mu.Lock()
	session, ok := t.sessions[sessionID]
	if !ok {
		t.mu.Unlock()
		return
	}
	delete(t.sessions, sessionID)
	t.mu.Unlock()

	t.logSvc.Info("terminal", "Closing terminal session for user: "+session.Username)

	// Close PTY
	if session.PTY != nil {
		session.PTY.Close()
	}

	// Gracefully terminate the process
	if session.Cmd != nil && session.Cmd.Process != nil {
		// Try SIGTERM first
		session.Cmd.Process.Signal(syscall.SIGTERM)

		// Wait for process to exit with timeout
		done := make(chan error, 1)
		go func() {
			done <- session.Cmd.Wait()
		}()

		select {
		case <-done:
			// Process exited
		case <-time.After(5 * time.Second):
			// Force kill if still running
			session.Cmd.Process.Kill()
		}
	}
}

// checkIdleSessions periodically checks for idle sessions and closes them
func (t *TerminalService) checkIdleSessions() {
	ticker := time.NewTicker(1 * time.Minute)
	for range ticker.C {
		t.mu.Lock()
		now := time.Now()
		for id, session := range t.sessions {
			if now.Sub(session.LastActive) > t.idleTimeout {
				t.logSvc.Info("terminal", "Session timed out due to inactivity: "+session.Username)
				// Close in a goroutine to avoid holding the lock
				go t.CloseSession(id)
			}
		}
		t.mu.Unlock()
	}
}

// CloseAllSessions closes all active terminal sessions (for shutdown)
func (t *TerminalService) CloseAllSessions() {
	t.mu.Lock()
	sessionIDs := make([]string, 0, len(t.sessions))
	for id := range t.sessions {
		sessionIDs = append(sessionIDs, id)
	}
	t.mu.Unlock()

	for _, id := range sessionIDs {
		t.CloseSession(id)
	}
}
