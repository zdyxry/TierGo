package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// App is the window's state. The manager owns one easytier-core process per
// network profile; the view draws the selected profile's snapshot.
type App struct {
	win     *mygo.Window
	mgr     *easytier.Manager
	central *easytier.CentralController

	profiles []easytier.Profile
	selected string
	// drafts are what the settings page edits, one per profile so switching
	// networks does not lose an unsaved edit.
	drafts map[string]*draftState

	page   string
	notice string

	// settings are the window's own (login item), service* cache what the
	// operating system last said about the system service.
	settings          easytier.AppSettings
	serviceState      easytier.ServiceState
	serviceCheckedAt  time.Time
	serviceErr        string
	serviceGeneration uint64
	// serviceStatus asks the OS; tests replace it.
	serviceStatus func(ctx context.Context) (easytier.ServiceState, error)
	// serviceOptions is the service page's draft of settings.Service; it is
	// copied into the settings only when the save button validates it.
	serviceOptions     easytier.ServiceSettings
	serviceOptionsInit bool
	// serviceDrift* report that the embedded core changed since the service
	// was installed; the banner offers to re-sync via 安装或更新.
	serviceDrift                     bool
	serviceDriftFrom, serviceDriftTo string
	// serviceDriftCheck compares the install-time marker with the current
	// embedded core; tests replace it.
	serviceDriftCheck func(easytier.ServiceSettings) (string, string, bool)
	// serviceInstallFn and serviceInstallCentralFn run the installs; tests
	// replace them, the real ones need the core binaries and elevation.
	serviceInstallFn        func(context.Context, []easytier.Profile, easytier.ServiceSettings) error
	serviceInstallCentralFn func(context.Context, easytier.CentralConfig, easytier.ServiceSettings) error
	serviceStartFn          func(context.Context) error
	stopAllAndWait          func(time.Duration) error
	// serviceLog* cache the Config Server state inferred from the service's
	// file log, refreshed on the remote page at most every few seconds.
	serviceLog    easytier.ServiceLogStatus
	serviceLogErr string
	serviceLogAt  time.Time
	// serviceLogScan reads the service's file log; tests replace it.
	serviceLogScan func() (easytier.ServiceLogStatus, error)
	// restore reconnects the enabled networks on launch; tests replace it.
	restore func(profiles []easytier.Profile, ids []string) error

	peersQuery  string
	peersTable  ui.ListState
	routesTable ui.ListState
	logsList    ui.ListState

	newProfileOpen bool
	newProfileName string
	confirmDelete  bool

	// toml* drive the configuration editor on the settings page.
	tomlOpen                    bool
	tomlText                    string
	tomlErr                     string
	advancedOpen                bool
	aclRuleOpen                 bool
	removeACLOpen               bool
	aclChainIndex, aclRuleIndex int
	aclRule                     aclRuleDraft
	aclErr                      string
	// aclGroupNames holds the rename drafts for the declared ACL groups;
	// a group name is only committed, together with all rule references,
	// when its rename button is clicked. aclGroupNamesFor keys the drafts
	// to the ACL they were taken from, so switching the editing target
	// (local profile or remote instance) never applies a stale name.
	aclGroupNames                                    []string
	aclGroupNamesFor                                 string
	remoteEditOpen, remoteStateConfirm               bool
	remoteEditText, remoteEditAddress, remoteEditErr string
	remoteEditConfig                                 easytier.RemoteConfig
	remoteStateTarget                                easytier.RemoteNetwork
	remoteCreateConfirm, remoteDeleteConfirm         bool
	remoteConnected                                  bool
	// remoteActive makes the selected remote network the current instance:
	// every page then shows remoteSnapshot instead of a local profile.
	remoteActive              bool
	remotePending             chan remotePollResult
	remoteRequestedSelection  string
	remotePoll                func(context.Context, string, string) remotePollResult
	remoteChecked             time.Time
	remoteErr, remoteSelected string
	remoteNetworks            []easytier.RemoteNetwork
	remoteSnapshot            easytier.Snapshot
	// Remote rate history lives outside remoteSnapshot, which each poll
	// replaces; the counter baseline resets when the instance or its
	// address changes.
	remoteTraffic  []easytier.TrafficSample
	remoteCounters map[string]easytier.ConnCounter
	remoteSampleAt time.Time
	remoteLogLevel string
	// Remote ACL editor state, keyed to the instance it was read from so a
	// context switch never saves one instance's ACL into another.
	remoteACL                      *easytier.ACL
	remoteACLKey, remoteACLMembers string
	remoteACLSave                  bool
	// Remote config access seams; tests replace them.
	getRemoteConfig                                                                                func(context.Context, string, easytier.RemoteNetwork) (easytier.RemoteConfig, error)
	saveRemoteConfig                                                                               func(context.Context, string, easytier.RemoteConfig) error
	portalInfo                                                                                     easytier.PortalInfo
	portalOwner, portalEndpoint                                                                    string
	portalDeviceSelected, portalDeviceDelete, portalDeviceName, portalDeviceIP, portalDeviceGroups string
	portalQR                                                                                       *ui.Bitmap
	portalQRText, portalQRError                                                                    string
	// portalPoll* drive the device list's periodic refresh while the Portal
	// page shows a connected device-mode portal. portalPoll is the read
	// seam; tests replace it.
	portalPollPending chan portalPollResult
	portalPollAt      time.Time
	portalPoll        func(context.Context) (easytier.PortalInfo, error)
	// portalDeviceChecked holds the declared-ACL-group checkboxes of the
	// add-device form; manual text stays as a supplement.
	portalDeviceChecked map[string]bool
	// credential* are the temporary-credential panel's state, keyed to the
	// same instance context as the portal (credentialOwner).
	credentialList                 []easytier.Credential
	credentialErr, credentialOwner string
	credentialGroups               string
	credentialTTL                  float64
	credentialAllowRelay           bool
	credentialReusable             bool
	credentialGenerated            easytier.GeneratedCredential
	credentialHasGenerated         bool
	credentialRevoke               string
	credentialGenerateFn           func(context.Context, easytier.CredentialRequest) (easytier.GeneratedCredential, error)
	credentialRevokeFn             func(context.Context, string) error
	credentialListFn               func(context.Context) ([]easytier.Credential, error)

	asyncJobs    map[string]bool
	asyncResults chan asyncCompletion

	tray      *mygo.Tray
	trayKey   string
	trayPhase easytier.Phase
}

