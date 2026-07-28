package hud

const (
	hudTextLeft       = 16
	hudTextTop        = 11
	hudTextHeight     = 20
	waveformLeft      = 16
	waveformCenter    = 50
	waveformMaxHeight = 22
	waveformBarWidth  = 4
)

func waveformLayout(right, barCount int) (step int, visible bool) {
	if barCount <= 0 || right-waveformLeft < waveformBarWidth {
		return 0, false
	}
	if barCount == 1 {
		return 0, true
	}

	step = (right - waveformLeft - waveformBarWidth) / (barCount - 1)
	if step <= waveformBarWidth {
		return step, false
	}
	return step, true
}
