package coverage

import "testing"

func TestExecFlagQuotesTheExecutable(t *testing.T) {
	saved := Executable
	defer func() { Executable = saved }()
	for exe, want := range map[string]string{
		"":                      "",
		`C:\Program Files\itos`: `"C:\Program Files\itos" ` + ExecArg,
		`/opt/it"s/itos-cc`:     `'/opt/it"s/itos-cc' ` + ExecArg,
		`/opt/"it's"/itos-cc`:   "",
	} {
		Executable = exe
		if got := execFlag(); got != want {
			t.Errorf("execFlag() with %q = %q, want %q", exe, got, want)
		}
	}
}
