package mock_test

import (
	"context"
	"errors"
	"testing"

	"github.com/glacius-labs/captain-compose/internal/adapter/mock"
	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
	"github.com/stretchr/testify/assert"
)

func TestRuntime_Deploy_RecordsCallAndReturnsError(t *testing.T) {
	mockRuntime := &mock.Runtime{
		DeployErr: errors.New("deploy error"),
	}

	ctx := context.TODO()
	d := deployment.Deployment{Name: "my-app"}
	payload := []byte("compose")

	err := mockRuntime.Deploy(ctx, d, payload)

	assert.EqualError(t, err, "deploy error")
	assert.Len(t, mockRuntime.DeployCalls, 1)
	call := mockRuntime.DeployCalls[0]
	assert.Equal(t, ctx, call.Ctx)
	assert.Equal(t, d, call.Deploy)
	assert.Equal(t, payload, call.Payload)
}

func TestRuntime_Remove_RecordsCallAndReturnsError(t *testing.T) {
	mockRuntime := &mock.Runtime{
		RemoveErr: errors.New("remove error"),
	}

	ctx := context.TODO()
	name := "frontend"

	err := mockRuntime.Remove(ctx, name)

	assert.EqualError(t, err, "remove error")
	assert.Len(t, mockRuntime.RemoveCalls, 1)
	call := mockRuntime.RemoveCalls[0]
	assert.Equal(t, ctx, call.Ctx)
	assert.Equal(t, name, call.Name)
}

func TestRuntime_List_ReturnsNil(t *testing.T) {
	mockRuntime := &mock.Runtime{}
	result, err := mockRuntime.List(context.TODO())

	assert.NoError(t, err)
	assert.Nil(t, result)
}