// draftState is one profile's half-edited configuration: the display name
// and the multi-line editors stay text until the user saves.
type draftState struct {
	name                string
	config              easytier.Config
	peersText           string
	listenersText       string
	proxyText           string
	forwardText         string
	manualRoutesText    string
	exitNodesText       string
	mappedListenersText string
	aclMembersText      string
	showSecret          bool
}

func newDraft(profile easytier.Profile) *draftState {
	// ACL contains slices; draft editing must never mutate the saved
	// profile before Save (notably when preparing system-service configs).
	data, _ := json.Marshal(profile.Config)
	var config easytier.Config
	_ = json.Unmarshal(data, &config)
	members := ""
	if config.ACL != nil {
		members = strings.Join(config.ACL.V1.Group.Members, ", ")
	}
	return &draftState{
		aclMembersText:      members,
		name:                profile.DisplayName(),
		config:              config,
		peersText:           easytier.JoinLines(config.Peers),
		listenersText:       easytier.JoinLines(config.Listeners),
		proxyText:           easytier.JoinLines(config.ProxyNetworks),
		forwardText:         easytier.JoinLines(config.PortForwards),
		manualRoutesText:    easytier.JoinLines(config.ManualRoutes),
		exitNodesText:       easytier.JoinLines(config.ExitNodes),
		mappedListenersText: easytier.JoinLines(config.MappedListeners),
	}
}

var navItems = []struct {
	id      string
	icon    *ui.SVG
	title   string
	section string
}{
	{id: "overview", icon: iconOverview, title: "概览"},
	{id: "peers", icon: iconPeers, title: "节点"},
	{id: "routes", icon: iconRoutes, title: "路由"},
	{id: "settings", icon: iconSettings, title: "网络配置"},
	{id: "monitor", icon: iconMonitor, title: "监控", section: "高级功能"},
	{id: "acl", icon: iconShield, title: "访问控制"},
	{id: "remote", icon: iconServer, title: "远程管理"},
	{id: "portal", icon: iconLink, title: "VPN Portal"},
	{id: "service", icon: iconService, title: "系统服务"},
	{id: "preferences", icon: iconSettings, title: "应用设置", section: "客户端"},
	{id: "logs", icon: iconLogs, title: "日志"},
	{id: "about", icon: iconAbout, title: "关于"},
}

// New makes the window's state from the saved profiles. A first run gets one
// default network so the settings page is never empty.
func New(manager *easytier.Manager, profiles []easytier.Profile, settings easytier.AppSettings, notice string) *App {
	if len(profiles) == 0 {
		profile := easytier.NewProfile(locale.T("默认网络"))
		if err := easytier.SaveProfile(profile); err != nil && notice == "" {
			notice = err.Error()
		}
		profiles = []easytier.Profile{profile}
	}
	// The window comes back to the network it was last showing.
	selected := profiles[0].ID
	for _, profile := range profiles {
		if profile.ID == settings.SelectedProfile {
			selected = profile.ID
			break
		}
	}
	return &App{
		mgr:                     manager,
		central:                 easytier.NewCentralController(),
		profiles:                profiles,
		selected:                selected,
		settings:                settings,
		drafts:                  map[string]*draftState{},
		page:                    "overview",
		notice:                  notice,
		serviceStatus:           easytier.ServiceStatus,
		serviceInstallFn:        easytier.ServiceInstall,
		serviceInstallCentralFn: easytier.ServiceInstallCentral,
		serviceStartFn:          easytier.ServiceStart,
		stopAllAndWait:          manager.StopAllAndWait,
		restore:                 manager.Restore,
		asyncJobs:               map[string]bool{},
		asyncResults:            make(chan asyncCompletion, 128),
	}
}

