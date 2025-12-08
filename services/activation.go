package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"syncbot/config"
)

type ActivationService struct {
	cfg      *config.Config
	logSvc   *LogService
	eventSvc *EventService
}

func NewActivationService(cfg *config.Config, logSvc *LogService, eventSvc *EventService) *ActivationService {
	return &ActivationService{cfg: cfg, logSvc: logSvc, eventSvc: eventSvc}
}

func (s *ActivationService) GetActive() string {
	target, err := os.Readlink(s.cfg.ActiveSymlink)
	if err != nil {
		return ""
	}
	return filepath.Base(target)
}

func (s *ActivationService) Activate(name string) error {
	endpointPath := filepath.Join(s.cfg.EndpointsPath, name)

	if _, err := os.Stat(endpointPath); os.IsNotExist(err) {
		s.logSvc.Error("activation", fmt.Sprintf("Endpoint '%s' does not exist", name))
		return fmt.Errorf("endpoint does not exist")
	}

	// The old sync used direct copy into the active directory. Some people may still have that setup.
	// If a directory exists at the symlink path (not a symlink), back it up to a numbered folder.
	if err := s.backupExistingDirectory(); err != nil {
		s.logSvc.Error("activation", fmt.Sprintf("Failed to backup existing directory: %s", err.Error()))
		return fmt.Errorf("failed to backup existing directory: %w", err)
	}

	previousActive := s.GetActive()
	os.Remove(s.cfg.ActiveSymlink)

	if err := os.Symlink(endpointPath, s.cfg.ActiveSymlink); err != nil {
		s.logSvc.Error("activation", fmt.Sprintf("Failed to create symlink for '%s': %s", name, err.Error()))
		return fmt.Errorf("failed to create symlink: %w", err)
	}

	if previousActive != "" && previousActive != name {
		s.logSvc.Info("activation", fmt.Sprintf("Switched active endpoint from '%s' to '%s'", previousActive, name))
	} else {
		s.logSvc.Info("activation", fmt.Sprintf("Activated endpoint '%s'", name))
	}

	if s.cfg.PostActivationCommand != "" {
		s.logSvc.Info("activation", fmt.Sprintf("Running post-activation command: %s", s.cfg.PostActivationCommand))
		if err := s.runPostCommand(); err != nil {
			s.logSvc.Error("activation", fmt.Sprintf("Post-activation command failed: %s", err.Error()))
			return fmt.Errorf("activation successful but post-command failed: %w", err)
		}
		s.logSvc.Info("activation", "Post-activation command completed successfully")
	}

	// Publish activation event
	s.eventSvc.Publish(Event{
		Type:    EventEndpointActivated,
		Payload: map[string]string{"name": name, "previous": previousActive},
	})

	return nil
}

func (s *ActivationService) runPostCommand() error {
	cmd := exec.Command("sh", "-c", s.cfg.PostActivationCommand)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// backupExistingDirectory checks if ActiveSymlink path exists as a real directory (not a symlink)
// and renames it to a numbered backup folder (e.g., folder.1, folder.2, etc.)
func (s *ActivationService) backupExistingDirectory() error {
	info, err := os.Lstat(s.cfg.ActiveSymlink)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Nothing exists, no backup needed
		}
		return err
	}

	// If it's a symlink, nothing to backup (normal case)
	if info.Mode()&os.ModeSymlink != 0 {
		return nil
	}

	// If it's not a directory, we have an unexpected situation
	if !info.IsDir() {
		return fmt.Errorf("active path exists but is not a directory or symlink")
	}

	// Find a free numbered backup name
	backupPath := s.findFreeBackupPath(s.cfg.ActiveSymlink)

	s.logSvc.Info("activation", fmt.Sprintf("Backing up existing directory to '%s'", backupPath))

	if err := os.Rename(s.cfg.ActiveSymlink, backupPath); err != nil {
		return fmt.Errorf("failed to rename directory: %w", err)
	}

	return nil
}

// findFreeBackupPath finds an available numbered path (e.g., path.1, path.2, etc.)
func (s *ActivationService) findFreeBackupPath(basePath string) string {
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s.%d", basePath, i)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}
