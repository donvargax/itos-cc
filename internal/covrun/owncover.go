package covrun

import (
	"os"
	"path/filepath"
	rtcoverage "runtime/coverage"
	"strings"
	"sync"
)

// The measuring binary's own Go coverage data: an itos-cc built with
// -cover writes its meta-data, covmeta.<hash>, during init and its
// counters, covcounters.<hash>.<pid>.<time>, at exit, to the GOCOVERDIR it
// read at init. go test without -cover hands a listed test's environment,
// whose GOCOVERDIR is that test's folder (eachTestEnv), unchanged to its
// -exec program, itos-cc itself (ExecTest), so that data lands beside the
// project's, and in another counter mode go tool covdata refuses the
// folder. Nothing the wrapper does can keep it out, so the conversion
// leaves it out, as Plan.dropRunnerData leaves out a Node test runner's
// own data.

// ownCoverHash is the meta-data hash of this process's own binary, learned
// once: "" when it was not built with -cover, and in a test binary, whose
// hash the runtime computes only at exit.
var ownCoverHash = sync.OnceValue(func() string { return coverHash(rtcoverage.WriteMetaDir) })

// coverHash is the hash of the meta-data file write writes to a scratch
// folder, removed after, or "" when it writes none.
func coverHash(write func(dir string) error) string {
	dir, err := os.MkdirTemp("", "itos-cc-covmeta-")
	if err != nil {
		return ""
	}
	// A scratch folder left behind costs nothing the hash depends on.
	defer func() { _ = os.RemoveAll(dir) }()
	if write(dir) != nil {
		return ""
	}
	found, _ := filepath.Glob(filepath.Join(dir, "covmeta.*"))
	if len(found) != 1 {
		return ""
	}
	return strings.TrimPrefix(filepath.Base(found[0]), "covmeta.")
}

// dropCoverData removes from dir the meta-data and counter files of the
// binary whose meta-data hash is own. No file is named after an empty
// hash, so then it removes nothing. One it cannot remove stays, and the
// conversion that reads it says why.
func dropCoverData(dir, own string) {
	counters, _ := filepath.Glob(filepath.Join(dir, "covcounters."+own+".*"))
	for _, file := range append(counters, filepath.Join(dir, "covmeta."+own)) {
		_ = os.Remove(file)
	}
}
