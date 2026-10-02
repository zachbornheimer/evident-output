// Package fixture is the hermetic stand-in for the homelab container layer.
package fixture

import "context"

// ContainerNames lists the containers in the stack.
func ContainerNames() []string { return []string{"homeassistant", "mosquitto"} }

// Current reports whether the container runs with its current config.
func Current(context.Context, string) (bool, error) { return false, nil }

// Down stops and removes the container.
func Down(context.Context, string) error { return nil }

// Config regenerates the container's config.
func Config(context.Context, string) error { return nil }

// Up starts the container.
func Up(context.Context, string) error { return nil }
