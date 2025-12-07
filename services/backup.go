package services

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"syncbot/config"
	"syncbot/models"
)

type BackupService struct {
	cfg        *config.Config
	logSvc     *LogService
	eventSvc   *EventService
	backupPath string
}

func NewBackupService(cfg *config.Config, logSvc *LogService, eventSvc *EventService) (*BackupService, error) {
	// Get the directory where syncbot is running (current working directory)
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current directory: %w", err)
	}
	backupPath := filepath.Join(cwd, "backups")

	// Create backups directory if it doesn't exist
	if err := os.MkdirAll(backupPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create backups directory: %w", err)
	}

	logSvc.Info("backup", fmt.Sprintf("Backup directory: %s", backupPath))

	return &BackupService{
		cfg:        cfg,
		logSvc:     logSvc,
		eventSvc:   eventSvc,
		backupPath: backupPath,
	}, nil
}

// GetBackupPath returns the path to the backups directory
func (s *BackupService) GetBackupDir() string {
	return s.backupPath
}

// Preview returns a list of files that would be backed up based on patterns
func (s *BackupService) Preview(endpointName string, patterns []string) (*models.BackupPreview, error) {
	if len(patterns) == 0 {
		return nil, fmt.Errorf("no backup patterns specified")
	}

	endpointPath := filepath.Join(s.cfg.EndpointsPath, endpointName)
	if _, err := os.Stat(endpointPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("endpoint does not exist")
	}

	// Validate patterns
	for _, pattern := range patterns {
		if err := s.validatePattern(pattern); err != nil {
			return nil, fmt.Errorf("invalid pattern '%s': %w", pattern, err)
		}
	}

	files, err := s.matchFiles(endpointPath, patterns)
	if err != nil {
		return nil, fmt.Errorf("failed to match files: %w", err)
	}

	var totalSize int64
	for _, f := range files {
		totalSize += f.Size
	}

	return &models.BackupPreview{
		EndpointName: endpointName,
		Patterns:     patterns,
		Files:        files,
		TotalSize:    totalSize,
		FileCount:    len(files),
	}, nil
}

// CreateBackup creates a .tgz archive of matched files
func (s *BackupService) CreateBackup(endpointName string, patterns []string) (*models.BackupResult, error) {
	preview, err := s.Preview(endpointName, patterns)
	if err != nil {
		return nil, err
	}

	if preview.FileCount == 0 {
		return nil, fmt.Errorf("no files match the specified patterns")
	}

	// Generate archive name: {endpoint}_{datetime}.tgz
	timestamp := time.Now().Format("20060102-150405")
	archiveName := fmt.Sprintf("%s_%s.tgz", endpointName, timestamp)
	archivePath := filepath.Join(s.backupPath, archiveName)

	endpointPath := filepath.Join(s.cfg.EndpointsPath, endpointName)

	// Create the archive
	if err := s.createTgzArchive(archivePath, endpointPath, preview.Files); err != nil {
		return nil, fmt.Errorf("failed to create archive: %w", err)
	}

	s.logSvc.Info("backup", fmt.Sprintf("Created backup '%s' with %d files (%.2f MB)",
		archiveName, preview.FileCount, float64(preview.TotalSize)/(1024*1024)))

	// Publish backup event
	s.eventSvc.Publish(Event{
		Type: "backup_created",
		Payload: map[string]string{
			"endpoint": endpointName,
			"archive":  archiveName,
		},
	})

	return &models.BackupResult{
		EndpointName: endpointName,
		ArchivePath:  archivePath,
		ArchiveName:  archiveName,
		FileCount:    preview.FileCount,
		TotalSize:    preview.TotalSize,
		CreatedAt:    time.Now(),
	}, nil
}

// matchFiles finds all files matching the given patterns
func (s *BackupService) matchFiles(basePath string, patterns []string) ([]models.BackupFile, error) {
	var files []models.BackupFile
	seen := make(map[string]bool)

	for _, pattern := range patterns {
		// Use doublestar for ** support
		err := doublestar.GlobWalk(os.DirFS(basePath), pattern, func(path string, d os.DirEntry) error {
			// Skip if already seen
			if seen[path] {
				return nil
			}

			// Skip directories
			if d.IsDir() {
				return nil
			}

			// Get file info
			info, err := d.Info()
			if err != nil {
				return nil // Skip files we can't stat
			}

			seen[path] = true
			files = append(files, models.BackupFile{
				Path:      path,
				Size:      info.Size(),
				ModTime:   info.ModTime().Format("2006-01-02 15:04:05"),
				MatchedBy: pattern,
			})
			return nil
		})
		if err != nil {
			s.logSvc.Warn("backup", fmt.Sprintf("Pattern match warning for '%s': %v", pattern, err))
		}
	}

	return files, nil
}

