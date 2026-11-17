package store

import (
	"context"
	"errors"
)

// ErrRegistrationClosed indicates that automatic account creation is disabled.
var ErrRegistrationClosed = errors.New("registration closed")

// RegistrationAllowed returns the policy supplied by configcenter.
func (s *Store) RegistrationAllowed(_ context.Context) (bool, error) {
	return s.registrationAllowed, nil
}

// ConfigureRegistration sets the startup policy from the configuration snapshot.
func (s *Store) ConfigureRegistration(allowed bool) { s.registrationAllowed = allowed }
