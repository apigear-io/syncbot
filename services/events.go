package services

import (
	"encoding/json"
	"log"
	"sync"
)

type EventType string

const (
	EventEndpointCreated   EventType = "endpoint_created"
	EventEndpointDeleted   EventType = "endpoint_deleted"
	EventEndpointActivated EventType = "endpoint_activated"
	EventCommandCreated    EventType = "command_created"
	EventCommandDeleted    EventType = "command_deleted"
	EventCommandExecuted   EventType = "command_executed"
	EventProcessStarted    EventType = "process_started"
	EventProcessOutput     EventType = "process_output"
	EventProcessStopped    EventType = "process_stopped"
)

type Event struct {
	Type    EventType   `json:"type"`
	Payload interface{} `json:"payload,omitempty"`
}

type EventService struct {
	mu        sync.RWMutex
	clients   map[chan Event]struct{}
	logSvc    *LogService
}

func NewEventService(logSvc *LogService) *EventService {
	return &EventService{
		clients: make(map[chan Event]struct{}),
		logSvc:  logSvc,
	}
}

// Subscribe adds a new client and returns a channel for receiving events
func (s *EventService) Subscribe() chan Event {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan Event, 10)
	s.clients[ch] = struct{}{}
	s.logSvc.Debug("events", "Client subscribed to SSE")
	return ch
}

// Unsubscribe removes a client
func (s *EventService) Unsubscribe(ch chan Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.clients, ch)
	close(ch)
	s.logSvc.Debug("events", "Client unsubscribed from SSE")
}

// Publish sends an event to all connected clients
func (s *EventService) Publish(event Event) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, _ := json.Marshal(event)
	log.Printf("[events] Publishing: %s", string(data))

	for ch := range s.clients {
		select {
		case ch <- event:
		default:
			// Client buffer full, skip
		}
	}
}

// ClientCount returns the number of connected clients
func (s *EventService) ClientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}
