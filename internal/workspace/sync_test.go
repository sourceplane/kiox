package workspace

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/sourceplane/tinx/internal/state"
)

func TestExecuteWorkspaceInstallRequestsDeduplicatesAndRunsInParallel(t *testing.T) {
	t.Setenv(syncInstallConcurrencyEnv, "2")
	requests := []workspaceInstallRequest{
		{Alias: "alpha", InstallSource: "ghcr.io/acme/provider@sha256:111", AllowCache: true},
		{Alias: "beta", InstallSource: "ghcr.io/acme/provider@sha256:111", AllowCache: true},
		{Alias: "gamma", InstallSource: "ghcr.io/acme/other@sha256:222", AllowCache: true},
	}

	var (
		calls      = map[string]int{}
		running    int
		maxRunning int
		mu         sync.Mutex
		results    map[string]workspaceInstallOutcome
		err        error
	)
	started := make(chan string, 4)
	release := make(chan struct{})
	done := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })

	go func() {
		results, err = executeWorkspaceInstallRequests(context.Background(), requests, nil, func(ctx context.Context, task workspaceInstallTask) (workspaceInstallPrepared, error) {
			key := workspaceInstallTaskKey(task.Request)
			mu.Lock()
			calls[key]++
			running++
			if running > maxRunning {
				maxRunning = running
			}
			mu.Unlock()
			started <- key
			<-release
			mu.Lock()
			running--
			mu.Unlock()
			if task.Request.InstallSource == "ghcr.io/acme/other@sha256:222" {
				return workspaceInstallPrepared{Task: task, Metadata: state.ProviderMetadata{Namespace: "acme", Name: "other", Version: "v1"}}, nil
			}
			return workspaceInstallPrepared{Task: task, Metadata: state.ProviderMetadata{Namespace: "acme", Name: "provider", Version: "v1"}}, nil
		}, func(context.Context, workspacePendingRuntime) error {
			return nil
		})
		close(done)
	}()

	seen := map[string]struct{}{}
	deadline := time.After(2 * time.Second)
	for len(seen) < 2 {
		select {
		case key := <-started:
			seen[key] = struct{}{}
		case <-deadline:
			releaseOnce.Do(func() { close(release) })
			t.Fatal("expected two install tasks to start in parallel")
		}
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for install requests to finish")
	}
	if err != nil {
		t.Fatalf("executeWorkspaceInstallRequests() error = %v", err)
	}
	providerKey := workspaceInstallTaskKey(requests[0])
	otherKey := workspaceInstallTaskKey(requests[2])
	if calls[providerKey] != 1 {
		t.Fatalf("expected duplicate install source to run once, got %d", calls[providerKey])
	}
	if calls[otherKey] != 1 {
		t.Fatalf("expected unique install source to run once, got %d", calls[otherKey])
	}
	if maxRunning < 2 {
		t.Fatalf("expected install requests to overlap, max concurrency = %d", maxRunning)
	}
	if got := state.MetadataKey(results["alpha"].Metadata); got != state.MetadataKey(results["beta"].Metadata) {
		t.Fatalf("expected duplicate install requests to share metadata, got %q and %q", got, state.MetadataKey(results["beta"].Metadata))
	}
	if got := state.MetadataKey(results["gamma"].Metadata); got != "acme/other@v1" {
		t.Fatalf("unexpected metadata for gamma: %q", got)
	}
}

func TestExecuteWorkspaceInstallRequestsLimitsRuntimeHydration(t *testing.T) {
	t.Setenv(syncInstallConcurrencyEnv, "4")
	requests := []workspaceInstallRequest{
		{Alias: "alpha", InstallSource: "ghcr.io/acme/a@sha256:111", AllowCache: true},
		{Alias: "beta", InstallSource: "ghcr.io/acme/b@sha256:222", AllowCache: true},
		{Alias: "gamma", InstallSource: "ghcr.io/acme/c@sha256:333", AllowCache: true},
	}
	var (
		running    int
		maxRunning int
		mu         sync.Mutex
	)
	started := make(chan struct{}, len(requests))
	release := make(chan struct{})
	done := make(chan struct{})
	var runErr error
	go func() {
		_, runErr = executeWorkspaceInstallRequests(context.Background(), requests, nil, func(ctx context.Context, task workspaceInstallTask) (workspaceInstallPrepared, error) {
			return workspaceInstallPrepared{
				Task:         task,
				Metadata:     state.ProviderMetadata{Namespace: "acme", Name: task.Request.Alias, Version: "v1"},
				NeedsRuntime: true,
			}, nil
		}, func(context.Context, workspacePendingRuntime) error {
			mu.Lock()
			running++
			if running > maxRunning {
				maxRunning = running
			}
			mu.Unlock()
			started <- struct{}{}
			<-release
			mu.Lock()
			running--
			mu.Unlock()
			return nil
		})
		close(done)
	}()
	deadline := time.After(2 * time.Second)
	for count := 0; count < defaultSyncRuntimeConcurrency; count++ {
		select {
		case <-started:
		case <-deadline:
			close(release)
			t.Fatal("expected runtime hydration tasks to start")
		}
	}
	select {
	case <-started:
		close(release)
		t.Fatal("expected runtime hydration concurrency to stay bounded")
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for runtime hydration tasks to finish")
	}
	if runErr != nil {
		t.Fatalf("executeWorkspaceInstallRequests() error = %v", runErr)
	}
	if maxRunning != defaultSyncRuntimeConcurrency {
		t.Fatalf("expected runtime hydration max concurrency %d, got %d", defaultSyncRuntimeConcurrency, maxRunning)
	}
}

func TestSyncInstallConcurrencyHonorsOverride(t *testing.T) {
	t.Setenv(syncInstallConcurrencyEnv, "2")
	if got := syncInstallConcurrency(5); got != 2 {
		t.Fatalf("syncInstallConcurrency(5) = %d, want 2", got)
	}
	t.Setenv(syncInstallConcurrencyEnv, "invalid")
	if got := syncInstallConcurrency(2); got != 2 {
		t.Fatalf("syncInstallConcurrency(2) = %d, want 2", got)
	}
}
