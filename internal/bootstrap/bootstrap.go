package bootstrap

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"

	"github.com/zivego/wiregate/internal/auth"
	"github.com/zivego/wiregate/internal/persistence/userrepo"
)

// MaybeCreateAdmin inserts the first admin user if no users exist yet.
// It is safe to call on every startup — it is a no-op when users already exist.
// The password is never logged.
func MaybeCreateAdmin(ctx context.Context, users *userrepo.Repo, email, password string, logger *log.Logger) error {
	n, err := users.Count(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap: count users: %w", err)
	}
	if n > 0 {
		logger.Printf("bootstrap: skipped — %d user(s) already exist", n)
		return nil
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("bootstrap: hash password: %w", err)
	}

	id, err := newUUID()
	if err != nil {
		return fmt.Errorf("bootstrap: generate id: %w", err)
	}

	if err := users.Insert(ctx, userrepo.User{
		ID:                 id,
		Email:              email,
		Role:               "admin",
		PasswordHash:       hash,
		MustChangePassword: true,
	}); err != nil {
		return fmt.Errorf("bootstrap: insert admin: %w", err)
	}

	logger.Printf("bootstrap: created admin user %q", email)
	return nil
}

// newUUID generates a random UUID v4 using crypto/rand without external dependencies.
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
