package features

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"syscall"
	"time"
)

// FixturePeers observes only controlled test helpers. They hold authenticated
// connections until process exit; their end, EOF or a reset, is lifetime
// evidence only for that fixture.
type FixturePeers interface {
	Environment() []string
	Capture(context.Context, string) (FixturePeer, error)
	Close(context.Context) error
}
type FixturePeer interface {
	Record() []byte
	Ping(context.Context) error
	EOF(context.Context) error
	Stop(context.Context) error // Cleanup only, never a public termination assertion.
}
type fixtureHello struct {
	Owner string `json:"owner"`
	Nonce string `json:"nonce"`
	PID   int    `json:"pid"`
}
type fixtureReady struct {
	Nonce string `json:"nonce"`
	PID   int    `json:"pid"`
}

type peerListener struct {
	listener    net.Listener
	owner       string
	mu          sync.Mutex
	connections map[net.Conn]bool
	peers       map[string]*fixtureConnection
	failure     error
	accepted    chan struct{}
	handshakes  sync.WaitGroup
}
type fixtureConnection struct {
	conn     net.Conn
	reader   *bufio.Reader
	hello    fixtureHello
	record   []byte
	recordMu sync.Mutex
	gate     chan struct{}
	ended    bool
}

func (b binary) FixturePeers(ctx context.Context) (FixturePeers, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if b.Platform() == "windows" {
		return nil, fmt.Errorf("%w: forced fixture exit on Windows does not provide the controlled TCP EOF oracle", ErrPhysicalUnsupported)
	}
	var config net.ListenConfig
	listener, err := config.Listen(ctx, "tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	var nonce [24]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		_ = listener.Close() // the nonce's failure is the error returned
		return nil, err
	}
	p := &peerListener{listener: listener, owner: hex.EncodeToString(nonce[:]), connections: map[net.Conn]bool{}, peers: map[string]*fixtureConnection{}, accepted: make(chan struct{})}
	go p.accept()
	return p, nil
}
func (p *peerListener) Environment() []string {
	return []string{"T80_OWNED_ADDR=" + p.listener.Addr().String(), "T80_OWNED_OWNER=" + p.owner}
}
func (p *peerListener) fail(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failure = errors.Join(p.failure, err)
}
func (p *peerListener) connectionFailure(conn net.Conn, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.connections[conn] {
		p.failure = errors.Join(p.failure, err)
	}
}
func (p *peerListener) accept() {
	defer close(p.accepted)
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				p.fail(err)
			}
			return
		}
		p.mu.Lock()
		p.connections[conn] = true
		p.mu.Unlock()
		p.handshakes.Add(1)
		go p.handshake(conn)
	}
}
func (p *peerListener) handshake(conn net.Conn) {
	defer p.handshakes.Done()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		p.connectionFailure(conn, err)
		_ = conn.Close() // the handshake's failure is recorded above; this only drops it
		return
	}
	reader := bufio.NewReader(conn)
	line, err := reader.ReadSlice('\n')
	if err != nil {
		p.connectionFailure(conn, err)
		_ = conn.Close() // the handshake's failure is recorded above; this only drops it
		return
	}
	var hello fixtureHello
	if len(line) > 4096 {
		err = errors.New("oversized fixture handshake")
	} else {
		err = json.Unmarshal(line, &hello)
	}
	if err != nil || hello.Owner != p.owner || hello.PID < 1 || len(hello.Nonce) < 32 {
		p.fail(fmt.Errorf("invalid fixture handshake: %v", err))
		_ = conn.Close() // the handshake's failure is recorded above; this only drops it
		return
	}
	peer := &fixtureConnection{conn: conn, reader: reader, hello: hello, gate: make(chan struct{}, 1)}
	peer.gate <- struct{}{}
	defer func() { <-peer.gate }()
	p.mu.Lock()
	if p.peers[hello.Nonce] != nil {
		p.mu.Unlock()
		p.fail(errors.New("duplicate fixture nonce"))
		_ = conn.Close() // the handshake's failure is recorded above; this only drops it
		return
	}
	p.peers[hello.Nonce] = peer
	p.mu.Unlock()
	if _, err = io.WriteString(conn, "owned\n"); err != nil {
		p.connectionFailure(conn, err)
		_ = conn.Close() // the handshake's failure is recorded above; this only drops it
		return
	}
	if err = conn.SetDeadline(time.Time{}); err != nil {
		p.connectionFailure(conn, err)
		_ = conn.Close() // the handshake's failure is recorded above; this only drops it
		return
	}
}

