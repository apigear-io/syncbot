package services

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"syncbot/config"
)

type ProcessState string

const (
	ProcessStateIdle    ProcessState = "idle"
	ProcessStateRunning ProcessState = "running"
	ProcessStateStopped ProcessState = "stopped"
	ProcessStateKilled  ProcessState = "killed"
	ProcessStateFailed  ProcessState = "failed"
)

type ProcessInfo struct {
	Command      string       `json:"command"`
	State        ProcessState `json:"state"`
	StartTime    *time.Time   `json:"start_time,omitempty"`
	EndTime      *time.Time   `json:"end_time,omitempty"`
	ExitCode     *int         `json:"exit_code,omitempty"`
	Error        string       `json:"error,omitempty"`
	EndpointName string       `json:"endpoint_name,omitempty"`
}

type ProcessService struct {
	mu               sync.RWMutex
	cfg              *config.Config
	logSvc           *LogService
	eventSvc         *EventService
	activationLogSvc *ActivationLogService

	cmd          *exec.Cmd
	state        ProcessState
	command      string
	endpointName string
	startTime    *time.Time
	endTime      *time.Time
	exitCode     *int
	errorMsg     string
	outputBuffer *RingBuffer
	cancel       context.CancelFunc
}

func NewProcessService(cfg *config.Config, logSvc *LogService, eventSvc *EventService, activationLogSvc *ActivationLogService) *ProcessService {
	return &ProcessService{
		cfg:              cfg,
		logSvc:           logSvc,
		eventSvc:         eventSvc,
		activationLogSvc: activationLogSvc,
		state:            ProcessStateIdle,
		outputBuffer:     NewRingBuffer(64 * 1024), // 64KB buffer
	}
}

// StartProcess starts a new background process, killing any existing one first
func (s *ProcessService) StartProcess(endpointName, command string) error {
	s.KillProcess() // Kill any existing process

	s.mu.Lock()

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	cmd := exec.CommandContext(ctx, "bash", "-c", command)

	// Create pipes for stdout/stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.mu.Unlock()
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		s.mu.Unlock()
		return err
	}

	// Clear output buffer for new process
	s.outputBuffer = NewRingBuffer(64 * 1024)

	if err := cmd.Start(); err != nil {
		s.state = ProcessStateFailed
		s.errorMsg = err.Error()
		s.mu.Unlock()
		s.logSvc.Error("process", "Failed to start process: "+err.Error())
		return err
	}

	now := time.Now()
	s.cmd = cmd
	s.state = ProcessStateRunning
	s.command = command
	s.endpointName = endpointName
	s.startTime = &now
	s.endTime = nil
	s.exitCode = nil
	s.errorMsg = ""

	s.mu.Unlock()

	s.logSvc.Info("process", "Started post-activation command for endpoint: "+endpointName)

	// Start activation log file
	if s.activationLogSvc != nil {
		if err := s.activationLogSvc.StartLog(endpointName); err != nil {
			s.logSvc.Warn("process", "Failed to start activation log: "+err.Error())
		}
	}

	// Publish process started event
	s.eventSvc.Publish(Event{
		Type: EventProcessStarted,
		Payload: map[string]interface{}{
			"endpoint_name": endpointName,
			"command":       command,
		},
	})

	// Stream output from stdout and stderr
	go s.streamOutput(stdout, "stdout")
	go s.streamOutput(stderr, "stderr")

	// Wait for completion in background
	go func() {
		err := cmd.Wait()
		s.handleCompletion(err)
	}()

	return nil
}

// streamOutput reads from a pipe and publishes output events
func (s *ProcessService) streamOutput(pipe io.ReadCloser, source string) {
	reader := bufio.NewReader(pipe)
	buf := make([]byte, 4096)

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			content := string(buf[:n])

			// Write to output buffer
			s.outputBuffer.Write(buf[:n])

			// Write to activation log file
			if s.activationLogSvc != nil {
				s.activationLogSvc.Write(buf[:n])
			}

			// Publish output event
			s.eventSvc.Publish(Event{
				Type: EventProcessOutput,
				Payload: map[string]interface{}{
					"source":  source,
					"content": content,
				},
			})
		}
		if err != nil {
			break
		}
	}
}

// handleCompletion is called when the process exits
func (s *ProcessService) handleCompletion(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.endTime = &now

	if err != nil {
		// Check if it was killed via context cancellation
		if s.cmd.ProcessState != nil && s.cmd.ProcessState.ExitCode() == -1 {
			s.state = ProcessStateKilled
			s.logSvc.Info("process", "Process was killed")
		} else {
			s.state = ProcessStateStopped
			if exitErr, ok := err.(*exec.ExitError); ok {
				code := exitErr.ExitCode()
				s.exitCode = &code
				s.logSvc.Warn("process", "Process exited with error code")
			} else {
				s.errorMsg = err.Error()
				s.logSvc.Error("process", "Process error: "+err.Error())
			}
		}
	} else {
		s.state = ProcessStateStopped
		code := 0
		s.exitCode = &code
		s.logSvc.Info("process", "Process completed successfully")
	}

	// Close activation log file
	if s.activationLogSvc != nil {
		s.activationLogSvc.CloseLog()
	}

	// Publish process stopped event
	s.eventSvc.Publish(Event{
		Type: EventProcessStopped,
		Payload: map[string]interface{}{
			"state":         s.state,
			"exit_code":     s.exitCode,
			"endpoint_name": s.endpointName,
		},
	})
}

// KillProcess terminates the running process
func (s *ProcessService) KillProcess() error {
	s.mu.Lock()

	if s.state != ProcessStateRunning || s.cmd == nil || s.cmd.Process == nil {
		s.mu.Unlock()
		return nil
	}

	// Cancel context to signal graceful shutdown
	if s.cancel != nil {
		s.cancel()
	}

	process := s.cmd.Process
	s.mu.Unlock()

	s.logSvc.Info("process", "Killing running process")

	// Try SIGTERM first
	process.Signal(syscall.SIGTERM)

	// Wait for process to exit with timeout
	done := make(chan struct{})
	go func() {
		process.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Process exited gracefully
	case <-time.After(5 * time.Second):
		// Force kill if still running
		s.logSvc.Warn("process", "Process did not respond to SIGTERM, sending SIGKILL")
		process.Kill()
	}

	// Close activation log immediately so it's ready for the next activation
	// (handleCompletion will also try to close but CloseLog is idempotent)
	if s.activationLogSvc != nil {
		s.activationLogSvc.CloseLog()
	}

	return nil
}

// GetProcessInfo returns the current process state
func (s *ProcessService) GetProcessInfo() ProcessInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return ProcessInfo{
		Command:      s.command,
		State:        s.state,
		StartTime:    s.startTime,
		EndTime:      s.endTime,
		ExitCode:     s.exitCode,
		Error:        s.errorMsg,
		EndpointName: s.endpointName,
	}
}

// GetOutputBuffer returns the buffered output
func (s *ProcessService) GetOutputBuffer() []byte {
	return s.outputBuffer.Bytes()
}

// ClearOutputBuffer clears the output buffer
func (s *ProcessService) ClearOutputBuffer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outputBuffer = NewRingBuffer(64 * 1024)
}

// IsRunning returns true if a process is currently running
func (s *ProcessService) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state == ProcessStateRunning
}
