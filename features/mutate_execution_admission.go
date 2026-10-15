package features

import (
	"fmt"
	"path/filepath"
)

const executionThreeReport = `mode: set
example.com/ownership/main.go:3.25,3.39 1 1
example.com/ownership/main.go:5.28,5.42 1 1
example.com/ownership/main.go:7.26,7.40 1 1
`

func (q *mutationExecution) admissionProject() error {
	if err := q.project(map[string]string{"main.go": executionThreeSource}); err != nil {
		return err
	}
	if err := q.m.w.d.Coverage(q.m.w.dir, Coverage{Language: "go", Report: executionThreeReport}); err != nil {
		return err
	}
	if err := q.m.w.write("testdata/list.go", admissionList); err != nil {
		return err
	}
	if err := q.m.w.write("itos-cc.yaml", `mutation:
  tests:
    list: go run ./testdata/list.go
    run: go test -count=1 -run '{pattern}' .
    ids_pattern: "^({ids})$"
    join:
      each: "{id}"
      sep: "|"
`); err != nil {
		return err
	}
	if err := q.m.w.d.Git(q.m.w.dir, "add", "testdata/list.go", "itos-cc.yaml"); err != nil {
		return err
	}
	if err := q.m.w.d.Git(q.m.w.dir, "commit", "-qm", "configured admission commands"); err != nil {
		return err
	}
	q.m.w.variables = append(q.m.w.variables, "T80_LIST="+filepath.Join(q.outside, "list.log"))
	if err := q.m.w.d.Write(filepath.Join(q.outside, "list.log"), nil); err != nil {
		return err
	}
	t := Tests{Language: "go", Command: "go test -count=1 -run '{pattern}' .", Listed: &ListedTests{Files: map[string]string{"TestTrial": "main_test.go"}, Lines: map[string]map[int][]string{"main.go": {3: {"TestTrial"}, 5: {"TestTrial"}, 7: {"TestTrial"}}}, Surviving: map[string][]Site{"TestTrial": {{File: "main.go", Line: 3, Original: ">", Replacement: ">="}}}}}
	if err := q.m.w.d.Tests(q.m.w.dir, t); err != nil {
		return err
	}
	return q.m.w.d.Write(filepath.Join(q.outside, "trace.jsonl"), nil)
}

const admissionList = `package main
import("fmt";"os")
func main(){f,err:=os.OpenFile(os.Getenv("T80_LIST"),os.O_CREATE|os.O_APPEND|os.O_WRONLY,0600);if err!=nil{panic(err)};if _,err=f.WriteString("list\n");err!=nil{panic(err)};if err=f.Close();err!=nil{panic(err)};fmt.Println("TestTrial\tmain_test.go")}
`

func (q *mutationExecution) noListedCommand() error {
	data, err := q.m.w.d.Read(filepath.Join(q.outside, "list.log"))
	if err != nil {
		return err
	}
	if len(data) != 0 {
		return fmt.Errorf("admission launched listed inventory: %s", data)
	}
	return nil
}