// Content is what the window shows.
func (a *App) Content() *ui.Content { return ui.View(a.view) }

// Attach remembers the window the state is drawn in.
func (a *App) Attach(window *mygo.Window) { a.win = window }

// Shutdown stops only processes owned by this GUI, never system services or
// remote instances. Called explicitly by the application's quit hook.
func (a *App) Shutdown(timeout time.Duration) error {
	stopAll := a.stopAllAndWait
	if stopAll == nil {
		stopAll = a.mgr.StopAllAndWait
	}
	results := make(chan error, 2)
	go func() { results <- a.central.StopAndWait(timeout) }()
	go func() { results <- stopAll(timeout) }()
	return errors.Join(<-results, <-results)
}

// RestoreSession reconnects the networks that were enabled when the client
// was last open. A running system service already owns them, so it wins.
func (a *App) RestoreSession() {
	if len(a.settings.EnabledNetworks) == 0 {
		return
	}
	profiles := append([]easytier.Profile(nil), a.profiles...)
	ids := append([]string(nil), a.settings.EnabledNetworks...)
	status := a.serviceStatus
	restore := a.restore
	if restore == nil {
		restore = a.mgr.Restore
	}
	a.runAsync("restore-session", func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		state, statusErr := status(ctx)
		cancel()
		if statusErr == nil && state == easytier.ServiceRunning {
			return nil
		}
		err := restore(profiles, ids)
		return func() {
			if err != nil {
				a.notice = err.Error()
			}
		}
	})
}

// rememberSession saves which network the window shows, so the next launch
// comes back to it.
func (a *App) rememberSession() {
	a.settings.SelectedProfile = a.selected
	if err := a.settings.Save(); err != nil {
		a.notice = err.Error()
	}
}

// setNetworkEnabled records that the user connected or disconnected a
// network; the next launch reconnects exactly the enabled ones.
func (a *App) setNetworkEnabled(id string, enabled bool) {
	next := make([]string, 0, len(a.settings.EnabledNetworks)+1)
	for _, current := range a.settings.EnabledNetworks {
		if current != id {
			next = append(next, current)
		}
	}
	if enabled {
		next = append(next, id)
	}
	a.settings.EnabledNetworks = next
	a.rememberSession()
}

// clearEnabledNetworks forgets every enabled network, for handing them to
// the system service.
func (a *App) clearEnabledNetworks() {
	a.settings.EnabledNetworks = nil
	a.rememberSession()
}

// draft is the selected profile's working copy, made on first edit.
func (a *App) draft() *draftState {
	if draft, ok := a.drafts[a.selected]; ok {
		return draft
	}
	for _, profile := range a.profiles {
		if profile.ID == a.selected {
			a.drafts[profile.ID] = newDraft(profile)
			return a.drafts[profile.ID]
		}
	}
	return &draftState{config: easytier.DefaultConfig()}
}

// selectedProfile is the network the pages are about.
func (a *App) selectedProfile() easytier.Profile {
	for _, profile := range a.profiles {
		if profile.ID == a.selected {
			return profile
		}
	}
	if len(a.profiles) > 0 {
		return a.profiles[0]
	}
	return easytier.Profile{}
}

// ctl is the selected profile's core supervisor.
func (a *App) ctl() *easytier.Controller { return a.mgr.Controller(a.selected) }

// currentSnapshot is the state the pages draw: the selected remote instance
// when one is active, otherwise the selected local profile's core.
func (a *App) currentSnapshot() easytier.Snapshot {
	if a.remoteActive {
		return a.remoteSnapshot
	}
	return a.ctl().Snapshot()
}

func (a *App) remoteSelectedNetwork() (easytier.RemoteNetwork, bool) {
	for _, network := range a.remoteNetworks {
		if network.ID == a.remoteSelected {
			return network, true
		}
	}
	return easytier.RemoteNetwork{}, false
}

// currentInstanceName is what the headings call the shown instance.
func (a *App) currentInstanceName() string {
	if a.remoteActive {
		if network, ok := a.remoteSelectedNetwork(); ok && network.Name != "" {
			return fmt.Sprintf(locale.T("远端 %s"), network.Name)
		}
		return locale.T("远端实例")
	}
	return a.selectedProfile().DisplayName()
}

