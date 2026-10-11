package coverage

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/donvargax/itos-cc/lang"
)

// KotlinTests is the runnable test classes (lang.TestClasses) the Kotlin
// files of tests, the test files that reach a source, declare, by fully
// qualified name, grouped by the build module whose tests they are: the
// directory of the nearest build.gradle.kts, build.gradle or pom.xml above
// each file. Each list is sorted, with no repeats. Files of another
// language or of no module, and those that declare no test class, such as
// test-support helpers, are left out, and so is a file that cannot be read
// or parsed.
func KotlinTests(tests []string) map[string][]string {
	out := map[string][]string{}
	for _, test := range tests {
		if spec := lang.Detect(test); spec == nil || spec.Name != "kotlin" {
			continue
		}
		module := lang.FindUp(test, markers["kotlin"]...)
		if module == "" {
			continue
		}
		for _, class := range kotlinTestClasses(test) {
			if !slices.Contains(out[module], class) {
				out[module] = append(out[module], class)
			}
		}
	}
	for module := range out {
		sort.Strings(out[module])
	}
	return out
}

// kotlinClasses caches each Kotlin test file's classes, by path, as long as
// its size and modification time stay the same: one file is asked for once
// per source it reaches.
var kotlinClasses = struct {
	sync.Mutex
	files map[string]kotlinClassesOf
}{files: map[string]kotlinClassesOf{}}

type kotlinClassesOf struct {
	size    int64
	modTime time.Time
	classes []string
}

// kotlinTestClasses is lang.TestClasses of the Kotlin file at path, none
// when it cannot be read or parsed.
func kotlinTestClasses(path string) []string {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	kotlinClasses.Lock()
	cached, ok := kotlinClasses.files[path]
	kotlinClasses.Unlock()
	if ok && cached.size == info.Size() && cached.modTime.Equal(info.ModTime()) {
		return cached.classes
	}
	f, err := lang.ParseFile(path)
	if err != nil {
		return nil
	}
	classes := lang.TestClasses(f)
	f.Close()
	kotlinClasses.Lock()
	kotlinClasses.files[path] = kotlinClassesOf{size: info.Size(), modTime: info.ModTime(), classes: classes}
	kotlinClasses.Unlock()
	return classes
}

// KotlinRunner is what measures the coverage of the Kotlin module at dir:
// "maven", JaCoCo through the project's jacoco-maven-plugin, for a module
// with a pom.xml; "kover", Kover's XML report, for a Gradle build file that
// mentions Kover; "jacoco", Gradle's jacocoTestReport, otherwise.
func KotlinRunner(dir string) string {
	switch {
	case exists(filepath.Join(dir, "pom.xml")):
		return "maven"
	case buildMentions(dir, "kover"):
		return "kover"
	}
	return "jacoco"
}

// GradleWrapper says whether the Gradle build of the module at dir runs
// through a gradlew in it or a directory above it, rather than the gradle
// on PATH.
func GradleWrapper(dir string) bool {
	return lang.FindUp(filepath.Join(dir, "x"), "gradlew") != ""
}
