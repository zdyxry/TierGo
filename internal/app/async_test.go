package app

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zdyxry/easytier-mygo/internal/easytier"
)

func flushAsync(t *testing.T, a *App) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for len(a.asyncJobs) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("asynchronous operation did not finish")
		}
		time.Sleep(time.Millisecond)
		a.drainAsync()
	}
	a.drainAsync()
}

func TestRunAsyncDoesNotBlockAndRejectsDuplicates(t *testing.T) {
	a := newTestApp(t)
	started := make(chan struct{})
	release := make(chan struct{})
	var applied atomic.Int32
	if !a.runAsync("blocked", func() func() {
		close(started)
		<-release
		return func() { applied.Add(1) }
	}) {
		t.Fatal("the first operation was rejected")
	}
	<-started
	if a.runAsync("blocked", func() func() { return nil }) {
		t.Fatal("a duplicate operation was accepted")
	}
	close(release)
	flushAsync(t, a)
	if got := applied.Load(); got != 1 {
		t.Fatalf("applied %d results, want 1", got)
	}
}

func TestServiceStatusDoesNotBlockRendering(t *testing.T) {
	a := newTestApp(t)
	started := make(chan struct{})
	release := make(chan struct{})
	a.serviceStatus = func(context.Context) (easytier.ServiceState, error) {
		close(started)
		<-release
		return easytier.ServiceStopped, nil
	}
	a.page = "service"
	before := time.Now()
	tt := ui.NewTester(a.view, 1040, 900)
	if elapsed := time.Since(before); elapsed > 500*time.Millisecond {
		t.Fatalf("rendering blocked for %v", elapsed)
	}
	<-started
	if !a.asyncBusy("service-status") {
		t.Fatal("the status operation is not pending")
	}
	close(release)
	flushAsync(t, a)
	tt.Frame()
	if !tt.HasText("已停止") {
		t.Fatalf("the completed status was not drawn: %q", tt.Texts())
	}
}

func TestConnectChecksServiceWithoutBlocking(t *testing.T) {
	a := newTestApp(t)
	a.draft().config.EnableTUN = false
	release := make(chan struct{})
	a.serviceStatus = func(context.Context) (easytier.ServiceState, error) {
		<-release
		return easytier.ServiceRunning, nil
	}
	tt := ui.NewTester(a.view, 1040, 700)
	before := time.Now()
	if err := tt.Click("连接"); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(before); elapsed > 500*time.Millisecond {
		t.Fatalf("connect blocked for %v", elapsed)
	}
	if a.ctl().Running() {
		t.Fatal("the core started before the service check finished")
	}
	close(release)
	flushAsync(t, a)
	if a.page != "service" {
		t.Fatalf("page = %q, want service", a.page)
	}
}

func TestConnectDiscardsResultForAnotherProfile(t *testing.T) {
	a := newTestApp(t)
	a.draft().config.EnableTUN = false
	first := a.selected
	second := easytier.NewProfile("另一个网络")
	a.profiles = append(a.profiles, second)
	release := make(chan struct{})
	a.serviceStatus = func(context.Context) (easytier.ServiceState, error) {
		<-release
		return easytier.ServiceNotInstalled, nil
	}
	tt := ui.NewTester(a.view, 1040, 700)
	if err := tt.Click("连接"); err != nil {
		t.Fatal(err)
	}
	a.selected = second.ID
	close(release)
	flushAsync(t, a)
	if len(a.mgr.RunningIDs()) != 0 || a.mgr.Running(first) {
		t.Fatal("a stale service check started the previously selected network")
	}
}

func TestServiceOperationIsAsynchronousAndReportsErrors(t *testing.T) {
	a := newTestApp(t)
	a.serviceStatus = func(context.Context) (easytier.ServiceState, error) {
		return easytier.ServiceStopped, nil
	}
	release := make(chan struct{})
	started := make(chan struct{})
	before := time.Now()
	a.runServiceOp("unexpected success", func(context.Context) error {
		close(started)
		<-release
		return errors.New("operation failed")
	})
	if elapsed := time.Since(before); elapsed > 500*time.Millisecond {
		t.Fatalf("service operation blocked for %v", elapsed)
	}
	<-started
	if !a.asyncBusy("service-operation") {
		t.Fatal("the service operation is not pending")
	}
	close(release)
	flushAsync(t, a)
	if a.notice != "operation failed" {
		t.Fatalf("notice = %q", a.notice)
	}
	if a.serviceState != easytier.ServiceStopped {
		t.Fatalf("state = %v, want stopped", a.serviceState)
	}
}

func TestServiceStartRequiresLocalCoresToStop(t *testing.T) {
	a := newTestApp(t)
	id := a.profiles[0].ID
	a.settings.EnabledNetworks = []string{id}
	a.serviceStatus = func(context.Context) (easytier.ServiceState, error) {
		return easytier.ServiceStopped, nil
	}
	a.stopAllAndWait = func(time.Duration) error { return errors.New("stop failed") }
	started := false
	a.serviceStartFn = func(context.Context) error {
		started = true
		return nil
	}
	a.startSystemService()
	flushAsync(t, a)
	if started {
		t.Fatal("the system service started while a local core was still stopping")
	}
	if len(a.settings.EnabledNetworks) != 1 || a.settings.EnabledNetworks[0] != id {
		t.Fatal("the restorable session was forgotten after a failed handoff")
	}
	if a.notice != "停止本地网络失败：stop failed" {
		t.Fatalf("notice = %q", a.notice)
	}
}
