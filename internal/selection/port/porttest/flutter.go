package porttest

import (
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Flutter apps and generated Dart, which a walk never enters or takes: a
// copy of project's rules, over the tree's Contents, which TreeContract
// keeps honest.

// platformDirs are the folders flutter create writes beside a Flutter
// app's pubspec.yaml, one for each platform.
var platformDirs = map[string]bool{"android": true, "ios": true, "macos": true, "linux": true, "windows": true, "web": true}

// generatedDartSuffixes are build_runner's and protoc's endings,
// pluginRegistrant flutter's registrant and generatedMarker source_gen's
// mark in a file's leading comments.
var generatedDartSuffixes = []string{
	".g.dart", ".freezed.dart", ".mocks.dart", ".gr.dart", ".config.dart",
	".pb.dart", ".pbenum.dart", ".pbjson.dart", ".pbserver.dart",
}

const (
	pluginRegistrant = "generated_plugin_registrant.dart"
	generatedMarker  = "GENERATED CODE - DO NOT MODIFY BY HAND"
)

// isPlatformDir says whether dir is one of platformDirs' names beside a
// pubspec.yaml of the tree whose dependencies name the flutter SDK.
func (t Tree) isPlatformDir(dir string) bool {
	if !platformDirs[filepath.Base(dir)] {
		return false
	}
	text, ok := t.text(filepath.Join(filepath.Dir(dir), "pubspec.yaml"))
	if !ok {
		return false
	}
	var pubspec struct {
		Dependencies map[string]any `yaml:"dependencies"`
	}
	if yaml.Unmarshal([]byte(text), &pubspec) != nil {
		return false
	}
	for _, dep := range pubspec.Dependencies {
		if m, ok := dep.(map[string]any); ok && m["sdk"] == "flutter" {
			return true
		}
	}
	return false
}

// generated says whether path, absolute, is generated Dart: a generated
// suffix, the registrant, gen-l10n's output as an l10n.yaml of the tree
// in a folder above it says, or a file whose leading comments carry the
// marker.
func (t Tree) generated(path string) bool {
	if filepath.Ext(path) != ".dart" {
		return false
	}
	name := filepath.Base(path)
	if name == pluginRegistrant {
		return true
	}
	for _, suffix := range generatedDartSuffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		if text, ok := t.text(filepath.Join(dir, "l10n.yaml")); ok && writesL10n(text, dir, path) {
			return true
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	text, ok := t.text(path)
	return ok && markedGenerated(text)
}

// text is what Contents holds of path, absolute, and whether it holds it.
func (t Tree) text(path string) (string, bool) {
	rel, ok := t.rel(path)
	if !ok {
		return "", false
	}
	text, ok := t.Contents[rel]
	return text, ok
}

// writesL10n says whether gen-l10n, run beside an l10n.yaml holding text
// in root, writes path: its output-localization-file (app_localizations.dart
// unless it names one) in its output-dir (lib/l10n unless it names one),
// or a locale's file beside it, named after it.
func writesL10n(text, root, path string) bool {
	var out struct {
		Dir  string `yaml:"output-dir"`
		File string `yaml:"output-localization-file"`
	}
	if yaml.Unmarshal([]byte(text), &out) != nil {
		out.Dir, out.File = "", ""
	}
	if out.Dir == "" {
		out.Dir = "lib/l10n"
	}
	if out.File == "" {
		out.File = "app_localizations.dart"
	}
	if filepath.Dir(path) != filepath.Join(root, filepath.FromSlash(out.Dir)) {
		return false
	}
	name, stem := filepath.Base(path), strings.TrimSuffix(out.File, ".dart")
	return name == out.File || strings.HasPrefix(name, stem+"_") && strings.HasSuffix(name, ".dart")
}

// markedGenerated says whether text's leading comments, before its first
// line of code, carry the marker.
func markedGenerated(text string) bool {
	block := false
	for line := range strings.Lines(strings.TrimPrefix(text, "\uFEFF")) {
		line = strings.TrimSpace(line)
		switch {
		case block:
		case line == "":
			continue
		case strings.HasPrefix(line, "//"):
		case strings.HasPrefix(line, "/*"):
			block = true
			line = line[2:]
		default:
			return false
		}
		if strings.Contains(line, generatedMarker) {
			return true
		}
		if block && strings.Contains(line, "*/") {
			block = false
		}
	}
	return false
}
