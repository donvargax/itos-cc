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
// serve takes a port to listen on, never 0, so the harness asks the system
// for a free one and closes it for serve; a scenario running beside it
// (T-128) can be handed the same port in between, and serve then refuses
// it as in use. That run is tried again on another port, up to
// portAttempts times; a refusal on every one is the scenario's failure.
func (b binary) Graph(ctx context.Context, r Run) ([]byte, error) {
	for attempt := 1; ; attempt++ {
		port, err := freePort()
		if err != nil {
			return nil, err
		}
		body, err := b.graphOn(ctx, r, port)
		if errors.Is(err, errPortTaken) && attempt < portAttempts {
			continue
		}
		return body, err
	}
}

// portAttempts is how many free ports Graph tries serve on.
const portAttempts = 3

// temporaryFailure is the exit code of a temporary failure, which serve
// exits with when its port is in use, the only one it has before it is
// ready (cli.feature's ID-CLI-20).
const temporaryFailure = 75

// errPortTaken is serve's refusal of a port something took after the
// harness found it free.
var errPortTaken = errors.New("serve found its port in use")

// freePort is a port of 127.0.0.1 free when asked.
func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	return port, listener.Close()
}

// graphOn runs serve on port and requests the graph from it.
func (b binary) graphOn(ctx context.Context, r Run, port int) (result []byte, runErr error) {
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
		output := <-scanned
		if exit := (*exec.ExitError)(nil); errors.As(err, &exit) && exit.ExitCode() == temporaryFailure {
			return nil, fmt.Errorf("%w: %v\n%s", errPortTaken, err, output)
		}
		return nil, fmt.Errorf("serve stopped before readiness: %v\n%s", err, output)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	response, err := get(ctx, port, "/api/graph")
	if err != nil {
		return nil, err
	}
	if response.status != http.StatusOK {
		return nil, fmt.Errorf("GET /api/graph: %d\n%s", response.status, response.body)
	}
	return response.body, nil
}

// response is what serve answered a request with: its status, its
// Content-Type and its body.
type response struct {
	status      int
	contentType string
	body        []byte
}

// get requests path from the serve listening on port of 127.0.0.1.
func get(ctx context.Context, port int, path string) (response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(port)+path, nil)
	if err != nil {
		return response{}, err
	}
	answer, err := http.DefaultClient.Do(req)
	if err != nil {
		return response{}, err
	}
	defer func() { _ = answer.Body.Close() }() // read whole below: closing it loses nothing
	body, err := io.ReadAll(answer.Body)
	if err != nil {
		return response{}, err
	}
	return response{status: answer.StatusCode, contentType: answer.Header.Get("Content-Type"), body: body}, nil
}
