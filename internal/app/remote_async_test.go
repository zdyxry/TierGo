package app

import (
	"context"
	"testing"
	"time"

	"github.com/zdyxry/easytier-mygo/internal/easytier"
)

func TestOpenRemoteConfigDoesNotBlock(t *testing.T) {
	a := newTestApp(t)
	network := easytier.RemoteNetwork{ID: "r1", Name: "remote"}
	a.settings.RemoteRPC = "127.0.0.1:15888"
	a.remoteSelected = network.ID
	started := make(chan struct{})
	release := make(chan struct{})
	a.getRemoteConfig = func(context.Context, string, easytier.RemoteNetwork) (easytier.RemoteConfig, error) {
		close(started)
		<-release
		return easytier.RemoteConfig{Network: network}, nil
	}
	before := time.Now()
	a.openRemoteConfig(a.settings.RemoteRPC, network)
	if elapsed := time.Since(before); elapsed > 500*time.Millisecond {
		t.Fatalf("reading remote config blocked for %v", elapsed)
	}
	<-started
	close(release)
	flushAsync(t, a)
	if !a.remoteEditOpen || a.remoteEditConfig.Network.ID != network.ID {
		t.Fatal("the completed remote config was not opened")
	}
}

func TestOpenRemoteConfigDiscardsStaleInstance(t *testing.T) {
	a := newTestApp(t)
	network := easytier.RemoteNetwork{ID: "r1", Name: "remote"}
	a.settings.RemoteRPC = "127.0.0.1:15888"
	a.remoteSelected = network.ID
	release := make(chan struct{})
	a.getRemoteConfig = func(context.Context, string, easytier.RemoteNetwork) (easytier.RemoteConfig, error) {
		<-release
		return easytier.RemoteConfig{Network: network}, nil
	}
	a.openRemoteConfig(a.settings.RemoteRPC, network)
	a.remoteSelected = "r2"
	close(release)
	flushAsync(t, a)
	if a.remoteEditOpen {
		t.Fatal("a stale response opened another instance's editor")
	}
}
