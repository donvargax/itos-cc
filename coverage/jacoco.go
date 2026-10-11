package coverage

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Kotlin: the JVMs a test starts are measured by JaCoCo's runtime agent
// when the project's harness opts in, as a Go one builds its binary with
// -cover: while the coverage command runs, JacocoAgentEnv names the agent
// jar, org.jacoco.agent-<version>-runtime.jar, found in the Gradle or Maven
// cache, and JacocoDestDirEnv a directory of the run's own, so a harness
// starts its JVMs with
//
//	-javaagent:$ITOS_CC_JACOCO_AGENT=destfile=$ITOS_CC_JACOCO_DESTDIR/<name>.exec
//
// a name of each JVM's own, or append=true. Once the commands ran, the
// .exec files written there are merged and reported as XML by jacococli,
// org.jacoco.cli-<version>-nodeps.jar from the same caches, run with the
// build's java, against the module's class files and sources, and read
// beside the in-process JaCoCo or Kover report as Integration data. Nothing
// is downloaded: a jar no build fetched is missing. Without the agent,
// JacocoAgentEnv is unset, which a harness can tell, and its JVMs go
// unmeasured, as before; .exec files without jacococli to report them are
// a missing collector (Report.IntegrationMissing). Kover measures with an
// agent of its own, which itos-cc does not name: a Kover project's harness
// uses JaCoCo's agent too, from caches some build filled.
//
// Gradle deems a test task up to date when its inputs did not change, and
// the environment is none of them, so while the agent is named the test
// task runs with --rerun (Gradle 7.6 and later): otherwise a test the JVMs
// of which wrote last run's data would write none to this run's directory.

// JacocoAgentEnv and JacocoDestDirEnv name the JaCoCo runtime agent jar a
// harness starts the JVMs its tests run with, and the directory their
// .exec files go to.
const (
	JacocoAgentEnv   = "ITOS_CC_JACOCO_AGENT"
	JacocoDestDirEnv = "ITOS_CC_JACOCO_DESTDIR"
)

// jacocoAgentMissing says why the JVMs the tests start go unmeasured when
// no runtime agent is cached.
const jacocoAgentMissing = "no JaCoCo runtime agent (org.jacoco:org.jacoco.agent:<version>:runtime) is in the Gradle or Maven cache, so " +
	JacocoAgentEnv + " is unset and the JVMs the tests start go unmeasured"

// jacocoCLIMissing says why .exec files the JVMs the tests started wrote
// cannot be reported.
const jacocoCLIMissing = "the JVMs the tests started wrote JaCoCo .exec files, and no jacococli (org.jacoco:org.jacoco.cli:<version>:nodeps) " +
	"is in the Gradle or Maven cache to report them; have a build resolve it"

// jacocoRepository is a local repository JaCoCo's jars may be cached in:
// Gradle's module cache, or Maven's local repository.
type jacocoRepository struct {
	dir    string
	gradle bool
}

// jacocoRepositories is where JaCoCo's jars are looked for: Gradle's module
// cache, under GRADLE_USER_HOME or ~/.gradle, and Maven's local repository,
// ~/.m2/repository.
var jacocoRepositories = func() []jacocoRepository {
	home, _ := os.UserHomeDir()
	gradle := os.Getenv("GRADLE_USER_HOME")
	if gradle == "" && home != "" {
		gradle = filepath.Join(home, ".gradle")
	}
	var out []jacocoRepository
	if gradle != "" {
		out = append(out, jacocoRepository{filepath.Join(gradle, "caches", "modules-2", "files-2.1", "org.jacoco"), true})
	}
	if home != "" {
		out = append(out, jacocoRepository{filepath.Join(home, ".m2", "repository", "org", "jacoco"), false})
	}
	return out
}

