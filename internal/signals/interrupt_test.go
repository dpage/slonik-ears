package signals_test

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/dpage/slonik-ears/internal/signals"
)

// The child half of TestSecondInterruptExits: it installs the handler, says so,
// and then hangs exactly as a shutdown that cannot finish would.
func TestMain(m *testing.M) {
	if os.Getenv("EARS_INTERRUPT_CHILD") == "1" {
		signals.ExitOnSecondInterrupt(os.Stderr, "interrupted again")
		_, _ = os.Stdout.WriteString("ready\n")
		select {} // a graceful shutdown that never completes
	}
	os.Exit(m.Run())
}

// A second interrupt must always get you out. Without this the process
// installs a signal handler, swallows every Ctrl-C after the first, and leaves
// the operator with no way to stop it — which is exactly what happened when a
// listener was started against an unreachable server.
func TestSecondInterruptExits(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "EARS_INTERRUPT_CHILD=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	// Wait until the handler is installed, otherwise the signals race it.
	ready := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(stdout)
		if sc.Scan() && sc.Text() == "ready" {
			close(ready)
		}
	}()
	select {
	case <-ready:
	case <-time.After(10 * time.Second):
		t.Fatal("the child never became ready")
	}

	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	// The first interrupt is handled gracefully elsewhere, so the child
	// deliberately keeps running.
	time.Sleep(300 * time.Millisecond)
	if cmd.ProcessState != nil {
		t.Fatal("the child exited on the first interrupt; it should shut down gracefully instead")
	}

	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("expected a non-zero exit, got %v", err)
		}
		if code := exit.ExitCode(); code != 130 {
			t.Errorf("exit code = %d, want 130 (terminated by Ctrl-C)", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("a second interrupt did not stop the process: there is no way out")
	}
}