func (a *App) view(c *ui.Context) {
	a.drainAsync()
	a.RefreshTray()
	t := style(c)
	snap := a.currentSnapshot()
	// While a remote instance is the current context its state must refresh
	// on every page, not just on the remote page.
	if a.remoteConnected && (a.remoteActive || a.page == "remote") {
		a.refreshRemote(c)
	}

	if a.notice != "" {
		c.Toast(a.notice)
		a.notice = ""
	}

	ui.Row(c).Fill().Children(func() {
		a.sidebar(c, t, snap)
		ui.Column(c).Grow(1).FillHeight().Children(func() {
			a.topbar(c, t, snap)
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).Padding(24).Gap(18).Children(func() {
					a.privilegeNotice(c, t, snap)
					switch a.page {
					case "peers":
						a.peersPage(c, t, snap)
					case "routes":
						a.routesPage(c, t, snap)
					case "settings":
						a.settingsPage(c, t, snap)
					case "preferences":
						a.preferencesPage(c, t)
					case "logs":
						a.logsPage(c, t, snap)
					case "portal":
						a.portalPage(c, t, snap)
					case "remote":
						a.remotePage(c, t)
					case "monitor":
						a.monitorPage(c, t, snap)
					case "acl":
						a.aclPage(c, t)
					case "service":
						a.servicePage(c, t)
					case "about":
						a.aboutPage(c, t)
					default:
						a.overviewPage(c, t, snap)
					}
				})
			})
		})
	})

	a.newProfileModal(c, t)
	a.remoteConfigModals(c, t)
}

func (a *App) sidebar(c *ui.Context, t *ui.Theme, snap easytier.Snapshot) {
	ui.Column(c).Width(236).FillHeight().Padding(14).Gap(4).
		Background(t.Background).
		BorderWidth(0, 1, 0, 0).BorderColor(t.Border).
		Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Padding(6, 8, 14, 8).Children(func() {
				ui.Box(c).Size(30, 30).Radius(9).Background(t.Accent).
					Center().Children(func() {
					ui.Icon(c, iconNetwork).FontSize(17).TextColor(t.AccentText)
				})
				ui.Column(c).Gap(1).Grow(1).Children(func() {
					ui.Text(c, "TierGo").FontSize(14).Bold()
					ui.Text(c, locale.T("非官方 EasyTier 客户端")).FontSize(11).TextColor(t.TextMuted)
				})
			})

			// Profiles can outgrow the window; keep all navigation reachable
			// instead of pushing About and the status card below the bottom.
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).Gap(4).Children(func() {
					a.networkList(c, t)
					ui.Box(c).Height(1).Margin(8, 4).Background(t.Border)
					for _, item := range navItems {
						if item.section != "" {
							ui.Text(c, locale.T(item.section)).FontSize(11).Bold().TextColor(t.TextMuted).Padding(10, 8, 2, 8)
						}
						a.navItem(c, t, item.id, item.icon, locale.T(item.title), snap)
					}
				})
			})
			a.sidebarStatus(c, t, snap)
		})
}

// networkList is the sidebar's profile switcher: every configured network
// with its live status, and the button that adds another one.
func (a *App) networkList(c *ui.Context, t *ui.Theme) {
	ui.Row(c).AlignItems(ui.Center).Padding(4, 8, 2, 8).Children(func() {
		ui.Text(c, locale.T("网络")).FontSize(11).Bold().TextColor(t.TextMuted).Grow(1)
		add := ui.Button(c, "").Children(func() {
			ui.Icon(c, iconPlus).FontSize(13)
		}).Label(locale.T("新建网络"))
		if add.Clicked() {
			a.newProfileName = ""
			a.newProfileOpen = true
		}
	})
	for _, profile := range a.profiles {
		a.networkRow(c, t, profile)
	}
	// The connected remote endpoint's instances join the switcher, so every
	// page can be about one of them.
	if a.remoteConnected && len(a.remoteNetworks) > 0 {
		ui.Box(c).Height(1).Margin(6, 4).Background(t.Border)
		ui.Row(c).Padding(4, 8, 2, 8).Children(func() {
			ui.Text(c, locale.T("远端实例")).FontSize(11).Bold().TextColor(t.TextMuted).Grow(1)
		})
		for _, network := range a.remoteNetworks {
			a.remoteNetworkRow(c, t, network)
		}
	}
}

// remoteNetworkRow is one remote instance in the switcher. Non-disabled
// entries are running on the remote; disabled ones are local backups.
func (a *App) remoteNetworkRow(c *ui.Context, t *ui.Theme, network easytier.RemoteNetwork) {
	selected := a.remoteActive && a.remoteSelected == network.ID
	color := t.Success
	if network.Disabled {
		color = t.TextMuted
	}
	name := network.Name
	if name == "" {
		name = network.ID
	}
	row := ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(6, 10).Radius(8).
		Cursor(ui.CursorPointer).Label(name)
	if selected {
		row = row.Background(t.Accent.Alpha(0.12))
	} else if row.Hovered() {
		row = row.Background(t.SurfaceHover)
	}
	row = row.Children(func() {
		dot(c, 7, color)
		ui.Text(c, name).FontSize(13).Grow(1).SingleLine()
		if network.Disabled {
			ui.Badge(c, locale.T("已停用")).Background(t.Surface).TextColor(t.TextMuted)
		}
	})
	if row.Clicked() {
		a.selectRemote(network.ID)
	}
}

