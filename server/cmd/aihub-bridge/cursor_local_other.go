//go:build !windows && !darwin && !linux

package main

import "errors"

var errCursorUnavailable = errors.New("cursor local auth unavailable on this platform")

func cursorAuthToken() (string, error) { return "", errCursorUnavailable }
func cursorPlanHint() string             { return "" }
func cursorAvailable() bool              { return false }
