package progress

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

type Tracker struct {
	w         io.Writer
	start     time.Time
	mu        sync.Mutex
	live      bool
	frame     int
	lastWidth int
	finished  bool
}

var frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func New(w io.Writer) *Tracker {
	if w == nil {
		return &Tracker{}
	}
	return &Tracker{w: w, start: time.Now(), live: isTTYWriter(w)}
}

func (t *Tracker) Writer() io.Writer {
	if t == nil {
		return nil
	}
	return t.w
}

func (t *Tracker) Step(name, detail string) {
	t.print("→", name, detail, false)
}

func (t *Tracker) Info(name, detail string) {
	t.print("•", name, detail, false)
}

func (t *Tracker) Cached(name, detail string) {
	t.print("↺", name, detail, false)
}

func (t *Tracker) Done(name, detail string) {
	t.print("✓", name, detail, true)
}

func (t *Tracker) Finish() {
	if t == nil || t.w == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished {
		return
	}
	t.finished = true
	if t.live {
		_, _ = fmt.Fprint(t.w, "\n")
	}
}

func (t *Tracker) print(icon, name, detail string, completed bool) {
	if t == nil || t.w == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished {
		return
	}
	elapsed := ""
	if !t.start.IsZero() {
		elapsed = fmt.Sprintf(" [%s]", shortDuration(time.Since(t.start)))
	}
	if t.live {
		lead := icon
		if !completed {
			lead = frames[t.frame%len(frames)]
			t.frame++
		}
		msg := fmt.Sprintf("%s %-11s %s%s", lead, name, detail, elapsed)
		clearPadding := ""
		if len(msg) < t.lastWidth {
			clearPadding = spaces(t.lastWidth - len(msg))
		}
		_, _ = fmt.Fprintf(t.w, "\r%s%s", msg, clearPadding)
		t.lastWidth = len(msg)
		return
	}
	if detail == "" {
		_, _ = fmt.Fprintf(t.w, "%s %-11s%s\n", icon, name, elapsed)
		return
	}
	_, _ = fmt.Fprintf(t.w, "%s %-11s %s%s\n", icon, name, detail, elapsed)
}

func spaces(count int) string {
	if count <= 0 {
		return ""
	}
	buf := make([]byte, count)
	for i := range buf {
		buf[i] = ' '
	}
	return string(buf)
}

func isTTYWriter(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func shortDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return d.Round(time.Second).String()
}
