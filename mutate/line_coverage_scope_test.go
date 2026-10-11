package mutate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scopeOf is LineCoverageUnsupported of the file at source in a project
// at root holding files.
func scopeOf(t *testing.T, language, source string, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	writeTree(t, root, files)
	got, err := LineCoverageUnsupported(language, filepath.Join(root, filepath.FromSlash(source)), root)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// Strict line coverage refuses only the scopes its fingerprints leave out:
// a workspace, or a local dependency outside the build root's inventory,
// outside the root or through a nested build root. One inside the
// inventory, which the fingerprints hold, is admitted.
func TestLineCoverageRefusesOnlyScopesOutsideItsInventory(t *testing.T) {
	const ts = "export function f(): number {\n  return 1;\n}\n"
	const py = "def f():\n    return 1\n"
	const kt = "fun f(): Int {\n    return 1\n}\n"
	for _, c := range []struct {
		name, language, source string
		files                  map[string]string
		want                   string // a word of the refusal, "" for none
	}{
		{"npm workspace above the package", "typescript", "packages/a/src/f.ts", map[string]string{
			"package.json": `{"workspaces": ["packages/*"]}`, "packages/a/package.json": `{"name": "a"}`, "packages/a/src/f.ts": ts}, "workspace"},
		{"yarn workspaces object", "typescript", "src/f.ts", map[string]string{
			"package.json": `{"workspaces": {"packages": ["x"]}}`, "src/f.ts": ts}, "workspace"},
		{"pnpm workspace", "typescript", "src/f.ts", map[string]string{
			"package.json": `{}`, "pnpm-workspace.yaml": "packages: []\n", "src/f.ts": ts}, "pnpm-workspace.yaml"},
		{"workspace: dependency", "typescript", "src/f.ts", map[string]string{
			"package.json": `{"dependencies": {"b": "workspace:*"}}`, "src/f.ts": ts}, "workspace:*"},
		{"file: dependency outside", "typescript", "src/f.ts", map[string]string{
			"package.json": `{"devDependencies": {"b": "file:../b"}}`, "src/f.ts": ts}, "file:../b"},
		{"link: dependency outside", "typescript", "src/f.ts", map[string]string{
			"package.json": `{"dependencies": {"b": "link:/elsewhere/b"}}`, "src/f.ts": ts}, "link:/elsewhere/b"},
		{"file: dependency on a nested package", "typescript", "src/f.ts", map[string]string{
			"package.json": `{"dependencies": {"b": "file:./vendor/b"}}`, "vendor/b/package.json": `{}`, "src/f.ts": ts}, "file:./vendor/b"},
		{"file: dependency on a tarball", "typescript", "src/f.ts", map[string]string{
			"package.json": `{"dependencies": {"b": "file:./b.tgz"}}`, "b.tgz": "", "src/f.ts": ts}, "file:./b.tgz"},
		{"file: dependency inside the inventory", "typescript", "src/f.ts", map[string]string{
			"package.json": `{"dependencies": {"b": "file:./lib/b"}, "scripts": {"t": "x"}}`, "lib/b/index.js": "", "src/f.ts": ts}, ""},
		{"registry dependencies", "typescript", "src/f.ts", map[string]string{
			"package.json": `{"dependencies": {"b": "^1.0.0", "c": "github:a/c"}}`, "src/f.ts": ts}, ""},

		{"uv path source outside", "python", "f.py", map[string]string{
			"pyproject.toml": "[tool.uv.sources]\nlib = { path = \"../lib\", editable = true }\n", "f.py": py}, "../lib"},
		{"poetry develop path outside", "python", "f.py", map[string]string{
			"pyproject.toml": "[tool.poetry.dependencies]\nlib = { path = \"/elsewhere/lib\", develop = true }\n", "f.py": py}, "/elsewhere/lib"},
		{"file: URL requirement outside", "python", "f.py", map[string]string{
			"pyproject.toml": "[project]\ndependencies = [\"lib @ file:///elsewhere/lib\"]\n", "f.py": py}, "/elsewhere/lib"},
		{"path source on a nested build root", "python", "f.py", map[string]string{
			"pyproject.toml": "[tool.uv.sources]\nlib = { path = \"libs/lib\" }\n", "libs/lib/pyproject.toml": "", "f.py": py}, "libs/lib"},
		{"path source inside the inventory", "python", "f.py", map[string]string{
			"pyproject.toml": "[tool.uv.sources]\nlib = { path = \"libs/lib\" }\n", "libs/lib/x.py": "", "f.py": py}, ""},
		{"editable requirement outside", "python", "f.py", map[string]string{
			"pyproject.toml": "", "requirements-dev.txt": "pytest==8\n-e ../lib\n", "f.py": py}, "../lib"},
		{"editable requirements inside or remote", "python", "f.py", map[string]string{
			"pyproject.toml": "", "requirements.txt": "-e .\n-e git+https://example.com/x.git#egg=x\nrequests>=2 # pinned\n", "f.py": py}, ""},
		{"virtualenv .pth outside", "python", "f.py", map[string]string{
			"pyproject.toml": "", "f.py": py, ".venv/pyvenv.cfg": "",
			".venv/lib/python3.13/site-packages/_lib.pth": "import _virtualenv\n/elsewhere/lib/src\n"}, "/elsewhere/lib/src"},
		{"virtualenv editable finder outside", "python", "f.py", map[string]string{
			"pyproject.toml": "", "f.py": py, ".venv/pyvenv.cfg": "",
			".venv/lib/python3.13/site-packages/__editable___lib_0_finder.py": "MAPPING: dict[str, str] = {'lib': '/elsewhere/lib/lib'}\n"}, "/elsewhere/lib/lib"},

		{"Gradle includeBuild outside", "kotlin", "src/main/kotlin/F.kt", map[string]string{
			"settings.gradle.kts": "pluginManagement {\n    includeBuild(\"../conventions\")\n}\n", "build.gradle.kts": "", "src/main/kotlin/F.kt": kt}, "../conventions"},
		{"Groovy includeBuild outside", "kotlin", "app/src/main/kotlin/F.kt", map[string]string{
			"settings.gradle": "include 'app'\nincludeBuild '../lib'\n", "app/build.gradle": "", "app/src/main/kotlin/F.kt": kt}, "../lib"},
		{"Gradle includeBuild inside", "kotlin", "src/main/kotlin/F.kt", map[string]string{
			"settings.gradle.kts": "includeBuild(\"build-logic\")\n", "build.gradle.kts": "", "build-logic/settings.gradle.kts": "", "src/main/kotlin/F.kt": kt}, ""},
		{"Maven module outside", "kotlin", "app/src/main/kotlin/F.kt", map[string]string{
			"pom.xml": "<project><modules><module>app</module><module>../lib</module></modules></project>", "app/pom.xml": "<project/>", "app/src/main/kotlin/F.kt": kt}, "../lib"},
		{"Maven 4 subproject outside", "kotlin", "src/main/kotlin/F.kt", map[string]string{
			"pom.xml": "<project><subprojects><subproject>../lib</subproject></subprojects></project>", "src/main/kotlin/F.kt": kt}, "../lib"},
		{"Maven parent relativePath outside", "kotlin", "src/main/kotlin/F.kt", map[string]string{
			"pom.xml": "<project><parent><relativePath>../parent/pom.xml</relativePath></parent></project>", "src/main/kotlin/F.kt": kt}, "../parent/pom.xml"},
		{"Maven modules inside, empty relativePath, a commented module", "kotlin", "app/src/main/kotlin/F.kt", map[string]string{
			"pom.xml":     "<project><parent><relativePath/></parent><modules><module>app</module><!-- <module>../old</module> --></modules></project>",
			"app/pom.xml": "<project><parent><relativePath>../pom.xml</relativePath></parent></project>", "app/src/main/kotlin/F.kt": kt}, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := scopeOf(t, c.language, c.source, c.files)
			switch {
			case c.want == "" && got != "":
				t.Errorf("refused %q, want it admitted", got)
			case c.want != "" && !strings.Contains(got, c.want):
				t.Errorf("refusal %q, want one naming %q", got, c.want)
			}
		})
	}
}

// A local dependency inside the root reached through a symbolic link may
// lead anywhere, so it is refused, as Go's replacements are; and a build
// root above the project root is read nowhere: there is no refusal of it.
func TestLineCoverageScopeIsReadBeneathTheProjectRootAlone(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"package.json": `{"dependencies": {"b": "file:./linked"}}`, "src/f.ts": "export const f = 1;\n",
	})
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "linked")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if got, err := LineCoverageUnsupported("typescript", filepath.Join(root, "src", "f.ts"), root); err != nil || !strings.Contains(got, "file:./linked") {
		t.Errorf("a dependency through a symbolic link: %q %v, want it refused", got, err)
	}
	outer := t.TempDir()
	writeTree(t, outer, map[string]string{"package.json": `{"workspaces": ["*"]}`, "inner/src/f.ts": "export const f = 1;\n"})
	inner := filepath.Join(outer, "inner")
	if got, err := LineCoverageUnsupported("typescript", filepath.Join(inner, "src", "f.ts"), inner); err != nil || got != "" {
		t.Errorf("a package root above the project root: %q %v, want nothing read, nothing refused", got, err)
	}
}
