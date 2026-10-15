package features

// The connection is held by each actual Go test/helper process until it exits.
// A stop message is only a cleanup escape hatch; assertions never send it.
const ownershipTests = `package main

import (
 "bufio"
 "crypto/rand"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "io"
 "net"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"
 "time"
)
type fixtureControl struct {Mode string;Block string;Closed bool;Release bool}
const ownedControl = CONTROL_PATH
func control(t *testing.T)fixtureControl {
 t.Helper();var c fixtureControl
 data,err:=os.ReadFile(ownedControl);if err!=nil{t.Fatal(err)}
 if err=json.Unmarshal(data,&c);err!=nil{t.Fatal(err)};return c
}
func released(t *testing.T)bool{
 t.Helper();_,err:=os.Stat(ownedControl+".release")
 if err==nil{return true};if os.IsNotExist(err){return false};t.Fatal(err);return false
}
func trial(t *testing.T)(string,string){
 t.Helper();name:="baseline"
 if Compare(5){name="Compare"}
 GATE_TRIAL
 LATER_TRIAL
 data,err:=os.ReadFile("main.go");if err!=nil{t.Fatal(err)}
 h:=sha256.Sum256(data)
 f,err:=os.OpenFile(os.Getenv("T80_TRACE"),os.O_WRONLY|os.O_APPEND|os.O_CREATE,0600);if err!=nil{t.Fatal(err)}
 if err=json.NewEncoder(f).Encode(map[string]string{"function":name,"source_sha256":hex.EncodeToString(h[:])});err!=nil{t.Fatal(err)}
 if err=f.Close();err!=nil{t.Fatal(err)}
 return name,hex.EncodeToString(h[:])
}
func TestTrial(t *testing.T){
 name,_:=trial(t);c:=control(t)
 if name=="baseline"&&c.Mode!="control"{return}
 if c.Mode=="characterize"{t.Fatal("boundary changed",name)}
 if c.Mode=="failfast"&&name=="Later"{t.Fatal("Later boundary")}
 if c.Mode=="failfast"&&name=="Gate"{
  connected(t,"command",name)
  ticker:=time.NewTicker(5*time.Millisecond);defer ticker.Stop()
  for !released(t){<-ticker.C};return
 }
 if c.Mode=="cancel"&&name!=c.Block{t.Fatal("first boundary changed",name)}
 connected(t,"command",name)
 startOwned(t,"child",name,c.Closed)
 if c.Mode=="success"{
  ticker:=time.NewTicker(5*time.Millisecond);defer ticker.Stop()
  for !released(t){<-ticker.C}
  return
 }
 select{}
}
func startOwned(t *testing.T,role,name string,closed bool){
 t.Helper();cmd:=exec.Command(os.Args[0],"-test.run=^TestOwned$")
 cmd.Env=append(os.Environ(),"T80_HELPER_ROLE="+role,"T80_HELPER_FUNCTION="+name)
 if !closed{cmd.Stdout=os.Stdout;cmd.Stderr=os.Stderr}
 if err:=cmd.Start();err!=nil{t.Fatal(err)}
 // These children intentionally outlive a successful test. The public product
 // scope owns cleanup and joins their processes/output pipes, not the fixture.
}
func TestOwned(t *testing.T){
 role:=os.Getenv("T80_HELPER_ROLE");if role==""{return}
 name:=os.Getenv("T80_HELPER_FUNCTION");c:=control(t)
 if role=="child"{startOwned(t,"grandchild",name,c.Closed)}
 connected(t,role,name)
 select{}
}
func connected(t *testing.T,role,name string){
 t.Helper()
 conn,err:=net.DialTimeout("tcp",os.Getenv("T80_OWNED_ADDR"),time.Second);if err!=nil{t.Fatal(err)}
 var nonceBytes [24]byte;if _,err=rand.Read(nonceBytes[:]);err!=nil{t.Fatal(err)};nonce:=hex.EncodeToString(nonceBytes[:])
 pid:=os.Getpid()
 if err=json.NewEncoder(conn).Encode(map[string]any{"owner":os.Getenv("T80_OWNED_OWNER"),"nonce":nonce,"pid":pid});err!=nil{t.Fatal(err)}
 reader:=bufio.NewReader(conn)
 if ack,err:=reader.ReadString('\n');err!=nil||ack!="owned\n"{t.Fatalf("capture acknowledgment %q: %v",ack,err)}
 cwd,err:=os.Getwd();if err!=nil{t.Fatal(err)};cwd,err=filepath.EvalSymlinks(cwd);if err!=nil{t.Fatal(err)}
 source,err:=os.ReadFile("main.go");if err!=nil{t.Fatal(err)};sha:=sha256.Sum256(source)
 original,err:=filepath.EvalSymlinks(os.Getenv("T80_ORIGINAL"));if err!=nil{t.Fatal(err)}
 record,err:=json.Marshal(map[string]any{"original_root":original,"nonce":nonce,"pid":pid,"parent_pid":os.Getppid(),"role":role,"function":name,"cwd":cwd,"source_sha256":hex.EncodeToString(sha[:]),"closed_streams":control(t).Closed});if err!=nil{t.Fatal(err)}
 final:=filepath.Join(filepath.Dir(ownedControl),name+"-"+role+".json")
 temp:=final+"."+nonce
 if err=os.WriteFile(temp,record,0600);err!=nil{t.Fatal(err)}
 if err=os.Rename(temp,final);err!=nil{t.Fatal(err)}
 go func(){
  for {
   line,err:=reader.ReadString('\n');if err!=nil{fmt.Fprintln(os.Stderr,"owned connection:",err);os.Exit(91)}
   switch strings.TrimSpace(line){
   case "ping "+nonce:if _,err=io.WriteString(conn,"live "+nonce+"\n");err!=nil{os.Exit(92)}
   case "stop "+nonce:os.Exit(0)
   default:os.Exit(93)
   }
  }
 }()
}
`
