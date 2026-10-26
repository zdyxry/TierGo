package easytier

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/zdyxry/easytier-mygo/internal/locale"
)

// Manager owns one Controller per network profile, so several networks run
// at the same time — each as its own easytier-core process with its own
// directory, ports and management address. A crash of one takes down only
// its own network.
type Manager struct {
	mu          sync.Mutex
	win         *mygo.Window
	controllers map[string]*Controller

	// newController builds the controller for one profile; tests replace it
	// to give every instance its own scripted core.
	newController func(key string) *Controller
}

func NewManager() *Manager {
	manager := &Manager{controllers: map[string]*Controller{}}
	manager.newController = NewController
	return manager
}

// AttachWindow lets every controller, present and future, redraw the window.
func (m *Manager) AttachWindow(win *mygo.Window) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.win = win
	for _, controller := range m.controllers {
		controller.AttachWindow(win)
	}
}

// Controller returns the controller of one profile, creating it on first
// use.
func (m *Manager) Controller(profileID string) *Controller {
	m.mu.Lock()
	defer m.mu.Unlock()
	if controller, ok := m.controllers[profileID]; ok {
		return controller
	}
	controller := m.newController(profileID)
	if m.win != nil {
		controller.AttachWindow(m.win)
	}
	m.controllers[profileID] = controller
	return controller
}

// Start brings one profile's network up.
func (m *Manager) Start(profile Profile) {
	m.Controller(profile.ID).Start(profile.Config)
}

// Restore reconnects the networks the user had enabled when the client was
// last open. Profiles that are gone or already running are skipped; every
// other one is still tried, and the first failure comes back.
func (m *Manager) Restore(profiles []Profile, ids []string) error {
	var firstErr error
	for _, id := range ids {
		var profile *Profile
		for index := range profiles {
			if profiles[index].ID == id {
				profile = &profiles[index]
				break
			}
		}
		if profile == nil || m.Running(id) {
			continue
		}
		// A device-mode Portal key made now would change on every launch;
		// keep it with the profile, the same way connecting by hand does.
		hadKey := profile.Config.VPNPortal != nil && profile.Config.VPNPortal.PrivateKey != ""
		if err := EnsurePortalKey(profile.Config.VPNPortal); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf(locale.T("恢复网络「%s」失败：%w"), profile.Name, err)
			}
			continue
		}
		if !hadKey && profile.Config.VPNPortal != nil && profile.Config.VPNPortal.PrivateKey != "" {
			if err := SaveProfile(*profile); err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf(locale.T("恢复网络「%s」失败：%w"), profile.Name, err)
				}
				continue
			}
		}
		m.Start(*profile)
	}
	return firstErr
}

// Stop asks one profile's network to go down. A profile that never ran has
// nothing to stop.
func (m *Manager) Stop(profileID string) {
	m.mu.Lock()
	controller, ok := m.controllers[profileID]
	m.mu.Unlock()
	if ok {
		controller.Stop()
	}
}

// Remove releases a stopped profile's controller and instance-local state.
func (m *Manager) Remove(profileID string) error {
	m.mu.Lock()
	controller, ok := m.controllers[profileID]
	if ok {
		controller.mu.Lock()
		active := controller.cancel != nil
		controller.mu.Unlock()
		if active {
			m.mu.Unlock()
			return fmt.Errorf("%s", locale.T("网络仍在运行或停止中"))
		}
	}
	delete(m.controllers, profileID)
	m.mu.Unlock()
	forgetSecrets(profileID)
	return nil
}

// Snapshot is one profile's state; a profile that never ran is stopped.
func (m *Manager) Snapshot(profileID string) Snapshot {
	m.mu.Lock()
	controller, ok := m.controllers[profileID]
	m.mu.Unlock()
	if !ok {
		return Snapshot{Phase: PhaseStopped}
	}
	return controller.Snapshot()
}

// Running reports whether the profile's core is up or on its way.
func (m *Manager) Running(profileID string) bool {
	m.mu.Lock()
	controller, ok := m.controllers[profileID]
	m.mu.Unlock()
	return ok && controller.Running()
}

// RunningIDs is the set of profiles whose core is up or on its way.
func (m *Manager) RunningIDs() map[string]bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	running := map[string]bool{}
	for id, controller := range m.controllers {
		if controller.Running() {
			running[id] = true
		}
	}
	return running
}

// AnyRunning reports whether any network is up or on its way.
func (m *Manager) AnyRunning() bool {
	return len(m.RunningIDs()) > 0
}

// StopAllAndWait stops every network and waits until all cores are gone, for
// a quit that must not leave TUN devices behind.
func (m *Manager) StopAllAndWait(timeout time.Duration) error {
	m.mu.Lock()
	controllers := make([]*Controller, 0, len(m.controllers))
	for _, controller := range m.controllers {
		controllers = append(controllers, controller)
	}
	m.mu.Unlock()
	for _, controller := range controllers {
		controller.Stop()
	}
	results := make(chan error, len(controllers))
	for _, controller := range controllers {
		go func() { results <- controller.StopAndWait(timeout) }()
	}
	var errs []error
	for range controllers {
		if err := <-results; err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
