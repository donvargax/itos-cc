// Package server publishes the architecture graph over HTTP for a viewer:
// the graph as JSON, a stream of change notifications, and the source of the
// files in the graph. It watches the repositories and rebuilds the graph when
// a source file or a .metrics snapshot changes.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/donvargax/itos-cc/graph"
)

// Server holds the latest graph and the clients listening for changes.
type Server struct {
	builder *graph.Builder
	ui      string // directory of a built viewer to serve at /, or ""
	log     *log.Logger

	mu        sync.Mutex
	body      []byte
	version   int
	files     map[string]bool // repo/file pairs the source endpoint may serve
	listeners map[chan int]bool
}

// New builds the first graph of roots.
func New(roots []string, ui string, logger *log.Logger) (*Server, error) {
	b, err := graph.NewBuilder(roots)
	if err != nil {
		return nil, err
	}
	s := &Server{builder: b, ui: ui, log: logger, listeners: map[chan int]bool{}}
	if err := s.rebuild(); err != nil {
		return nil, err
	}
	return s, nil
}

// rebuild refreshes the graph and tells listeners when it changed.
func (s *Server) rebuild() error {
	g, changed, err := s.builder.Build()
	if err != nil || !changed {
		return err
	}
	body, err := json.Marshal(g)
	if err != nil {
		return err
	}
	files := map[string]bool{}
	for _, n := range g.Nodes {
		repo, _, _ := strings.Cut(n.ID, "/")
		for _, f := range n.Files {
			files[repo+"\x00"+f] = true
		}
	}
	s.mu.Lock()
	s.body, s.version, s.files = body, g.Version, files
	for ch := range s.listeners {
		select {
		case ch <- g.Version:
		default: // a slow client gets the next version instead
		}
	}
	s.mu.Unlock()
	return nil
}

// Watch rebuilds every interval until ctx ends. A failed build keeps the
// previous graph: a file caught mid-save parses on the next tick.
func (s *Server) Watch(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.rebuild(); err != nil {
				s.log.Printf("rebuild: %v", err)
			}
		}
	}
}

// Handler routes the API and, when configured, the viewer.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/graph", s.graph)
	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("GET /api/source", s.source)
	if s.ui != "" {
		mux.Handle("GET /", spa(s.ui))
	} else {
		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "itos-cc API: /api/graph, /api/events, /api/source. Start the viewer, or pass --ui viewer/dist.")
		})
	}
	return mux
}

func (s *Server) graph(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	body := s.body
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(body)
}

// events streams the graph version: once on connect, then on every change.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	ch := make(chan int, 1)
	s.mu.Lock()
	s.listeners[ch] = true
	version := s.version
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.listeners, ch)
		s.mu.Unlock()
	}()
	send := func(v int) {
		fmt.Fprintf(w, "event: version\ndata: %d\n\n", v)
		flusher.Flush()
	}
	send(version)
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case v := <-ch:
			send(v)
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// source serves a file of the graph by repository name and relative path.
// Anything else is refused, so the endpoint never reads arbitrary files.
func (s *Server) source(w http.ResponseWriter, r *http.Request) {
	repo, file := r.URL.Query().Get("repo"), r.URL.Query().Get("file")
	s.mu.Lock()
	allowed := s.files[repo+"\x00"+file]
	s.mu.Unlock()
	root, ok := s.builder.Roots()[repo]
	if !allowed || !ok {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	io.Copy(w, f)
}

// spa serves a built single-page app, falling back to its index.html for
// client-side routes.
func spa(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(r.URL.Path))); err != nil {
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		files.ServeHTTP(w, r)
	})
}
