package store

import (
	"crypto/rand"
	"testing"
)

func TestMigrateTOTPDevicesPreservesReplayState(t *testing.T) {
	dsn := "file:" + rand.Text() + "?mode=memory&cache=shared"
	s := registrationStore(t, dsn)
	legacy := User{Email: "legacy@example.com", Name: "Legacy", Role: RoleUser, TOTPSecret: "encrypted-secret", TOTPStep: 42}
	seedRegistrationRecords(t, s, &legacy)
	for range 2 {
		s = registrationStore(t, dsn)
		devices, err := s.TOTPDevices(t.Context(), legacy.ID)
		if err != nil || len(devices) != 1 {
			t.Fatal("migration duplicated or lost a device", err)
		}
		if devices[0].Secret != legacy.TOTPSecret || devices[0].Step != legacy.TOTPStep {
			t.Fatal("migration changed credentials")
		}
		user, err := s.UserByID(t.Context(), legacy.ID)
		if err != nil || user.TOTPSecret != "" || user.TOTPStep != 0 {
			t.Fatal("legacy secret retained", err)
		}
	}
}
