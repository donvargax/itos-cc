package lang

import (
	"regexp"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Endpoint is an HTTP route a module serves or a request it makes, with the
// path normalized so the two can be matched: parameters become *, and a
// call's scheme, host, and query are dropped.
type Endpoint struct {
	Method string // GET, POST, …, or "" when the source does not say
	Path   string
	Line   int
}

func (e Endpoint) String() string {
	if e.Method == "" {
		return e.Path
	}
	return e.Method + " " + e.Path
}

// Endpoints finds the routes a file serves and the HTTP requests it makes,
// where both name their path with a literal (template parameters allowed).
func (f *File) Endpoints() (serves, calls []Endpoint) {
	h := httpScan{f: f}
	if f.Spec.Name == "kotlin" {
		h.kotlin(f.Root, "", "")
		return h.serves, h.calls
	}
	Walk(f.Root, func(n *sitter.Node) bool {
		h.node(n)
		return true
	})
	return h.serves, h.calls
}

type httpScan struct {
	f             *File
	serves, calls []Endpoint
}

var verbs = map[string]string{
	"get": "GET", "post": "POST", "put": "PUT", "delete": "DELETE", "patch": "PATCH",
	"head": "HEAD", "options": "OPTIONS",
}

// verb maps a method name such as get, Get, or GET to its HTTP method.
func verb(name string) (string, bool) {
	m, ok := verbs[strings.ToLower(name)]
	return m, ok
}

func (h *httpScan) serve(method, raw string, n *sitter.Node) {
	if p, ok := normalizePath(raw); ok {
		h.serves = append(h.serves, Endpoint{method, p, line(n)})
	}
}

func (h *httpScan) call(method, raw string, n *sitter.Node) {
	if p, ok := normalizePath(raw); ok && hasLiteralSegment(p) {
		h.calls = append(h.calls, Endpoint{method, p, line(n)})
	}
}

func (h *httpScan) node(n *sitter.Node) {
	switch h.f.Spec.Name {
	case "go":
		h.golang(n)
	case "typescript":
		h.typescript(n)
	case "python":
		h.python(n)
	}
}

// --- Go: net/http, chi, gin, echo, gorilla ---

func (h *httpScan) golang(n *sitter.Node) {
	object, name, ok := h.f.Spec.Syntax.Callee(n, h.f.Src)
	if !ok {
		return
	}
	args := h.args(n)
	switch {
	case object == "http" && (name == "Get" || name == "Post" || name == "Head" || name == "PostForm"):
		if s, ok := h.str(args, 0); ok {
			h.call(strings.ToUpper(strings.TrimSuffix(name, "Form")), s, n)
		}
	case object == "http" && (name == "NewRequest" || name == "NewRequestWithContext"):
		at := 0
		if name == "NewRequestWithContext" {
			at = 1
		}
		method, _ := h.str(args, at)
		if s, ok := h.str(args, at+1); ok {
			h.call(strings.ToUpper(method), s, n)
		}
	case name == "HandleFunc" || name == "Handle" || name == "Any":
		if s, ok := h.str(args, 0); ok && len(args) >= 2 {
			method, path := splitMethod(s)
			h.serve(method, path, n)
		}
	default:
		if m, isVerb := verb(name); isVerb && object != "http" && len(args) >= 2 {
			if s, ok := h.str(args, 0); ok && strings.HasPrefix(s, "/") {
				h.serve(m, s, n)
			}
		}
	}
}

// splitMethod reads Go 1.22's "GET /path" patterns.
func splitMethod(pattern string) (method, path string) {
	if m, p, ok := strings.Cut(pattern, " "); ok {
		if _, isVerb := verb(m); isVerb {
			return strings.ToUpper(m), strings.TrimSpace(p)
		}
	}
	return "", pattern
}

// --- TypeScript: fetch, EventSource, WebSocket, axios; Express, Fastify, Hono ---

var tsClients = map[string]bool{"axios": true, "ky": true, "http": true, "https": true, "api": true,
	"client": true, "request": true, "got": true, "superagent": true, "$http": true, "httpClient": true}

func (h *httpScan) typescript(n *sitter.Node) {
	if n.Kind() == "new_expression" {
		ctor := fieldText(n, "constructor", h.f.Src)
		if ctor == "EventSource" || ctor == "WebSocket" {
			if s, ok := h.str(h.args(n), 0); ok {
				h.call("GET", s, n)
			}
		}
		return
	}
	object, name, ok := h.f.Spec.Syntax.Callee(n, h.f.Src)
	if !ok {
		return
	}
	args := h.args(n)
	switch {
	case name == "fetch" && (object == "" || object == "window" || object == "globalThis"):
		if s, ok := h.str(args, 0); ok {
			h.call("", s, n)
		}
	case tsClients[lastSegment(object)]:
		if m, isVerb := verb(name); isVerb {
			if s, ok := h.str(args, 0); ok {
				h.call(m, s, n)
			}
		}
	case object != "":
		// app.get("/x", handler): a route needs a handler after its path.
		m, isVerb := verb(name)
		if name == "all" {
			m, isVerb = "", true
		}
		if isVerb && len(args) >= 2 && isHandler(args[len(args)-1]) {
			if s, ok := h.str(args, 0); ok && strings.HasPrefix(s, "/") {
				h.serve(m, s, n)
			}
		}
	}
}

func isHandler(n *sitter.Node) bool {
	return kindIn(n, "arrow_function", "function_expression", "identifier", "member_expression", "call_expression")
}

// --- Python: Flask, FastAPI decorators; requests, httpx ---

var pyClients = map[string]bool{"requests": true, "httpx": true, "session": true, "client": true, "http": true}

func (h *httpScan) python(n *sitter.Node) {
	if n.Kind() == "decorator" {
		for i := uint(0); i < n.NamedChildCount(); i++ {
			call := n.NamedChild(i)
			_, name, ok := h.f.Spec.Syntax.Callee(call, h.f.Src)
			if !ok {
				continue
			}
			m, isVerb := verb(name)
			if name == "route" || name == "api_route" || name == "websocket" {
				m, isVerb = "", true
			}
			if s, ok := h.str(h.args(call), 0); ok && isVerb && strings.HasPrefix(s, "/") {
				h.serve(m, s, call)
			}
		}
		return
	}
	object, name, ok := h.f.Spec.Syntax.Callee(n, h.f.Src)
	if !ok {
		return
	}
	if m, isVerb := verb(name); isVerb && pyClients[lastSegment(object)] {
		if s, ok := h.str(h.args(n), 0); ok {
			h.call(m, s, n)
		}
	}
}

// --- Kotlin: Spring annotations, Ktor routing; RestTemplate, WebClient, Ktor client ---

var springMappings = map[string]string{"GetMapping": "GET", "PostMapping": "POST", "PutMapping": "PUT",
	"DeleteMapping": "DELETE", "PatchMapping": "PATCH", "RequestMapping": ""}

var ktClientCalls = map[string]string{"getForObject": "GET", "getForEntity": "GET", "postForObject": "POST",
	"postForEntity": "POST", "put": "PUT", "uri": "", "url": ""}

// kotlin walks down the tree carrying the path prefixes in effect: a
// class's @RequestMapping and the enclosing Ktor route("/…") blocks.
func (h *httpScan) kotlin(n *sitter.Node, spring, ktor string) {
	switch n.Kind() {
	case "class_declaration":
		spring = ""
		for _, a := range annotations(n) {
			if annotationName(a, h.f.Src) == "RequestMapping" {
				spring = strings.TrimSuffix(h.firstString(a), "/")
			}
		}
	case "function_declaration":
		for _, a := range annotations(n) {
			if m, isMapping := springMappings[annotationName(a, h.f.Src)]; isMapping {
				path := spring + h.firstString(a)
				if path == "" {
					path = "/"
				}
				h.serve(m, path, n)
			}
		}
	case "call_expression":
		ktor = h.ktorCall(n, ktor)
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		h.kotlin(n.NamedChild(i), spring, ktor)
	}
}

// ktorCall records a Ktor route or an HTTP client call, and returns the
// route prefix for the call's lambda.
func (h *httpScan) ktorCall(n *sitter.Node, prefix string) string {
	object, name, ok := h.f.Spec.Syntax.Callee(n, h.f.Src)
	if !ok {
		return prefix
	}
	s, hasPath := h.str(h.args(n), 0)
	if !hasPath {
		return prefix
	}
	switch {
	case hasTrailingLambda(n) && object == "" && name == "route":
		return prefix + strings.TrimSuffix(s, "/")
	case hasTrailingLambda(n) && object == "":
		if m, isVerb := verb(name); isVerb {
			h.serve(m, prefix+s, n)
		}
	case object != "":
		if m, isClient := ktClientCalls[name]; isClient && (strings.HasPrefix(s, "/") || strings.Contains(s, "://")) {
			h.call(m, s, n)
		} else if m, isVerb := verb(name); isVerb && (lastSegment(object) == "client" || lastSegment(object) == "httpClient") {
			h.call(m, s, n)
		}
	}
	return prefix
}

func annotations(n *sitter.Node) []*sitter.Node {
	var out []*sitter.Node
	for i := uint(0); i < n.NamedChildCount(); i++ {
		if c := n.NamedChild(i); c.Kind() == "modifiers" {
			for j := uint(0); j < c.NamedChildCount(); j++ {
				if a := c.NamedChild(j); a.Kind() == "annotation" {
					out = append(out, a)
				}
			}
		}
	}
	return out
}

func annotationName(a *sitter.Node, src []byte) string {
	name := strings.TrimPrefix(a.Utf8Text(src), "@")
	if i := strings.IndexAny(name, "( \n"); i >= 0 {
		name = name[:i]
	}
	return name[strings.LastIndex(name, ".")+1:]
}

func hasTrailingLambda(call *sitter.Node) bool {
	suffix := firstChildOfKind(call, "call_suffix")
	return suffix != nil && firstChildOfKind(suffix, "annotated_lambda") != nil
}

func (h *httpScan) firstString(n *sitter.Node) string {
	found := ""
	Walk(n, func(c *sitter.Node) bool {
		if found != "" {
			return false
		}
		if s, ok := h.stringValue(c); ok {
			found = s
			return false
		}
		return true
	})
	return found
}

// --- arguments and string literals ---

// args are a call's positional arguments, in each grammar's shape. A Kotlin
// call with a trailing lambda nests: get("/x") { } is call(call(get, args), lambda).
func (h *httpScan) args(call *sitter.Node) []*sitter.Node {
	var list *sitter.Node
	switch h.f.Spec.Name {
	case "kotlin":
		suffix := firstChildOfKind(call, "call_suffix")
		if suffix == nil {
			return nil
		}
		list = firstChildOfKind(suffix, "value_arguments")
		if list == nil {
			return nil
		}
		var out []*sitter.Node
		for i := uint(0); i < list.NamedChildCount(); i++ {
			if a := list.NamedChild(i); a.NamedChildCount() > 0 {
				out = append(out, a.NamedChild(a.NamedChildCount()-1))
			}
		}
		return out
	default:
		list = call.ChildByFieldName("arguments")
	}
	if list == nil {
		return nil
	}
	var out []*sitter.Node
	for i := uint(0); i < list.NamedChildCount(); i++ {
		if a := list.NamedChild(i); a.Kind() != "keyword_argument" && a.Kind() != "comment" {
			out = append(out, a)
		}
	}
	return out
}

func (h *httpScan) str(args []*sitter.Node, i int) (string, bool) {
	if i >= len(args) {
		return "", false
	}
	return h.stringValue(args[i])
}

var stringKinds = map[string]bool{
	"string": true, "template_string": true, // TypeScript, Python
	"interpreted_string_literal": true, "raw_string_literal": true, // Go
	"string_literal": true, // Kotlin
}

// templateParts are interpolations in each language's string templates.
var templateParts = regexp.MustCompile(`\$\{[^}]*\}|\$[A-Za-z_][A-Za-z0-9_]*`)

// stringValue is a string literal's text, with template interpolations
// replaced by a {} parameter, or false for anything that is not a literal.
func (h *httpScan) stringValue(n *sitter.Node) (string, bool) {
	if kindIn(n, "binary_expression", "binary_operator") && fieldText(n, "operator", h.f.Src) == "+" {
		if left := n.ChildByFieldName("left"); left != nil {
			if s, ok := h.stringValue(left); ok {
				right, isString := h.stringValue(n.ChildByFieldName("right"))
				if !isString {
					right = "{}"
				}
				return s + right, true
			}
		}
		return "", false
	}
	if !stringKinds[n.Kind()] {
		return "", false
	}
	s := n.Utf8Text(h.f.Src)
	if h.f.Spec.Name == "python" {
		s = strings.TrimLeft(s, "rRbBfFuU")
	}
	for _, q := range []string{`"""`, `'''`, `"`, "'", "`"} {
		if len(s) >= 2*len(q) && strings.HasPrefix(s, q) && strings.HasSuffix(s, q) {
			s = s[len(q) : len(s)-len(q)]
			break
		}
	}
	return templateParts.ReplaceAllString(s, "{}"), true
}

var (
	schemeHost = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*://[^/]*`)
	paramParts = regexp.MustCompile(`^(\{.*\}|:.+|<.+>|\*.*)$`)
)

// normalizePath turns a route or URL into slash-separated segments with
// every parameter as *. A URL loses its scheme and host, a leading template
// base such as ${API} is dropped, and the query is cut. A string that does
// not look like a path is not one.
func normalizePath(raw string) (string, bool) {
	s := schemeHost.ReplaceAllString(strings.TrimSpace(raw), "")
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimPrefix(s, "{}")
	if !strings.HasPrefix(s, "/") {
		return "", false
	}
	s = strings.TrimSuffix(strings.ReplaceAll(s, "{$}", ""), "/")
	segments := strings.Split(strings.TrimPrefix(s, "/"), "/")
	for i, seg := range segments {
		if paramParts.MatchString(seg) || strings.Contains(seg, "{}") {
			segments[i] = "*"
		}
	}
	return "/" + strings.Join(segments, "/"), true
}

// hasLiteralSegment rejects calls such as `${base}/${path}`, which would
// match every route.
func hasLiteralSegment(path string) bool {
	for _, seg := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if seg != "*" && seg != "" {
			return true
		}
	}
	return false
}

// Matches reports whether a request to call reaches the route serve: the
// same number of segments, each equal or a parameter on either side, and
// the same method when both say.
func Matches(serve, call Endpoint) bool {
	if serve.Method != "" && call.Method != "" && serve.Method != call.Method {
		return false
	}
	a := strings.Split(serve.Path, "/")
	b := strings.Split(call.Path, "/")
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] && a[i] != "*" && b[i] != "*" {
			return false
		}
	}
	return true
}

// lastSegment keeps the final name of a dotted receiver: client for
// this.client, requests for requests.
func lastSegment(s string) string {
	if i := strings.LastIndexAny(s, ".)"); i >= 0 && i < len(s)-1 {
		return s[i+1:]
	}
	return s
}
