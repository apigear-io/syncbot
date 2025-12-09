package cmd

import (
	"fmt"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/spf13/cobra"

	"syncbot/config"
	"syncbot/handlers"
	"syncbot/services"
)

var (
	// Version information set by build flags
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"

	// Flags
	configPath string
)

var rootCmd = &cobra.Command{
	Use:   "syncbot",
	Short: "SyncBot - Endpoint sync manager for test devices",
	Long: `SyncBot is a web application for managing software deployment endpoints
on test devices. It allows teams to rsync software releases to named
endpoints on a device, then activate specific endpoints via a web UI.`,
	Run: runServer,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "config.yaml", "path to configuration file")

	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("SyncBot %s\n", Version)
		fmt.Printf("  Commit:     %s\n", Commit)
		fmt.Printf("  Build Date: %s\n", BuildDate)
	},
}

func runServer(cmd *cobra.Command, args []string) {
	// Initialize logging service first
	logSvc := services.NewLogService()

	logSvc.Info("startup", "Loading configuration from "+configPath)

	cfg, err := config.Load(configPath)
	if err != nil {
		logSvc.Error("startup", "Failed to load config: "+err.Error())
		os.Exit(1)
	}

	if err := os.MkdirAll(cfg.EndpointsPath, 0755); err != nil {
		logSvc.Error("startup", "Failed to create endpoints directory: "+err.Error())
		os.Exit(1)
	}

	eventSvc := services.NewEventService(logSvc)
	processSvc := services.NewProcessService(cfg, logSvc, eventSvc)
	activationSvc := services.NewActivationService(cfg, logSvc, eventSvc, processSvc)
	endpointSvc := services.NewEndpointService(cfg, activationSvc, logSvc, eventSvc)
	commandSvc := services.NewCommandService(cfg, logSvc, eventSvc, activationSvc)
	backupSvc, err := services.NewBackupService(cfg, logSvc, eventSvc)
	if err != nil {
		logSvc.Error("startup", "Failed to initialize backup service: "+err.Error())
		os.Exit(1)
	}
	authSvc := services.NewAuthService(cfg, logSvc)
	termSvc := services.NewTerminalService(cfg, authSvc, logSvc)

	h, err := handlers.NewHandlers(cfg, endpointSvc, activationSvc, commandSvc, backupSvc, logSvc, eventSvc, authSvc, termSvc, processSvc)
	if err != nil {
		logSvc.Error("startup", "Failed to initialize handlers: "+err.Error())
		os.Exit(1)
	}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(logSvc))

	// Static assets
	r.Handle("/static/*", http.StripPrefix("/static/", handlers.StaticHandler()))

	// Page routes
	r.Get("/", h.Index)
	r.Get("/endpoints", h.EndpointsPage)
	r.Get("/active", h.ActivePage)
	r.Get("/devices", h.DevicesPage)
	r.Get("/commands", h.CommandsPage)
	r.Get("/logs", h.LogsPage)
	r.Get("/settings", h.SettingsPage)
	r.Get("/sync", h.SyncPage)
	r.Get("/terminal", h.TerminalPage)
	r.Get("/backups", h.BackupsPage)

	// API routes
	r.Route("/api", func(r chi.Router) {
		r.Get("/endpoints", h.ListEndpoints)
		r.Post("/endpoints", h.CreateEndpoint)
		r.Put("/endpoints/{name}", h.UpdateEndpoint)
		r.Delete("/endpoints/{name}", h.DeleteEndpoint)
		r.Post("/endpoints/{name}/activate", h.ActivateEndpoint)
		r.Put("/endpoints/{name}/build-info-path", h.UpdateEndpointBuildInfoPath)
		r.Get("/endpoints/{name}/build-info", h.GetEndpointBuildInfo)
		r.Get("/active", h.GetActive)
		r.Get("/config", h.GetConfig)
		r.Get("/settings", h.GetSettings)
		r.Put("/settings", h.UpdateSettings)
		r.Get("/logs", h.GetLogs)
		r.Delete("/logs", h.ClearLogs)
		r.Get("/events", h.Events)

		// Process routes
		r.Get("/process", h.GetProcess)
		r.Post("/process/kill", h.KillProcess)
		r.Get("/process/output", h.GetProcessOutput)
		r.Delete("/process/output", h.ClearProcessOutput)

		// Command routes
		r.Get("/commands", h.ListCommands)
		r.Post("/commands", h.CreateCommand)
		r.Get("/commands/vars", h.GetTemplateVars)
		r.Get("/commands/{id}", h.GetCommand)
		r.Put("/commands/{id}", h.UpdateCommand)
		r.Delete("/commands/{id}", h.DeleteCommand)
		r.Post("/commands/{id}/execute", h.ExecuteCommand)

		// Sync routes
		r.Get("/sync/settings", h.GetSyncSettings)
		r.Post("/sync/fetch", h.SyncFetch)
		r.Post("/sync/apply", h.SyncApply)

		// Backup routes
		r.Get("/endpoints/{name}/backup/patterns", h.GetBackupPatterns)
		r.Put("/endpoints/{name}/backup/patterns", h.UpdateBackupPatterns)
		r.Get("/endpoints/{name}/backup/preview", h.PreviewBackup)
		r.Post("/endpoints/{name}/backup", h.CreateBackup)
		r.Get("/endpoints/{name}/backups", h.ListBackups)
		r.Get("/backups", h.ListAllBackups)
		r.Get("/backups/{archive}/download", h.DownloadBackup)
		r.Delete("/backups/{archive}", h.DeleteBackup)

		// Terminal routes
		r.Post("/terminal/auth", h.TerminalLogin)
		r.Post("/terminal/logout", h.TerminalLogout)
		r.Get("/terminal/session", h.TerminalSession)
		r.Get("/terminal/ws", h.TerminalWS)
	})

	addr := fmt.Sprintf(":%d", cfg.Port)
	logSvc.Info("startup", fmt.Sprintf("SyncBot %s starting on http://localhost%s", Version, addr))
	logSvc.Info("startup", fmt.Sprintf("Endpoints path: %s", cfg.EndpointsPath))
	logSvc.Info("startup", fmt.Sprintf("Active symlink: %s", cfg.ActiveSymlink))

	if err := http.ListenAndServe(addr, r); err != nil {
		logSvc.Error("startup", "Server failed: "+err.Error())
		os.Exit(1)
	}
}

// requestLogger creates a middleware that logs HTTP requests
func requestLogger(logSvc *services.LogService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip logging for static assets and frequent polling
			if r.URL.Path != "/api/logs" {
				logSvc.Debug("http", fmt.Sprintf("%s %s", r.Method, r.URL.Path))
			}
			next.ServeHTTP(w, r)
		})
	}
}
