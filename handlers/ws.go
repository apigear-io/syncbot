package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow connections from any origin for local use
	},
}

// WSMessage represents a WebSocket message
type WSMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// ResizePayload contains terminal resize dimensions
type ResizePayload struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// TerminalWS handles WebSocket connections for the terminal
func (h *Handlers) TerminalWS(w http.ResponseWriter, r *http.Request) {
	// Validate token from query parameter
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Missing token", http.StatusUnauthorized)
		return
	}

	session, err := h.authService.ValidateSession(token)
	if err != nil {
		http.Error(w, "Invalid or expired session", http.StatusUnauthorized)
		return
	}

	// Get terminal dimensions from query params
	cols := parseUint16(r.URL.Query().Get("cols"), 80)
	rows := parseUint16(r.URL.Query().Get("rows"), 24)

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logService.Error("terminal", "WebSocket upgrade failed: "+err.Error())
		return
	}
	defer conn.Close()

	// Get or create terminal session (reconnects to existing if available)
	termSession, reconnected, err := h.termService.GetOrCreateSession(token, session.Username, cols, rows)
	if err != nil {
		h.logService.Error("terminal", "Failed to create terminal session: "+err.Error())
		conn.WriteJSON(map[string]string{"error": err.Error()})
		return
	}

	// On disconnect, just mark as disconnected (don't close the session)
	defer h.termService.DisconnectSession(token)

	// If reconnecting, send buffered output first
	if reconnected {
		h.logService.Info("terminal", "Reconnected to existing session: "+token[:8]+"...")
		bufferedOutput := termSession.OutputBuffer.Bytes()
		if len(bufferedOutput) > 0 {
			// Send a clear screen first, then the buffered content
			conn.WriteMessage(websocket.BinaryMessage, []byte("\x1b[2J\x1b[H")) // Clear screen, cursor home
			conn.WriteMessage(websocket.BinaryMessage, bufferedOutput)
		}
		// Resize to current dimensions
		h.termService.ResizeSession(token, cols, rows)
	}

	// Send session info to client
	conn.WriteJSON(map[string]interface{}{
		"type":        "session_info",
		"session_id":  token[:8],
		"reconnected": reconnected,
	})

	// Channel to signal when to stop
	done := make(chan struct{})

	// Read from PTY and write to WebSocket
	go func() {
		buf := make([]byte, 4096)
		for {
			select {
			case <-done:
				return
			case <-termSession.Done:
				conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Terminal exited"))
				return
			default:
				n, err := termSession.PTY.Read(buf)
				if err != nil {
					if err != io.EOF {
						h.logService.Debug("terminal", "PTY read error: "+err.Error())
					}
					return
				}
				if n > 0 {
					h.termService.UpdateActivity(token)
					// Buffer the output for potential reconnection
					termSession.OutputBuffer.Write(buf[:n])
					if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
						return
					}
				}
			}
		}
	}()

	// Read from WebSocket and write to PTY
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				h.logService.Debug("terminal", "WebSocket closed: "+err.Error())
			}
			break
		}

		// Handle different message types
		if messageType == websocket.TextMessage {
			// JSON control message
			var msg WSMessage
			if err := json.Unmarshal(message, &msg); err != nil {
				continue
			}

			switch msg.Type {
			case "resize":
				var resize ResizePayload
				if err := json.Unmarshal(msg.Payload, &resize); err != nil {
					continue
				}
				h.termService.ResizeSession(token, resize.Cols, resize.Rows)

			case "ping":
				h.termService.UpdateActivity(token)
				h.authService.RefreshSession(token)
			}
		} else if messageType == websocket.BinaryMessage {
			// Terminal input
			h.termService.UpdateActivity(token)
			h.authService.RefreshSession(token)
			if _, err := termSession.PTY.Write(message); err != nil {
				h.logService.Debug("terminal", "PTY write error: "+err.Error())
				break
			}
		}
	}

	close(done)
}

// parseUint16 parses a string to uint16 with a default value
func parseUint16(s string, defaultVal uint16) uint16 {
	if s == "" {
		return defaultVal
	}
	var val uint16
	if err := json.Unmarshal([]byte(s), &val); err != nil {
		return defaultVal
	}
	return val
}
