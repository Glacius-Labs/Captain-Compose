package remove_test

import (
	"errors"
	"testing"

	"github.com/glacius-labs/captain-compose/internal/app/deployment/remove"
	"github.com/stretchr/testify/assert"
)

func TestNewRemovalFailed_ReturnsWrappedError(t *testing.T) {
	root := errors.New("not found")
	err := remove.NewRemovalFailed(root)

	assert.NotNil(t, err)
	assert.ErrorIs(t, err, root)
}

func TestRemovalFailed_ErrorMethodFormatsMessage(t *testing.T) {
	root := errors.New("permission denied")
	err := remove.NewRemovalFailed(root)

	assert.Equal(t, "removal failed: permission denied", err.Error())
}
