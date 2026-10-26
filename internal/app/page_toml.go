package app

import (
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// The TOML flow: export turns the current draft into a core configuration
// file; import loads one into the editor for review; the editor applies it
// to the draft. Importing never touches the saved profile until the user
// also saves the settings.

// openTOMLEditor shows the current draft as TOML for editing.
func (a *App) openTOMLEditor() {
	text, err := easytier.ExportConfigTOML(a.connectionConfig())
	if err != nil {
		a.notice = err.Error()
		return
	}
	a.tomlText = text
	a.tomlErr = ""
	a.tomlOpen = true
}

// importTOMLFile reads a file into the editor, so the user reviews it before
// it replaces the draft.
func (a *App) importTOMLFile() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:  a.win,
		Title:   locale.T("导入网络配置"),
		Filters: []mygo.FileFilter{{Name: "TOML", Extensions: []string{"toml"}}},
	})
	if err != nil || len(paths) == 0 {
		return
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		a.notice = locale.T("读取文件失败：") + err.Error()
		return
	}
	a.tomlText = string(data)
	a.tomlErr = ""
	a.tomlOpen = true
}

// exportTOMLFile writes the current draft as a TOML file.
func (a *App) exportTOMLFile() {
	text, err := easytier.ExportConfigTOML(a.connectionConfig())
	if err != nil {
		a.notice = err.Error()
		return
	}
	name := a.draft().config.NetworkName
	if name == "" {
		name = "easytier"
	}
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Parent:      a.win,
		Title:       locale.T("导出网络配置"),
		DefaultPath: name + ".toml",
		Filters:     []mygo.FileFilter{{Name: "TOML", Extensions: []string{"toml"}}},
	})
	if err != nil || path == "" {
		return
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		a.notice = locale.T("写入文件失败：") + err.Error()
		return
	}
	a.notice = locale.T("已导出到 ") + path
}

// applyTOML replaces the draft with the editor's content.
func (a *App) applyTOML() {
	config, err := easytier.ParseConfigTOML(a.tomlText)
	if err != nil {
		a.tomlErr = err.Error()
		return
	}
	// An enabled secure_mode without keys gets its identity now, so it is
	// stable from this draft on instead of changing on every export.
	if err := easytier.EnsureSecureModeKeys(&config); err != nil {
		a.tomlErr = err.Error()
		return
	}
	// The log level is the client's own setting, not the network's: keep it.
	config.LogLevel = a.draft().config.LogLevel
	a.drafts[a.selected] = newDraft(easytier.Profile{
		ID:     a.selected,
		Name:   a.draft().name,
		Config: config,
	})
	a.tomlOpen = false
	a.notice = locale.T("已从 TOML 更新，记得保存设置")
}

// tomlEditorModal edits the configuration as text.
func (a *App) tomlEditorModal(c *ui.Context, t *ui.Theme) {
	ui.Modal(c, &a.tomlOpen, func() {
		ui.Column(c).Padding(20).Gap(12).Width(620).Children(func() {
			ui.Text(c, locale.T("编辑 TOML")).FontSize(15).Bold()
			ui.TextArea(c, &a.tomlText).Height(340).Label(locale.T("TOML 配置"))
			if a.tomlErr != "" {
				ui.Text(c, a.tomlErr).FontSize(12).TextColor(t.Danger)
			}
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, locale.T("取消")).Clicked() {
					a.tomlOpen = false
				}
				if ui.PrimaryButton(c, locale.T("应用")).Clicked() {
					a.applyTOML()
				}
			})
		})
	})
}
