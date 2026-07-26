package hud

const (
	labelLeft     = 16
	labelWaveGap  = 12
	minWaveLeft   = 100
	waveBarWidth  = 4
	minWaveBarGap = 1
)

func waveformLayout(labelWidth, waveRight, barCount int) (left, step int, visible bool) {
	left = minWaveLeft
	if afterLabel := labelLeft + labelWidth + labelWaveGap; afterLabel > left {
		left = afterLabel
	}
	if barCount <= 0 || waveRight-left < waveBarWidth {
		return left, 0, false
	}
	if barCount == 1 {
		return left, 0, true
	}

	step = (waveRight - left - waveBarWidth) / (barCount - 1)
	if step < waveBarWidth+minWaveBarGap {
		return left, step, false
	}
	return left, step, true
}
