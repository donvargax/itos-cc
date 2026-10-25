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
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Builder is what the server publishes from, which cmd binds to the
// graph slice's builder, so the server imports nothing under internal/:
// Build is the current graph and whether it changed since the last call,
// built under ctx, and Roots the absolute repository roots, by name.
type Builder interface {
	Build(ctx context.Context) (Graph, bool, error)
	Roots() map[string]string
}

// Graph is a built graph as the server publishes it: its version, what
// /api/graph answers, as JSON, and the files of each repository, by name,
// which /api/source may serve.
type Graph struct {
	Version int
	Body    any
	Files   map[string][]string
}

// Server holds the latest graph and the clients listening for changes.
type Server struct {
	builder Builder
	ui      string // directory of a built viewer to serve at / instead of the built-in one, or ""
	log     *log.Logger

	mu        sync.Mutex
	body      []byte
	version   int
	files     map[string]bool // repo/file pairs the source endpoint may serve
	listeners map[chan int]bool
}

// New builds b's first graph, under ctx.
func New(ctx context.Context, b Builder, ui string, logger *log.Logger) (*Server, error) {
	s := &Server{builder: b, ui: ui, log: logger, listeners: map[chan int]bool{}}
	if err := s.rebuild(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

// rebuild refreshes the graph under ctx and tells listeners when it
// changed.
func (s *Server) rebuild(ctx context.Context) error {
	g, changed, err := s.builder.Build(ctx)
	if err != nil || !changed {
		return err
	}
	body, err := json.Marshal(g.Body)
	if err != nil {
		return err
	}
	files := map[string]bool{}
	for repo, list := range g.Files {
		for _, f := range list {
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

// Watch rebuilds every interval under ctx until it ends. A failed build
// keeps the previous graph: a file caught mid-save parses on the next tick.
// A build ctx's end cuts short is no failure, and is not logged.
func (s *Server) Watch(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.rebuild(ctx); err != nil && ctx.Err() == nil {
				s.log.Printf("rebuild: %v", err)
			}
		}
	}
}

// Handler routes the API and the viewer at /: the one the ui directory
// holds, else the one the binary was built with. With neither, / says how
// to get one.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/graph", s.graph)
	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("GET /api/source", s.source)
	ui := builtIn
	if s.ui != "" {
		ui = os.DirFS(s.ui)
	}
	if ui != nil {
		mux.Handle("GET /", spa(ui))
	} else {
		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprintln(w, "itos-cc API: /api/graph, /api/events, /api/source. Start the viewer, or pass --ui viewer/dist.") // the client has gone if the write fails; there is no one left to tell
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
	_, _ = w.Write(body) // the client has gone if the write fails; there is no one left to tell
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
		_, _ = fmt.Fprintf(w, "event: version\ndata: %d\n\n", v) // a client gone ends the stream through its request's context
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
			_, _ = fmt.Fprint(w, ": keepalive\n\n") // a client gone ends the stream through its request's context
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
	f, err := os.Open(filepath.Join(root, filepath.FromSlash(file))) //nolint:gosec // only a file the built graph lists, under its repository's root, as checked above
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }() // read alone: closing it loses nothing
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.Copy(w, f) // the client has gone if the write fails; there is no one left to tell
}

// spa serves a built single-page app, falling back to its index.html for
// client-side routes and for /, which names no file.
func spa(fsys fs.FS) http.Handler {
	files := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ServeMux has cleaned the rooted request path, and fs.FS refuses any
		// name leaving fsys; only existence is asked.
		if _, err := fs.Stat(fsys, strings.TrimPrefix(r.URL.Path, "/")); err != nil {
			http.ServeFileFS(w, r, fsys, "index.html")
			return
		}
		files.ServeHTTP(w, r)
	})
}
