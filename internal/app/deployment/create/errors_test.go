package create_test

import (
	"errors"
	"testing"

	"github.com/glacius-labs/captain-compose/internal/app/deployment/create"
	"github.com/stretchr/testify/assert"
)

func TestNewDeploymentFailed_ReturnsWrappedError(t *testing.T) {
	root := errors.New("deploy err")
	err := create.NewDeploymentFailed(root)

	assert.NotNil(t, err)
	assert.ErrorIs(t, err, root)
}

func TestDeploymentFailed_ErrorMethodFormatsMessage(t *testing.T) {
	root := errors.New("network issue")
	err := create.NewDeploymentFailed(root)

	assert.Equal(t, "deployment failed: network issue", err.Error())
}

func TestDeploymentFailed_Unwrap(t *testing.T) {
	root := errors.New("root cause")
	err := create.NewDeploymentFailed(root)

	unwrapped := errors.Unwrap(err)
	assert.Equal(t, root, unwrapped)
}
