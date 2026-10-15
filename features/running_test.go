package features

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProductRunningHandle(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	bin, err := build(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d := binary{bin: bin, root: root}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if _, err = d.Start(ctx, Run{Binary: "some-other-program"}); err == nil {
		t.Fatal("accepted Binary override")
	}
	stopped, stop := context.WithCancel(ctx)
	stop()
	if _, err = d.Start(stopped, Run{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Start: %v", err)
	}
	p, err := d.Start(ctx, Run{Args: []string{"--help"}})
	if d.Platform() == "windows" {
		if !errors.Is(err, ErrPhysicalUnsupported) {
			t.Fatalf("Windows Start: %v", err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := p.Close(ctx); err != nil {
			t.Error(err)
		}
	}()
	result, err := p.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, stderr := p.Output()
	if result.Exit != 0 || result.Stdout != out || result.Stderr != stderr || !strings.Contains(out, "usage: itos-cc") {
		t.Fatalf("joined result/output: %#v %q %q", result, out, stderr)
	}
	if err = p.Interrupt(ctx); err != nil {
		t.Fatalf("interrupt completed product: %v", err)
	}
}

func TestFixturePeerLivenessAndOwnedExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	pool, err := (binary{}).FixturePeers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := pool.Close(ctx); err != nil {
			t.Error(err)
		}
	}()
	file := filepath.Join(t.TempDir(), "ready.json")
	cmd := exec.Command(os.Args[0], "-test.run=^TestFixturePeerProcess$")
	cmd.Env = append(os.Environ(), append(pool.Environment(), "T80_CAP_READY="+file)...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	joined := false
	defer func() {
		if !joined {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	peer, err := pool.Capture(ctx, file)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	original := peer.Record()
	copy := peer.Record()
	copy[0] = '!'
	if !bytes.Equal(original, peer.Record()) {
		t.Fatal("Record exposes stored bytes")
	}
	wait, stop := context.WithTimeout(ctx, 30*time.Millisecond)
	err = peer.EOF(wait)
	stop()
	var timeout net.Error
	if !errors.As(err, &timeout) || !timeout.Timeout() {
		t.Fatalf("live peer falsely ended: %v", err)
	}
	if err = peer.Ping(ctx); err != nil {
		t.Fatalf("live after bounded observation: %v", err)
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	joined = true
	if err = peer.EOF(ctx); err != nil {
		t.Fatalf("actual process exit: %v; %s", err, output.String())
	}
	if err = peer.Ping(ctx); err == nil {
		t.Fatal("ended peer answered ping")
	}
}

func TestFixturePeerRejectsPublishedPartialRecord(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	pool, err := (binary{}).FixturePeers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := pool.Close(ctx); err != nil {
			t.Error(err)
		}
	}()
	file := filepath.Join(t.TempDir(), "ready.json")
	if err = os.WriteFile(file, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Capture(ctx, file); err == nil || !strings.Contains(err.Error(), "published ready record") {
		t.Fatalf("partial record: %v", err)
	}
}

// This test's subprocess is an actual controlled helper, not a product fault
// injection seam. Its connection stays open until the helper process exits.
func TestFixturePeerProcess(t *testing.T) {
	file := os.Getenv("T80_CAP_READY")
	if file == "" {
		return
	}
	conn, err := net.DialTimeout("tcp", os.Getenv("T80_OWNED_ADDR"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	nonce := strings.Repeat("a", 48)
	hello := fixtureHello{Owner: os.Getenv("T80_OWNED_OWNER"), Nonce: nonce, PID: os.Getpid()}
	if err = json.NewEncoder(conn).Encode(hello); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	if line, err := reader.ReadString('\n'); err != nil || line != "owned\n" {
		t.Fatalf("owner acknowledgment: %q %v", line, err)
	}
	data, _ := json.Marshal(fixtureReady{Nonce: nonce, PID: os.Getpid()})
	if err = os.WriteFile(file+".tmp", data, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(file+".tmp", file); err != nil {
		t.Fatal(err)
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		switch line {
		case "ping " + nonce + "\n":
			if _, err = fmt.Fprintln(conn, "live "+nonce); err != nil {
				t.Fatal(err)
			}
		case "stop " + nonce + "\n":
			os.Exit(0)
		default:
			t.Fatal("unexpected owned command", line)
		}
	}
}

var _ io.Writer = (*capturedOutput)(nil)

// The read-start handshake makes cancellation happen during a blocking read,
// rather than merely testing a context cancelled before the operation begins.
type fixtureReadStarted struct {
	net.Conn
	once    sync.Once
	started chan struct{}
}

func (c *fixtureReadStarted) Read(b []byte) (int, error) {
	c.once.Do(func() { close(c.started) })
	return c.Conn.Read(b)
}
func TestFixturePeerCancellationJoinsBeforeCleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	observed := &fixtureReadStarted{Conn: client, started: make(chan struct{})}
	p := &fixtureConnection{conn: observed, reader: bufio.NewReader(observed), hello: fixtureHello{Nonce: "owned"}, gate: make(chan struct{}, 1)}
	waiting, stop := context.WithCancel(ctx)
	result := make(chan error, 1)
	go func() { result <- p.EOF(waiting) }()
	select {
	case <-observed.started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	stop()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("mid-read cancellation: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("cancellation waited for original deadline")
	}
	done := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(server).ReadString('\n')
		if err == nil && line != "stop owned\n" {
			err = fmt.Errorf("cleanup command %q", line)
		}
		_ = server.Close()
		done <- err
	}()
	if err := p.Stop(ctx); err != nil {
		t.Fatalf("cleanup after cancellation: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
func TestFixturePeerCloseRetainsProtocolFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	pool, err := (binary{}).FixturePeers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p := pool.(*peerListener)
	conn, err := net.DialTimeout("tcp", p.listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	if err = conn.SetDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	if _, err = io.WriteString(conn, "not-json\n"); err != nil {
		t.Fatal(err)
	}
	if _, err = bufio.NewReader(conn).ReadString('\n'); !errors.Is(err, io.EOF) {
		t.Fatalf("invalid peer wasn't closed: %v", err)
	}
	if err = pool.Close(ctx); err == nil || !strings.Contains(err.Error(), "invalid fixture handshake") {
		t.Fatalf("Close lost unconsumed protocol error: %v", err)
	}
}

func TestFixtureRemovalRequiresSuccessfulOwnedCleanup(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "source.go")
	if err := os.WriteFile(file, []byte("fixture still needed"), 0600); err != nil {
		t.Fatal(err)
	}
	w := &world{d: binary{}, dir: dir}
	stopped := errors.New("owned join failed")
	w.beforeRemove = func(ctx context.Context) error {
		if _, err := os.ReadFile(file); err != nil {
			t.Error("fixture deleted before cleanup", err)
		}
		return stopped
	}
	if err := w.cleanUp(t.Context()); !errors.Is(err, stopped) {
		t.Fatalf("cleanup error lost: %v", err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal("failed join deleted fixture", err)
	}
	w.beforeRemove = func(ctx context.Context) error {
		if _, err := os.ReadFile(file); err != nil {
			return err
		}
		return nil
	}
	if err := w.cleanUp(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("successful cleanup retained scratch: %v", err)
	}
}
