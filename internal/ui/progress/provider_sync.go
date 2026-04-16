package progress

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

type ProviderSyncState string

const (
	ProviderSyncStateResolving  ProviderSyncState = "resolving"
	ProviderSyncStatePulling    ProviderSyncState = "pulling"
	ProviderSyncStateExtracting ProviderSyncState = "extracting"
	ProviderSyncStateInstalling ProviderSyncState = "installing"
	ProviderSyncStateReady      ProviderSyncState = "ready"
	ProviderSyncStateCached     ProviderSyncState = "cached"
	ProviderSyncStateFailed     ProviderSyncState = "failed"
)

type providerSyncRow struct {
	label   string
	state   ProviderSyncState
	note    string
	started time.Time
	elapsed time.Duration
	done    bool
	failed  bool
}

type ProviderSyncSurface struct {
	w       io.Writer
	start   time.Time
	live    bool
	verbose bool
	total   int

	mu            sync.Mutex
	rows          map[string]*providerSyncRow
	order         []string
	frame         int
	renderedLines int
	finished      bool
	finishErr     bool
}

func NewProviderSyncSurface(w io.Writer, total int, verbose bool) *ProviderSyncSurface {
	if w == nil {
		return nil
	}
	return &ProviderSyncSurface{
		w:       w,
		start:   time.Now(),
		live:    isTTYWriter(w) && !verbose,
		verbose: verbose,
		total:   total,
		rows:    map[string]*providerSyncRow{},
		order:   make([]string, 0, total),
	}
}

func (s *ProviderSyncSurface) Start(key, label string) {
	s.update(key, label, ProviderSyncStateResolving, "", false, nil)
}

func (s *ProviderSyncSurface) Update(key, label string, state ProviderSyncState, note string) {
	s.update(key, label, state, note, false, nil)
}

func (s *ProviderSyncSurface) Complete(key, label string, cached bool) {
	state := ProviderSyncStateReady
	if cached {
		state = ProviderSyncStateCached
	}
	s.update(key, label, state, "", true, nil)
}

func (s *ProviderSyncSurface) Fail(key, label string, err error) {
	s.update(key, label, ProviderSyncStateFailed, errorDetail(err), true, err)
}

func (s *ProviderSyncSurface) Finish(err error) {
	if s == nil || s.w == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished {
		return
	}
	s.finished = true
	s.finishErr = err != nil
	if s.verbose || !s.live {
		s.printFinalLocked()
		return
	}
	s.renderLocked(true)
}

func (s *ProviderSyncSurface) update(key, label string, state ProviderSyncState, note string, done bool, err error) {
	if s == nil || s.w == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished {
		return
	}
	row, ok := s.rows[key]
	if !ok {
		row = &providerSyncRow{started: time.Now()}
		s.rows[key] = row
		s.order = append(s.order, key)
	}
	if strings.TrimSpace(label) != "" {
		row.label = label
	}
	changed := row.state != state || row.note != note || row.done != done
	row.state = state
	row.note = note
	row.done = done
	row.failed = err != nil
	if done {
		row.elapsed = time.Since(row.started)
	}
	if s.verbose {
		if changed {
			s.printTransitionLocked(row)
		}
		return
	}
	if s.live {
		s.renderLocked(false)
	}
}

func (s *ProviderSyncSurface) printTransitionLocked(row *providerSyncRow) {
	icon := rowIcon(row)
	detail := fmt.Sprintf("%s %s", row.label, row.state)
	if row.note != "" {
		detail += "  " + row.note
	}
	_, _ = fmt.Fprintf(s.w, "%s %-11s %s [%s]\n", icon, "provider", detail, syncSurfaceDuration(row))
}

func (s *ProviderSyncSurface) renderLocked(finished bool) {
	lines := s.renderLinesLocked(finished)
	if len(lines) == 0 {
		return
	}
	if s.renderedLines > 0 {
		_, _ = fmt.Fprintf(s.w, "\x1b[%dF\x1b[J", s.renderedLines)
	}
	_, _ = fmt.Fprint(s.w, strings.Join(lines, "\n")+"\n")
	s.renderedLines = len(lines)
}

func (s *ProviderSyncSurface) printFinalLocked() {
	lines := s.renderLinesLocked(true)
	if len(lines) == 0 {
		return
	}
	_, _ = fmt.Fprint(s.w, strings.Join(lines, "\n")+"\n")
}

func (s *ProviderSyncSurface) renderLinesLocked(finished bool) []string {
	if len(s.order) == 0 {
		return nil
	}
	labelWidth := 0
	readyCount := 0
	failedCount := 0
	for _, key := range s.order {
		row := s.rows[key]
		if row == nil {
			continue
		}
		if width := len(row.label); width > labelWidth {
			labelWidth = width
		}
		if row.state == ProviderSyncStateReady || row.state == ProviderSyncStateCached {
			readyCount++
		}
		if row.failed {
			failedCount++
		}
	}
	if labelWidth < 16 {
		labelWidth = 16
	}
	if labelWidth > 32 {
		labelWidth = 32
	}
	elapsed := shortDuration(time.Since(s.start))
	lead := frames[s.frame%len(frames)]
	if finished {
		if failedCount > 0 || s.finishErr {
			lead = "✗"
		} else {
			lead = "✓"
		}
	} else {
		s.frame++
	}
	lines := []string{fmt.Sprintf("%s Installing providers (%d)  [%d/%d ready]  %s", lead, s.total, readyCount, s.total, elapsed), ""}
	for _, key := range s.order {
		row := s.rows[key]
		if row == nil {
			continue
		}
		label := row.label
		if len(label) > labelWidth {
			label = label[:labelWidth]
		}
		line := fmt.Sprintf("  %-*s  %s %-11s %6s", labelWidth, label, rowIcon(row), row.state, syncSurfaceDuration(row))
		if row.note != "" {
			line += "  " + row.note
		}
		lines = append(lines, line)
	}
	if finished {
		lines = append(lines, strings.Repeat("─", 40))
		if failedCount > 0 || s.finishErr {
			lines = append(lines, fmt.Sprintf("✗ Failed after %s", elapsed))
		} else {
			lines = append(lines, fmt.Sprintf("✓ Installed %d providers in %s", s.total, elapsed))
		}
	}
	return lines
}

func syncSurfaceDuration(row *providerSyncRow) string {
	if row == nil {
		return "0ms"
	}
	if row.done {
		return shortDuration(row.elapsed)
	}
	return shortDuration(time.Since(row.started))
}

func rowIcon(row *providerSyncRow) string {
	if row == nil {
		return "⟳"
	}
	switch row.state {
	case ProviderSyncStateReady, ProviderSyncStateCached:
		return "✓"
	case ProviderSyncStateFailed:
		return "✗"
	default:
		return "⟳"
	}
}

func errorDetail(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
