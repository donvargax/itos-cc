package coverage

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The coverage package's tests find no JaCoCo jar in the machine's Gradle
// or Maven cache: those that need one make a cache of their own
// (fakeJacocoCaches).
func init() {
	jacocoRepositories = func() []jacocoRepository { return nil }
}

// fakeJacocoCaches makes a Gradle cache and a Maven local repository
// holding jars, each "gradle/<artifact>-<version>-<classifier>.jar" or
// "maven/…", in their layouts, and has jacocoJar look in them for t.
func fakeJacocoCaches(t *testing.T, jars ...string) (gradle, maven string) {
	t.Helper()
	root := t.TempDir()
	gradle, maven = filepath.Join(root, "gradle"), filepath.Join(root, "maven")
	for _, jar := range jars {
		repo, name, _ := strings.Cut(jar, "/")
		// <artifact>-<version>-<classifier>.jar, the artifact holding dots
		// and no hyphen.
		artifact, rest, _ := strings.Cut(name, "-")
		version := rest[:strings.LastIndex(rest, "-")]
		path := filepath.Join(maven, artifact, version, name)
		if repo == "gradle" {
			path = filepath.Join(gradle, artifact, version, "0123abcd", name)
		}
		writeFiles(t, filepath.Dir(path), map[string]string{name: "jar"})
	}
	previous := jacocoRepositories
	jacocoRepositories = func() []jacocoRepository {
		return []jacocoRepository{{gradle, true}, {maven, false}}
	}
	t.Cleanup(func() { jacocoRepositories = previous })
	return gradle, maven
}

func TestJacocoJarsComeFromTheCachesAtTheVersionTheBuildNames(t *testing.T) {
	gradle, maven := fakeJacocoCaches(t,
		"gradle/org.jacoco.agent-0.8.12-runtime.jar", "gradle/org.jacoco.agent-0.8.15-runtime.jar",
		"maven/org.jacoco.agent-0.8.14-runtime.jar", "maven/org.jacoco.agent-0.8.12-runtime.jar",
		"gradle/org.jacoco.cli-0.8.14-nodeps.jar", "maven/org.jacoco.cli-0.8.12-nodeps.jar")
	agent := func(version string) string {
		return filepath.Join(gradle, "org.jacoco.agent", version, "0123abcd", "org.jacoco.agent-"+version+"-runtime.jar")
	}
	for _, c := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"the newest, when the build names none", map[string]string{"build.gradle.kts": "plugins { jacoco }\n"}, agent("0.8.15")},
		{"the build's toolVersion", map[string]string{"build.gradle.kts": "jacoco {\n    toolVersion = \"0.8.12\"\n}\n"}, agent("0.8.12")},
		{"a version the root build names", map[string]string{"build.gradle.kts": "", "../settings.gradle.kts": "", "../gradle/libs.versions.toml": "jacoco = \"0.8.14\"\n"},
			filepath.Join(maven, "org.jacoco.agent", "0.8.14", "org.jacoco.agent-0.8.14-runtime.jar")},
		{"not a version that only begins another", map[string]string{"build.gradle.kts": "version = \"0.8.1\"\n"}, agent("0.8.15")},
		{"Maven's cache first for a Maven build", map[string]string{"pom.xml": "<version>0.8.12</version>"},
			filepath.Join(maven, "org.jacoco.agent", "0.8.12", "org.jacoco.agent-0.8.12-runtime.jar")},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "app")
			for name, text := range c.files {
				writeFiles(t, dir, map[string]string{filepath.FromSlash(name): text})
			}
			if got := jacocoJar(dir, "org.jacoco.agent", "runtime", ""); got != c.want {
				t.Errorf("agent %s, want %s", got, c.want)
			}
		})
	}
	dir := t.TempDir()
	if got := jacocoJar(dir, "org.jacoco.cli", "nodeps", "0.8.12"); got != filepath.Join(maven, "org.jacoco.cli", "0.8.12", "org.jacoco.cli-0.8.12-nodeps.jar") {
		t.Errorf("jacococli %s, want the agent's version, 0.8.12, Maven's", got)
	}
	if got := jacocoJar(dir, "org.jacoco.cli", "nodeps", "0.8.15"); got != filepath.Join(gradle, "org.jacoco.cli", "0.8.14", "0123abcd", "org.jacoco.cli-0.8.14-nodeps.jar") {
		t.Errorf("jacococli %s, want the newest when the agent's version is not cached", got)
	}
	if got := jacocoJar(dir, "org.jacoco.agent", "nodeps", ""); got != "" {
		t.Errorf("a classifier no cache holds: %s, want none", got)
	}
	for _, path := range []string{agent("0.8.15"), filepath.Join(maven, "org.jacoco.agent", "0.8.14", "org.jacoco.agent-0.8.14-runtime.jar")} {
		if v := jarVersion(path); v != filepath.Base(filepath.Dir(path)) && v != filepath.Base(filepath.Dir(filepath.Dir(path))) {
			t.Errorf("jarVersion(%s) = %q", path, v)
		}
	}
}