// Ready records must be atomically published regular files. A missing file may
// wait; a published malformed/partial file or failed read is always an error.
func (p *peerListener) Capture(ctx context.Context, path string) (FixturePeer, error) {
	if _, ok := ctx.Deadline(); !ok {
		return nil, errors.New("fixture capture requires a bounded context")
	}
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		p.mu.Lock()
		failure := p.failure
		p.mu.Unlock()
		if failure != nil {
			return nil, failure
		}
		info, err := os.Lstat(path)
		if err == nil {
			if !info.Mode().IsRegular() || info.Size() > 65536 {
				return nil, errors.New("ready record is not a bounded regular file")
			}
			record, err := os.ReadFile(path) //nolint:gosec // a ready record a fixture published in the scenario's own folder
			if err != nil {
				return nil, err
			}
			var ready fixtureReady
			if err = json.Unmarshal(record, &ready); err != nil {
				return nil, fmt.Errorf("published ready record: %w", err)
			}
			p.mu.Lock()
			peer := p.peers[ready.Nonce]
			p.mu.Unlock()
			if peer == nil || ready.PID < 1 || ready.PID != peer.hello.PID {
				return nil, errors.New("ready record has no matching captured peer")
			}
			peer.recordMu.Lock()
			peer.record = bytes.Clone(record)
			peer.recordMu.Unlock()
			if err = peer.Ping(ctx); err != nil {
				return nil, err
			}
			return peer, nil
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
func (p *fixtureConnection) Record() []byte {
	p.recordMu.Lock()
	defer p.recordMu.Unlock()
	return bytes.Clone(p.record)
}
func (p *fixtureConnection) operation(ctx context.Context, command string) error {
	select {
	case p.gate <- struct{}{}:
		defer func() { <-p.gate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.ended {
		if command == "ping" {
			return errors.New("fixture peer already ended")
		}
		return nil
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return errors.New("fixture peer operation requires a bounded context")
	}
	if err := p.conn.SetDeadline(deadline); err != nil {
		return err
	}
	cancelled := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { _ = p.conn.SetDeadline(time.Now()); close(cancelled) })
	defer func() {
		if !stop() {
			<-cancelled
		}
	}()

	if command != "eof" {
		if _, err := io.WriteString(p.conn, command+" "+p.hello.Nonce+"\n"); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if command != "stop" {
				return err
			}
			if _, readErr := p.reader.ReadSlice('\n'); peerEnded(readErr) {
				p.ended = true
				return nil
			}
			return err
		}
	}
	reply, err := p.reader.ReadSlice('\n')
	line := string(reply)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if peerEnded(err) {
		p.ended = true
		if command == "ping" {
			return errors.New("fixture peer ended before ping acknowledgment")
		}
		return nil
	}
	if err != nil {
		return err
	}
	if command == "ping" && line == "live "+p.hello.Nonce+"\n" {
		return nil
	}
	return fmt.Errorf("unexpected fixture peer reply %q during %s", line, command)
}

// peerEnded is whether a read's err is the fixture's end of its connection:
// EOF, or a reset. A controlled fixture holds its connection until its
// process exits, so either says the process is gone. Which one the harness
// reads is the system's choice: a socket closed with bytes unread, or with a
// zero SO_LINGER, resets, and one whose owner has exited answers what is
// written to it with a reset, which macOS reports to the next read before
// the end of stream already queued. Any other failure is not an end.
func peerEnded(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, syscall.ECONNRESET)
}
func (p *fixtureConnection) Ping(ctx context.Context) error { return p.operation(ctx, "ping") }
func (p *fixtureConnection) EOF(ctx context.Context) error  { return p.operation(ctx, "eof") }
func (p *fixtureConnection) Stop(ctx context.Context) error { return p.operation(ctx, "stop") }

func (p *peerListener) Close(ctx context.Context) error {
	err := p.listener.Close()
	if errors.Is(err, net.ErrClosed) {
		err = nil
	}
	select {
	case <-p.accepted:
	case <-ctx.Done():
		return errors.Join(err, ctx.Err())
	}
	p.mu.Lock()
	owned := map[net.Conn]bool{}
	for _, peer := range p.peers {
		owned[peer.conn] = true
	}
	for conn := range p.connections {
		if !owned[conn] {
			p.connections[conn] = false
			err = errors.Join(err, conn.Close())
		}
	}
	p.mu.Unlock()
	joined := make(chan struct{})
	go func() { p.handshakes.Wait(); close(joined) }()
	select {
	case <-joined:
	case <-ctx.Done():
		return errors.Join(err, ctx.Err())
	}
	p.mu.Lock()
	peers := make([]*fixtureConnection, 0, len(p.peers))
	for _, peer := range p.peers {
		peers = append(peers, peer)
	}
	p.mu.Unlock()
	for _, peer := range peers {
		err = errors.Join(err, peer.Stop(ctx), peer.conn.Close())
	}
	p.mu.Lock()
	err = errors.Join(err, p.failure)
	p.mu.Unlock()
	return err
}
