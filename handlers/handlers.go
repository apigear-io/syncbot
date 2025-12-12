package handlers

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"

	"syncbot/config"
	"syncbot/models"
	"syncbot/services"
)

//go:embed templates/*.html
var templatesFS embed.FS

type Handlers struct {
	config               *config.Config
	endpointService      *services.EndpointService
	activationService    *services.ActivationService
	commandService       *services.CommandService
	backupService        *services.BackupService
	logService           *services.LogService
	eventService         *services.EventService
	authService          *services.AuthService
	termService          *services.TerminalService
	processService       *services.ProcessService
	activationLogService *services.ActivationLogService
	viewTemplates        map[string]*template.Template
}

func NewHandlers(cfg *config.Config, endpointSvc *services.EndpointService, activationSvc *services.ActivationService, commandSvc *services.CommandService, backupSvc *services.BackupService, logSvc *services.LogService, eventSvc *services.EventService, authSvc *services.AuthService, termSvc *services.TerminalService, processSvc *services.ProcessService, activationLogSvc *services.ActivationLogService) (*Handlers, error) {
	// Shared template files
	sharedFiles := []string{
		"templates/base.html",
		"templates/modals.html",
		"templates/drawers.html",
		"templates/shared_js.html",
	}

	// View-specific template files with their help partials
	views := map[string][]string{
		"endpoints":       {"templates/endpoints.html", "templates/_help_endpoints.html"},
		"devices":         {"templates/devices.html", "templates/_help_devices.html"},
		"commands":        {"templates/commands.html", "templates/_help_commands.html"},
		"logs":            {"templates/logs.html", "templates/_help_logs.html"},
		"settings":        {"templates/settings.html", "templates/_help_settings.html"},
		"sync":            {"templates/sync.html", "templates/_help_sync.html"},
		"terminal":        {"templates/terminal.html", "templates/_help_terminal.html"},
		"backups":         {"templates/backups.html", "templates/_help_backups.html"},
		"active":          {"templates/active.html", "templates/_help_active.html"},
		"activation-logs": {"templates/activation-logs.html", "templates/_help_activation_logs.html"},
	}

	viewTemplates := make(map[string]*template.Template)

	for viewName, viewFiles := range views {
		// Combine shared files with view-specific files
		files := append(sharedFiles, viewFiles...)
		tmpl, err := template.ParseFS(templatesFS, files...)
		if err != nil {
			return nil, err
		}
		viewTemplates[viewName] = tmpl
	}

	return &Handlers{
		config:               cfg,
		endpointService:      endpointSvc,
		activationService:    activationSvc,
		commandService:       commandSvc,
		backupService:        backupSvc,
		logService:           logSvc,
		eventService:         eventSvc,
		authService:          authSvc,
		termService:          termSvc,
		processService:       processSvc,
		activationLogService: activationLogSvc,
		viewTemplates:        viewTemplates,
	}, nil
}

// TemplatesFS returns the embedded templates filesystem (for testing)
func TemplatesFS() fs.FS {
	return templatesFS
}

type PageData struct {
	Config                *config.Config
	Title                 string
	ActiveView            string
	DeviceName            string
	DeviceDescription     string
	DeviceLocation        string
	DeviceIP              string
	SSHUsername           string
	EndpointsPath         string
	ActiveSymlink         string
	Port                  int
	PostActivationCommand string
	Commands              []models.Command
	Devices               []config.Device
	Endpoints             []models.Endpoint
	ActiveEndpoint        string
	TemplateVars          models.CommandTemplateVars
}

func (h *Handlers) renderPage(w http.ResponseWriter, view string, data PageData) {
	tmpl, ok := h.viewTemplates[view]
	if !ok {
		http.Error(w, "View not found", http.StatusInternalServerError)
		return
	}

	if err := tmpl.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handlers) defaultPageData(view, title string) PageData {
	return PageData{
		Config:                h.config,
		Title:                 title,
		ActiveView:            view,
		DeviceName:            h.config.DeviceName,
		DeviceDescription:     h.config.DeviceDescription,
		DeviceLocation:        h.config.DeviceLocation,
		DeviceIP:              h.config.DeviceIP,
		SSHUsername:           h.config.SSHUsername,
		EndpointsPath:         h.config.EndpointsPath,
		ActiveSymlink:         h.config.ActiveSymlink,
		Port:                  h.config.Port,
		PostActivationCommand: h.config.PostActivationCommand,
		Devices:               h.config.Devices,
		Commands:              h.config.Commands,
	}
}

func (h *Handlers) Index(w http.ResponseWriter, r *http.Request) {
	// Redirect root to /endpoints
	http.Redirect(w, r, "/endpoints", http.StatusFound)
}

func (h *Handlers) EndpointsPage(w http.ResponseWriter, r *http.Request) {
	data := h.defaultPageData("endpoints", "Endpoints")
	data.Endpoints, _ = h.endpointService.List()
	data.ActiveEndpoint = h.activationService.GetActive()
	h.renderPage(w, "endpoints", data)
}

func (h *Handlers) DevicesPage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, "devices", h.defaultPageData("devices", "Devices"))
}

func (h *Handlers) CommandsPage(w http.ResponseWriter, r *http.Request) {
	data := h.defaultPageData("commands", "Commands")
	data.TemplateVars = h.commandService.GetTemplateVars()
	h.renderPage(w, "commands", data)
}

func (h *Handlers) LogsPage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, "logs", h.defaultPageData("logs", "Logs"))
}

func (h *Handlers) SettingsPage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, "settings", h.defaultPageData("settings", "Settings"))
}

func (h *Handlers) SyncPage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, "sync", h.defaultPageData("sync", "Sync Settings"))
}

func (h *Handlers) BackupsPage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, "backups", h.defaultPageData("backups", "Backups"))
}

func (h *Handlers) ActivePage(w http.ResponseWriter, r *http.Request) {
	data := h.defaultPageData("active", "Active")
	data.ActiveEndpoint = h.activationService.GetActive()
	h.renderPage(w, "active", data)
}

func (h *Handlers) ActivationLogsPage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, "activation-logs", h.defaultPageData("activation-logs", "Activation Logs"))
}
