//go:build desktop && !windows

package hud

type Overlay struct{}

func New() (*Overlay, error) {
	return &Overlay{}, nil
}

func (*Overlay) SetStatus(string) {}

func (*Overlay) SetLevel(float32) {}

func (*Overlay) Close() {}
