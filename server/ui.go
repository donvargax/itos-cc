package server

import (
	"embed"
	"io/fs"
)

// embedded is what server/ui holds when the binary is built: the built
// viewer a release copies there from viewer/dist before each build
// (release.yml), or only the .gitignore that keeps the folder in the
// module, ignoring anything copied in, for every other build. all: takes
// that dotfile too, as without it a folder holding nothing else would
// match nothing and no build would compile.
//
//go:embed all:ui
var embedded embed.FS

// builtIn is the viewer this binary was built with, nil when it carries
// none.
var builtIn = carried(embedded)

// carried is the viewer in fsys's ui folder, or nil when the folder holds
// no index.html.
func carried(fsys fs.FS) fs.FS {
	if _, err := fs.Stat(fsys, "ui/index.html"); err != nil {
		return nil
	}
	ui, _ := fs.Sub(fsys, "ui") // "ui" is a valid path, the only thing Sub refuses
	return ui
}