// validatePattern checks if a pattern is valid
func (s *BackupService) validatePattern(pattern string) error {
	if pattern == "" {
		return fmt.Errorf("pattern cannot be empty")
	}

	// Check for path traversal attempts
	if strings.Contains(pattern, "..") {
		return fmt.Errorf("path traversal not allowed")
	}

	// Try to validate the pattern
	if !doublestar.ValidatePattern(pattern) {
		return fmt.Errorf("invalid glob pattern")
	}

	return nil
}

// createTgzArchive creates a .tgz file containing the specified files
func (s *BackupService) createTgzArchive(archivePath, basePath string, files []models.BackupFile) error {
	outFile, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	gzWriter := gzip.NewWriter(outFile)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	for _, file := range files {
		fullPath := filepath.Join(basePath, file.Path)

		info, err := os.Stat(fullPath)
		if err != nil {
			s.logSvc.Warn("backup", fmt.Sprintf("Skipping file %s: %v", file.Path, err))
			continue
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		// Use relative path in archive
		header.Name = file.Path

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		f, err := os.Open(fullPath)
		if err != nil {
			return err
		}

		if _, err := io.Copy(tarWriter, f); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}

	return nil
}

// ListBackups returns all backup archives for an endpoint
func (s *BackupService) ListBackups(endpointName string) ([]models.BackupResult, error) {
	pattern := filepath.Join(s.backupPath, fmt.Sprintf("%s_*.tgz", endpointName))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	var backups []models.BackupResult
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			continue
		}
		backups = append(backups, models.BackupResult{
			EndpointName: endpointName,
			ArchivePath:  match,
			ArchiveName:  filepath.Base(match),
			TotalSize:    info.Size(),
			CreatedAt:    info.ModTime(),
		})
	}

	return backups, nil
}

// ListAllBackups returns all backup archives
func (s *BackupService) ListAllBackups() ([]models.BackupResult, error) {
	pattern := filepath.Join(s.backupPath, "*.tgz")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	var backups []models.BackupResult
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			continue
		}

		// Extract endpoint name from archive name (format: {endpoint}_{timestamp}.tgz)
		baseName := filepath.Base(match)
		endpointName := ""
		if idx := strings.LastIndex(baseName, "_"); idx > 0 {
			endpointName = baseName[:idx]
		}

		backups = append(backups, models.BackupResult{
			EndpointName: endpointName,
			ArchivePath:  match,
			ArchiveName:  baseName,
			TotalSize:    info.Size(),
			CreatedAt:    info.ModTime(),
		})
	}

	return backups, nil
}

// GetBackupPath returns the full path to a backup archive
func (s *BackupService) GetBackupPath(archiveName string) (string, error) {
	// Security: validate archive name to prevent path traversal
	if strings.Contains(archiveName, "/") || strings.Contains(archiveName, "\\") || strings.Contains(archiveName, "..") {
		return "", fmt.Errorf("invalid archive name")
	}
	if !strings.HasSuffix(archiveName, ".tgz") {
		return "", fmt.Errorf("invalid archive extension")
	}

	archivePath := filepath.Join(s.backupPath, archiveName)
	if _, err := os.Stat(archivePath); os.IsNotExist(err) {
		return "", fmt.Errorf("backup not found")
	}

	return archivePath, nil
}

// DeleteBackup deletes a backup archive
func (s *BackupService) DeleteBackup(archiveName string) error {
	archivePath, err := s.GetBackupPath(archiveName)
	if err != nil {
		return err
	}

	if err := os.Remove(archivePath); err != nil {
		return fmt.Errorf("failed to delete backup: %w", err)
	}

	s.logSvc.Info("backup", fmt.Sprintf("Deleted backup '%s'", archiveName))

	// Publish backup deleted event
	s.eventSvc.Publish(Event{
		Type: "backup_deleted",
		Payload: map[string]string{
			"archive": archiveName,
		},
	})

	return nil
}