func (a *App) networkRow(c *ui.Context, t *ui.Theme, profile easytier.Profile) {
	selected := a.selected == profile.ID
	snap := a.mgr.Snapshot(profile.ID)
	row := ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(6, 10).Radius(8).
		Cursor(ui.CursorPointer).Label(profile.DisplayName())
	if selected {
		row = row.Background(t.Accent.Alpha(0.12))
	} else if row.Hovered() {
		row = row.Background(t.SurfaceHover)
	}
	row = row.Children(func() {
		dot(c, 7, statusColor(t, snap.Phase))
		ui.Text(c, profile.DisplayName()).FontSize(13).Grow(1).SingleLine()
		if snap.Phase == easytier.PhaseRunning {
			ui.Badge(c, locale.T("已连接")).Background(t.Success.Alpha(0.14)).TextColor(t.Success)
		}
	})
	if row.Clicked() {
		a.selected = profile.ID
		a.remoteActive = false
		a.page = "overview"
		a.notice = ""
		a.rememberSession()
	}
}

// newProfileModal asks for the new network's display name.
func (a *App) newProfileModal(c *ui.Context, t *ui.Theme) {
	ui.Modal(c, &a.newProfileOpen, func() {
		ui.Column(c).Padding(20).Gap(14).Width(340).Children(func() {
			ui.Text(c, locale.T("新建网络")).FontSize(15).Bold()
			ui.TextInput(c, &a.newProfileName).Placeholder(locale.T("网络名称，例如“公司”")).Label(locale.T("网络名称"))
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, locale.T("取消")).Clicked() {
					a.newProfileOpen = false
				}
				if ui.PrimaryButton(c, locale.T("创建")).Clicked() {
					a.addProfile()
				}
			})
		})
	})
}

func (a *App) addProfile() {
	profile := easytier.NewProfile(a.newProfileName)
	if err := easytier.SaveProfile(profile); err != nil {
		a.notice = err.Error()
		a.newProfileOpen = false
		return
	}
	a.profiles = append(a.profiles, profile)
	a.selected = profile.ID
	a.page = "settings"
	a.newProfileOpen = false
	a.rememberSession()
}

func (a *App) navItem(c *ui.Context, t *ui.Theme, id string, icon *ui.SVG, title string, snap easytier.Snapshot) {
	selected := a.page == id
	color := t.Text
	if selected {
		color = t.Accent
	}
	row := ui.Row(c).Gap(10).AlignItems(ui.Center).Padding(7, 10).Radius(8).
		Cursor(ui.CursorPointer).Label(title)
	if selected {
		row = row.Background(t.Accent.Alpha(0.12))
	} else if row.Hovered() {
		row = row.Background(t.SurfaceHover)
	}
	row = row.Children(func() {
		ui.Icon(c, icon).FontSize(16).TextColor(color)
		ui.Text(c, title).FontSize(13.5).Grow(1).TextColor(color)
		count := len(snap.Peers)
		if id == "peers" && count > 1 {
			// The list includes this device.
			ui.Badge(c, fmt.Sprint(count-1)).Background(t.Surface).TextColor(t.TextMuted)
		}
	})
	if row.Clicked() {
		a.page = id
		a.notice = ""
	}
}

func (a *App) sidebarStatus(c *ui.Context, t *ui.Theme, snap easytier.Snapshot) {
	ui.Column(c).Padding(12).Gap(8).Radius(10).
		Background(t.Surface).Border(1, t.Border).
		Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				dot(c, 8, statusColor(t, snap.Phase))
				ui.Text(c, statusLabel(snap.Phase)).FontSize(12.5).Bold().Grow(1)
			})
			detail := locale.T("未加入任何网络")
			switch {
			case snap.Phase == easytier.PhaseRunning:
				detail = a.draft().config.NetworkName
				if a.remoteActive {
					detail = a.currentInstanceName()
				}
				if snap.Node.IPv4 != "" {
					detail += " · " + snap.Node.IPv4
				}
			case snap.Phase == easytier.PhaseFailed && snap.Err != "":
				detail = snap.Err
			}
			ui.Text(c, detail).FontSize(11).TextColor(t.TextMuted).SingleLine()
		})
}

func (a *App) pageHeading(snap easytier.Snapshot) (string, string) {
	name := a.currentInstanceName()
	switch a.page {
	case "peers":
		return locale.T("节点"), fmt.Sprintf(locale.T("「%s」中的其他设备"), name)
	case "routes":
		return locale.T("路由"), fmt.Sprintf(locale.T("「%s」的转发表，共 %d 条"), name, len(snap.Routes))
	case "settings":
		return locale.T("网络配置"), fmt.Sprintf(locale.T("「%s」的网络身份、连接方式与虚拟网卡"), name)
	case "preferences":
		return locale.T("应用设置"), locale.T("语言与登录启动")
	case "logs":
		return locale.T("日志"), fmt.Sprintf(locale.T("「%s」的内核事件与本机操作记录"), name)
	case "portal":
		return "VPN Portal", locale.T("WireGuard 客户端接入")
	case "remote":
		return locale.T("远程管理"), locale.T("远程 RPC 状态查看")
	case "monitor":
		return locale.T("监控"), locale.T("实时速率与状态变化")
	case "acl":
		return locale.T("访问控制"), locale.T("Inbound、Outbound、Forward 规则与访问控制组")
	case "service":
		return locale.T("系统服务"), locale.T("系统服务的安装、启动与停止")
	case "about":
		return locale.T("关于"), locale.T("版本与信息")
	default:
		return locale.T("概览"), name
	}
}

