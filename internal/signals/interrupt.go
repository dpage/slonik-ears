// Package signals holds the interrupt handling shared by both commands.
package signals

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

// ExitOnSecondInterrupt makes a second Ctrl-C leave immediately.
//
// signal.NotifyContext registers a handler, which stops Go's default
// behaviour of killing the process on SIGINT. Its own goroutine cancels the
// context on the first signal and then exits, so every later Ctrl-C is
// delivered to a channel nobody is reading and is silently swallowed. If
// graceful shutdown then takes a while — a transcription still running, a
// server that cannot be reached — the operator is left hammering Ctrl-C at
// something that looks hung, with no way out but another terminal.
//
// There is always a way out now. The first interrupt shuts down cleanly; the
// second gives up on that and exits, which is what somebody pressing it twice
// is asking for.
//
// It returns a function that stops watching, for tests.
func ExitOnSecondInterrupt(w io.Writer, message string) func() {
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		select {
		case <-sig: // the first one: handled gracefully elsewhere
		case <-done:
			return
		}
		select {
		case <-sig: // the second: the operator wants out now
		case <-done:
			return
		}
		fmt.Fprintf(w, "\n%s\n", message)
		// 130 is the conventional "terminated by Ctrl-C" status.
		os.Exit(130)
	}()

	return func() {
		signal.Stop(sig)
		close(done)
	}
}
