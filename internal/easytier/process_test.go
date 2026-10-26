package easytier

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

const coreProcessHelperEnv = "EASYTIER_MYGO_CORE_PROCESS_HELPER"

func TestCoreProcessHelperProcess(t *testing.T) {
	if os.Getenv(coreProcessHelperEnv) == "" {
		return
	}
	mode := ""
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) {
			mode = os.Args[i+1]
			break
		}
	}
	if mode == "child" {
		fmt.Fprintln(os.Stdout, "child stdout")
		fmt.Fprintln(os.Stderr, "child stderr")
		time.Sleep(time.Minute)
		return
	}
	if mode != "parent-exit" && mode != "parent-kill" {
		os.Exit(2)
	}

	child := exec.Command(os.Args[0], "-test.run=^TestCoreProcessHelperProcess$", "--", "child")
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if err := child.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Printf("ready %d\n", child.Process.Pid)
	if mode == "parent-kill" {
		time.Sleep(time.Minute)
	}
}

func TestCoreProcessClosesInheritedPipes(t *testing.T) {
	t.Setenv(coreProcessHelperEnv, "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name string
		mode string
		kill bool
	}{
		{name: "normal exit", mode: "parent-exit"},
		{name: "killed", mode: "parent-kill", kill: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			process, err := startCoreProcess(executable, []string{
				"-test.run=^TestCoreProcessHelperProcess$", "--", test.mode,
			}, "")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(process.Kill)

			childPID, logs := waitForCoreProcessChild(t, process)
			child, err := os.FindProcess(childPID)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = child.Kill() })

			if test.kill {
				process.Kill()
			}
			waitErr := waitForCoreProcess(t, process)
			for line := range process.output() {
				logs = append(logs, line)
			}
			for _, want := range []string{"child stdout", "child stderr"} {
				if !containsProcessLog(logs, want) {
					t.Errorf("merged output does not contain %q: %q", want, logs)
				}
			}
			if test.kill {
				var exitErr *exec.ExitError
				if !errors.As(waitErr, &exitErr) {
					t.Fatalf("Wait() error = %v, want an exec.ExitError", waitErr)
				}
			} else if waitErr != nil {
				t.Fatalf("Wait() error = %v, want nil", waitErr)
			}
		})
	}
}

func waitForCoreProcessChild(t *testing.T, process *coreProcess) (int, []string) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	var logs []string
	for {
		select {
		case line, ok := <-process.output():
			if !ok {
				t.Fatalf("process exited before spawning its child: %v", process.Wait())
			}
			logs = append(logs, line)
			if pidText, ok := strings.CutPrefix(line, "ready "); ok {
				pid, err := strconv.Atoi(pidText)
				if err != nil {
					t.Fatalf("invalid child pid: %v", err)
				}
				return pid, logs
			}
		case <-timer.C:
			t.Fatal("timed out waiting for the helper child")
		}
	}
}

func waitForCoreProcess(t *testing.T, process *coreProcess) error {
	t.Helper()
	select {
	case <-process.done:
		return process.Wait()
	case <-time.After(3 * coreProcessWaitDelay):
		t.Fatal("timed out waiting for coreProcess.done")
		return nil
	}
}

func containsProcessLog(logs []string, want string) bool {
	for _, line := range logs {
		if line == want {
			return true
		}
	}
	return false
}
