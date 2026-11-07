//go:build desktop && !darwin

package doubletap

import "errors"

// Listen is unavailable outside macOS.
func Listen() (<-chan struct{}, func(), error) {
	return nil, nil, errors.New("double-tap activation is only available on macOS")
}