func (a *App) topbar(c *ui.Context, t *ui.Theme, snap easytier.Snapshot) {
	title, subtitle := a.pageHeading(snap)
	ui.Row(c).Padding(24, 18).Gap(14).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).
		Children(func() {
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Text(c, title).FontSize(19).Bold()
				ui.Text(c, subtitle).FontSize(12.5).TextColor(t.TextMuted).SingleLine()
			})
			if a.remoteActive {
				ui.Badge(c, locale.T("远端实例")).Background(t.Accent.Alpha(0.14)).TextColor(t.Accent)
			}
			ui.Row(c).Gap(9).AlignItems(ui.Center).Padding(5, 12).Radius(999).
				Background(statusColor(t, snap.Phase).Alpha(0.14)).
				Children(func() {
					dot(c, 7, statusColor(t, snap.Phase))
					ui.Text(c, statusLabel(snap.Phase)).FontSize(12).
						TextColor(statusColor(t, snap.Phase)).Bold()
				})
			if a.remoteActive {
				a.remoteStateButton(c)
			} else {
				switch snap.Phase {
				case easytier.PhaseStarting, easytier.PhaseStopping:
					ui.Button(c, locale.T("断开连接")).Disabled(true)
				case easytier.PhaseRunning:
					if ui.Button(c, locale.T("断开连接")).Clicked() {
						a.mgr.Stop(a.selected)
						a.setNetworkEnabled(a.selected, false)
					}
				default:
					pending := a.asyncBusy("connect:" + a.selected)
					label := locale.T("连接")
					if pending {
						label = locale.T("检查运行环境")
					}
					if ui.PrimaryButton(c, label).Disabled(pending).Clicked() {
						a.connect()
					}
				}
			}
		})
}

// remoteStateButton replaces connect/disconnect while a remote instance is
// the current context: enabling/disabling it goes through the same
// confirmation as the remote page.
func (a *App) remoteStateButton(c *ui.Context) {
	network, ok := a.remoteSelectedNetwork()
	if !ok {
		return
	}
	label := locale.T("停用远端网络")
	if network.Disabled {
		label = locale.T("启用远端网络")
	}
	if ui.Button(c, label).Disabled(a.remoteBusy() || network.Permissions&3 != 0).Clicked() {
		a.remoteStateTarget = network
		a.remoteEditAddress = a.settings.RemoteRPC
		a.remoteStateConfirm = true
	}
}

// connect starts the selected profile's core with what the settings page
// currently holds.
func (a *App) connect() {
	config := a.connectionConfig()
	if err := easytier.EnsurePortalKey(config.VPNPortal); err != nil {
		a.notice = err.Error()
		return
	}
	hadKeys := config.SecureModePrivateKey != "" && config.SecureModePublicKey != ""
	if err := easytier.EnsureSecureModeKeys(&config); err != nil {
		a.notice = err.Error()
		return
	}
	if !hadKeys && config.SecureModePrivateKey != "" {
		// The identity just generated must survive a restart; keep it in the
		// draft and in the saved profile without touching other unsaved edits.
		a.draft().config.SecureModePrivateKey = config.SecureModePrivateKey
		a.draft().config.SecureModePublicKey = config.SecureModePublicKey
		saved := a.selectedProfile()
		saved.Config.SecureModePrivateKey = config.SecureModePrivateKey
		saved.Config.SecureModePublicKey = config.SecureModePublicKey
		if err := easytier.SaveProfile(saved); err != nil {
			a.notice = err.Error()
			return
		}
		for index := range a.profiles {
			if a.profiles[index].ID == saved.ID {
				a.profiles[index] = saved
			}
		}
	}
	if config.NetworkName == "" {
		a.notice = locale.T("请先在设置里填写网络名称")
		a.page = "settings"
		if a.win != nil {
			a.win.Invalidate()
		}
		return
	}
	if config.EnableTUN && !easytier.IsElevated() {
		a.notice = locale.T("创建虚拟网卡需要管理员权限，请先以管理员身份重启")
		a.page = "overview"
		if a.win != nil {
			a.win.Invalidate()
		}
		return
	}
	// A running service owns these networks: connecting here too would
	// fight it for the TUN device and the ports.
	id := a.selected
	profile := a.selectedProfile()
	profile.Config = config
	status := a.serviceStatus
	a.runAsync("connect:"+id, func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		state, err := status(ctx)
		cancel()
		return func() {
			if a.selected != id {
				return
			}
			if err == nil && state == easytier.ServiceRunning {
				a.notice = locale.T("系统服务正在运行网络；请先在「服务」页停止服务")
				a.page = "service"
				return
			}
			a.mgr.Start(profile)
			a.setNetworkEnabled(id, true)
		}
	})
}

