package listener

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/dpage/slonik-ears/internal/protocol"
)

// ConsolePublisher prints the transcript to a writer instead of sending it
// anywhere. It backs --dry-run, which is the quickest way to find out whether
// a microphone, a model and a room are on speaking terms.
type ConsolePublisher struct {
	mu   sync.Mutex
	w    io.Writer
	n    int
	last string
}

// NewConsolePublisher returns a publisher that writes to w.
func NewConsolePublisher(w io.Writer) *ConsolePublisher {
	return &ConsolePublisher{w: w}
}

// PublishFinal implements Publisher.
func (p *ConsolePublisher) PublishFinal(seg protocol.Segment) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.n++
	p.clearPartial()
	fmt.Fprintf(p.w, "[%s] %s\n", clock(seg.StartMs), seg.Text)
}

// PublishPartial implements Publisher.
func (p *ConsolePublisher) PublishPartial(partial protocol.Partial) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.clearPartial()
	if partial.Text == "" {
		return
	}
	// Overwrite in place so the preview does not scroll the committed text
	// off the screen.
	line := "  ... " + partial.Text
	fmt.Fprint(p.w, line+"\r")
	p.last = line
}

func (p *ConsolePublisher) clearPartial() {
	if p.last == "" {
		return
	}
	blank := make([]byte, len(p.last))
	for i := range blank {
		blank[i] = ' '
	}
	fmt.Fprint(p.w, "\r"+string(blank)+"\r")
	p.last = ""
}

// PublishStatus implements Publisher; the console does not need telemetry.
func (p *ConsolePublisher) PublishStatus(protocol.Status) {}

// Connected implements Publisher.
func (p *ConsolePublisher) Connected() bool { return true }

// Queued implements Publisher.
func (p *ConsolePublisher) Queued() int { return 0 }

func clock(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	return fmt.Sprintf("%02d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}
