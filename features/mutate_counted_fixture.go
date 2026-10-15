package features

const countedOneProfile = "mode: set\nexample.com/ownership/main.go:3.28,3.42 1 1\n"

// These real tests write their observations themselves. Controls change only
// owned external fixture state; committed source/tests/config remain fixed.
const countedGoTests = `package main
import("crypto/sha256";"encoding/hex";"encoding/json";"os";"path/filepath";"strings";"testing")
const tracePath=TRACE_PATH
const controlPath=CONTROL_PATH
const originalRoot=ORIGINAL_ROOT
func TestCompare(t *testing.T){
 data,err:=os.ReadFile(controlPath);if err!=nil{t.Fatal(err)}
 var c struct{Mode string};if err=json.Unmarshal(data,&c);err!=nil{t.Fatal(err)}
 cwd,err:=os.Getwd();if err!=nil{t.Fatal(err)};cwd,err=filepath.EvalSymlinks(cwd);if err!=nil{t.Fatal(err)}
 original:=originalRoot;if value:=os.Getenv("T81_ORIGINAL");value!=""{original=value};original,err=filepath.EvalSymlinks(original);if err!=nil{t.Fatal(err)}
 source,err:=os.ReadFile("main.go");if err!=nil{t.Fatal(err)};hash:=sha256.Sum256(source)
 f,err:=os.OpenFile(tracePath,os.O_WRONLY|os.O_APPEND|os.O_CREATE,0600);if err!=nil{t.Fatal(err)}
 record:=map[string]any{"cwd":cwd,"original_root":original,"source_sha256":hex.EncodeToString(hash[:]),"coverage":testing.CoverMode()!=""}
 if info,err:=os.Stat(filepath.Join(original,".git"));err==nil&&!info.IsDir(){
  b,err:=os.ReadFile(filepath.Join(original,".git"));if err!=nil{t.Fatal(err)};record["git_pointer"]=string(b)
  gitdir:=strings.TrimSpace(strings.TrimPrefix(string(b),"gitdir:"));if !filepath.IsAbs(gitdir){gitdir=filepath.Join(original,gitdir)};gitdir,err=filepath.EvalSymlinks(gitdir);if err!=nil{t.Fatal(err)};record["git_directory"]=gitdir
 }
 if err=json.NewEncoder(f).Encode(record);err!=nil{t.Fatal(err)};if err=f.Close();err!=nil{t.Fatal(err)}
 if c.Mode=="uncovered"&&testing.CoverMode()!=""{return}
 if c.Mode=="baseline-fail"&&testing.CoverMode()==""{t.Fatal("T81 baseline fails")}
 if c.Mode=="survive"{if !Compare(6){t.Fatal("Compare positive")};return}
 if c.Mode=="timeout"&&testing.CoverMode()==""&&Compare(5){select{}}
 if !Compare(6)||Compare(5){t.Fatal("Compare boundary")}
 if c.Mode=="coverage-fail"&&testing.CoverMode()!=""{t.Fatal("T81 coverage fails after report")}
}
`

// TestMain records every real invocation without reaching production code.
const countedUnreachedTests = `package PACKAGE
import("crypto/sha256";"encoding/hex";"encoding/json";"fmt";"os";"path/filepath";"testing")
func TestMain(m *testing.M){
 cwd,err:=os.Getwd();if err!=nil{panic(err)};cwd,err=filepath.EvalSymlinks(cwd);if err!=nil{panic(err)}
 original,err:=filepath.EvalSymlinks(ORIGINAL_ROOT);if err!=nil{panic(err)}
 b,err:=os.ReadFile(SOURCE_BASE);if err!=nil{panic(err)};h:=sha256.Sum256(b)
 f,err:=os.OpenFile(TRACE_PATH,os.O_WRONLY|os.O_CREATE|os.O_APPEND,0600);if err!=nil{panic(err)}
 if err=json.NewEncoder(f).Encode(map[string]any{"cwd":cwd,"original_root":original,"source":SOURCE_PATH,"source_sha256":hex.EncodeToString(h[:]),"coverage":testing.CoverMode()!=""});err!=nil{panic(err)}
 if err=f.Close();err!=nil{panic(err)};fmt.Fprint(os.Stderr,"");os.Exit(m.Run())
}
`