func (a *App) connectionConfig() easytier.Config {
	draft := a.draft()
	config := draft.config
	config.Peers = easytier.SplitLines(draft.peersText)
	config.Listeners = easytier.SplitLines(draft.listenersText)
	config.ProxyNetworks = easytier.SplitLines(draft.proxyText)
	config.PortForwards = easytier.SplitLines(draft.forwardText)
	config.ManualRoutes = easytier.SplitLines(draft.manualRoutesText)
	config.ExitNodes = easytier.SplitLines(draft.exitNodesText)
	config.MappedListeners = easytier.SplitLines(draft.mappedListenersText)
	if config.ACL != nil {
		acl := *config.ACL
		acl.V1.Group.Members = splitComma(draft.aclMembersText)
		config.ACL = &acl
	}
	return config
}

// needsElevation reports whether the settings ask for a TUN the process
// cannot create.
func (a *App) needsElevation() bool {
	return a.draft().config.EnableTUN && !easytier.IsElevated()
}

// privilegeNotice asks for the administrative rights the TUN device needs,
// before the user tries to connect.
func (a *App) privilegeNotice(c *ui.Context, t *ui.Theme, snap easytier.Snapshot) {
	if a.remoteActive {
		return
	}
	if !a.needsElevation() || snap.Phase == easytier.PhaseRunning || snap.Phase == easytier.PhaseStarting {
		return
	}
	// It belongs where the user connects and where the option is set.
	if a.page != "overview" && a.page != "settings" {
		return
	}
	card(c, func() {
		ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconShield).FontSize(18).TextColor(t.Warning)
			ui.Column(c).Gap(2).Grow(1).Children(func() {
				ui.Text(c, locale.T("需要管理员权限")).FontSize(13.5).Bold()
				ui.Text(c, locale.T("创建虚拟网卡要 root/CAP_NET_ADMIN。重启为管理员后，本机所有程序都能")+
					locale.T("直接访问虚拟网络里的任意主机和端口；否则只能使用端口转发。")).FontSize(12).
					TextColor(t.TextMuted)
			})
			if ui.PrimaryButton(c, locale.T("以管理员身份重启")).Clicked() {
				root, err := easytier.InstanceRoot()
				if err != nil {
					a.notice = err.Error()
					return
				}
				if err := easytier.RelaunchElevated(root); err != nil {
					a.notice = err.Error()
				} else {
					mygo.App.Quit()
				}
			}
		})
	})
}

// ---- small building blocks ----

func dot(c *ui.Context, size float32, color ui.Color) ui.Element {
	return ui.Box(c).Size(size, size).Radius(size / 2).Background(color)
}

// card is the app's panel: a raised surface with a hairline border.
func card(c *ui.Context, fn func()) ui.Element {
	t := c.Theme()
	return ui.Column(c).Padding(18).Gap(14).Radius(12).
		Background(t.Surface).Border(1, t.Border).Children(fn)
}

// keyValue is a label and its value, as in a properties list.
func keyValue(c *ui.Context, label string, value string, copyable bool) {
	t := c.Theme()
	if value == "" {
		value = "—"
	}
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(12.5).TextColor(t.TextMuted).Width(96)
		ui.Text(c, value).FontSize(12.5).Grow(1).SingleLine()
		if copyable && value != "—" {
			if ui.Button(c, "").Children(func() {
				ui.Icon(c, iconCopy).FontSize(13)
			}).Label(locale.T("复制")).Clicked() {
				c.WriteClipboard(value)
				c.Toast(locale.T("已复制 ") + value)
			}
		}
	})
}

func (a *App) statCard(c *ui.Context, t *ui.Theme, icon *ui.SVG, label, value string) {
	card(c, func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon).FontSize(16).TextColor(t.Accent)
			ui.Text(c, label).FontSize(12).TextColor(t.TextMuted)
		})
		ui.Text(c, value).FontSize(22).Bold()
	})
}

// peerCount is the number of other devices in the network.
func peerCount(snap easytier.Snapshot) int {
	others := 0
	for _, peer := range snap.Peers {
		if !peer.Local {
			others++
		}
	}
	return others
}

// ---- pages ----

func (a *App) emptyState(c *ui.Context, t *ui.Theme, message string) {
	ui.Column(c).MinHeight(280).Center().Gap(10).Children(func() {
		ui.Icon(c, iconNetwork).FontSize(28).TextColor(t.TextMuted.Alpha(0.6))
		ui.Text(c, message).FontSize(13).TextColor(t.TextMuted)
	})
}

