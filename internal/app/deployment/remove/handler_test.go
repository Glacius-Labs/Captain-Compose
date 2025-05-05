package remove_test

import (
	"context"
	"errors"
	"testing"

	"github.com/glacius-labs/captain-compose/internal/adapter/mock"
	"github.com/glacius-labs/captain-compose/internal/app/deployment/remove"
	"github.com/glacius-labs/captain-compose/internal/app/deployment/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCommand() remove.Command {
	return remove.Command{
		Name: "test-service",
	}
}

func Test_Handle_RemoveFails_And_PublishFails(t *testing.T) {
	runtime := &mock.Runtime{RemoveErr: errors.New("remove failed")}
	publisher := &mock.Publisher{Err: errors.New("publish failed")}

	handler := remove.NewHandler(runtime, publisher)
	err := handler.Handle(context.TODO(), newCommand())

	require.Error(t, err)

	var remErr remove.RemovalFailed
	assert.ErrorAs(t, err, &remErr)
	assert.Contains(t, remErr.Error(), "remove failed")
	assert.Contains(t, remErr.Error(), "publish failed")

	assert.Len(t, publisher.Calls, 1)
	assert.Equal(t, "test-service", publisher.Calls[0].Event.Name)
}

func Test_Handle_RemoveFails_And_PublishSucceeds(t *testing.T) {
	runtime := &mock.Runtime{RemoveErr: errors.New("remove failed")}
	publisher := &mock.Publisher{}

	handler := remove.NewHandler(runtime, publisher)
	err := handler.Handle(context.TODO(), newCommand())

	require.Error(t, err)

	var remErr remove.RemovalFailed
	assert.ErrorAs(t, err, &remErr)
	assert.Contains(t, remErr.Error(), "remove failed")

	assert.Len(t, publisher.Calls, 1)
}

func Test_Handle_RemoveSucceeds_And_PublishFails(t *testing.T) {
	runtime := &mock.Runtime{}
	publisher := &mock.Publisher{Err: errors.New("publish failed")}

	handler := remove.NewHandler(runtime, publisher)
	err := handler.Handle(context.TODO(), newCommand())

	require.Error(t, err)

	var pubErr shared.PublishEventFailed
	assert.ErrorAs(t, err, &pubErr)
	assert.Contains(t, pubErr.Error(), "publish failed")

	assert.Len(t, publisher.Calls, 1)
}

func Test_Handle_RemoveSucceeds_And_PublishSucceeds(t *testing.T) {
	runtime := &mock.Runtime{}
	publisher := &mock.Publisher{}

	handler := remove.NewHandler(runtime, publisher)
	err := handler.Handle(context.TODO(), newCommand())

	assert.NoError(t, err)
	assert.Len(t, publisher.Calls, 1)
}