func TestKotlinPlansNameTheCachedJaCoCoAgentToTheJVMsTheTestsStart(t *testing.T) {
	files := map[string]string{"build.gradle.kts": "plugins { jacoco }\n", "src/main/kotlin/a/A.kt": "package a\n\nfun a() = 1\n"}
	plan := func(t *testing.T, out string) Plan {
		t.Helper()
		dir := t.TempDir()
		writeFiles(t, dir, files)
		plans := Plans([]string{filepath.Join(dir, "src", "main", "kotlin", "a", "A.kt")}, out, AllTests, nil)
		if len(plans) != 1 {
			t.Fatalf("plans %+v, want one", plans)
		}
		return plans[0]
	}
	t.Run("no agent cached", func(t *testing.T) {
		p := plan(t, t.TempDir())
		if p.CoverDir != "" || len(p.CoverEnv) != 0 || !strings.Contains(p.Notice, JacocoAgentEnv+" is unset") {
			t.Errorf("plan %+v, want no collector, and a notice that %s is unset", p, JacocoAgentEnv)
		}
		if got := strings.Join(p.Commands[0][2:], " "); strings.Contains(got, "--rerun") {
			t.Errorf("command %q, want Gradle's test task as before, without --rerun", got)
		}
	})
	gradle, _ := fakeJacocoCaches(t, "gradle/org.jacoco.agent-0.8.15-runtime.jar")
	agent := filepath.Join(gradle, "org.jacoco.agent", "0.8.15", "0123abcd", "org.jacoco.agent-0.8.15-runtime.jar")
	t.Run("the agent without jacococli", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "run-1")
		p := plan(t, out)
		if rel, err := filepath.Rel(out, p.CoverDir); err != nil || strings.HasPrefix(rel, "..") || rel == "." {
			t.Errorf("CoverDir %q, want a directory under the run's %s", p.CoverDir, out)
		}
		if want := []string{JacocoAgentEnv + "=" + agent, JacocoDestDirEnv + "=" + p.CoverDir}; !slices.Equal(p.CoverEnv, want) {
			t.Errorf("CoverEnv %q, want %q", p.CoverEnv, want)
		}
		if p.Written != "*.exec" || !strings.Contains(p.ConverterMissing, "jacococli") || p.ConvertWritten != nil {
			t.Errorf("plan %+v, want .exec files, which no jacococli reports", p)
		}
		if got, want := strings.Join(p.Commands[0][2:], " "), "test --rerun jacocoTestReport"; !strings.HasSuffix(got, want) {
			t.Errorf("command %q, want it to end %q: the test task runs though Gradle deems it up to date", got, want)
		}
	})
	writeFiles(t, filepath.Join(gradle, "org.jacoco.cli", "0.8.15", "0123abcd"), map[string]string{"org.jacoco.cli-0.8.15-nodeps.jar": "jar"})
	cli := filepath.Join(gradle, "org.jacoco.cli", "0.8.15", "0123abcd", "org.jacoco.cli-0.8.15-nodeps.jar")
	t.Run("the agent and jacococli", func(t *testing.T) {
		t.Setenv("JAVA_HOME", "")
		p := plan(t, filepath.Join(t.TempDir(), "run-1"))
		if p.ConverterMissing != "" || p.ConvertWritten == nil {
			t.Fatalf("plan %+v, want jacococli to report what the JVMs write", p)
		}
		// The class files and sources the build wrote by then.
		os.MkdirAll(filepath.Join(p.Dir, "build", "classes", "kotlin", "main"), 0o755)
		execs := []string{filepath.Join(p.CoverDir, "a.exec"), filepath.Join(p.CoverDir, "b.exec")}
		got := p.ConvertWritten(execs)
		want := []string{"java", "-jar", cli, "report", execs[0], execs[1],
			"--classfiles", filepath.Join(p.Dir, "build", "classes", "kotlin", "main"),
			"--sourcefiles", filepath.Join(p.Dir, "src", "main", "kotlin"),
			"--name", "integration", "--xml", p.Integration}
		if len(got) != 1 || !slices.Equal(got[0], want) {
			t.Errorf("conversion %q, want [%q]", got, want)
		}
	})
}

