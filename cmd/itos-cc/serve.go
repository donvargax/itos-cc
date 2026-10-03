package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/donvargax/itos-cc/server"
)

func runServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), `usage: itos-cc serve [options] [repo ...]

Serves the architecture of one or more repositories for the viewer:

  GET /api/graph    repositories, directories, modules, functions, and
                    their dependencies and metrics, as JSON
  GET /api/events   server-sent events: the graph version, on every change
  GET /api/source   ?repo=NAME&file=PATH, the text of a file in the graph

Sources and .metrics snapshots are watched: saving a file updates
complexity and dependencies at once, and rerunning crap, mutate, or dry
updates their numbers. Listens on localhost only.

`)
		fs.PrintDefaults()
	}
	port := fs.Int("port", 7070, "port to listen on")
	ui := fs.String("ui", "", "directory of a built viewer to serve at / (e.g. viewer/dist)")
	interval := fs.Duration("interval", 500*time.Millisecond, "how often to check for changes")
	roots, err := parse(fs, args)
	if err != nil {
		return exitUsage
	}
	if len(roots) == 0 {
		roots = []string{"."}
	}
	logger := log.New(os.Stderr, "itos-cc: ", 0)
	srv, err := server.New(roots, *ui, logger)
	if err != nil {
		logger.Println(err)
		return exitUsage
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go srv.Watch(ctx, *interval)

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(*port))
	httpServer := &http.Server{Addr: addr, Handler: srv.Handler()}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		httpServer.Shutdown(shutdown)
	}()
	logger.Printf("serving %v on http://%s", roots, addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Println(err)
		return exitUsage
	}
	return exitOK
}
