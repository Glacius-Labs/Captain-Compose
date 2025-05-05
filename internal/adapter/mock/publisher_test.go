package mock_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/glacius-labs/captain-compose/internal/adapter/mock"
	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestPublisher_Publish_RecordsCallAndReturnsError(t *testing.T) {
	expectedErr := errors.New("publish failure")
	p := &mock.Publisher{
		Err: expectedErr,
	}

	event := deployment.Event{
		ID:        uuid.New(),
		Name:      "my-app",
		Action:    deployment.ActionCreate,
		Success:   true,
		Message:   "OK",
		Timestamp: time.Now(),
	}

	ctx := context.TODO()
	err := p.Publish(ctx, event)

	assert.EqualError(t, err, "publish failure")
	assert.Len(t, p.Calls, 1)
	call := p.Calls[0]
	assert.Equal(t, ctx, call.Ctx)
	assert.Equal(t, event, call.Event)
}

func TestPublisher_CalledOnce(t *testing.T) {
	p := &mock.Publisher{}
	assert.False(t, p.CalledOnce())

	p.Publish(context.TODO(), deployment.Event{
		ID:        uuid.New(),
		Name:      "foo",
		Action:    deployment.ActionCreate,
		Timestamp: time.Now(),
	})

	assert.True(t, p.CalledOnce())
}
