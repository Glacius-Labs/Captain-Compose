package shared_test

import (
	"errors"
	"testing"

	"github.com/glacius-labs/captain-compose/internal/app/deployment/shared"
	"github.com/stretchr/testify/assert"
)

func TestNewPublishEventFailed_ReturnsWrappedError(t *testing.T) {
	root := errors.New("mqtt unavailable")
	err := shared.NewPublishEventFailed(root)

	assert.NotNil(t, err)
	assert.ErrorIs(t, err, root)
}

func TestPublishEventFailed_ErrorMethodFormatsMessage(t *testing.T) {
	root := errors.New("timeout")
	err := shared.NewPublishEventFailed(root)

	assert.Equal(t, "failed to publish event: timeout", err.Error())
}