// jars is each version of artifact's jar with classifier the repository
// holds, by version.
func (r jacocoRepository) jars(artifact, classifier string) map[string]string {
	out := map[string]string{}
	versions, _ := os.ReadDir(filepath.Join(r.dir, artifact))
	for _, v := range versions {
		dir := filepath.Join(r.dir, artifact, v.Name())
		name := artifact + "-" + v.Name() + "-" + classifier + ".jar"
		if !r.gradle {
			if path := filepath.Join(dir, name); exists(path) {
				out[v.Name()] = path
			}
			continue
		}
		// files-2.1/<group>/<artifact>/<version>/<sha1>/<file>
		hashes, _ := os.ReadDir(dir)
		for _, h := range hashes {
			if path := filepath.Join(dir, h.Name(), name); exists(path) {
				out[v.Name()] = path
				break
			}
		}
	}
	return out
}

// jacocoJar is the path of JaCoCo's artifact jar with classifier, from the
// caches of the build at dir, the build tool's own first: version prefer
// when it is there, else the newest version the build's files mention,
// such as jacoco's toolVersion or jacoco-maven-plugin's version, else the
// newest. It is "" when no cache holds one.
func jacocoJar(dir, artifact, classifier, prefer string) string {
	repos := jacocoRepositories()
	if exists(filepath.Join(dir, "pom.xml")) {
		sort.SliceStable(repos, func(i, j int) bool { return !repos[i].gradle && repos[j].gradle })
	}
	found := map[string]string{}
	for _, r := range repos {
		for v, path := range r.jars(artifact, classifier) {
			if _, ok := found[v]; !ok {
				found[v] = path
			}
		}
	}
	if path, ok := found[prefer]; ok && prefer != "" {
		return path
	}
	text := buildFilesText(dir)
	best, mentioned := "", false
	for v := range found {
		m := mentionsVersion(text, v)
		switch {
		case best == "", m && !mentioned, m == mentioned && versionLess(best, v):
			best, mentioned = v, m
		}
	}
	return found[best]
}

// jarVersion is the version of a jar jacocoJar found, <artifact>-<version>-
// <classifier>.jar in a directory of that version: that directory's name in
// Maven's layout, its parent's in Gradle's.
func jarVersion(path string) string {
	for d := filepath.Dir(path); d != filepath.Dir(d); d = filepath.Dir(d) {
		if v := filepath.Base(d); strings.Contains(filepath.Base(path), "-"+v+"-") {
			return v
		}
	}
	return ""
}

// buildFilesText is the build files of the module at dir and of the
// directories above it that hold build files too, such as a multi-module
// build's root, read and joined.
func buildFilesText(dir string) string {
	names := []string{"build.gradle.kts", "build.gradle", "settings.gradle.kts", "settings.gradle", "gradle.properties",
		filepath.Join("gradle", "libs.versions.toml"), "pom.xml"}
	var text strings.Builder
	for d := dir; ; d = filepath.Dir(d) {
		held := false
		for _, name := range names {
			if data, err := os.ReadFile(filepath.Join(d, name)); err == nil {
				held = true
				text.Write(data)
				text.WriteByte('\n')
			}
		}
		if (!held && d != dir) || filepath.Dir(d) == d {
			return text.String()
		}
	}
}

// mentionsVersion says whether text holds version v whole, not as part of
// a longer one: 0.8.1 is not in 0.8.15.
func mentionsVersion(text, v string) bool {
	return regexp.MustCompile(`(^|[^0-9A-Za-z.])` + regexp.QuoteMeta(v) + `($|[^0-9A-Za-z.])`).MatchString(text)
}

// versionLess orders versions by their dot-separated parts, numerically
// where both are numbers.
func versionLess(a, b string) bool {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		if as[i] == bs[i] {
			continue
		}
		an, aerr := strconv.Atoi(as[i])
		bn, berr := strconv.Atoi(bs[i])
		if aerr == nil && berr == nil {
			return an < bn
		}
		return as[i] < bs[i]
	}
	return len(as) < len(bs)
}

// javaFor is the build's java: JAVA_HOME's, which Gradle and Maven run
// with, else the one on PATH.
func javaFor() string {
	if home := os.Getenv("JAVA_HOME"); home != "" {
		java := filepath.Join(home, "bin", "java")
		if exists(java) || exists(java+".exe") {
			return java
		}
	}
	return "java"
}

