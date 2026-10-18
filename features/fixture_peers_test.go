package features

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// How a fixture's end of its connection reaches the harness is the
// operating system's choice, which no scenario can make: a FIN read as EOF,
// or a reset, which macOS reported once in ID-MUT-250's cleanup (T-134).
// These tests make a peer's end a reset and hold that the harness reads it
// as that peer's end, as it reads EOF, while any other failure still fails.

func TestFixturePeerResetIsItsEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	pool, err := (binary{}).FixturePeers(ctx)
	requireFixturePeerFactory(t, err)
	peer, conn := capturedFakePeer(ctx, t, pool.(*peerListener))
	// A zero linger makes Close reset the connection, as a socket whose
	// owner leaves bytes unread in it, or holds SO_LINGER, does when the
	// owner is killed.
	if err = conn.SetLinger(0); err != nil {
		t.Fatal(err)
	}
	if err = conn.Close(); err != nil {
		t.Fatal(err)
	}
	if err = peer.EOF(ctx); err != nil {
		t.Fatalf("a reset peer's end: %v", err)
	}
	if err = peer.Ping(ctx); err == nil {
		t.Fatal("a reset peer answered ping")
	}
	if err = pool.Close(ctx); err != nil {
		t.Fatalf("cleanup after a reset peer: %v", err)
	}
}

func TestFixturePeerStopReadsAResetAsItsEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	conn := &endedPeerConn{err: peerReadFailure(syscall.ECONNRESET)}
	p := &fixtureConnection{conn: conn, reader: bufio.NewReader(conn), hello: fixtureHello{Nonce: "owned"}, gate: make(chan struct{}, 1)}
	if err := p.Stop(ctx); err != nil {
		t.Fatalf("a stop answered by a reset: %v", err)
	}
	if conn.wrote.String() != "stop owned\n" {
		t.Fatalf("cleanup command %q", conn.wrote.String())
	}
	if err := p.Ping(ctx); err == nil || !strings.Contains(err.Error(), "already ended") {
		t.Fatalf("ping after a reset end: %v", err)
	}
}

func TestFixturePeerOtherConnectionFailuresStillFail(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for _, failure := range []error{peerReadFailure(syscall.ECONNABORTED), peerReadFailure(syscall.EPIPE), os.ErrDeadlineExceeded} {
		for name, operation := range map[string]func(*fixtureConnection, context.Context) error{"eof": (*fixtureConnection).EOF, "stop": (*fixtureConnection).Stop} {
			conn := &endedPeerConn{err: failure}
			p := &fixtureConnection{conn: conn, reader: bufio.NewReader(conn), hello: fixtureHello{Nonce: "owned"}, gate: make(chan struct{}, 1)}
			if err := operation(p, ctx); !errors.Is(err, failure) {
				t.Fatalf("%s over %v: %v", name, failure, err)
			}
			if p.ended {
				t.Fatalf("%s over %v read the peer as ended", name, failure)
			}
		}
	}
}

// capturedFakePeer connects to pool as a controlled fixture does, in this
// process, answers the ping of its capture and returns the captured peer and
// the fixture's end of the connection.
func capturedFakePeer(ctx context.Context, t *testing.T, pool *peerListener) (FixturePeer, *net.TCPConn) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", pool.listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() }) // closed again only to free it
	nonce := strings.Repeat("b", 48)
	if err = json.NewEncoder(conn).Encode(fixtureHello{Owner: pool.owner, Nonce: nonce, PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	if line, err := reader.ReadString('\n'); err != nil || line != "owned\n" {
		t.Fatalf("owner acknowledgment: %q %v", line, err)
	}
	answered := make(chan error, 1)
	go func() {
		line, err := reader.ReadString('\n')
		if err == nil && line != "ping "+nonce+"\n" {
			err = errors.New("unexpected capture command " + line)
		}
		if err == nil {
			_, err = conn.Write([]byte("live " + nonce + "\n"))
		}
		answered <- err
	}()
	file := filepath.Join(t.TempDir(), "ready.json")
	data, _ := json.Marshal(fixtureReady{Nonce: nonce, PID: os.Getpid()})
	if err = os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	peer, err := pool.Capture(ctx, file)
	if err != nil {
		t.Fatal(err)
	}
	if err = <-answered; err != nil {
		t.Fatal(err)
	}
	return peer, conn.(*net.TCPConn)
}

// endedPeerConn is a peer's connection whose writes are taken and whose
// reads fail with err: with a reset, a connection as macOS leaves it once
// the fixture has exited, where the stop written to it draws a reset that
// the next read reports before the end of stream already queued.
type endedPeerConn struct {
	net.Conn
	wrote bytes.Buffer
	err   error
}

func (c *endedPeerConn) SetDeadline(time.Time) error { return nil }
func (c *endedPeerConn) Write(b []byte) (int, error) { return c.wrote.Write(b) }
func (c *endedPeerConn) Read([]byte) (int, error)    { return 0, c.err }

// peerReadFailure is a TCP read's failure with errno, as the net package
// returns it.
func peerReadFailure(errno syscall.Errno) error {
	return &net.OpError{Op: "read", Net: "tcp4", Err: os.NewSyscallError("read", errno)}
}
