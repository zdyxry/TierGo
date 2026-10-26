package app

import (
	"context"
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

func (a *App) monitorPage(c *ui.Context, t *ui.Theme, snap easytier.Snapshot) {
	if a.remoteActive {
		a.remoteLogLevelCard(c, t)
	}
	card(c, func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locale.T("实时流量（字节/秒）")).Bold().Grow(1)
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				dot(c, 7, t.Success)
				ui.Text(c, locale.T("下载")).FontSize(11.5).TextColor(t.TextMuted)
			})
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				dot(c, 7, t.Accent)
				ui.Text(c, locale.T("上传")).FontSize(11.5).TextColor(t.TextMuted)
			})
		})
		ui.Text(c, locale.T("物理对端连接计数差值，含协议开销；不是虚拟网卡应用吞吐。最近 120 次采样。")).FontSize(12).TextColor(t.TextMuted)
		if len(snap.Traffic) < 2 {
			ui.Text(c, locale.T("等待有效流量采样"))
			return
		}
		last := snap.Traffic[len(snap.Traffic)-1]
		ui.Text(c, fmt.Sprintf(locale.T("下载 %s · 上传 %s"), formatRate(last.RxPerSecond), formatRate(last.TxPerSecond)))
		maxRate := 1.0
		for _, sample := range snap.Traffic {
			maxRate = max(maxRate, sample.RxPerSecond, sample.TxPerSecond)
		}
		start, end := snap.Traffic[0].At, last.At
		if !end.After(start) {
			end = start.Add(time.Second)
		}
		x := chart.NewTime(start, end)
		y := chart.NewLinear(0, maxRate).Nice(4)
		xs, rx, tx := []float32{}, []float32{}, []float32{}
		for _, sample := range snap.Traffic {
			xs = append(xs, x.At(sample.At))
			rx = append(rx, y.At(sample.RxPerSecond))
			tx = append(tx, y.At(sample.TxPerSecond))
		}
		ticks := y.Ticks(4)
		chart.Plot(c, chart.Axes{
			Left:   ticks,
			Bottom: x.Ticks(5),
			Label: func(side chart.Side, tick chart.Tick) {
				label := tick.Label
				if side == chart.Left {
					label = formatRate(tick.Value)
				}
				ui.Text(c, label).FontSize(10).TextColor(t.TextMuted).NoWrap()
			},
		}, func(area ui.Element) {
			area.Label(locale.T("实时流量（字节/秒）")).
				Description(locale.T("物理对端连接计数差值，含协议开销；不是虚拟网卡应用吞吐。最近 120 次采样。")).
				Draw(func(p *ui.Painter, r ui.Rect) {
					for _, tick := range ticks {
						p.Line(r.X, chart.Y(r, tick.At), r.X+r.W, chart.Y(r, tick.At), 1, t.Border)
					}
					p.StrokePath(chart.Line(r, xs, rx, chart.CurveLinear), 2, t.Success)
					p.StrokePath(chart.Line(r, xs, tx, chart.CurveLinear), 2, t.Accent)
				})
		}).FillWidth().Height(180).Gap(6)
	})
	card(c, func() {
		ui.Text(c, locale.T("网络变化时间线")).Bold()
		ui.Text(c, locale.T("来源标记为 Core 事件或状态轮询；Core 仅保留最近 20 条，采样间过量事件可能遗漏。")).FontSize(12).TextColor(t.TextMuted)
		for i := len(snap.Events) - 1; i >= 0; i-- {
			event := snap.Events[i]
			ui.Text(c, event.At.Format("15:04:05")+" · "+event.Kind+" · "+event.Detail+" ["+event.Source+"]").FontSize(12)
		}
		if len(snap.Events) == 0 {
			ui.Text(c, locale.T("暂无变化"))
		}
	})
}

// formatRate renders a bytes-per-second rate with a unit that keeps the
// number small, for the live-traffic readout and the chart's axis labels.
func formatRate(bytesPerSecond float64) string {
	const unit = 1000
	if bytesPerSecond < unit {
		return fmt.Sprintf("%.0f B/s", bytesPerSecond)
	}
	value := bytesPerSecond
	for _, suffix := range []string{"kB/s", "MB/s", "GB/s"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f TB/s", value/unit)
}

// remoteLogLevelCard adjusts the remote process's runtime log level. There
// is no RPC to read the current level back, so the control starts at the
// Core default and only sends when the user changes it.
func (a *App) remoteLogLevelCard(c *ui.Context, t *ui.Theme) {
	card(c, func() {
		ui.Text(c, locale.T("远端运行时日志级别")).Bold()
		ui.Text(c, locale.T("立即生效，但作用于远端整个进程：同一进程内所有实例都会改变，重启后恢复配置值。")).FontSize(12).TextColor(t.Warning)
		if a.remoteLogLevel == "" {
			a.remoteLogLevel = "info"
		}
		levels := easytier.LogLevels
		labels := make([]string, len(levels))
		chosen := 0
		for index, candidate := range levels {
			labels[index] = easytier.LogLevelName(candidate)
			if candidate == a.remoteLogLevel {
				chosen = index
			}
		}
		address := a.settings.RemoteRPC
		key := "remote-log-level:" + address
		ui.Segmented(c, &chosen, labels...).Width(420).Disabled(a.asyncBusy(key))
		if levels[chosen] != a.remoteLogLevel {
			level := levels[chosen]
			setLevel := easytier.SetRuntimeLogLevel
			a.remoteLogLevel = level
			a.runAsync(key, func() func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				err := setLevel(ctx, address, level)
				cancel()
				return func() {
					if a.settings.RemoteRPC != address {
						return
					}
					if err != nil {
						a.notice = err.Error()
					} else {
						a.notice = locale.T("远端日志级别已应用")
					}
				}
			})
		}
	})
}
