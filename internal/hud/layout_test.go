package hud

import "testing"

func TestWaveformLayoutClearsLabelAndPin(t *testing.T) {
	const (
		labelWidth = 101
		waveRight  = 210
		barCount   = 12
	)

	left, step, visible := waveformLayout(labelWidth, waveRight, barCount)
	if !visible {
		t.Fatal("waveform should fit")
	}
	if left < labelLeft+labelWidth+labelWaveGap {
		t.Fatalf("waveform starts at %d, before label clearance %d", left, labelLeft+labelWidth+labelWaveGap)
	}
	if right := left + (barCount-1)*step + waveBarWidth; right > waveRight {
		t.Fatalf("waveform ends at %d, beyond protected boundary %d", right, waveRight)
	}
}

func TestWaveformLayoutHidesWhenLabelConsumesSpace(t *testing.T) {
	_, _, visible := waveformLayout(181, 210, 12)
	if visible {
		t.Fatal("waveform should hide instead of overlapping a long label")
	}
}
