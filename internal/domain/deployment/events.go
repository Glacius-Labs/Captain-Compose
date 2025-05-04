package deployment

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Event struct {
	ID        uuid.UUID         `json:"id"`
	Timestamp time.Time         `json:"timestamp"`
	Action    string            `json:"action"`
	Name      string            `json:"name"`
	Success   bool              `json:"success"`
	Message   string            `json:"message"`
	Labels    map[string]string `json:"labels"`
}

const (
	ActionCreate = "create"
	ActionDelete = "delete"
)

func NewCreatedEvent(name string) Event {
	return Event{
		ID:        uuid.New(),
		Timestamp: time.Now(),
		Action:    ActionCreate,
		Name:      name,
		Success:   true,
		Message:   fmt.Sprintf("Deployment %q created successfully", name),
	}
}

func NewRemovedEvent(name string) Event {
	return Event{
		ID:        uuid.New(),
		Timestamp: time.Now(),
		Action:    ActionDelete,
		Name:      name,
		Success:   true,
		Message:   fmt.Sprintf("Deployment %q removed successfully", name),
	}
}

func NewCreationFailedEvent(name string, err error) Event {
	return Event{
		ID:        uuid.New(),
		Timestamp: time.Now(),
		Action:    ActionCreate,
		Name:      name,
		Success:   false,
		Message:   fmt.Sprintf("Failed to create deployment %q: %v", name, err),
		Labels: map[string]string{
			"error": err.Error(),
		},
	}
}

func NewRemovalFailedEvent(name string, err error) Event {
	return Event{
		ID:        uuid.New(),
		Timestamp: time.Now(),
		Action:    ActionDelete,
		Name:      name,
		Success:   false,
		Message:   fmt.Sprintf("Failed to remove deployment %q: %v", name, err),
		Labels: map[string]string{
			"error": err.Error(),
		},
	}
}
