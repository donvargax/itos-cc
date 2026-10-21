package features

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
)

// The Flutter app of file_selection.feature's Flutter scenarios is the
// scratch folder itself, beside the Background project: a pubspec.yaml
// depending on the flutter SDK, as flutter create writes it, and its Dart.

// flutterPubspec is a Flutter app's pubspec.yaml: its dependencies name
// the flutter SDK.
const flutterPubspec = "name: app\npublish_to: none\nenvironment:\n  sdk: ^3.5.0\n" +
	"dependencies:\n  flutter:\n    sdk: flutter\n  cupertino_icons: ^1.0.8\n" +
	"dev_dependencies:\n  flutter_test:\n    sdk: flutter\n"

// pureDartPubspec is a pure Dart package's pubspec.yaml: no dependency
// names the flutter SDK.
const pureDartPubspec = "name: held\nenvironment:\n  sdk: ^3.5.0\ndependencies:\n  path: ^1.9.0\n" +
	"dev_dependencies:\n  test: ^1.25.0\n"

// platformFolders are the folders flutter create writes beside a Flutter
// app's pubspec.yaml, one for each platform.
var platformFolders = []string{"android", "ios", "macos", "linux", "windows", "web"}

// flutterApp writes a Flutter app's pubspec.yaml and the Dart of its lib
// folder.
func (s *selectionState) flutterApp() error {
	if err := s.write("pubspec.yaml", flutterPubspec); err != nil {
		return err
	}
	s.lib = []string{"lib/main.dart", "lib/src/counter.dart"}
	for _, name := range s.lib {
		if err := s.write(name, sourceFor(name)); err != nil {
			return err
		}
	}
	return nil
}

// platformFolders writes Dart and TypeScript sources into each platform
// folder of the app, and into android the Kotlin activity flutter create
// writes there, which reads as a Kotlin project's source.
func (s *selectionState) platformFolders() error {
	for _, dir := range platformFolders {
		for _, name := range []string{"held.dart", "held.ts"} {
			if err := s.write(dir+"/"+name, sourceFor(name)); err != nil {
				return err
			}
		}
		s.dirs = append(s.dirs, dir)
	}
	const activity = "android/app/src/main/kotlin/com/example/app/MainActivity.kt"
	return s.write(activity, sourceFor(activity))
}

// libSelected is whether the discovery listed each file the app's lib
// folder holds.
func (s *selectionState) libSelected() error {
	if s.found == nil || len(s.lib) == 0 {
		return errors.New("no discovery of a Flutter app's lib folder")
	}
	for _, file := range s.lib {
		if !slices.Contains(s.found.sources, file) {
			return fmt.Errorf("%s is not selected: %v\n%s", file, s.found.sources, s.w.report())
		}
	}
	return nil
}

// pureDartLinux writes a pure Dart package, pkg, whose pubspec.yaml names
// no flutter SDK, with a source in its linux folder, discovers the
// project's files again, and is whether that source is selected.
func (s *selectionState) pureDartLinux() error {
	const held = "pkg/linux/held.dart"
	if err := s.write("pkg/pubspec.yaml", pureDartPubspec); err != nil {
		return err
	}
	if err := s.write(held, sourceFor(held)); err != nil {
		return err
	}
	if err := s.discover(s.w.dir); err != nil {
		return err
	}
	if !slices.Contains(s.found.sources, held) {
		return fmt.Errorf("%s, beside a pure Dart pubspec.yaml, is not selected: %v\n%s", held, s.found.sources, s.w.report())
	}
	return nil
}

// What a Flutter app's Given says of the file it holds: its first line,
// an l10n.yaml naming no output or the output it names, or none.
var (
	firstLineRow = regexp.MustCompile(`^, whose first line is "([^"]*)"$`)
	l10nRow      = regexp.MustCompile(`^, beside an l10n\.yaml (?:that names no output|with output-dir (\S+) and output-localization-file (\S+))$`)
)

// flutterAppHolding writes a Flutter app's pubspec.yaml and file, a
// function of Dart, after the first line written says it starts with, and
// the l10n.yaml it says is beside it.
func (s *selectionState) flutterAppHolding(file, written string) error {
	if err := s.write("pubspec.yaml", flutterPubspec); err != nil {
		return err
	}
	text := sourceFor(file)
	switch m, l := firstLineRow.FindStringSubmatch(written), l10nRow.FindStringSubmatch(written); {
	case written == "":
	case written == ", with no l10n.yaml":
		if err := s.noneCalled("l10n.yaml"); err != nil {
			return err
		}
	case m != nil:
		text = m[1] + "\n" + text
	case l != nil && l[1] == "":
		if err := s.write("l10n.yaml", "template-arb-file: app_en.arb\n"); err != nil {
			return err
		}
	case l != nil:
		if err := s.write("l10n.yaml", "output-dir: "+l[1]+"\noutput-localization-file: "+l[2]+"\n"); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%q says nothing this step lays out", written)
	}
	s.named = []string{file}
	return s.write(file, text)
}

// isSelected is whether the discovery listed name, a file of the project,
// as production code or as test code.
func (s *selectionState) isSelected(name string) error {
	if _, ok := s.files[name]; !ok {
		return fmt.Errorf("the project never held %s", name)
	}
	got, err := s.selected()
	if err != nil {
		return err
	}
	if !slices.Contains(got, name) {
		return fmt.Errorf("%s is not selected: %v\n%s", name, got, s.w.report())
	}
	return nil
}
