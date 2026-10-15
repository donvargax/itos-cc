package features

const executionPartialA = `package main

func A1(n int) bool { return n > 2 }

func Done(n int) bool { return n > 4 }

func A2(n int) bool { return n > 3 }
`
const executionPartialB = `package main

func B(n int) bool { return n > 9 }

func Later(n int) bool { return n > 8 }
`
const executionPartialTests = `package main
import(
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "os"
 "path/filepath"
 "testing"
 "time"
)
const records=RECORD_PATH
func exists(t *testing.T,name string)bool{t.Helper();_,err:=os.Stat(filepath.Join(records,name));if err==nil{return true};if os.IsNotExist(err){return false};t.Fatal(err);return false}
func await(t *testing.T,name string){t.Helper();ticker:=time.NewTicker(5*time.Millisecond);defer ticker.Stop();for !exists(t,name){<-ticker.C}}
func TestTrial(t *testing.T){
 root,err:=os.Getwd();if err!=nil{t.Fatal(err)};root,err=filepath.EvalSymlinks(root);if err!=nil{t.Fatal(err)}
 original,err:=filepath.EvalSymlinks(os.Getenv("T80_ORIGINAL"));if err!=nil{t.Fatal(err)}
 source,err:=os.ReadFile("a.go");if err!=nil{t.Fatal(err)};hash:=sha256.Sum256(source)
 f,err:=os.OpenFile(os.Getenv("T80_TRACE"),os.O_WRONLY|os.O_CREATE|os.O_APPEND,0600);if err!=nil{t.Fatal(err)}
 err=json.NewEncoder(f).Encode(map[string]string{"cwd":root,"original_root":original,"source_sha256":hex.EncodeToString(hash[:])});closed:=f.Close();if err!=nil{t.Fatal(err)};if closed!=nil{t.Fatal(closed)}
 if A1(2){t.Fatal("A1 boundary")}
 if Done(4){t.Fatal("Done boundary")}
 if A2(3){
  if exists(t,"hold"){
   temp:=filepath.Join(records,"started.tmp")
   if err=os.WriteFile(temp,[]byte("A2 running\n"),0600);err!=nil{t.Fatal(err)}
   if err=os.Rename(temp,filepath.Join(records,"started"));err!=nil{t.Fatal(err)}
   await(t,"release")
  }
  t.Fatal("A2 boundary")
 }
 if B(9){
  if exists(t,"simple"){return}
  if exists(t,"hold"){await(t,"started");return}
  t.Fatal("B boundary")
 }
 if Later(8){t.Fatal("Later boundary")}
}
`
