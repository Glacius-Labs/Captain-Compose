package app_test

import (
	"testing"

	"github.com/glacius-labs/captain-compose/internal/adapter/mock"
	"github.com/glacius-labs/captain-compose/internal/app"
	"github.com/stretchr/testify/assert"
)

func TestNewApp_WiresDependenciesCorrectly(t *testing.T) {
	runtime := &mock.Runtime{}
	publisher := &mock.Publisher{}

	a := app.New(runtime, publisher)

	assert.NotNil(t, a)
	assert.NotNil(t, a.Deployment.Create)
	assert.NotNil(t, a.Deployment.Remove)
}
