package features

// These are real Go tests. Weak checks only a value far from the boundary;
// Compare and Other check the boundary too. The external trace is an observation
// of the source actually opened by each command, not a scripted execution log.
const executionThreeSource = `package main

func Weak(n int) bool { return n > 5 }

func Compare(n int) bool { return n > 6 }

func Other(n int) bool { return n > 7 }
`

const executionTests = `package main

import (
 "crypto/sha256"
 "encoding/json"
 "os"
 "path/filepath"
 "testing"
)

func TestTrial(t *testing.T) {
 source,err:=os.ReadFile("main.go");if err!=nil{t.Fatal(err)}
 root,err:=os.Getwd();if err!=nil{t.Fatal(err)}
 root,err=filepath.EvalSymlinks(root);if err!=nil{t.Fatal(err)}
 original,err:=filepath.EvalSymlinks(os.Getenv("T80_ORIGINAL"));if err!=nil{t.Fatal(err)}
 record:=map[string]string{"cwd":root,"original_root":original,"source_sha256":fmtHash(source)}
 f,err:=os.OpenFile(os.Getenv("T80_TRACE"),os.O_WRONLY|os.O_CREATE|os.O_APPEND,0600);if err!=nil{t.Fatal(err)}
 err=json.NewEncoder(f).Encode(record);closeErr:=f.Close();if err!=nil{t.Fatal(err)};if closeErr!=nil{t.Fatal(closeErr)}
 if !Weak(50){t.Fatal("Weak far from boundary")}
 if !Compare(7)||Compare(6){t.Fatal("Compare boundary")}
 if !Other(8)||Other(7){t.Fatal("Other boundary")}
}
func fmtHash(source []byte)string {const hex="0123456789abcdef";h:=sha256.Sum256(source);out:=make([]byte,64);for i,v:=range h{out[2*i]=hex[v>>4];out[2*i+1]=hex[v&15]};return string(out)}
`
