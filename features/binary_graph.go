package features

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Graph starts the real serve command and waits for its public readiness
// message before requesting the graph. It never calls the graph library.
func (b binary) Graph(ctx context.Context, r Run) (result []byte, runErr error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, b.bin, "serve", "--port", strconv.Itoa(port), "--interval", "1h") //nolint:gosec // the itos-cc binary under test, serving on a port this harness chose
	cmd.Dir = r.Dir
	cmd.Env = environment(r.Env)
	cmd.WaitDelay = time.Second
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	ready := make(chan struct{})
	scanned := make(chan string, 1)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go func() {
		var lines []string
		scanner := bufio.NewScanner(stderr)
		announced := false
		for scanner.Scan() {
			line := scanner.Text()
			lines = append(lines, line)
			if !announced && strings.HasPrefix(line, "itos-cc: serving ") {
				announced = true
				close(ready)
			}
		}
		scanned <- strings.Join(lines, "\n")
	}()
	stopped := make(chan error, 1)
	go func() { stopped <- cmd.Wait() }()
	defer func() {
		_ = cmd.Process.Kill()
		select {
		case <-stopped:
		case <-time.After(3 * time.Second):
			runErr = errors.Join(runErr, errors.New("serve child did not stop within three seconds"))
		}
	}()
	select {
	case <-ready:
	case err := <-stopped:
		stopped <- err
		return nil, fmt.Errorf("serve stopped before readiness: %v\n%s", err, <-scanned)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(port)+"/api/graph", nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }() // read whole below: closing it loses nothing
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET /api/graph: %s\n%s", response.Status, body)
	}
	return body, nil
}
