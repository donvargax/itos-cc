package project

import (
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Flutter apps and generated Dart, which discovery never walks into or
// takes (slice-30, q-75 and q-77). Selection's fake Tree holds a copy of
// these rules, which its TreeContract keeps honest.

// platformDirs are the folders flutter create writes beside a Flutter
// app's pubspec.yaml, one for each platform: part of the app, never its
// Dart. android's MainActivity.kt and Gradle build would otherwise read as
// a Kotlin project.
var platformDirs = map[string]bool{"android": true, "ios": true, "macos": true, "linux": true, "windows": true, "web": true}

// generatedDartSuffixes are the endings of the files build_runner's
// builders (json_serializable, freezed, mockito, auto_route, injectable)
// and protoc write.
var generatedDartSuffixes = []string{
	".g.dart", ".freezed.dart", ".mocks.dart", ".gr.dart", ".config.dart",
	".pb.dart", ".pbenum.dart", ".pbjson.dart", ".pbserver.dart",
}

// pluginRegistrant is the file flutter writes to register its plugins.
const pluginRegistrant = "generated_plugin_registrant.dart"

// generatedMarker is what source_gen's builders write in a generated
// file's leading comments.
const generatedMarker = "GENERATED CODE - DO NOT MODIFY BY HAND"

// gen-l10n's defaults: where it writes its output, from the folder of the
// l10n.yaml, and the file it writes there, beside one for each locale.
const (
	l10nOutputDir  = "lib/l10n"
	l10nOutputFile = "app_localizations.dart"
)

// flutterRules answer, for one discovery, whether a folder is a Flutter
// app's platform folder and a file generated Dart, remembering each
// pubspec.yaml and l10n.yaml they read.
type flutterRules struct {
	apps map[string]bool
	l10n map[string]*l10nOutput
}

// l10nOutput is where an l10n.yaml says gen-l10n writes: a folder,
// slash-separated from the l10n.yaml's, and a file's name.
type l10nOutput struct {
	Dir  string `yaml:"output-dir"`
	File string `yaml:"output-localization-file"`
}

func newFlutterRules() *flutterRules {
	return &flutterRules{apps: map[string]bool{}, l10n: map[string]*l10nOutput{}}
}

// platformDir reports whether dir is a platform folder of a Flutter app:
// one of platformDirs' names beside a pubspec.yaml that depends on the
// flutter SDK.
func (r *flutterRules) platformDir(dir string) bool {
	if !platformDirs[filepath.Base(dir)] {
		return false
	}
	app := filepath.Dir(dir)
	flutter, ok := r.apps[app]
	if !ok {
		text, err := os.ReadFile(filepath.Join(app, "pubspec.yaml")) //nolint:gosec // the manifest beside a folder of the tree the person selected
		flutter = err == nil && dependsOnFlutter(text)
		r.apps[app] = flutter
	}
	return flutter
}

// FlutterPlatformDir reports whether dir is a platform folder of a Flutter
// app, as discovery leaves it out: one of the folders flutter create writes
// beside a pubspec.yaml that depends on the flutter SDK. A Dart package's
// strict coverage evidence leaves them out the same way.
func FlutterPlatformDir(dir string) bool { return newFlutterRules().platformDir(dir) }

// dependsOnFlutter reports whether a pubspec.yaml's dependencies name the
// flutter SDK: one of them is "sdk: flutter". One that does not parse
// names nothing.
func dependsOnFlutter(text []byte) bool {
	var pubspec struct {
		Dependencies map[string]any `yaml:"dependencies"`
	}
	if yaml.Unmarshal(text, &pubspec) != nil {
		return false
	}
	for _, dep := range pubspec.Dependencies {
		if m, ok := dep.(map[string]any); ok && m["sdk"] == "flutter" {
			return true
		}
	}
	return false
}

// generated reports whether path is generated Dart: a build_runner or
// protoc suffix, flutter's plugin registrant, what gen-l10n wrote as an
// l10n.yaml in a folder above it says, or a file whose leading comments
// carry source_gen's marker.
func (r *flutterRules) generated(path string) bool {
	if filepath.Ext(path) != ".dart" {
		return false
	}
	name := filepath.Base(path)
	if name == pluginRegistrant || hasGeneratedSuffix(name) || r.localization(path) {
		return true
	}
	text, err := os.ReadFile(path) //nolint:gosec // a file of the tree the person selected, walked to
	return err == nil && markedGenerated(string(text))
}

// GeneratedDart reports whether path is generated Dart, as discovery
// leaves it out: a build_runner or protoc suffix, flutter's plugin
// registrant, what gen-l10n wrote as an l10n.yaml in a folder above it
// says, or a file whose leading comments carry source_gen's marker.
func GeneratedDart(path string) bool { return newFlutterRules().generated(path) }

// hasGeneratedSuffix reports whether name ends as build_runner's and
// protoc's files do.
func hasGeneratedSuffix(name string) bool {
	for _, suffix := range generatedDartSuffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

// localization reports whether path is what gen-l10n wrote as the
// l10n.yaml of a folder above it says.
func (r *flutterRules) localization(path string) bool {
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		out, ok := r.l10n[dir]
		if !ok {
			text, err := os.ReadFile(filepath.Join(dir, "l10n.yaml")) //nolint:gosec // gen-l10n's settings at or above a selected file's folder
			if err == nil {
				out = l10nOutputOf(text)
			}
			r.l10n[dir] = out
		}
		if out != nil && out.writes(dir, path) {
			return true
		}
		if filepath.Dir(dir) == dir {
			return false
		}
	}
}

// l10nOutputOf is where an l10n.yaml's text says gen-l10n writes, its
// defaults where it names nothing or does not parse.
func l10nOutputOf(text []byte) *l10nOutput {
	var out l10nOutput
	if yaml.Unmarshal(text, &out) != nil {
		out = l10nOutput{}
	}
	if out.Dir == "" {
		out.Dir = l10nOutputDir
	}
	if out.File == "" {
		out.File = l10nOutputFile
	}
	return &out
}

// writes reports whether gen-l10n, run beside an l10n.yaml in root,
// writes path: its output file in its output folder, or the file of a
// locale beside it, named after the output file and the locale.
func (o *l10nOutput) writes(root, path string) bool {
	if filepath.Dir(path) != filepath.Join(root, filepath.FromSlash(o.Dir)) {
		return false
	}
	name := filepath.Base(path)
	stem := strings.TrimSuffix(o.File, ".dart")
	return name == o.File || strings.HasPrefix(name, stem+"_") && strings.HasSuffix(name, ".dart")
}

// markedGenerated reports whether text's leading comments, those before
// its first line of code, carry source_gen's marker.
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
