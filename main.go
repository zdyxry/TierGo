// TierGo desktop client: a native-UI window, drawn by MyGo, over an
// easytier-core that the client runs as its own process.
package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/locales/zh-Hans"
	"github.com/zdyxry/easytier-mygo/internal/app"
	"github.com/zdyxry/easytier-mygo/internal/buildinfo"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

func main() {
	showVersion := flag.Bool("version", false, "print the client version and commit")
	// Set when this copy was restarted with administrative rights: an
	// elevation helper gives the new process root's home, and the user's
	// settings are where this says they are.
	instanceRoot := flag.String("instance-root", "",
		"directory holding this client's configuration; empty means the user's own")
	flag.Parse()
	if *showVersion {
		fmt.Println("TierGo " + buildinfo.Display())
		return
	}
	if *instanceRoot != "" {
		easytier.UseInstanceRoot(*instanceRoot)
	}

	profiles, err := easytier.LoadProfiles()
	notice := ""
	if err != nil {
		notice = err.Error()
	}
	settings, err := easytier.LoadAppSettings()
	if err != nil && notice == "" {
		notice = err.Error()
	}

	locale.Set(settings.Language)
	mygo.App.SetLanguages("zh-Hans", "en")
	mygo.App.SetLocale(locale.Language())
	mygo.App.OnLocaleChanged(func(tag string) { locale.Set(tag) })
	manager := easytier.NewManager()
	desktop := app.New(manager, profiles, settings, notice)

	// quitting distinguishes “the user closed the window” (hide to the tray,
	// the networks keep running) from “the user quit” (stop everything).
	quitting := false

	// A core left running would hold the TUN device after the window is
	// gone, so quitting stops every network and waits for them to leave.
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		quitting = true
		if err := desktop.Shutdown(5 * time.Second); err != nil {
			log.Printf(locale.T("停止网络失败：%v"), err)
		}
	})

	mygo.App.WhenReady(func() {
		// Keep the login item in step with the saved setting.
		if err := mygo.App.SetOpenAtLogin(settings.OpenAtLogin); err != nil {
			log.Printf(locale.T("设置开机启动失败：%v"), err)
		}
		window := mygo.NewWindow(mygo.WindowOptions{
			Title:     "TierGo",
			Width:     1040,
			Height:    700,
			MinWidth:  900,
			MinHeight: 560,
			StateKey:  "main",
			Content:   desktop.Content(),
		})
		desktop.Attach(window)
		manager.AttachWindow(window)

		// Reconnect the networks that were enabled when the client was last
		// open; a running system service already owns them and wins.
		desktop.RestoreSession()

		// Closing the window hides it; the networks keep running in the
		// background, reachable from the tray icon.
		trayReady := false
		window.OnClose(func(e *mygo.CloseEvent) {
			if !trayReady {
				mygo.App.Quit()
				return
			}
			if quitting {
				return
			}
			e.PreventDefault()
			window.Hide()
		})

		if tray, err := mygo.NewTray(mygo.TrayOptions{
			ID:             "main",
			Icon:           app.TrayIconPNG(),
			IconIsTemplate: true,
			ToolTip:        "TierGo",
		}); err != nil {
			log.Printf(locale.T("托盘不可用：%v"), err)
		} else {
			trayReady = true
			desktop.AttachTray(tray)
			mygo.App.OnLocaleChanged(func(string) {
				desktop.RefreshTray()
			})
		}
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
