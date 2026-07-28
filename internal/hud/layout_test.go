package hud

import "testing"

func TestWaveformLayoutFillsLowerRail(t *testing.T) {
	const (
		waveRight = 252
		barCount  = 24
	)

	step, visible := waveformLayout(waveRight, barCount)
	if !visible {
		t.Fatal("waveform should fit")
	}
	if right := waveformLeft + (barCount-1)*step + waveformBarWidth; right > waveRight {
		t.Fatalf("waveform ends at %d, beyond protected boundary %d", right, waveRight)
	}
}

func TestWaveformRailSitsBelowText(t *testing.T) {
	textBottom := hudTextTop + hudTextHeight
	waveformTop := waveformCenter - waveformMaxHeight/2
	if textBottom >= waveformTop {
		t.Fatalf("text ends at %d, waveform starts at %d", textBottom, waveformTop)
	}
}
