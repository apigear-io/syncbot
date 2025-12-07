package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"syncbot/config"
	"syncbot/models"
)

const metadataDir = ".syncbot-metadata"

var validNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]$|^[a-zA-Z0-9]$`)

func (s *EndpointService) metadataPath(name string) string {
	return filepath.Join(s.cfg.EndpointsPath, metadataDir, name+".json")
}

func (s *EndpointService) ensureMetadataDir() error {
	dir := filepath.Join(s.cfg.EndpointsPath, metadataDir)
	return os.MkdirAll(dir, 0755)
}

type EndpointService struct {
	cfg               *config.Config
	activationService *ActivationService
	logSvc            *LogService
	eventSvc          *EventService
}

func NewEndpointService(cfg *config.Config, activationSvc *ActivationService, logSvc *LogService, eventSvc *EventService) *EndpointService {
	return &EndpointService{
		cfg:               cfg,
		activationService: activationSvc,
		logSvc:            logSvc,
		eventSvc:          eventSvc,
	}
}

func (s *EndpointService) List() ([]models.Endpoint, error) {
	entries, err := os.ReadDir(s.cfg.EndpointsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []models.Endpoint{}, nil
		}
		s.logSvc.Error("endpoint", "Failed to read endpoints directory: "+err.Error())
		return nil, fmt.Errorf("failed to read endpoints directory: %w", err)
	}

	activeEndpoint := s.activationService.GetActive()

	var endpoints []models.Endpoint
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Skip the metadata directory
		if entry.Name() == metadataDir {
			continue
		}

		endpoint, err := s.Get(entry.Name())
		if err != nil {
			continue
		}

		endpoint.IsActive = endpoint.Name == activeEndpoint
		endpoints = append(endpoints, *endpoint)
	}

	return endpoints, nil
}

func (s *EndpointService) Get(name string) (*models.Endpoint, error) {
	metadataPath := s.metadataPath(name)

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read endpoint metadata: %w", err)
	}

	var metadata models.EndpointMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse endpoint metadata: %w", err)
	}

	activeEndpoint := s.activationService.GetActive()

	return &models.Endpoint{
		Name:      name,
		Username:  metadata.Username,
		CreatedAt: metadata.CreatedAt,
		IsActive:  name == activeEndpoint,
	}, nil
}

func (s *EndpointService) Create(name, username string) (*models.Endpoint, error) {
	if !validNameRegex.MatchString(name) {
		s.logSvc.Warn("endpoint", fmt.Sprintf("Invalid endpoint name attempted: '%s'", name))
		return nil, fmt.Errorf("invalid endpoint name: must be alphanumeric with hyphens, cannot start/end with hyphen")
	}

	if username == "" {
		s.logSvc.Warn("endpoint", "Endpoint creation attempted without username")
		return nil, fmt.Errorf("username is required")
	}

	endpointPath := filepath.Join(s.cfg.EndpointsPath, name)

	if _, err := os.Stat(endpointPath); err == nil {
		s.logSvc.Warn("endpoint", fmt.Sprintf("Endpoint '%s' already exists", name))
		return nil, fmt.Errorf("endpoint already exists")
	}

	if err := os.MkdirAll(endpointPath, 0755); err != nil {
		s.logSvc.Error("endpoint", fmt.Sprintf("Failed to create endpoint directory '%s': %s", name, err.Error()))
		return nil, fmt.Errorf("failed to create endpoint directory: %w", err)
	}

	if err := s.ensureMetadataDir(); err != nil {
		os.RemoveAll(endpointPath)
		s.logSvc.Error("endpoint", "Failed to create metadata directory: "+err.Error())
		return nil, fmt.Errorf("failed to create metadata directory: %w", err)
	}

	metadata := models.EndpointMetadata{
		Username:  username,
		CreatedAt: time.Now().UTC(),
	}

	metadataPath := s.metadataPath(name)
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		os.RemoveAll(endpointPath)
		s.logSvc.Error("endpoint", "Failed to marshal metadata: "+err.Error())
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.WriteFile(metadataPath, data, 0644); err != nil {
		os.RemoveAll(endpointPath)
		s.logSvc.Error("endpoint", "Failed to write metadata: "+err.Error())
		return nil, fmt.Errorf("failed to write metadata: %w", err)
	}

	s.logSvc.Info("endpoint", fmt.Sprintf("Created endpoint '%s' by owner '%s'", name, username))

	endpoint := &models.Endpoint{
		Name:      name,
		Username:  metadata.Username,
		CreatedAt: metadata.CreatedAt,
		IsActive:  false,
	}

	// Publish endpoint created event
	s.eventSvc.Publish(Event{
		Type:    EventEndpointCreated,
		Payload: map[string]string{"name": name, "username": username},
	})

	return endpoint, nil
}

func (s *EndpointService) Delete(name, username string) error {
	endpoint, err := s.Get(name)
	if err != nil {
		s.logSvc.Warn("endpoint", fmt.Sprintf("Delete attempted on non-existent endpoint '%s'", name))
		return fmt.Errorf("endpoint not found")
	}

	if endpoint.Username != username {
		s.logSvc.Warn("endpoint", fmt.Sprintf("Unauthorized delete attempt on '%s' by '%s' (owner: '%s')", name, username, endpoint.Username))
		return fmt.Errorf("unauthorized: only the owner can delete this endpoint")
	}

	if endpoint.IsActive {
		s.logSvc.Warn("endpoint", fmt.Sprintf("Cannot delete active endpoint '%s'", name))
		return fmt.Errorf("cannot delete active endpoint: deactivate it first")
	}

	endpointPath := filepath.Join(s.cfg.EndpointsPath, name)
	if err := os.RemoveAll(endpointPath); err != nil {
		s.logSvc.Error("endpoint", fmt.Sprintf("Failed to delete endpoint '%s': %s", name, err.Error()))
		return fmt.Errorf("failed to delete endpoint: %w", err)
	}

	// Also delete the metadata file
	metadataPath := s.metadataPath(name)
	if err := os.Remove(metadataPath); err != nil && !os.IsNotExist(err) {
		s.logSvc.Error("endpoint", fmt.Sprintf("Failed to delete metadata for '%s': %s", name, err.Error()))
		return fmt.Errorf("failed to delete endpoint metadata: %w", err)
	}

	s.logSvc.Info("endpoint", fmt.Sprintf("Deleted endpoint '%s' by owner '%s'", name, username))

	// Publish endpoint deleted event
	s.eventSvc.Publish(Event{
		Type:    EventEndpointDeleted,
		Payload: map[string]string{"name": name, "username": username},
	})

	return nil
}
