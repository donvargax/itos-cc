package domain_test

import (
	"path/filepath"
	"reflect"
	"testing"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

// The rule of slice-25 is the platform's as values, so every direction
// holds on any machine: a file only Windows builds on Linux, and one Windows
// leaves out on Windows.
func TestOnlyConstraintsRulingOutThePlatformLeaveAFileOut(t *testing.T) {
	linux := mutants.Platform{OS: "linux", Arch: "amd64"}
	windows := mutants.Platform{OS: "windows", Arch: "amd64"}
	darwin := mutants.Platform{OS: "darwin", Arch: "arm64"}
	android := mutants.Platform{OS: "android", Arch: "arm64"}
	const body = "package probe\n\nfunc Probe() {}\n"
	for _, c := range []struct {
		name, file, text string
		on               mutants.Platform
		built            bool
	}{
		{"another system's suffix", "probe_windows.go", body, linux, false},
		{"its own system's suffix", "probe_windows.go", body, windows, true},
		{"a suffix ruling out Windows", "probe_linux.go", body, windows, false},
		{"another architecture's suffix", "probe_arm64.go", body, linux, false},
		{"a system and architecture suffix", "probe_darwin_arm64.go", body, darwin, true},
		{"the wrong architecture of a system", "probe_darwin_amd64.go", body, darwin, false},
		{"a test's suffix", "probe_windows_test.go", body, linux, false},
		{"a name that is only a system", "windows.go", body, linux, true},
		{"no suffix word", "probe_here.go", body, windows, true},
		{"android builds linux files", "probe_linux.go", body, android, true},
		{"unix is no file suffix", "probe_unix.go", body, windows, true},
		{"another system's line", "probe.go", "//go:build windows\n\n" + body, linux, false},
		{"a line ruling out Windows on Windows", "probe.go", "//go:build !windows\n\n" + body, windows, false},
		{"a line ruling out Windows elsewhere", "probe.go", "//go:build !windows\n\n" + body, linux, true},
		{"unix on Windows", "probe.go", "//go:build unix\n\n" + body, windows, false},
		{"unix on macOS", "probe.go", "//go:build unix\n\n" + body, darwin, true},
		{"another architecture's line", "probe.go", "//go:build arm64\n\n" + body, linux, false},
		{"a project tag alone", "probe.go", "//go:build integration\n\n" + body, linux, true},
		{"ignore alone", "probe.go", "//go:build ignore\n\n" + body, linux, true},
		{"a tag the platform rules out whatever it is", "probe.go", "//go:build windows && integration\n\n" + body, linux, false},
		{"a tag that could build it", "probe.go", "//go:build windows || integration\n\n" + body, linux, true},
		{"a contradiction of the project's tags", "probe.go", "//go:build integration && !integration\n\n" + body, linux, true},
		{"+build lines, and-ed", "probe.go", "// +build linux\n// +build arm64\n\n" + body, linux, false},
		{"//go:build over +build", "probe.go", "//go:build linux\n// +build windows\n\n" + body, linux, true},
		{"a line the package doc holds", "probe.go", "// Package probe.\n//go:build windows\npackage probe\n", linux, true},
		{"a line after the package clause", "probe.go", body + "\n//go:build windows\n", linux, true},
		{"a line after a block comment", "probe.go", "/* Copyright.\n */\n\n//go:build windows\n\n" + body, linux, false},
		{"a line that does not parse", "probe.go", "//go:build windows &&\n\n" + body, linux, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := mutants.BuiltOn(c.file, c.text, c.on); got != c.built {
				t.Fatalf("BuiltOn(%s) on %s is %v, not %v", c.file, c.on, got, c.built)
			}
		})
	}
}

// SplitBuilt takes the platform from the toolchain, GOOS and GOARCH as the
// run sets them over the system's, and leaves every other language as it
// is.
func TestSplitBuiltLeavesOutTheGoFilesTheToolchainsPlatformRulesOut(t *testing.T) {
	root, err := filepath.Abs(filepath.FromSlash("/slice-25-project"))
	if err != nil {
		t.Fatal(err)
	}
	at := func(name string) string { return filepath.Join(root, name) }
	files := map[string]string{
		at("probe_windows.go"): "package probe\n",
		at("probe_here.go"):    "//go:build !windows\n\npackage probe\n",
		at("probe_arm64.go"):   "package probe\n",
		at("plain.go"):         "package probe\n",
		at("probe_windows.py"): "def probe(): pass\n",
	}
	order := []string{at("probe_windows.go"), at("probe_here.go"), at("probe_arm64.go"), at("plain.go"), at("probe_windows.py")}
	for _, c := range []struct {
		name     string
		platform string
		env      map[string]string
		built    []string
		notBuilt []mutants.NotBuilt
	}{
		{"on Linux", "linux", nil, []string{at("probe_here.go"), at("plain.go"), at("probe_windows.py")},
			[]mutants.NotBuilt{{Path: at("probe_windows.go"), Platform: mutants.Platform{OS: "linux", Arch: "amd64"}}, {Path: at("probe_arm64.go"), Platform: mutants.Platform{OS: "linux", Arch: "amd64"}}}},
		{"on Windows", "windows", nil, []string{at("probe_windows.go"), at("plain.go"), at("probe_windows.py")},
			[]mutants.NotBuilt{{Path: at("probe_here.go"), Platform: mutants.Platform{OS: "windows", Arch: "amd64"}}, {Path: at("probe_arm64.go"), Platform: mutants.Platform{OS: "windows", Arch: "amd64"}}}},
		{"built for Windows on arm64 from Linux", "linux", map[string]string{"GOOS": "windows", "GOARCH": "arm64"}, []string{at("probe_windows.go"), at("probe_arm64.go"), at("plain.go"), at("probe_windows.py")},
			[]mutants.NotBuilt{{Path: at("probe_here.go"), Platform: mutants.Platform{OS: "windows", Arch: "arm64"}}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			inputs := &porttest.Inputs{Dir: root, Files: files, Runtime: mutants.RuntimeInputs{Platform: c.platform, Architecture: "amd64", Env: c.env}}
			built, notBuilt, err := mutants.SplitBuilt(t.Context(), order, inputs, inputs)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(built, c.built) || !reflect.DeepEqual(notBuilt, c.notBuilt) {
				t.Fatalf("built %v, not built %v; want %v and %v", built, notBuilt, c.built, c.notBuilt)
			}
		})
	}
}
