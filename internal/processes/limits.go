package processes

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Limits caps one command's run: Output the bytes it may write, stdout and
// stderr together, and Memory the resident memory its owned process group
// may hold, summed over its members, where the system measures it
// (MemoryMeasured). Zero caps nothing. A command over a cap is killed, its
// group joined as at its own timeout, and its Result's OverLimit names the
// cap.
type Limits struct {
	Output, Memory int64
}

// The caps a command can go over, as Result.OverLimit names them.
const (
	OverOutput = "output"
	OverMemory = "memory"
)

// keptOutputEnd is how much of each end of a command's output a run keeps:
// past twice as much, the bytes between the two ends are left out, and
// only the lines they held are counted, whatever a line's length.
const keptOutputEnd = 64 << 10

// memorySampling is how often the resident memory of a command's owned
// process group is read.
const memorySampling = 50 * time.Millisecond

// MemoryMeasured says whether a command's memory is measured on platform:
// on Linux, from /proc. macOS and Windows measure none, so no memory cap
// holds there.
func MemoryMeasured(platform string) bool { return platform == "linux" }

// keptOutput is a command's output as a run keeps it, whichever goroutine
// writes it: its first keptOutputEnd bytes, then its last, the lines of
// those between counted, and all it wrote counted against a cap.
type keptOutput struct {
	mu      sync.Mutex
	head    []byte
	tail    []byte
	written int64
	leftOut int // the lines of the bytes dropped from the front of tail
	limit   int64
	over    func()
	crossed bool
}

// Write keeps p as the output's next bytes, and calls the cap's action
// once when the output passes its limit.
func (k *keptOutput) Write(p []byte) (int, error) {
	n := len(p)
	k.mu.Lock()
	if room := keptOutputEnd - len(k.head); room > 0 {
		take := min(room, len(p))
		k.head = append(k.head, p[:take]...)
		p = p[take:]
	}
	switch {
	case len(p) >= keptOutputEnd:
		k.leftOut += bytes.Count(k.tail, newline) + bytes.Count(p[:len(p)-keptOutputEnd], newline)
		k.tail = append(k.tail[:0], p[len(p)-keptOutputEnd:]...)
	case len(p) > 0:
		k.tail = append(k.tail, p...)
		if len(k.tail) > 2*keptOutputEnd {
			k.trim()
		}
	}
	k.mu.Unlock()
	k.count(n)
	return n, nil
}

var newline = []byte{'\n'}

// trim drops the front of tail, counting its lines, down to keptOutputEnd.
func (k *keptOutput) trim() {
	if drop := len(k.tail) - keptOutputEnd; drop > 0 {
		k.leftOut += bytes.Count(k.tail[:drop], newline)
		k.tail = append(k.tail[:0], k.tail[drop:]...)
	}
}

// count counts n bytes the command wrote, kept or written elsewhere,
// against the cap, calling its action once past it.
func (k *keptOutput) count(n int) {
	k.mu.Lock()
	k.written += int64(n)
	cross := k.limit > 0 && k.written > k.limit && !k.crossed
	if cross {
		k.crossed = true
	}
	over := k.over
	k.mu.Unlock()
	if cross && over != nil {
		over()
	}
}

// arm sets what passing the cap does, once the command has started: the
// cap passed before is acted on now.
func (k *keptOutput) arm(over func()) {
	k.mu.Lock()
	k.over = over
	crossed := k.crossed
	k.mu.Unlock()
	if crossed {
		over()
	}
}

// kept is the output as kept, all of it or its two ends, where in text the
// bytes left out were, and how many lines they held.
func (k *keptOutput) kept() (text string, written int64, cut, leftOut int) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.trim()
	return string(k.head) + string(k.tail), k.written, len(k.head), k.leftOut
}

// countedWriter passes a command's writes to a writer of its caller's,
// counting them against the cap of its kept output.
type countedWriter struct {
	to   io.Writer
	kept *keptOutput
}

func (w *countedWriter) Write(p []byte) (int, error) {
	n, err := w.to.Write(p)
	w.kept.count(n)
	return n, err
}

// overLimit is the first cap a command went over, which killed it.
type overLimit struct {
	mu    sync.Mutex
	name  string
	ended bool
}

// hit records the cap name as the one the command went over, unless it
// went over another first, and says whether to kill it: not once its run
// has ended, when a cap passed as its last output is read names the cap
// but kills nothing.
func (o *overLimit) hit(name string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.name != "" {
		return false
	}
	o.name = name
	return !o.ended
}

// end marks the command's run ended.
func (o *overLimit) end() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.ended = true
}

func (o *overLimit) which() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.name
}

// memoryWatch samples the resident memory of an owned process group while
// its command runs, keeping its peak.
type memoryWatch struct {
	stop chan struct{}
	done chan struct{}
	peak int64
}

// watchMemory samples the group pgid every memorySampling, from now until
// stopped, calling over once its memory passes limit, when limit is set.
func watchMemory(pgid int, limit int64, over func()) *memoryWatch {
	w := &memoryWatch{stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(w.done)
		ticker := time.NewTicker(memorySampling)
		defer ticker.Stop()
		crossed := false
		for {
			if held, err := groupMemory(pgid); err == nil {
				w.peak = max(w.peak, held)
				if limit > 0 && held > limit && !crossed {
					crossed = true
					over()
				}
			}
			select {
			case <-w.stop:
				return
			case <-ticker.C:
			}
		}
	}()
	return w
}

// stopped stops the sampling and gives the peak it read.
func (w *memoryWatch) stopped() int64 {
	close(w.stop)
	<-w.done
	return w.peak
}

// groupMemory is the resident memory of the members of process group pgid,
// summed, in bytes, read from Linux's /proc.
func groupMemory(pgid int) (int64, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, err
	}
	page := int64(os.Getpagesize())
	var held int64
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
				continue
			}
			return 0, err
		}
		group, pages, err := statGroupAndPages(data)
		if err != nil {
			return 0, fmt.Errorf("process %s: %w", entry.Name(), err)
		}
		if group == pgid {
			held += pages * page
		}
	}
	return held, nil
}

// statGroupAndPages is the process group and the resident pages of a
// process's /proc stat line: its fifth and its twenty-fourth fields.
func statGroupAndPages(stat []byte) (int, int64, error) {
	closeParen := bytes.LastIndexByte(stat, ')')
	if closeParen < 0 {
		return 0, 0, errors.New("malformed process stat")
	}
	fields := strings.Fields(string(stat[closeParen+1:]))
	if len(fields) < 22 {
		return 0, 0, errors.New("short process stat")
	}
	group, err := strconv.Atoi(fields[2])
	if err != nil {
		return 0, 0, fmt.Errorf("parse process group: %w", err)
	}
	pages, err := strconv.ParseInt(fields[21], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse resident pages: %w", err)
	}
	return group, pages, nil
}
