package domain

import "strings"

// GradleTests is the --tests filter of a Gradle test task that runs
// classes, test classes by fully qualified name.
func GradleTests(classes []string) []string {
	var out []string
	for _, class := range classes {
		out = append(out, "--tests", class)
	}
	return out
}

// MavenTests is the properties that have Surefire run classes, test classes
// by fully qualified name, in each module of a build that holds any of
// them, and none in one that holds none; nothing when classes is nil.
func MavenTests(classes []string) []string {
	if classes == nil {
		return nil
	}
	return []string{"-Dtest=" + strings.Join(classes, ","), "-Dsurefire.failIfNoSpecifiedTests=false"}
}