// TestHelperJVM is the coverage command of
// TestExecFilesNoJacococliReportsAreAMissingCollector, run as this test
// binary: a JVM started with the agent writes an .exec file where
// ITOS_CC_JACOCO_DESTDIR says, and the in-process report goes where
// HELPER_JVM_REPORT says.
func TestHelperJVM(t *testing.T) {
	report := os.Getenv("HELPER_JVM_REPORT")
	if report == "" {
		return
	}
	if dest := os.Getenv(JacocoDestDirEnv); dest != "" && os.Getenv(JacocoAgentEnv) != "" {
		os.WriteFile(filepath.Join(dest, "jvm.exec"), []byte("exec"), 0o644)
	}
	os.MkdirAll(filepath.Dir(report), 0o755)
	os.WriteFile(report, []byte(`<?xml version="1.0"?><report name="r"></report>`), 0o644)
	os.Exit(0)
}

func TestExecFilesNoJacococliReportsAreAMissingCollector(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src", "main", "kotlin", "a", "A.kt")
	report := filepath.Join(dir, "build", "report.xml")
	t.Setenv("HELPER_JVM_REPORT", report)
	out := t.TempDir()
	base := Plan{Language: "kotlin", Dir: dir, Sources: []string{src},
		Commands: [][]string{{os.Args[0], "-test.run=^TestHelperJVM$"}}, Reports: []string{report},
		CoverDir: filepath.Join(out, "integration"), Written: "*.exec", Integration: filepath.Join(out, "integration.xml"),
		ConverterMissing: jacocoCLIMissing}
	for _, c := range []struct {
		name    string
		agent   bool
		missing bool
	}{
		{"a JVM wrote an .exec file", true, true},
		{"no JVM wrote one", false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := base
			p.CoverEnv = []string{JacocoDestDirEnv + "=" + p.CoverDir}
			if c.agent {
				p.CoverEnv = append(p.CoverEnv, JacocoAgentEnv+"=agent.jar")
			}
			var log bytes.Buffer
			r := Run([]Plan{p}, []string{src}, &log)
			missing := r.IntegrationMissing()
			if got := len(missing) == 1 && missing[0].Language == "kotlin" && missing[0].Cause == ToolMissing && strings.Contains(missing[0].Reason, "jacococli"); got != c.missing {
				t.Errorf("integration missing %+v, want it naming jacococli: %v\nlog:\n%s", missing, c.missing, &log)
			}
			if c.missing && !strings.Contains(log.String(), "going on without integration coverage") {
				t.Errorf("log %q, want a line saying the run goes on without integration coverage", &log)
			}
			if !c.missing && len(missing) != 0 {
				t.Errorf("integration missing %+v, want none: a collector the harness did not use is not missing", missing)
			}
			if !r.Measures("kotlin") {
				t.Errorf("the in-process report no longer measures kotlin\nlog:\n%s", &log)
			}
			if _, err := os.Stat(p.CoverDir); !os.IsNotExist(err) {
				t.Errorf("%s is still there (%v)", p.CoverDir, err)
			}
		})
	}
}
