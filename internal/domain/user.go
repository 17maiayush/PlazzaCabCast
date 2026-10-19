package domain

import (
	"fmt"
	"strings"
	"time"
)

type User struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

func NewUser(name string, now time.Time) (User, error) {
	name, err := requireName(name)
	if err != nil {
		return User{}, err
	}
	return User{ID: NewID("usr"), Name: name, CreatedAt: now}, nil
}

func requireName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("%w: name is required", ErrValidation)
	}
	return name, nil
}
