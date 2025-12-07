package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"syncbot/config"
	"syncbot/models"
)

type CreateEndpointRequest struct {
	Name     string `json:"name"`
	Username string `json:"username"`
}

type DeleteEndpointRequest struct {
	Username       string `json:"username"`
	DeleteContents *bool  `json:"delete_contents,omitempty"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type ConfigResponse struct {
	DeviceIP      string `json:"device_ip"`
	SSHUsername   string `json:"ssh_username"`
	EndpointsPath string `json:"endpoints_path"`
}

type ActiveResponse struct {
	Name string `json:"name"`
}

func (h *Handlers) ListEndpoints(w http.ResponseWriter, r *http.Request) {
	endpoints, err := h.endpointService.List()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, endpoints)
}

func (h *Handlers) CreateEndpoint(w http.ResponseWriter, r *http.Request) {
	var req CreateEndpointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}

	endpoint, err := h.endpointService.Create(req.Name, req.Username)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, endpoint)
}

func (h *Handlers) DeleteEndpoint(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var req DeleteEndpointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}

	// Default to deleting contents if not specified
	deleteContents := true
	if req.DeleteContents != nil {
		deleteContents = *req.DeleteContents
	}

	if err := h.endpointService.Delete(name, req.Username, deleteContents); err != nil {
		status := http.StatusBadRequest
		if err.Error() == "unauthorized: only the owner can delete this endpoint" {
			status = http.StatusForbidden
		}
		writeJSON(w, status, ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) ActivateEndpoint(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	if err := h.activationService.Activate(name); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	// Update the last activated timestamp
	if err := h.endpointService.UpdateLastActivated(name); err != nil {
		h.logService.Warn("activation", "Failed to update last activated timestamp: "+err.Error())
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "activated", "name": name})
}

type UpdateEndpointRequest struct {
	Username      string `json:"username"`
	BuildInfoPath string `json:"build_info_path"`
}

type UpdateBuildInfoPathRequest struct {
	BuildInfoPath string `json:"build_info_path"`
}

func (h *Handlers) UpdateEndpoint(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var req UpdateEndpointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}

	if req.Username == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "username is required"})
		return
	}

	if err := h.endpointService.Update(name, req.Username, req.BuildInfoPath); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handlers) UpdateEndpointBuildInfoPath(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var req UpdateBuildInfoPathRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}

	if err := h.endpointService.UpdateBuildInfoPath(name, req.BuildInfoPath); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handlers) GetEndpointBuildInfo(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	content, err := h.endpointService.GetBuildInfo(name)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"content": content})
}

func (h *Handlers) GetActive(w http.ResponseWriter, r *http.Request) {
	active := h.activationService.GetActive()
	writeJSON(w, http.StatusOK, ActiveResponse{Name: active})
}

func (h *Handlers) GetConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, ConfigResponse{
		DeviceIP:      h.config.DeviceIP,
		SSHUsername:   h.config.SSHUsername,
		EndpointsPath: h.config.EndpointsPath,
	})
}

type SettingsResponse struct {
	DeviceName            string          `json:"device_name"`
	DeviceDescription     string          `json:"device_description"`
	DeviceLocation        string          `json:"device_location"`
	EndpointsPath         string          `json:"endpoints_path"`
	ActiveSymlink         string          `json:"active_symlink"`
	PostActivationCommand string          `json:"post_activation_command"`
	Port                  int             `json:"port"`
	DeviceIP              string          `json:"device_ip"`
	SSHUsername           string          `json:"ssh_username"`
	Devices               []config.Device `json:"devices"`
}

type UpdateSettingsRequest struct {
	DeviceName            string          `json:"device_name"`
	DeviceDescription     string          `json:"device_description"`
	DeviceLocation        string          `json:"device_location"`
	EndpointsPath         string          `json:"endpoints_path"`
	ActiveSymlink         string          `json:"active_symlink"`
	PostActivationCommand string          `json:"post_activation_command"`
	DeviceIP              string          `json:"device_ip"`
	SSHUsername           string          `json:"ssh_username"`
	Devices               []config.Device `json:"devices"`
}

func (h *Handlers) GetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, SettingsResponse{
		DeviceName:            h.config.DeviceName,
		DeviceDescription:     h.config.DeviceDescription,
		DeviceLocation:        h.config.DeviceLocation,
		EndpointsPath:         h.config.EndpointsPath,
		ActiveSymlink:         h.config.ActiveSymlink,
		PostActivationCommand: h.config.PostActivationCommand,
		Port:                  h.config.Port,
		DeviceIP:              h.config.DeviceIP,
		SSHUsername:           h.config.SSHUsername,
		Devices:               h.config.Devices,
	})
}

func (h *Handlers) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req UpdateSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}

	// Update config in memory
	if req.DeviceName != "" {
		h.config.DeviceName = req.DeviceName
	}
	if req.DeviceDescription != "" {
		h.config.DeviceDescription = req.DeviceDescription
	}
	// Allow empty location
	h.config.DeviceLocation = req.DeviceLocation

	if req.EndpointsPath != "" {
		h.config.EndpointsPath = req.EndpointsPath
	}
	if req.ActiveSymlink != "" {
		h.config.ActiveSymlink = req.ActiveSymlink
	}
	// Allow empty post activation command
	h.config.PostActivationCommand = req.PostActivationCommand

	if req.DeviceIP != "" {
		h.config.DeviceIP = req.DeviceIP
	}
	if req.SSHUsername != "" {
		h.config.SSHUsername = req.SSHUsername
	}

	// Update devices list
	if req.Devices != nil {
		h.config.Devices = req.Devices
	}

	// Save to file
	if err := h.config.Save(); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to save settings: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, SettingsResponse{
		DeviceName:            h.config.DeviceName,
		DeviceDescription:     h.config.DeviceDescription,
		DeviceLocation:        h.config.DeviceLocation,
		EndpointsPath:         h.config.EndpointsPath,
		ActiveSymlink:         h.config.ActiveSymlink,
		PostActivationCommand: h.config.PostActivationCommand,
		Port:                  h.config.Port,
		DeviceIP:              h.config.DeviceIP,
		SSHUsername:           h.config.SSHUsername,
		Devices:               h.config.Devices,
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *Handlers) jsonError(w http.ResponseWriter, message string, status int) {
	writeJSON(w, status, ErrorResponse{Error: message})
}

type LogsResponse struct {
	Entries []LogEntryResponse `json:"entries"`
	Total   int                `json:"total"`
}

type LogEntryResponse struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Component string `json:"component,omitempty"`
}

func (h *Handlers) GetLogs(w http.ResponseWriter, r *http.Request) {
	level := r.URL.Query().Get("level")
	search := r.URL.Query().Get("search")
	limitStr := r.URL.Query().Get("limit")

	limit := 100
	if limitStr != "" {
		if l, err := parseInt(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	entries := h.logService.GetEntries(level, search, limit)

	response := LogsResponse{
		Entries: make([]LogEntryResponse, len(entries)),
		Total:   h.logService.Count(),
	}

	for i, entry := range entries {
		response.Entries[i] = LogEntryResponse{
			Timestamp: entry.Timestamp.Format("2006-01-02 15:04:05"),
			Level:     entry.Level,
			Message:   entry.Message,
			Component: entry.Component,
		}
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *Handlers) ClearLogs(w http.ResponseWriter, r *http.Request) {
	h.logService.Clear()
	h.logService.Info("logs", "Log buffer cleared")
	w.WriteHeader(http.StatusNoContent)
}

func parseInt(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, nil
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// Events handles SSE connections for real-time updates
func (h *Handlers) Events(w http.ResponseWriter, r *http.Request) {
	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Subscribe to events
	eventChan := h.eventService.Subscribe()
	defer h.eventService.Unsubscribe(eventChan)

	// Get the flusher for streaming
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Send initial connection event
	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\"}\n\n")
	flusher.Flush()

	// Stream events to client
	for {
		select {
		case event, ok := <-eventChan:
			if !ok {
				return
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// Command API handlers

type CreateCommandRequest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Command     string `json:"command"`
}

type UpdateCommandRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Command     string `json:"command"`
}

// ListCommands returns all configured commands
func (h *Handlers) ListCommands(w http.ResponseWriter, r *http.Request) {
	commands := h.commandService.List()
	writeJSON(w, http.StatusOK, commands)
}

// GetCommand returns a single command by ID
func (h *Handlers) GetCommand(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cmd, err := h.commandService.Get(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, cmd)
}

// CreateCommand adds a new command
func (h *Handlers) CreateCommand(w http.ResponseWriter, r *http.Request) {
	var req CreateCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}

	if req.ID == "" || req.Name == "" || req.Command == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "id, name, and command are required"})
		return
	}

	cmd := models.Command{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
		Command:     req.Command,
	}

	if err := h.commandService.Create(cmd); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, cmd)
}

// UpdateCommand modifies an existing command
func (h *Handlers) UpdateCommand(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req UpdateCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}

	cmd := models.Command{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		Command:     req.Command,
	}

	if err := h.commandService.Update(cmd); err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, cmd)
}

// DeleteCommand removes a command by ID
func (h *Handlers) DeleteCommand(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.commandService.Delete(id); err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ExecuteCommand runs a command and returns the result
func (h *Handlers) ExecuteCommand(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result := h.commandService.Execute(id)

	status := http.StatusOK
	if !result.Success {
		status = http.StatusInternalServerError
	}

	writeJSON(w, status, result)
}

// GetTemplateVars returns the available template variables
func (h *Handlers) GetTemplateVars(w http.ResponseWriter, r *http.Request) {
	vars := h.commandService.GetTemplateVars()
	writeJSON(w, http.StatusOK, vars)
}

// Sync API types and handlers

// SyncableSettings represents all settings that can be synced between devices
type SyncableSettings struct {
	// Device info (typically device-specific, skip by default)
	DeviceName        string `json:"device_name"`
	DeviceDescription string `json:"device_description"`
	DeviceLocation    string `json:"device_location"`

	// Paths (typically device-specific, skip by default)
	EndpointsPath         string `json:"endpoints_path"`
	ActiveSymlink         string `json:"active_symlink"`
	PostActivationCommand string `json:"post_activation_command"`

	// Network (typically device-specific, skip by default)
	DeviceIP    string `json:"device_ip"`
	SSHUsername string `json:"ssh_username"`

	// Commands (usually should be synced)
	Commands []models.Command `json:"commands"`

	// Devices registry (usually should be synced)
	Devices []config.Device `json:"devices"`
}

// SyncFetchRequest is the request body for POST /api/sync/fetch
type SyncFetchRequest struct {
	MasterURL string `json:"masterUrl"`
}

// SyncFetchResponse contains both master and local settings for comparison
type SyncFetchResponse struct {
	Master SyncableSettings `json:"master"`
	Local  SyncableSettings `json:"local"`
}

// SyncApplyRequest contains the settings selected by the user to apply
type SyncApplyRequest struct {
	// Device info
	DeviceName        *string `json:"device_name,omitempty"`
	DeviceDescription *string `json:"device_description,omitempty"`
	DeviceLocation    *string `json:"device_location,omitempty"`

	// Paths
	EndpointsPath         *string `json:"endpoints_path,omitempty"`
	ActiveSymlink         *string `json:"active_symlink,omitempty"`
	PostActivationCommand *string `json:"post_activation_command,omitempty"`

	// Network
	DeviceIP    *string `json:"device_ip,omitempty"`
	SSHUsername *string `json:"ssh_username,omitempty"`

	// Commands - map of command ID to command (nil means don't change)
	Commands []models.Command `json:"commands,omitempty"`

	// Devices registry
	Devices []config.Device `json:"devices,omitempty"`
}

// SyncApplyResponse indicates the result of applying sync settings
type SyncApplyResponse struct {
	Success     bool   `json:"success"`
	NeedsReload bool   `json:"needsReload"`
	Message     string `json:"message,omitempty"`
}

// GetSyncSettings returns all syncable settings from this device
// GET /api/sync/settings
func (h *Handlers) GetSyncSettings(w http.ResponseWriter, r *http.Request) {
	settings := SyncableSettings{
		DeviceName:            h.config.DeviceName,
		DeviceDescription:     h.config.DeviceDescription,
		DeviceLocation:        h.config.DeviceLocation,
		EndpointsPath:         h.config.EndpointsPath,
		ActiveSymlink:         h.config.ActiveSymlink,
		PostActivationCommand: h.config.PostActivationCommand,
		DeviceIP:              h.config.DeviceIP,
		SSHUsername:           h.config.SSHUsername,
		Commands:              h.config.Commands,
		Devices:               h.config.Devices,
	}

	// Ensure slices are not nil for consistent JSON
	if settings.Commands == nil {
		settings.Commands = []models.Command{}
	}
	if settings.Devices == nil {
		settings.Devices = []config.Device{}
	}

	writeJSON(w, http.StatusOK, settings)
}

// SyncFetch fetches settings from a master device and returns both for comparison
// POST /api/sync/fetch
func (h *Handlers) SyncFetch(w http.ResponseWriter, r *http.Request) {
	var req SyncFetchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}

	if req.MasterURL == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "masterUrl is required"})
		return
	}

	// Normalize the URL
	masterURL := strings.TrimSuffix(req.MasterURL, "/")
	if !strings.HasPrefix(masterURL, "http://") && !strings.HasPrefix(masterURL, "https://") {
		masterURL = "http://" + masterURL
	}

	// Fetch settings from master device
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(masterURL + "/api/sync/settings")
	if err != nil {
		h.logService.Error("sync", "Failed to connect to master: "+err.Error())
		writeJSON(w, http.StatusBadGateway, ErrorResponse{Error: "failed to connect to master device: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		h.logService.Error("sync", fmt.Sprintf("Master returned status %d: %s", resp.StatusCode, string(body)))
		writeJSON(w, http.StatusBadGateway, ErrorResponse{Error: fmt.Sprintf("master device returned status %d", resp.StatusCode)})
		return
	}

	var masterSettings SyncableSettings
	if err := json.NewDecoder(resp.Body).Decode(&masterSettings); err != nil {
		h.logService.Error("sync", "Failed to parse master response: "+err.Error())
		writeJSON(w, http.StatusBadGateway, ErrorResponse{Error: "failed to parse master device response"})
		return
	}

	// Get local settings
	localSettings := SyncableSettings{
		DeviceName:            h.config.DeviceName,
		DeviceDescription:     h.config.DeviceDescription,
		DeviceLocation:        h.config.DeviceLocation,
		EndpointsPath:         h.config.EndpointsPath,
		ActiveSymlink:         h.config.ActiveSymlink,
		PostActivationCommand: h.config.PostActivationCommand,
		DeviceIP:              h.config.DeviceIP,
		SSHUsername:           h.config.SSHUsername,
		Commands:              h.config.Commands,
		Devices:               h.config.Devices,
	}

	// Ensure slices are not nil
	if localSettings.Commands == nil {
		localSettings.Commands = []models.Command{}
	}
	if localSettings.Devices == nil {
		localSettings.Devices = []config.Device{}
	}
	if masterSettings.Commands == nil {
		masterSettings.Commands = []models.Command{}
	}
	if masterSettings.Devices == nil {
		masterSettings.Devices = []config.Device{}
	}

	h.logService.Info("sync", fmt.Sprintf("Fetched settings from master: %s", masterURL))

	writeJSON(w, http.StatusOK, SyncFetchResponse{
		Master: masterSettings,
		Local:  localSettings,
	})
}

// SyncApply applies the selected settings from the sync wizard
// POST /api/sync/apply
func (h *Handlers) SyncApply(w http.ResponseWriter, r *http.Request) {
	var req SyncApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}

	needsReload := false
	changesApplied := 0

	// Apply device info if provided
	if req.DeviceName != nil {
		h.config.DeviceName = *req.DeviceName
		changesApplied++
	}
	if req.DeviceDescription != nil {
		h.config.DeviceDescription = *req.DeviceDescription
		changesApplied++
	}
	if req.DeviceLocation != nil {
		h.config.DeviceLocation = *req.DeviceLocation
		changesApplied++
	}

	// Apply paths if provided (these may need reload)
	if req.EndpointsPath != nil {
		h.config.EndpointsPath = *req.EndpointsPath
		needsReload = true
		changesApplied++
	}
	if req.ActiveSymlink != nil {
		h.config.ActiveSymlink = *req.ActiveSymlink
		needsReload = true
		changesApplied++
	}
	if req.PostActivationCommand != nil {
		h.config.PostActivationCommand = *req.PostActivationCommand
		changesApplied++
	}

	// Apply network settings if provided
	if req.DeviceIP != nil {
		h.config.DeviceIP = *req.DeviceIP
		changesApplied++
	}
	if req.SSHUsername != nil {
		h.config.SSHUsername = *req.SSHUsername
		changesApplied++
	}

	// Apply commands if provided
	if req.Commands != nil {
		h.config.Commands = req.Commands
		changesApplied++
	}

	// Apply devices registry if provided
	if req.Devices != nil {
		h.config.Devices = req.Devices
		changesApplied++
	}

	// Save configuration to file
	if err := h.config.Save(); err != nil {
		h.logService.Error("sync", "Failed to save config: "+err.Error())
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "failed to save configuration: " + err.Error()})
		return
	}

	h.logService.Info("sync", fmt.Sprintf("Applied %d settings from sync", changesApplied))

	writeJSON(w, http.StatusOK, SyncApplyResponse{
		Success:     true,
		NeedsReload: needsReload,
		Message:     fmt.Sprintf("Successfully applied %d settings", changesApplied),
	})
}

// Backup API handlers

type UpdateBackupPatternsRequest struct {
	Patterns []string `json:"patterns"`
}

type BackupPreviewResponse struct {
	EndpointName string               `json:"endpoint_name"`
	Patterns     []string             `json:"patterns"`
	Files        []BackupFileResponse `json:"files"`
	TotalSize    int64                `json:"total_size"`
	FileCount    int                  `json:"file_count"`
}

type BackupFileResponse struct {
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	ModTime   string `json:"mod_time"`
	MatchedBy string `json:"matched_by"`
}

type BackupResultResponse struct {
	EndpointName string `json:"endpoint_name"`
	ArchivePath  string `json:"archive_path"`
	ArchiveName  string `json:"archive_name"`
	FileCount    int    `json:"file_count"`
	TotalSize    int64  `json:"total_size"`
	CreatedAt    string `json:"created_at"`
	DownloadURL  string `json:"download_url"`
}

// GetBackupPatterns returns the backup patterns for an endpoint
// GET /api/endpoints/{name}/backup/patterns
func (h *Handlers) GetBackupPatterns(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	patterns, err := h.endpointService.GetBackupPatterns(name)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string][]string{"patterns": patterns})
}

// UpdateBackupPatterns updates the backup patterns for an endpoint
// PUT /api/endpoints/{name}/backup/patterns
func (h *Handlers) UpdateBackupPatterns(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var req UpdateBackupPatternsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}

	if err := h.endpointService.UpdateBackupPatterns(name, req.Patterns); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// PreviewBackup returns a list of files that would be backed up
// GET /api/endpoints/{name}/backup/preview
func (h *Handlers) PreviewBackup(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	// Get stored patterns for this endpoint
	patterns, err := h.endpointService.GetBackupPatterns(name)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}

	if len(patterns) == 0 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "no backup patterns configured"})
		return
	}

	preview, err := h.backupService.Preview(name, patterns)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	// Convert to response type
	files := make([]BackupFileResponse, len(preview.Files))
	for i, f := range preview.Files {
		files[i] = BackupFileResponse{
			Path:      f.Path,
			Size:      f.Size,
			ModTime:   f.ModTime,
			MatchedBy: f.MatchedBy,
		}
	}

	writeJSON(w, http.StatusOK, BackupPreviewResponse{
		EndpointName: preview.EndpointName,
		Patterns:     preview.Patterns,
		Files:        files,
		TotalSize:    preview.TotalSize,
		FileCount:    preview.FileCount,
	})
}

// CreateBackup creates a backup archive
// POST /api/endpoints/{name}/backup
func (h *Handlers) CreateBackup(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	// Get stored patterns for this endpoint
	patterns, err := h.endpointService.GetBackupPatterns(name)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}

	if len(patterns) == 0 {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "no backup patterns configured"})
		return
	}

	result, err := h.backupService.CreateBackup(name, patterns)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, BackupResultResponse{
		EndpointName: result.EndpointName,
		ArchivePath:  result.ArchivePath,
		ArchiveName:  result.ArchiveName,
		FileCount:    result.FileCount,
		TotalSize:    result.TotalSize,
		CreatedAt:    result.CreatedAt.Format("2006-01-02 15:04:05"),
		DownloadURL:  fmt.Sprintf("/api/backups/%s/download", result.ArchiveName),
	})
}

// ListBackups returns all backups for an endpoint
// GET /api/endpoints/{name}/backups
func (h *Handlers) ListBackups(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	backups, err := h.backupService.ListBackups(name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, backups)
}

// ListAllBackups returns all backups across all endpoints
// GET /api/backups
func (h *Handlers) ListAllBackups(w http.ResponseWriter, r *http.Request) {
	backups, err := h.backupService.ListAllBackups()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, backups)
}

// DownloadBackup serves a backup archive for download
// GET /api/backups/{archive}/download
func (h *Handlers) DownloadBackup(w http.ResponseWriter, r *http.Request) {
	archiveName := chi.URLParam(r, "archive")

	archivePath, err := h.backupService.GetBackupPath(archiveName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", archiveName))
	w.Header().Set("Content-Type", "application/gzip")
	http.ServeFile(w, r, archivePath)
}

// DeleteBackup deletes a backup archive
// DELETE /api/backups/{archive}
func (h *Handlers) DeleteBackup(w http.ResponseWriter, r *http.Request) {
	archiveName := chi.URLParam(r, "archive")

	if err := h.backupService.DeleteBackup(archiveName); err != nil {
		status := http.StatusNotFound
		if strings.Contains(err.Error(), "failed to delete") {
			status = http.StatusInternalServerError
		}
		writeJSON(w, status, ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
