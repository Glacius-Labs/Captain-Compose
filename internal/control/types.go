// Package control defines the versioned operator contract shared by the agent and CLI.
package control

import (
	"context"
	"errors"
	"time"
)

const Version = 2

// RequestID is supplied by the validated envelope, never by the data object.
type Request struct {
	Name             string  `json:"name,omitempty"`
	Payload          []byte  `json:"payload,omitempty"`
	ExpectedRevision *string `json:"expected_revision,omitempty"`
	Revision         string  `json:"revision,omitempty"`
	RequestID        string  `json:"-"`
	ResultID         string  `json:"request_id,omitempty"`
	AllowDataRisk    bool    `json:"allow_data_risk,omitempty"`
}

// Runtime executes v2 operations; queries must not expose manifests or credentials.
type Runtime interface {
	Execute(context.Context, string, Request) (any, error)
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string           { return e.Message }
func Failure(code, message string) error { return &Error{Code: code, Message: message} }
func ErrorCode(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	return "operation_failed"
}

type Service struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Health string `json:"health,omitempty"`
	Image  string `json:"image,omitempty"`
}

type DeploymentStatus struct {
	Name               string     `json:"name"`
	DesiredRevision    string     `json:"desired_revision,omitempty"`
	SuccessfulRevision string     `json:"successful_revision,omitempty"`
	LastRequestID      string     `json:"last_request_id,omitempty"`
	Phase              string     `json:"phase"`
	ObservedAt         time.Time  `json:"observed_at"`
	Services           []Service  `json:"services"`
	Drift              bool       `json:"drift"`
	Warnings           []string   `json:"warnings,omitempty"`
	Revisions          []Revision `json:"revisions,omitempty"`
}

// Revision is safe history metadata; no Compose content is exposed to queries.
type Revision struct {
	Revision  string    `json:"revision"`
	CreatedAt time.Time `json:"created_at"`
}

type Plan struct {
	Name            string   `json:"name"`
	CurrentRevision string   `json:"current_revision"`
	Revision        string   `json:"revision"`
	Added           []string `json:"added"`
	Changed         []string `json:"changed"`
	Removed         []string `json:"removed"`
	Warnings        []string `json:"warnings,omitempty"`
}

type Check struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}