// logLevelRow chooses how much the core writes down. The core is chatty at
// info, so the default is errors only.
func logLevelRow(c *ui.Context, t *ui.Theme, level *string) {
	levels := easytier.LogLevels
	labels := make([]string, len(levels))
	chosen := 0
	for index, candidate := range levels {
		labels[index] = easytier.LogLevelName(candidate)
		if candidate == *level {
			chosen = index
		}
	}
	ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(2).Grow(1).Children(func() {
			ui.Text(c, locale.T("日志级别")).FontSize(13)
			ui.Text(c, locale.T("内核写入磁盘的日志量，级别越高越费磁盘")).
				FontSize(11.5).TextColor(t.TextMuted)
		})
		ui.Segmented(c, &chosen, labels...).Width(360)
	})
	if levels[chosen] != *level {
		*level = levels[chosen]
	}
}

// toggleRow is a labelled switch. The whole row toggles, so clicking the
// label works too; a disabled row disables the switch inside it.
func toggleRow(c *ui.Context, t *ui.Theme, label string, hint string, value *bool, disabled bool) {
	row := ui.Row(c).Gap(14).AlignItems(ui.Center).Cursor(ui.CursorPointer).Disabled(disabled)
	row.Children(func() {
		ui.Column(c).Gap(2).Grow(1).Children(func() {
			ui.Text(c, label).FontSize(13)
			ui.Text(c, hint).FontSize(11.5).TextColor(t.TextMuted)
		})
		ui.Switch(c, value).Label(label)
	})
	if row.Clicked() {
		*value = !*value
	}
}

func logRow(c *ui.Context, t *ui.Theme, entry easytier.LogEntry) {
	ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
		ui.Text(c, entry.At.Format("15:04:05")).FontSize(11.5).
			TextColor(t.TextMuted).Width(64)
		ui.Badge(c, levelName(entry.Kind)).Background(levelColor(t, entry.Kind).Alpha(0.14)).
			TextColor(levelColor(t, entry.Kind)).Width(64)
		ui.Text(c, entry.Message).FontSize(12.5).Grow(1)
		// A line that kept repeating says so once instead of filling the log.
		if entry.Count > 1 {
			ui.Text(c, fmt.Sprintf("×%d", entry.Count)).FontSize(11.5).
				TextColor(t.TextMuted)
		}
	})
}

// levelName keeps the badge short.
func levelName(kind string) string {
	switch kind {
	case "warn":
		return locale.T("警告")
	case "error":
		return locale.T("错误")
	default:
		return locale.T("信息")
	}
}

func levelColor(t *ui.Theme, kind string) ui.Color {
	switch kind {
	case "error":
		return t.Danger
	case "warn":
		return t.Warning
	case "info":
		return t.Accent
	default:
		return t.TextMuted
	}
}

func peerColor(t *ui.Theme, peer easytier.PeerView) ui.Color {
	if peer.Local {
		return t.TextMuted
	}
	if peer.Direct {
		return t.Success
	}
	return t.Warning
}

// pathLabel says how traffic reaches a peer.
func pathLabel(peer easytier.PeerView) string {
	switch {
	case peer.Local:
		return locale.T("本机")
	case peer.Direct:
		if peer.Tunnel != "" {
			return locale.T("直连 · ") + peer.Tunnel
		}
		return locale.T("直连")
	default:
		return locale.T("中继")
	}
}

func pathColor(t *ui.Theme, peer easytier.PeerView) ui.Color {
	switch {
	case peer.Local:
		return t.TextMuted
	case peer.Direct:
		return t.Success
	default:
		return t.Warning
	}
}

func trafficText(peer easytier.PeerView) string {
	if peer.Local || (peer.RxBytes == "-" && peer.TxBytes == "-") {
		return "—"
	}
	return peer.RxBytes + " / " + peer.TxBytes
}

func latencyText(ms float64) string {
	switch {
	case ms <= 0:
		return "—"
	case ms < 1:
		return fmt.Sprintf("%.2f ms", ms)
	case ms < 10:
		return fmt.Sprintf("%.1f ms", ms)
	default:
		return fmt.Sprintf("%.0f ms", ms)
	}
}

func latencyColor(t *ui.Theme, ms float64) ui.Color {
	switch {
	case ms <= 0:
		return t.TextMuted
	case ms < 60:
		return t.Success
	case ms < 200:
		return t.Warning
	default:
		return t.Danger
	}
}

// natName turns EasyTier's NAT type number into something readable.
func natName(kind int) string {
	switch kind {
	case 0:
		return "Unknown"
	case 1:
		return "OpenInternet"
	case 2:
		return "NoPAT"
	case 3:
		return "FullCone"
	case 4:
		return "Restricted"
	case 5:
		return "PortRestricted"
	case 6:
		return "Symmetric"
	case 7:
		return "SymmetricUDPFirewall"
	case 8:
		return "SymmetricEasyInc"
	case 9:
		return "SymmetricEasyDec"
	default:
		return fmt.Sprint(kind)
	}
}

func shortDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf(locale.T("%d 秒"), int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf(locale.T("%d 分 %d 秒"), int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf(locale.T("%d 时 %d 分"), int(d.Hours()), int(d.Minutes())%60)
}
