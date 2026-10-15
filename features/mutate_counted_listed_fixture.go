package features

const countedListedSubject = "package main\n\nfunc Compare(n int) bool { return n > 5 }\n"
const countedListedOther = "package main\n\nfunc Other(n int) bool { return n > 9 }\n"
const countedListedMain = `package main
import("fmt";"os";"strconv")
func main(){
 n,err:=strconv.Atoi(os.Args[1])
 if err!=nil{panic(err)}
 fmt.Println(Compare(n))
}
`
const countedListedOwn = `package main
import("crypto/sha256";"encoding/hex";"encoding/json";"os";"path/filepath";"testing")
func mode()string{b,e:=os.ReadFile(CONTROL_PATH);if e!=nil{panic(e)};var c struct{Mode string};if e=json.Unmarshal(b,&c);e!=nil{panic(e)};return c.Mode}
func TestMain(m *testing.M){
 cwd,e:=os.Getwd();if e!=nil{panic(e)};cwd,e=filepath.EvalSymlinks(cwd);if e!=nil{panic(e)};original,e:=filepath.EvalSymlinks(ORIGINAL_ROOT);if e!=nil{panic(e)}
 b,e:=os.ReadFile("subject.go");if e!=nil{panic(e)};h:=sha256.Sum256(b);f,e:=os.OpenFile(TRACE_PATH,os.O_WRONLY|os.O_CREATE|os.O_APPEND,0600);if e!=nil{panic(e)}
 if e=json.NewEncoder(f).Encode(map[string]any{"cwd":cwd,"original_root":original,"source_sha256":hex.EncodeToString(h[:]),"test":"own","coverage":testing.CoverMode()!=""});e!=nil{panic(e)};if e=f.Close();e!=nil{panic(e)};os.Exit(m.Run())
}
func TestCompare(t *testing.T){
 c:=mode();if c=="own-baseline-fail"&&testing.CoverMode()==""{t.Fatal("T81 own baseline fails")}
 if c=="timeout"&&testing.CoverMode()==""&&Compare(5){select{}}
 if !Compare(6){t.Fatal("Compare positive")}
 if c=="own-kill"&&Compare(5){t.Fatal("Compare boundary")}
 if c=="coverage-fail"&&testing.CoverMode()!=""{t.Fatal("T81 coverage fails after report")}
}
func TestOther(t *testing.T){if !Other(10)||Other(9){t.Fatal("Other boundary")}}
`
const countedListedHarness = `package e2e
import("crypto/sha256";"encoding/hex";"encoding/json";"flag";"os";"os/exec";"path/filepath";"slices";"strings";"testing")
var selected=flag.String("tests","","selected IDs")
func TestListed(t *testing.T){
 b,e:=os.ReadFile(CONTROL_PATH);if e!=nil{t.Fatal(e)};var c struct{Mode string};if e=json.Unmarshal(b,&c);e!=nil{t.Fatal(e)}
 split:=os.Getenv("ITOS_CC_TEST_COVERDIR");covered:=split!=""||os.Getenv("GOCOVERDIR")!=""
 cwd,e:=os.Getwd();if e!=nil{t.Fatal(e)};cwd,e=filepath.EvalSymlinks(cwd);if e!=nil{t.Fatal(e)};original,e:=filepath.EvalSymlinks(ORIGINAL_ROOT);if e!=nil{t.Fatal(e)}
 src,e:=os.ReadFile(filepath.Join("..","subject.go"));if e!=nil{t.Fatal(e)};h:=sha256.Sum256(src)
 ids:=[]string{"ID-A-01","ID-A-02"};if *selected!=""{ids=strings.Split(*selected,",")}
 f,e:=os.OpenFile(TRACE_PATH,os.O_WRONLY|os.O_CREATE|os.O_APPEND,0600);if e!=nil{t.Fatal(e)}
 if e=json.NewEncoder(f).Encode(map[string]any{"cwd":cwd,"original_root":original,"source_sha256":hex.EncodeToString(h[:]),"test":"listed","ids":ids,"coverage":covered});e!=nil{t.Fatal(e)};if e=f.Close();e!=nil{t.Fatal(e)}
 if c.Mode=="listed-baseline-fail"&&!covered{t.Fatal("T81 listed baseline fails")}
 bin:=filepath.Join(t.TempDir(),"subject.exe");args:=[]string{"build","-o",bin,".."};if covered{args=[]string{"build","-cover","-o",bin,".."}};if out,e:=exec.Command("go",args...).CombinedOutput();e!=nil{t.Fatalf("build %v: %s",e,out)}
 for _,id:=range ids{
  if !slices.Contains([]string{"ID-A-01","ID-A-02"},id){t.Fatal("unexpected test ID",id)}
  arg,want:="6","true";if id=="ID-A-01"&&c.Mode!="survive"{arg,want="5","false"}
  cmd:=exec.Command(bin,arg)
  if split!=""{dir:=filepath.Join(split,id);if e:=os.MkdirAll(dir,0700);e!=nil{t.Fatal(e)}
   if c.Mode=="bad-metadata"{meta:=filepath.Join(dir,"covmeta.T81-invalid");if e:=os.WriteFile(meta,[]byte("T81 invalid Go coverage metadata"),0600);e!=nil{t.Fatal(e)};data,e:=os.ReadFile(meta);if e!=nil{t.Fatal(e)};if e=os.WriteFile(filepath.Join(filepath.Dir(TRACE_PATH),"observed-covmeta"),data,0600);e!=nil{t.Fatal(e)}}else{cmd.Env=append(os.Environ(),"GOCOVERDIR="+dir)}
  }
  out,e:=cmd.Output();if e!=nil||strings.TrimSpace(string(out))!=want{t.Errorf("%s: %v output%s want%s",id,e,out,want)}
 }
 if c.Mode=="listed-coverage-fail"&&covered{t.Error("T81 listed coverage fails after data")}
}
`
const countedListedList = `package main
import("encoding/json";"fmt";"os")
func main(){b,e:=os.ReadFile(CONTROL_PATH);if e!=nil{panic(e)};var c struct{Mode string};if e=json.Unmarshal(b,&c);e!=nil{panic(e)};if c.Mode=="list-fail"{fmt.Fprintln(os.Stderr,"T81 list fails");os.Exit(1)};if c.Mode=="list-malformed"{fmt.Print("ID-A-01\nID-A-01\n");return};fmt.Print("ID-A-01\te2e/e2e_test.go\nID-A-02\te2e/e2e_test.go\n")}
`
const countedListedConfig = `mutation:
  tests:
    list: go run ./testdata/list.go
    run: go test -count=1 ./e2e -args -tests={pattern}
    ids_pattern: "{ids}"
    join:
      each: "{id}"
      sep: ","
`
