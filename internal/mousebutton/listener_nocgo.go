//go:build desktop && !windows && !cgo

package mousebutton

import "fmt"

func Listen(button int) (<-chan struct{}, func(), error) {
	return nil, nil, fmt.Errorf("mouse-button activation requires a native desktop build")
}
