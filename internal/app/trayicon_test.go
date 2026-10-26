package app

import (
	"bytes"
	"image/png"
	"testing"

	"github.com/zdyxry/easytier-mygo/internal/easytier"
)

func TestTrayIconsChangeWithConnectionPhase(t *testing.T) {
	phases := []easytier.Phase{easytier.PhaseStopped, easytier.PhaseStarting, easytier.PhaseRunning, easytier.PhaseFailed}
	seen := map[string]bool{}
	for _, phase := range phases {
		data := TrayIconForPhase(phase)
		if seen[string(data)] {
			t.Fatalf("phase %s reused another phase's icon", phase)
		}
		seen[string(data)] = true
		image, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if image.Bounds().Dx() != 22 || image.Bounds().Dy() != 22 {
			t.Fatalf("icon dimensions = %v", image.Bounds())
		}
	}
}
