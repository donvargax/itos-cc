package lang

import (
	"slices"
	"testing"
)

func endpoints(t *testing.T, path, src string) (serves, calls []string) {
	t.Helper()
	s, c := parse(t, path, src).Endpoints()
	for _, e := range s {
		serves = append(serves, e.String())
	}
	for _, e := range c {
		calls = append(calls, e.String())
	}
	return serves, calls
}

func assertEndpoints(t *testing.T, path, src string, wantServes, wantCalls []string) {
	t.Helper()
	serves, calls := endpoints(t, path, src)
	if !slices.Equal(serves, wantServes) {
		t.Errorf("%s serves:\n got  %q\n want %q", path, serves, wantServes)
	}
	if !slices.Equal(calls, wantCalls) {
		t.Errorf("%s calls:\n got  %q\n want %q", path, calls, wantCalls)
	}
}

func TestGoEndpoints(t *testing.T) {
	assertEndpoints(t, "testdata/x.go", `package x

func routes(mux *http.ServeMux, r chi.Router, e *echo.Echo) {
	mux.HandleFunc("GET /api/graph", graph)
	mux.Handle("/static/", files)
	mux.HandleFunc("GET /{$}", home)
	r.Get("/users/{id}", user)
	e.POST("/users/:id/tags", tag)
	cache.Get("not-a-route")
}

func client(id string) {
	http.Get("http://localhost:7070/api/graph?x=1")
	http.NewRequest("DELETE", "https://svc/users/"+id, nil)
	http.NewRequestWithContext(ctx, "PUT", "/users/x", nil)
}
`, []string{"GET /api/graph", "/static", "GET /", "GET /users/*", "POST /users/*/tags"},
		[]string{"GET /api/graph", "DELETE /users/*", "PUT /users/x"})
}

func TestTypeScriptEndpoints(t *testing.T) {
	assertEndpoints(t, "testdata/ts/src/x.ts", "const API = '';\n"+`
app.get("/users/:id", (req, res) => res.json({}));
router.post("/users", createUser);
axios.get("/api/users", { params: {} });
fetch("/api/graph");
fetch(`+"`${API}/api/source?repo=${repo}`"+`);
const events = new EventSource("/api/events");
api.delete(`+"`/users/${id}`"+`);
fetch(`+"`${base}/${path}`"+`);
cache.get("key", fallback);
`, []string{"GET /users/*", "POST /users"},
		[]string{"GET /api/users", "/api/graph", "/api/source", "GET /api/events", "DELETE /users/*"})
}

func TestPythonEndpoints(t *testing.T) {
	assertEndpoints(t, "testdata/py/x.py", `
@app.route("/health")
def health():
    return "ok"

@router.get("/users/{user_id}")
async def user(user_id: int):
    return requests.get(f"http://billing/invoices/{user_id}").json()

@bp.post("/users/<int:id>/tags")
def tag(id):
    httpx.post("/audit", json={})
    cache.get("key")
`, []string{"/health", "GET /users/*", "POST /users/*/tags"},
		[]string{"GET /invoices/*", "POST /audit"})
}

func TestKotlinEndpoints(t *testing.T) {
	assertEndpoints(t, "testdata/x.kt", `
@RestController
@RequestMapping("/api/users")
class UserController(private val rest: RestTemplate) {
    @GetMapping("/{id}")
    fun get(@PathVariable id: Long) = rest.getForObject("http://billing/invoices/$id", String::class.java)

    @PostMapping
    fun create() {}
}

fun Application.routes() {
    routing {
        route("/v1") {
            get("/health") { call.respond("ok") }
        }
    }
}
`, []string{"GET /api/users/*", "POST /api/users", "GET /v1/health"},
		[]string{"GET /invoices/*"})
}

func TestMatches(t *testing.T) {
	cases := []struct {
		serve, call Endpoint
		want        bool
	}{
		{Endpoint{"GET", "/users/*", 0}, Endpoint{"", "/users/42", 0}, true},
		{Endpoint{"GET", "/users/*", 0}, Endpoint{"POST", "/users/42", 0}, false},
		{Endpoint{"", "/users", 0}, Endpoint{"DELETE", "/users", 0}, true},
		{Endpoint{"GET", "/users/*/tags", 0}, Endpoint{"GET", "/users/*", 0}, false},
		// A parameter can be anything: /api/${x} may reach /api/source.
		{Endpoint{"GET", "/api/source", 0}, Endpoint{"", "/api/*", 0}, true},
	}
	for _, c := range cases {
		if got := Matches(c.serve, c.call); got != c.want {
			t.Errorf("Matches(%v, %v) = %v, want %v", c.serve, c.call, got, c.want)
		}
	}
}
