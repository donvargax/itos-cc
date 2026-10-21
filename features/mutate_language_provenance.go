package features

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
)

func (l *languageMutation) instrumentInstalled(files map[string]string) error {
	if err := l.ownTrace(); err != nil {
		return err
	}
	trace, _ := json.Marshal(l.trace)
	original, _ := json.Marshal(l.m.w.dir)
	switch l.language {
	case "typescript":
		prelude := "import {appendFileSync,readFileSync,realpathSync} from 'node:fs';\nimport {createHash} from 'node:crypto';\nimport {createRequire} from 'node:module';\nimport {dirname,resolve} from 'node:path';\nimport {fileURLToPath} from 'node:url';\n"
		record := "const root=resolve(dirname(fileURLToPath(import.meta.url)),'..');appendFileSync(" + string(trace) + ",JSON.stringify({cwd:realpathSync(root),original_root:realpathSync(" + string(original) + "),source_sha256:createHash('sha256').update(readFileSync(resolve(root,'src/board.ts'))).digest('hex'),dependency:realpathSync(createRequire(import.meta.url).resolve('vitest'))})+'\\n');"
		files["src/board.test.ts"] = prelude + strings.Replace(files["src/board.test.ts"], "test('low',()=>{", "test('low',()=>{"+record, 1)
	case "python":
		prelude := "import pathlib,json,hashlib,boardhelp,board\n"
		record := "    with open(" + string(trace) + ",'a') as f: f.write(json.dumps({'cwd':str(pathlib.Path(__file__).resolve().parent),'original_root':str(pathlib.Path(" + string(original) + ").resolve()),'source_sha256':hashlib.sha256(pathlib.Path(board.__file__).read_bytes()).hexdigest(),'dependency':str(pathlib.Path(boardhelp.__file__).resolve())})+'\\n')\n"
		files["test_board.py"] = prelude + strings.Replace(files["test_board.py"], "def test_low():\n", "def test_low():\n"+record, 1)
	case "kotlin":
		prelude := "import java.io.File\nimport java.security.MessageDigest\n"
		text := strings.Replace(files["src/test/kotlin/board/BoardTest.kt"], "import kotlin.test.*\n", "import kotlin.test.*\n"+prelude, 1)
		text = strings.Replace(text, "fun judges(){", "fun judges(){ recordInstalled();", 1)
		text += fmt.Sprintf(`
fun installedQuote(s: String): String = "\"" + s.replace("\\", "\\\\").replace("\"", "\\\"") + "\""
fun recordInstalled() {
 val sha = MessageDigest.getInstance("SHA-256").digest(File("src/main/kotlin/board/Board.kt").readBytes()).joinToString("") { "%%02x".format(it) }
 val dependency = File(kotlin.Unit::class.java.protectionDomain.codeSource.location.toURI()).canonicalPath
 File(%s).appendText("{\"cwd\":" + installedQuote(File(".").canonicalPath) + ",\"original_root\":" + installedQuote(File(%s).canonicalPath) + ",\"source_sha256\":" + installedQuote(sha) + ",\"dependency\":" + installedQuote(dependency) + "}\n")
}
`, string(trace), string(original))
		files["src/test/kotlin/board/BoardTest.kt"] = text
	case "dart":
		// Dart's core libraries hash nothing, so the test records the
		// source itself, which dependencyRecords hashes, and, as the
		// installed dependency, where its package config resolves matcher,
		// flutter_test's, from: the pub cache.
		prelude := "import 'dart:convert';\nimport 'dart:io';\n"
		record := "void recordInstalled() {\n" +
			"  final config = File('.dart_tool/package_config.json');\n" +
			"  final packages = jsonDecode(config.readAsStringSync())['packages'] as List;\n" +
			"  final matcher = packages.firstWhere((p) => p['name'] == 'matcher');\n" +
			"  final dependency = Directory.fromUri(config.absolute.uri.resolve(matcher['rootUri'] as String)).resolveSymbolicLinksSync();\n" +
			"  File(" + string(trace) + ").writeAsStringSync(jsonEncode({\n" +
			"    'cwd': Directory.current.resolveSymbolicLinksSync(),\n" +
			"    'original_root': Directory(" + string(original) + ").resolveSymbolicLinksSync(),\n" +
			"    'source': File('lib/board.dart').readAsStringSync(),\n" +
			"    'dependency': dependency,\n" +
			"  }) + '\\n', mode: FileMode.append, flush: true);\n}\n\n"
		text := prelude + strings.Replace(files["test/board_test.dart"], "void main() {\n", record+"void main() {\n", 1)
		files["test/board_test.dart"] = strings.Replace(text, "test('low', () {\n", "test('low', () {\n    recordInstalled();\n", 1)
	}
	return nil
}
func (l *languageMutation) dependencyRecords() ([]map[string]any, error) {
	b, err := l.m.w.d.Read(l.trace)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var a map[string]any
		if err := json.Unmarshal([]byte(line), &a); err != nil {
			return nil, err
		}
		if source, ok := a["source"].(string); ok {
			a["source_sha256"] = fmt.Sprintf("%x", sha256.Sum256([]byte(source)))
			delete(a, "source")
		}
		out = append(out, a)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no actual dependency provenance")
	}
	return out, nil
}
func (l *languageMutation) rememberInstalledTarget() error {
	records, err := l.dependencyRecords()
	if err != nil {
		return err
	}
	want := fmt.Sprintf("%x", sha256.Sum256([]byte(l.frozenSource)))
	for _, r := range records {
		target, ok := r["dependency"].(string)
		if !ok || target == "" || r["cwd"] != r["original_root"] || r["source_sha256"] != want {
			return fmt.Errorf("invalid original installed dependency observation: %v", r)
		}
		if l.dependencyTarget != "" && l.dependencyTarget != target {
			return fmt.Errorf("setup dependency changed target: %v", records)
		}
		l.dependencyTarget = target
	}
	return nil
}
func (l *languageMutation) installedProvenance() error {
	records, err := l.dependencyRecords()
	if err != nil {
		return err
	}
	operator := "=="
	replacement := "!="
	if l.language == "typescript" {
		operator = "==="
		replacement = "!=="
	}
	base := fmt.Sprintf("%x", sha256.Sum256([]byte(l.frozenSource)))
	mutant := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Replace(l.frozenSource, operator, replacement, 1))))
	live := fmt.Sprintf("%x", sha256.Sum256([]byte(l.m.read(l.file))))
	if live == base || base == mutant {
		return fmt.Errorf("frozen/live/mutant fixture hashes are not distinct")
	}
	seenBase, seenMutant := 0, 0
	for _, r := range records {
		if r["dependency"] != l.dependencyTarget || r["cwd"] == r["original_root"] || r["source_sha256"] == live {
			return fmt.Errorf("counted run used live input or different installed dependency: %v; setup target%s", r, l.dependencyTarget)
		}
		switch r["source_sha256"] {
		case base:
			seenBase++
		case mutant:
			seenMutant++
		default:
			return fmt.Errorf("counted source was neither frozen original nor actual fixture mutant: %v", r)
		}
	}
	if seenBase < 2 || seenMutant < 1 {
		return fmt.Errorf("missing frozen coverage/baseline/mutant provenance: %v", records)
	}
	return nil
}
