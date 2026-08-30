//go:build desktop && !windows && !linux

package hud

type Overlay struct{}

func New() (*Overlay, error) {
	return &Overlay{}, nil
}

func (*Overlay) SetStatus(string) {}

func (*Overlay) SetLevel(float32) {}

func (*Overlay) SetError() {}

func (*Overlay) Close() {}
