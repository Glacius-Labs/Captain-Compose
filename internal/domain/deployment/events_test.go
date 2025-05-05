package deployment_test

import (
	"errors"
	"testing"
	"time"

	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
	"github.com/stretchr/testify/assert"
)

func TestNewCreatedEvent(t *testing.T) {
	name := "my-app"
	event := deployment.NewCreatedEvent(name)

	assert.Equal(t, deployment.ActionCreate, event.Action)
	assert.Equal(t, name, event.Name)
	assert.True(t, event.Success)
	assert.Contains(t, event.Message, name)
	assert.NotZero(t, event.ID)
	assert.WithinDuration(t, time.Now(), event.Timestamp, time.Second)
	assert.Nil(t, event.Labels)
}

func TestNewRemovedEvent(t *testing.T) {
	name := "web"
	event := deployment.NewRemovedEvent(name)

	assert.Equal(t, deployment.ActionDelete, event.Action)
	assert.Equal(t, name, event.Name)
	assert.True(t, event.Success)
	assert.Contains(t, event.Message, name)
	assert.NotZero(t, event.ID)
	assert.WithinDuration(t, time.Now(), event.Timestamp, time.Second)
	assert.Nil(t, event.Labels)
}

func TestNewCreationFailedEvent(t *testing.T) {
	name := "backend"
	cause := errors.New("invalid config")
	event := deployment.NewCreationFailedEvent(name, cause)

	assert.Equal(t, deployment.ActionCreate, event.Action)
	assert.Equal(t, name, event.Name)
	assert.False(t, event.Success)
	assert.Contains(t, event.Message, "Failed to create")
	assert.Contains(t, event.Message, cause.Error())
	assert.Equal(t, cause.Error(), event.Labels["error"])
	assert.NotZero(t, event.ID)
	assert.WithinDuration(t, time.Now(), event.Timestamp, time.Second)
}

func TestNewRemovalFailedEvent(t *testing.T) {
	name := "frontend"
	cause := errors.New("network issue")
	event := deployment.NewRemovalFailedEvent(name, cause)

	assert.Equal(t, deployment.ActionDelete, event.Action)
	assert.Equal(t, name, event.Name)
	assert.False(t, event.Success)
	assert.Contains(t, event.Message, "Failed to remove")
	assert.Contains(t, event.Message, cause.Error())
	assert.Equal(t, cause.Error(), event.Labels["error"])
	assert.NotZero(t, event.ID)
	assert.WithinDuration(t, time.Now(), event.Timestamp, time.Second)
}