// kotlinClassDirs is the class directories of the module at dir that its
// build wrote: Gradle's Kotlin and Java main classes, or Maven's.
func kotlinClassDirs(dir string) []string {
	return existingDirs(dir, "build/classes/kotlin/main", "build/classes/java/main", "target/classes")
}

// kotlinSourceDirs is the main source directories of the module at dir.
func kotlinSourceDirs(dir string) []string {
	return existingDirs(dir, "src/main/kotlin", "src/main/java")
}

func existingDirs(dir string, names ...string) []string {
	var out []string
	for _, name := range names {
		if path := filepath.Join(dir, filepath.FromSlash(name)); exists(path) {
			out = append(out, path)
		}
	}
	return out
}

// jacocoReport is the command that has jacococli merge the .exec files
// execs and write their XML report to out, against the class files and
// sources of the module at dir.
func jacocoReport(cli, dir string, execs []string, out string) []string {
	args := append([]string{javaFor(), "-jar", cli, "report"}, execs...)
	for _, d := range kotlinClassDirs(dir) {
		args = append(args, "--classfiles", d)
	}
	for _, d := range kotlinSourceDirs(dir) {
		args = append(args, "--sourcefiles", d)
	}
	return append(args, "--name", "integration", "--xml", out)
}

// execFiles is the .exec files in d, sorted.
func execFiles(d string) []string {
	found, _ := filepath.Glob(filepath.Join(d, "*.exec"))
	sort.Strings(found)
	return found
}

// kotlinIntegration sets plan's integration coverage: the JVMs the tests
// of the module at dir start with the agent JacocoAgentEnv names write
// their .exec files under out, which jacococli reports. Without the agent
// it only says so (Notice).
func kotlinIntegration(plan *Plan, dir, out string) {
	agent := jacocoJar(dir, "org.jacoco.agent", "runtime", "")
	if agent == "" {
		plan.Notice = jacocoAgentMissing
		return
	}
	plan.CoverDir = filepath.Join(out, "integration")
	plan.CoverEnv = []string{JacocoAgentEnv + "=" + agent, JacocoDestDirEnv + "=" + plan.CoverDir}
	plan.Written = "*.exec"
	report := filepath.Join(out, "integration.xml")
	plan.Integration = report
	cli := jacocoJar(dir, "org.jacoco.cli", "nodeps", jarVersion(agent))
	if cli == "" {
		plan.ConverterMissing = jacocoCLIMissing
		return
	}
	plan.ConvertWritten = func(written []string) [][]string {
		return [][]string{jacocoReport(cli, dir, written, report)}
	}
}

// kotlinTestCollector is the collector of the JVMs the listed tests of the
// module at root start, with dir for their data.
func kotlinTestCollector(root, dir string) testCollector {
	c := testCollector{
		language: "kotlin",
		dir:      root,
		written:  func(d string) bool { return len(execFiles(d)) > 0 },
	}
	agent := jacocoJar(root, "org.jacoco.agent", "runtime", "")
	if agent == "" {
		c.missing = jacocoAgentMissing
		return c
	}
	cli := jacocoJar(root, "org.jacoco.cli", "nodeps", jarVersion(agent))
	if cli == "" {
		c.missing = "no jacococli (org.jacoco:org.jacoco.cli:<version>:nodeps) is in the Gradle or Maven cache to report the .exec files they write"
		return c
	}
	env := func(d string) []string { return []string{JacocoAgentEnv + "=" + agent, JacocoDestDirEnv + "=" + d} }
	c.vars = env("")
	// A harness that does not split writes where JacocoDestDirEnv says,
	// which no test's coverage reads.
	c.whole = env(filepath.Join(dir, ".kotlin", "unsplit"))
	c.each = env
	c.convert = func(d, out string) ([]string, string) {
		return jacocoReport(cli, root, execFiles(d), out+".xml"), out + ".xml"
	}
	return c
}
