package models

import "time"

type Endpoint struct {
	Name              string     `json:"name"`
	Username          string     `json:"username"`
	CreatedAt         time.Time  `json:"created_at"`
	LastActivatedAt   *time.Time `json:"last_activated_at,omitempty"`
	BuildInfoPath     string     `json:"build_info_path,omitempty"`
	BackupPatterns    []string   `json:"backup_patterns,omitempty"`
	ActivationCommand string     `json:"activation_command,omitempty"`
	IsActive          bool       `json:"is_active"`
}

type EndpointMetadata struct {
	Username          string     `json:"username"`
	CreatedAt         time.Time  `json:"created_at"`
	LastActivatedAt   *time.Time `json:"last_activated_at,omitempty"`
	BuildInfoPath     string     `json:"build_info_path,omitempty"`
	BackupPatterns    []string   `json:"backup_patterns,omitempty"`
	ActivationCommand string     `json:"activation_command,omitempty"`
}

// BackupResult represents the result of a backup operation
type BackupResult struct {
	EndpointName string    `json:"endpoint_name"`
	ArchivePath  string    `json:"archive_path"`
	ArchiveName  string    `json:"archive_name"`
	FileCount    int       `json:"file_count"`
	TotalSize    int64     `json:"total_size"`
	CreatedAt    time.Time `json:"created_at"`
}

// BackupPreview represents files that would be included in a backup
type BackupPreview struct {
	EndpointName string       `json:"endpoint_name"`
	Patterns     []string     `json:"patterns"`
	Files        []BackupFile `json:"files"`
	TotalSize    int64        `json:"total_size"`
	FileCount    int          `json:"file_count"`
}

// BackupFile represents a single file in a backup preview
type BackupFile struct {
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	ModTime   string `json:"mod_time"`
	MatchedBy string `json:"matched_by"`
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
