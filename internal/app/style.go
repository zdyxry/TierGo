package app

import (
	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/locale"

	"github.com/zdyxry/easytier-mygo/internal/easytier"
)

// The icons are Lucide shapes: 24×24, stroked with the current text color,
// so ui.Icon colors and sizes them like text.
func svg(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(
		`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" ` +
			`fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" ` +
			`stroke-linejoin="round">` + body + `</svg>`,
	))
}

var (
	iconOverview = svg(`<rect width="7" height="9" x="3" y="3" rx="1"/><rect width="7" height="5" x="14" y="3" rx="1"/><rect width="7" height="9" x="14" y="12" rx="1"/><rect width="7" height="5" x="3" y="16" rx="1"/>`)
	iconPeers    = svg(`<rect width="20" height="14" x="2" y="3" rx="2"/><line x1="8" x2="16" y1="21" y2="21"/><line x1="12" x2="12" y1="17" y2="21"/>`)
	iconRoutes   = svg(`<circle cx="6" cy="19" r="3"/><path d="M9 19h8.5a3.5 3.5 0 0 0 0-7h-11a3.5 3.5 0 0 1 0-7H15"/><circle cx="18" cy="5" r="3"/>`)
	iconSettings = svg(`<line x1="21" x2="14" y1="4" y2="4"/><line x1="10" x2="3" y1="4" y2="4"/><line x1="21" x2="12" y1="12" y2="12"/><line x1="8" x2="3" y1="12" y2="12"/><line x1="21" x2="16" y1="20" y2="20"/><line x1="12" x2="3" y1="20" y2="20"/><line x1="14" x2="14" y1="2" y2="6"/><line x1="8" x2="8" y1="10" y2="14"/><line x1="16" x2="16" y1="18" y2="22"/>`)
	iconLogs     = svg(`<path d="M15 12h-5"/><path d="M15 8h-5"/><path d="M19 17V5a2 2 0 0 0-2-2H4"/><path d="M8 21h12a2 2 0 0 0 2-2v-1a1 1 0 0 0-1-1H11a1 1 0 0 0-1 1v1a2 2 0 1 1-4 0V5a2 2 0 1 0-4 0v2a1 1 0 0 0 1 1h3"/>`)
	iconAbout    = svg(`<circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/>`)
	iconCopy     = svg(`<rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/>`)
	iconRefresh  = svg(`<path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M8 16H3v5"/>`)
	iconSearch   = svg(`<circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>`)
	iconTrash    = svg(`<path d="M3 6h18"/><path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6"/><path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2"/><line x1="10" x2="10" y1="11" y2="17"/><line x1="14" x2="14" y1="11" y2="17"/>`)
	iconNetwork  = svg(`<circle cx="12" cy="12" r="10"/><path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20"/><path d="M2 12h20"/>`)
	iconShield   = svg(`<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/><path d="m9 12 2 2 4-4"/>`)
	iconServer   = svg(`<rect width="20" height="8" x="2" y="2" rx="2" ry="2"/><rect width="20" height="8" x="2" y="14" rx="2" ry="2"/><line x1="6" x2="6.01" y1="6" y2="6"/><line x1="6" x2="6.01" y1="18" y2="18"/>`)
	iconPlus     = svg(`<path d="M5 12h14"/><path d="M12 5v14"/>`)
	iconLink     = svg(`<path d="M7 7h10v10"/><path d="M7 17 17 7"/>`)
	iconEye      = svg(`<path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/>`)
	iconEyeOff   = svg(`<path d="M9.88 9.88a3 3 0 1 0 4.24 4.24"/><path d="M10.73 5.08A10.43 10.43 0 0 1 12 5c7 0 10 7 10 7a13.16 13.16 0 0 1-1.67 2.68"/><path d="M6.61 6.61A13.526 13.526 0 0 0 2 12s3 7 10 7a9.74 9.74 0 0 0 5.39-1.61"/><line x1="2" x2="22" y1="2" y2="22"/>`)
	iconMonitor  = svg(`<polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/>`)
	iconService  = svg(`<path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/>`)
)

// style derives the app's theme: the desktop's, with the accent and
// roundness of a modern client.
func style(c *ui.Context) *ui.Theme {
	t := *c.Theme()
	t.Accent = ui.Hex("#4b6bfb")
	t.AccentHover = ui.Hex("#5c7bfc")
	t.AccentPressed = ui.Hex("#3b58dd")
	t.Radius = 10
	c.SetTheme(&t)
	return &t
}

// statusColor is the dot's color for a phase.
func statusColor(t *ui.Theme, phase easytier.Phase) ui.Color {
	switch phase {
	case easytier.PhaseRunning:
		return t.Success
	case easytier.PhaseStarting, easytier.PhaseStopping:
		return t.Warning
	case easytier.PhaseFailed:
		return t.Danger
	default:
		return t.TextMuted
	}
}

func statusLabel(phase easytier.Phase) string {
	switch phase {
	case easytier.PhaseStarting:
		return locale.T("连接中")
	case easytier.PhaseRunning:
		return locale.T("已连接")
	case easytier.PhaseStopping:
		return locale.T("断开中")
	case easytier.PhaseFailed:
		return locale.T("连接失败")
	default:
		return locale.T("未连接")
	}
}
