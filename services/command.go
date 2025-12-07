package services

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"text/template"

	"syncbot/config"
	"syncbot/models"
)

type CommandService struct {
	cfg      *config.Config
	logSvc   *LogService
	eventSvc *EventService
	actSvc   *ActivationService
}

func NewCommandService(cfg *config.Config, logSvc *LogService, eventSvc *EventService, actSvc *ActivationService) *CommandService {
	return &CommandService{
		cfg:      cfg,
		logSvc:   logSvc,
		eventSvc: eventSvc,
		actSvc:   actSvc,
	}
}

// List returns all configured commands
func (s *CommandService) List() []models.Command {
	if s.cfg.Commands == nil {
		return []models.Command{}
	}
	return s.cfg.Commands
}

// Get returns a command by ID
func (s *CommandService) Get(id string) (*models.Command, error) {
	for _, cmd := range s.cfg.Commands {
		if cmd.ID == id {
			return &cmd, nil
		}
	}
	return nil, fmt.Errorf("command not found: %s", id)
}

// Create adds a new command
func (s *CommandService) Create(cmd models.Command) error {
	// Check for duplicate ID
	for _, existing := range s.cfg.Commands {
		if existing.ID == cmd.ID {
			return fmt.Errorf("command with ID '%s' already exists", cmd.ID)
		}
	}

	s.cfg.Commands = append(s.cfg.Commands, cmd)

	if err := s.cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	s.logSvc.Info("commands", fmt.Sprintf("Created command '%s' (%s)", cmd.Name, cmd.ID))

	s.eventSvc.Publish(Event{
		Type:    EventCommandCreated,
		Payload: map[string]string{"id": cmd.ID, "name": cmd.Name},
	})

	return nil
}

// Update modifies an existing command
func (s *CommandService) Update(cmd models.Command) error {
	for i, existing := range s.cfg.Commands {
		if existing.ID == cmd.ID {
			s.cfg.Commands[i] = cmd
			if err := s.cfg.Save(); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}
			s.logSvc.Info("commands", fmt.Sprintf("Updated command '%s' (%s)", cmd.Name, cmd.ID))
			return nil
		}
	}
	return fmt.Errorf("command not found: %s", cmd.ID)
}

// Delete removes a command by ID
func (s *CommandService) Delete(id string) error {
	for i, cmd := range s.cfg.Commands {
		if cmd.ID == id {
			s.cfg.Commands = append(s.cfg.Commands[:i], s.cfg.Commands[i+1:]...)
			if err := s.cfg.Save(); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}
			s.logSvc.Info("commands", fmt.Sprintf("Deleted command '%s' (%s)", cmd.Name, id))

			s.eventSvc.Publish(Event{
				Type:    EventCommandDeleted,
				Payload: map[string]string{"id": id, "name": cmd.Name},
			})

			return nil
		}
	}
	return fmt.Errorf("command not found: %s", id)
}

// Execute runs a command by ID, expanding template variables
func (s *CommandService) Execute(id string) models.CommandResult {
	cmd, err := s.Get(id)
	if err != nil {
		return models.CommandResult{
			CommandID: id,
			Success:   false,
			Error:     err.Error(),
		}
	}

	// Build template variables
	vars := s.buildTemplateVars()

	// Parse and execute template
	expandedCmd, err := s.expandTemplate(cmd.Command, vars)
	if err != nil {
		s.logSvc.Error("commands", fmt.Sprintf("Failed to expand template for command '%s': %s", cmd.Name, err.Error()))
		return models.CommandResult{
			CommandID: id,
			Success:   false,
			Error:     fmt.Sprintf("template error: %s", err.Error()),
		}
	}

	s.logSvc.Info("commands", fmt.Sprintf("Executing command '%s': %s", cmd.Name, expandedCmd))

	// Execute the command
	output, err := s.runShellCommand(expandedCmd)

	result := models.CommandResult{
		CommandID: id,
		Success:   err == nil,
		Output:    output,
	}

	if err != nil {
		result.Error = err.Error()
		s.logSvc.Error("commands", fmt.Sprintf("Command '%s' failed: %s", cmd.Name, err.Error()))
	} else {
		s.logSvc.Info("commands", fmt.Sprintf("Command '%s' completed successfully", cmd.Name))
	}

	// Publish event
	s.eventSvc.Publish(Event{
		Type: EventCommandExecuted,
		Payload: map[string]interface{}{
			"id":      id,
			"name":    cmd.Name,
			"success": result.Success,
			"output":  result.Output,
			"error":   result.Error,
		},
	})

	return result
}

// buildTemplateVars creates the template variables struct
func (s *CommandService) buildTemplateVars() models.CommandTemplateVars {
	homeDir, _ := os.UserHomeDir()

	return models.CommandTemplateVars{
		HomeDir:       homeDir,
		EndpointsPath: s.cfg.EndpointsPath,
		ActiveSymlink: s.cfg.ActiveSymlink,
		ActiveName:    s.actSvc.GetActive(),
		DeviceName:    s.cfg.DeviceName,
		DeviceIP:      s.cfg.DeviceIP,
	}
}

// expandTemplate parses and executes a Go template with the given variables
func (s *CommandService) expandTemplate(cmdTemplate string, vars models.CommandTemplateVars) (string, error) {
	tmpl, err := template.New("command").Parse(cmdTemplate)
	if err != nil {
		return "", fmt.Errorf("invalid template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("template execution failed: %w", err)
	}

	return buf.String(), nil
}

// runShellCommand executes a shell command and returns the combined output
func (s *CommandService) runShellCommand(command string) (string, error) {
	cmd := exec.Command("sh", "-c", command)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Combine stdout and stderr
	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	return output, err
}

// GetTemplateVars returns the current template variables (for display in UI)
func (s *CommandService) GetTemplateVars() models.CommandTemplateVars {
	return s.buildTemplateVars()
}
