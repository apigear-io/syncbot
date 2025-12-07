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
