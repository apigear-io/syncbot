package services

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"syncbot/config"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrRateLimited        = errors.New("too many failed attempts, please try again later")
	ErrSessionExpired     = errors.New("session expired")
	ErrSessionNotFound    = errors.New("session not found")
)

// Session represents an authenticated terminal session
type Session struct {
	Token      string
	Username   string
	CreatedAt  time.Time
	LastActive time.Time
	ExpiresAt  time.Time
}

// AuthService handles terminal authentication and session management
type AuthService struct {
	mu            sync.RWMutex
	cfg           *config.Config
	sessions      map[string]*Session
	loginAttempts map[string]int       // IP -> failed attempts
	loginLockouts map[string]time.Time // IP -> lockout until
	logSvc        *LogService

	sessionTimeout  time.Duration
	maxAttempts     int
	lockoutDuration time.Duration
}

// NewAuthService creates a new authentication service
func NewAuthService(cfg *config.Config, logSvc *LogService) *AuthService {
	svc := &AuthService{
		cfg:             cfg,
		sessions:        make(map[string]*Session),
		loginAttempts:   make(map[string]int),
		loginLockouts:   make(map[string]time.Time),
		logSvc:          logSvc,
		sessionTimeout:  15 * time.Minute,
		maxAttempts:     5,
		lockoutDuration: 15 * time.Minute,
	}

	// Start session cleanup goroutine
	go svc.cleanupSessions()

	return svc
}

// Login validates credentials and returns a session token
// With browser-based auth, the client sends username and password hash from localStorage
func (a *AuthService) Login(username, passwordHash, ip string) (string, time.Time, error) {
	// Check rate limiting
	if err := a.checkRateLimit(ip); err != nil {
		return "", time.Time{}, err
	}

	// Validate credentials - username and hash must both be non-empty
	if username == "" || passwordHash == "" {
		a.recordFailedLogin(ip)
		a.logSvc.Warn("auth", "Failed login attempt: missing credentials")
		return "", time.Time{}, ErrInvalidCredentials
	}

	// Clear failed attempts on successful login
	a.clearFailedAttempts(ip)

	// Generate session token
	token, err := generateToken()
	if err != nil {
		return "", time.Time{}, err
	}

	// Create session
	now := time.Now()
	expiresAt := now.Add(a.sessionTimeout)
	session := &Session{
		Token:      token,
		Username:   username,
		CreatedAt:  now,
		LastActive: now,
		ExpiresAt:  expiresAt,
	}

	a.mu.Lock()
	a.sessions[token] = session
	a.mu.Unlock()

	a.logSvc.Info("auth", "User logged in: "+username)

	return token, expiresAt, nil
}

// ValidateSession checks if a session token is valid and updates last active time
func (a *AuthService) ValidateSession(token string) (*Session, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	session, ok := a.sessions[token]
	if !ok {
		return nil, ErrSessionNotFound
	}

	now := time.Now()

	// Check if session has expired
	if now.After(session.ExpiresAt) {
		delete(a.sessions, token)
		return nil, ErrSessionExpired
	}

	// Update last active and extend expiration
	session.LastActive = now
	session.ExpiresAt = now.Add(a.sessionTimeout)

	return session, nil
}

// Logout invalidates a session
func (a *AuthService) Logout(token string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if session, ok := a.sessions[token]; ok {
		a.logSvc.Info("auth", "User logged out: "+session.Username)
		delete(a.sessions, token)
	}
}

// RefreshSession extends the session expiration time
func (a *AuthService) RefreshSession(token string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	session, ok := a.sessions[token]
	if !ok {
		return ErrSessionNotFound
	}

	now := time.Now()
	if now.After(session.ExpiresAt) {
		delete(a.sessions, token)
		return ErrSessionExpired
	}

	session.LastActive = now
	session.ExpiresAt = now.Add(a.sessionTimeout)

	return nil
}

// checkRateLimit checks if an IP is rate limited
func (a *AuthService) checkRateLimit(ip string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if lockoutUntil, ok := a.loginLockouts[ip]; ok {
		if time.Now().Before(lockoutUntil) {
			return ErrRateLimited
		}
		// Lockout expired, clear it
		delete(a.loginLockouts, ip)
		delete(a.loginAttempts, ip)
	}

	return nil
}

// recordFailedLogin records a failed login attempt
func (a *AuthService) recordFailedLogin(ip string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.loginAttempts[ip]++
	if a.loginAttempts[ip] >= a.maxAttempts {
		a.loginLockouts[ip] = time.Now().Add(a.lockoutDuration)
		a.logSvc.Warn("auth", "IP locked out due to failed login attempts: "+ip)
	}
}

// clearFailedAttempts clears failed login attempts for an IP
func (a *AuthService) clearFailedAttempts(ip string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.loginAttempts, ip)
	delete(a.loginLockouts, ip)
}

// cleanupSessions periodically removes expired sessions
func (a *AuthService) cleanupSessions() {
	ticker := time.NewTicker(1 * time.Minute)
	for range ticker.C {
		a.mu.Lock()
		now := time.Now()
		for token, session := range a.sessions {
			if now.After(session.ExpiresAt) {
				a.logSvc.Debug("auth", "Session expired for user: "+session.Username)
				delete(a.sessions, token)
			}
		}
		a.mu.Unlock()
	}
}

// generateToken creates a secure random token
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
