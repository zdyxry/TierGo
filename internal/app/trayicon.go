package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"

	"github.com/zdyxry/easytier-mygo/internal/easytier"
)

// TrayIconPNG draws the disconnected menu bar icon.
func TrayIconPNG() []byte { return TrayIconForPhase(easytier.PhaseStopped) }

// TrayIconForPhase draws a template image whose alpha channel follows the
// menu bar's light and dark appearance.
func TrayIconForPhase(phase easytier.Phase) []byte {
	const (
		size  = 22
		scale = 4
	)
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	center := float64(size) / 2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			covered := 0
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					fx := float64(x) + (float64(sx)+0.5)/scale
					fy := float64(y) + (float64(sy)+0.5)/scale
					if iconCovered(fx-center, fy-center, phase) {
						covered++
					}
				}
			}
			alpha := uint8(covered * 255 / (scale * scale))
			img.SetNRGBA(x, y, color.NRGBA{A: alpha})
		}
	}
	var out bytes.Buffer
	_ = png.Encode(&out, img)
	return out.Bytes()
}

func iconCovered(x, y float64, phase easytier.Phase) bool {
	d := math.Hypot(x, y)
	if phase == easytier.PhaseFailed {
		return d <= 4.5 && (math.Abs(x-y) <= 1.2 || math.Abs(x+y) <= 1.2)
	}
	if d >= 6.4 && d <= 8.6 {
		return true
	}
	switch phase {
	case easytier.PhaseRunning:
		return d <= 3.4
	case easytier.PhaseStarting, easytier.PhaseStopping:
		return d >= 2 && d <= 3.4
	default:
		return false
	}
}
