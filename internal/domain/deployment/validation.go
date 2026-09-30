package deployment

import (
	"fmt"
	"regexp"
)

const MaxPayloadBytes = 1024 * 1024

var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

func ValidateName(name string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("deployment name must contain 1-63 lowercase letters, digits, underscores or hyphens and start with a letter or digit")
	}
	return nil
}

func ValidatePayload(payload []byte) error {
	if len(payload) == 0 || len(payload) > MaxPayloadBytes {
		return fmt.Errorf("compose payload must contain 1-%d bytes", MaxPayloadBytes)
	}
	return nil
}
