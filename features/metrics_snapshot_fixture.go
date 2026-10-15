package features

// This is an actual Go test, including its collector barrier. The peer stop
// protocol is solely an error-cleanup escape hatch; a separate owned marker
// releases a successful measurement. No expected process events are scripted.
const snapshotConcurrentTests = `package board

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
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func TestValue(t *testing.T) {
 full:=os.Getenv("T82_SIDE")=="mutation"
 if Value(1)!=1 { t.Fatal("positive") }
 if full&&Value(0)!=0 { t.Fatal("zero") }
 cwd,err:=os.Getwd();if err!=nil{t.Fatal(err)}
 cwd,err=filepath.EvalSymlinks(cwd);if err!=nil{t.Fatal(err)}
 original,err:=filepath.EvalSymlinks(ORIGINAL_ROOT);if err!=nil{t.Fatal(err)}
 source,err:=os.ReadFile("board.go");if err!=nil{t.Fatal(err)};hash:=sha256.Sum256(source)
 trace:=os.Getenv("T82_TRACE")
 if trace!="" {
  f,err:=os.OpenFile(trace,os.O_CREATE|os.O_WRONLY|os.O_APPEND,0600);if err!=nil{t.Fatal(err)}
  err=json.NewEncoder(f).Encode(map[string]any{"cwd":cwd,"original_root":original,"source_sha256":hex.EncodeToString(hash[:]),"coverage":testing.CoverMode()!="","side":os.Getenv("T82_SIDE")})
  if err!=nil{t.Fatal(err)};if err=f.Close();err!=nil{t.Fatal(err)}
 }
 if os.Getenv("T82_HOLD")!="yes"||filepath.Base(cwd)!="other"||testing.CoverMode()==""{return}
 conn,err:=net.DialTimeout("tcp",os.Getenv("T80_OWNED_ADDR"),time.Second);if err!=nil{t.Fatal(err)}
 var random [24]byte;if _,err=rand.Read(random[:]);err!=nil{t.Fatal(err)};nonce:=hex.EncodeToString(random[:])
 if err=json.NewEncoder(conn).Encode(map[string]any{"owner":os.Getenv("T80_OWNED_OWNER"),"nonce":nonce,"pid":os.Getpid()});err!=nil{t.Fatal(err)}
 reader:=bufio.NewReader(conn);if ack,err:=reader.ReadString('\n');err!=nil||ack!="owned\n"{t.Fatalf("capture acknowledgment %q: %v",ack,err)}
 coverDir,err:=filepath.EvalSymlinks(os.Getenv("GOCOVERDIR"));if err!=nil{t.Fatal(err)}
 record,err:=json.Marshal(map[string]any{"nonce":nonce,"pid":os.Getpid(),"cwd":cwd,"original_root":original,"source_sha256":hex.EncodeToString(hash[:]),"side":os.Getenv("T82_SIDE"),"cover_dir":coverDir});if err!=nil{t.Fatal(err)}
 ready:=os.Getenv("T82_READY");temp:=ready+"."+nonce
 if err=os.WriteFile(temp,record,0600);err!=nil{t.Fatal(err)};if err=os.Rename(temp,ready);err!=nil{t.Fatal(err)}
 go func(){for{
  line,err:=reader.ReadString('\n');if err!=nil{fmt.Fprintln(os.Stderr,"fixture connection",err);os.Exit(91)}
  switch strings.TrimSpace(line){
  case "ping "+nonce:if _,err=io.WriteString(conn,"live "+nonce+"\n");err!=nil{os.Exit(92)}
  case "stop "+nonce:os.Exit(0)
  default:os.Exit(93)
  }
 }}()
 ticker:=time.NewTicker(5*time.Millisecond);defer ticker.Stop()
 for{_,err=os.Stat(os.Getenv("T82_RELEASE"));if err==nil{return};if !os.IsNotExist(err){t.Fatal(err)};<-ticker.C}
}
`
