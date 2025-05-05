package create_test

import (
	"context"
	"errors"
	"testing"

	"github.com/glacius-labs/captain-compose/internal/adapter/mock"
	"github.com/glacius-labs/captain-compose/internal/app/deployment/create"
	"github.com/glacius-labs/captain-compose/internal/app/deployment/shared"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCommand() create.Command {
	return create.Command{
		Name:    "web",
		Payload: []byte("compose..."),
	}
}

func Test_Handle_DeployFails_And_PublishFails(t *testing.T) {
	runtime := &mock.Runtime{DeployErr: errors.New("deployment failed")}
	publisher := &mock.Publisher{Err: errors.New("publish failed")}

	handler := create.NewHandler(runtime, publisher)
	err := handler.Handle(context.TODO(), newCommand())

	require.Error(t, err)

	var depErr create.DeploymentFailed
	assert.ErrorAs(t, err, &depErr)

	assert.Contains(t, depErr.Error(), "deployment failed")
	assert.Contains(t, depErr.Error(), "publish failed")

	assert.Len(t, publisher.Calls, 1)
	assert.Equal(t, "web", publisher.Calls[0].Event.Name)
}

func Test_Handle_DeployFails_And_PublishSucceeds(t *testing.T) {
	runtime := &mock.Runtime{DeployErr: errors.New("deployment failed")}
	publisher := &mock.Publisher{}

	handler := create.NewHandler(runtime, publisher)
	err := handler.Handle(context.TODO(), newCommand())

	require.Error(t, err)

	var depErr create.DeploymentFailed
	assert.ErrorAs(t, err, &depErr)
	assert.Contains(t, depErr.Error(), "deployment failed")

	assert.Len(t, publisher.Calls, 1)
}

func Test_Handle_DeploySucceeds_And_PublishFails(t *testing.T) {
	runtime := &mock.Runtime{}
	publisher := &mock.Publisher{Err: errors.New("publish failed")}

	handler := create.NewHandler(runtime, publisher)
	err := handler.Handle(context.TODO(), newCommand())

	require.Error(t, err)

	var pubErr shared.PublishEventFailed
	assert.ErrorAs(t, err, &pubErr)
	assert.Contains(t, pubErr.Error(), "publish failed")

	assert.Len(t, publisher.Calls, 1)
}

func Test_Handle_DeploySucceeds_And_PublishSucceeds(t *testing.T) {
	runtime := &mock.Runtime{}
	publisher := &mock.Publisher{}

	handler := create.NewHandler(runtime, publisher)
	err := handler.Handle(context.TODO(), newCommand())

	assert.NoError(t, err)
	assert.Len(t, publisher.Calls, 1)
}
