package models

import "time"

type Endpoint struct {
	Name      string    `json:"name"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
	IsActive  bool      `json:"is_active"`
}

type EndpointMetadata struct {
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

// Command represents a user-defined command that can be executed on the device
type Command struct {
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	Command     string `json:"command" yaml:"command"` // Shell command with Go template syntax
}

// CommandTemplateVars contains variables available in command templates
type CommandTemplateVars struct {
	HomeDir       string // User's home directory
	EndpointsPath string // Path to endpoints directory
	ActiveSymlink string // Path to active symlink
	ActiveName    string // Name of currently active endpoint
	DeviceName    string // Device name from config
	DeviceIP      string // Device IP from config
}

// CommandResult represents the result of executing a command
type CommandResult struct {
	CommandID string `json:"command_id"`
	Success   bool   `json:"success"`
	Output    string `json:"output"`
	Error     string `json:"error,omitempty"`
}
