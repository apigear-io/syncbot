package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"syncbot/services"
)

// LoginRequest represents a terminal login request
// With browser-based auth, the client sends the password hash from localStorage
type LoginRequest struct {
	Username     string `json:"username"`
	PasswordHash string `json:"passwordHash"`
}

// LoginResponse represents a successful login response
type LoginResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

// SessionResponse represents a session validation response
type SessionResponse struct {
	Valid    bool   `json:"valid"`
	Username string `json:"username,omitempty"`
}

// TerminalPage renders the terminal page
func (h *Handlers) TerminalPage(w http.ResponseWriter, r *http.Request) {
	data := h.defaultPageData("terminal", "Terminal")
	data.Commands = h.commandService.List()
	h.renderPage(w, "terminal", data)
}

// TerminalLogin handles terminal authentication
func (h *Handlers) TerminalLogin(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get client IP for rate limiting
	ip := getClientIP(r)

	token, expiresAt, err := h.authService.Login(req.Username, req.PasswordHash, ip)
	if err != nil {
		switch err {
		case services.ErrInvalidCredentials:
			h.jsonError(w, "Invalid credentials", http.StatusUnauthorized)
		case services.ErrRateLimited:
			h.jsonError(w, "Too many failed attempts, please try again later", http.StatusTooManyRequests)
		default:
			h.jsonError(w, "Authentication failed", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(LoginResponse{
		Token:     token,
		ExpiresAt: expiresAt.Unix(),
	})
}

// TerminalLogout handles terminal logout
func (h *Handlers) TerminalLogout(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r)
	if token == "" {
		h.jsonError(w, "Missing token", http.StatusUnauthorized)
		return
	}

	h.authService.Logout(token)
	h.termService.CloseSession(token)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// TerminalSession validates a session token
func (h *Handlers) TerminalSession(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r)
	if token == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SessionResponse{Valid: false})
		return
	}

	session, err := h.authService.ValidateSession(token)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SessionResponse{Valid: false})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SessionResponse{
		Valid:    true,
		Username: session.Username,
	})
}

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for reverse proxies)
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}

	// Check X-Real-IP header
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return strings.Split(r.RemoteAddr, ":")[0]
}

// extractToken extracts the token from Authorization header or query param
func extractToken(r *http.Request) string {
	// Check Authorization header
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}

	// Check query parameter
	return r.URL.Query().Get("token")
}
