package hideas

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func runCLI(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, err bytes.Buffer
	code := Run(args, &out, &err)
	return out.String(), err.String(), code
}

// newCLIServer starts an in-memory hideas HTTP server backed by a fresh
// SQLite store. The static token "secret" is used to authorize CLI requests.
func newCLIServer(t *testing.T) (*httptest.Server, *SQLiteStore) {
	t.Helper()
	store := newTestStore(t)
	auth, err := newServerAuth(ServerAuthConfig{StaticToken: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewHTTPHandler(store, auth, "/"))
	t.Cleanup(server.Close)
	return server, store
}

func mustOK(t *testing.T, server string, args ...string) string {
	t.Helper()
	full := append([]string{"--server", server, "--token", "secret"}, args...)
	out, errOut, code := runCLI(t, full...)
	if code != 0 {
		t.Fatalf("command failed: %v\nstdout=%s\nstderr=%s", full, out, errOut)
	}
	return out
}

func firstID(t *testing.T, out, prefix string) int64 {
	t.Helper()
	re := regexp.MustCompile(prefix + ` ([0-9]+)`)
	m := re.FindStringSubmatch(out)
	if len(m) != 2 {
		t.Fatalf("missing %s id in output: %s", prefix, out)
	}
	var id int64
	if _, err := fmtSscan(m[1], &id); err != nil {
		t.Fatal(err)
	}
	return id
}

func fmtSscan(s string, a ...interface{}) (int, error) {
	return fmt.Sscan(s, a...)
}

func TestCLIAllCommands(t *testing.T) {
	srv, _ := newCLIServer(t)

	e1 := firstID(t, mustOK(t, srv.URL, "entity", "add", "李雷", "--type", "person"), "entity")
	e2 := firstID(t, mustOK(t, srv.URL, "entity", "add", "李雷", "--type", "person"), "entity")
	if e1 == e2 {
		t.Fatal("duplicate entity names should still create distinct IDs")
	}

	if out := mustOK(t, srv.URL, "entity", "list", "--type", "person"); strings.Count(out, "李雷") != 2 {
		t.Fatalf("entity list should include duplicate names: %s", out)
	}

	traceOut := mustOK(t, srv.URL, "add", "今天和李雷讨论 SQLite 记忆库", "--type", "thought", "--at", "2026-06-05", "--entity-id", strconvFormat(e1))
	traceID := firstID(t, traceOut, "trace")
	if out := mustOK(t, srv.URL, "trace", "update", strconvFormat(traceID), "--happened-at", "2026-04-19"); !strings.Contains(out, "trace "+strconvFormat(traceID)+" updated") {
		t.Fatalf("trace update output: %s", out)
	}
	if out := mustOK(t, srv.URL, "search", "SQLite", "--since", "2026-06-01", "--format", "json"); strings.Contains(out, strconvFormat(traceID)) {
		t.Fatalf("updated trace happened_at should affect date filtering: %s", out)
	}
	if out := mustOK(t, srv.URL, "trace", "update", strconvFormat(traceID), "--happened-at", "2026-06-05"); !strings.Contains(out, "trace "+strconvFormat(traceID)+" updated") {
		t.Fatalf("trace update restore output: %s", out)
	}

	out, errOut, code := runCLI(t, "--server", srv.URL, "--token", "secret", "add", "这条会歧义", "--entity", "李雷")
	if code == 0 || !strings.Contains(errOut, "ambiguous entity name") {
		t.Fatalf("expected ambiguous entity failure, code=%d stdout=%s stderr=%s", code, out, errOut)
	}

	if out := mustOK(t, srv.URL, "search", "SQLite", "--entity-id", strconvFormat(e1), "--type", "thought", "--format", "json"); !strings.Contains(out, "SQLite") {
		t.Fatalf("search json output: %s", out)
	}
	if out := mustOK(t, srv.URL, "add", "KeywordAlpha 记忆系统 命中测试", "--type", "thought"); !strings.Contains(out, "trace") {
		t.Fatalf("keyword trace add output: %s", out)
	}
	if out := mustOK(t, srv.URL, "search", "MissingPhrase 记忆系统", "--format", "json"); !strings.Contains(out, "KeywordAlpha") {
		t.Fatalf("keyword search should match eligible token: %s", out)
	}
	if out := mustOK(t, srv.URL, "search", "MissingPhrase 记忆系统", "--literal", "--format", "json"); strings.Contains(out, "KeywordAlpha") {
		t.Fatalf("literal search should not expand tokens: %s", out)
	}
	if out := mustOK(t, srv.URL, "search", "MissingPhrase SQLite", "--format", "json"); strings.Contains(out, "SQLite") {
		t.Fatalf("pure ascii token should not expand keyword search: %s", out)
	}
	if out := mustOK(t, srv.URL, "add", "TruncateTest short", "--type", "thought"); !strings.Contains(out, "trace") {
		t.Fatalf("short trace add output: %s", out)
	}
	if out := mustOK(t, srv.URL, "add", "TruncateTest "+strings.Repeat("abcdef", 20), "--type", "thought"); !strings.Contains(out, "trace") {
		t.Fatalf("long trace add output: %s", out)
	}
	today := time.Now().In(time.Local).Format("2006-01-02")
	timeFilteredOut := mustOK(t, srv.URL, "add", "DateFilterTarget", "--type", "thought")
	timeFilteredID := firstID(t, timeFilteredOut, "trace")
	untilOut := mustOK(t, srv.URL, "search", "DateFilterTarget", "--until", today, "--format", "json")
	if !strings.Contains(untilOut, strconvFormat(timeFilteredID)) {
		t.Fatalf("date-based until search should include today's trace: %s", untilOut)
	}
	recentOut := mustOK(t, srv.URL, "search", "DateFilterTarget", "--recent", "1h", "--format", "json")
	if !strings.Contains(recentOut, strconvFormat(timeFilteredID)) {
		t.Fatalf("recent window search should include today's trace: %s", recentOut)
	}
	out, errOut, code = runCLI(t, "--server", srv.URL, "--token", "secret", "search", "DateFilterTarget", "--recent", "24")
	if code == 0 || !strings.Contains(errOut, "invalid recent window") {
		t.Fatalf("expected invalid recent window failure, code=%d stdout=%s stderr=%s", code, out, errOut)
	}
	searchOut := mustOK(t, srv.URL, "search", "TruncateTest", "--limit", "1")
	longContent := "TruncateTest " + strings.Repeat("abcdef", 20)
	expectedSummary := summarizeText(longContent, 100)
	if strings.Contains(searchOut, longContent) {
		t.Fatalf("search output should not include full content: %s", searchOut)
	}
	if !strings.Contains(searchOut, expectedSummary) {
		t.Fatalf("search output should include summary: %s", searchOut)
	}
	if !strings.Contains(searchOut, "more results available") {
		t.Fatalf("search output should indicate truncation: %s", searchOut)
	}
	if out := mustOK(t, srv.URL, "show", "trace", strconvFormat(traceID)); !strings.Contains(out, "entity "+strconvFormat(e1)) {
		t.Fatalf("show trace output: %s", out)
	}

	relID := firstID(t, mustOK(t, srv.URL, "link", "entity", strconvFormat(e1), "entity", strconvFormat(e2), "--type", "related_to"), "relation")
	if out := mustOK(t, srv.URL, "show", "relation", strconvFormat(relID)); !strings.Contains(out, "related_to") {
		t.Fatalf("show relation output: %s", out)
	}
	out, errOut, code = runCLI(t, "--server", srv.URL, "--token", "secret", "delete", "entity", strconvFormat(e1))
	if code == 0 || !strings.Contains(errOut, "delete blocked") {
		t.Fatalf("expected blocked entity delete, code=%d stdout=%s stderr=%s", code, out, errOut)
	}
	if out := mustOK(t, srv.URL, "delete", "relation", strconvFormat(relID)); !strings.Contains(out, "deleted relation "+strconvFormat(relID)) {
		t.Fatalf("delete relation output: %s", out)
	}
	out, errOut, code = runCLI(t, "--server", srv.URL, "--token", "secret", "show", "relation", strconvFormat(relID))
	if code == 0 || !strings.Contains(errOut, "not found") {
		t.Fatalf("expected deleted relation not found, code=%d stdout=%s stderr=%s", code, out, errOut)
	}

	profileID := firstID(t, mustOK(t, srv.URL, "profile", "set", strconvFormat(e1), "前同事，做后端"), "profile")
	if profileID == 0 {
		t.Fatalf("profile set output: %s", out)
	}
	if out := mustOK(t, srv.URL, "profile", "show", strconvFormat(e1)); !strings.Contains(out, "前同事") {
		t.Fatalf("profile show output: %s", out)
	}
	if out := mustOK(t, srv.URL, "entity", "show", strconvFormat(e1)); !strings.Contains(out, "profile: 前同事") {
		t.Fatalf("entity show output: %s", out)
	}
	out, errOut, code = runCLI(t, "--server", srv.URL, "--token", "secret", "delete", "trace", strconvFormat(profileID))
	if code == 0 || !strings.Contains(errOut, "delete blocked") {
		t.Fatalf("expected blocked profile trace delete, code=%d stdout=%s stderr=%s", code, out, errOut)
	}
	if out := mustOK(t, srv.URL, "delete", "trace", strconvFormat(profileID), "--cascade"); !strings.Contains(out, "profiles_cleared=1") {
		t.Fatalf("delete profile trace cascade output: %s", out)
	}

	if out := mustOK(t, srv.URL, "entity", "rename", strconvFormat(e2), "李雷-设计师"); !strings.Contains(out, "renamed") {
		t.Fatalf("rename output: %s", out)
	}
	if out := mustOK(t, srv.URL, "delete", "entity", strconvFormat(e1), "--cascade"); !strings.Contains(out, "relations_deleted=") {
		t.Fatalf("delete entity cascade output: %s", out)
	}

	if out := mustOK(t, srv.URL, "type", "add", "trace", "decision"); !strings.Contains(out, "type") {
		t.Fatalf("type add output: %s", out)
	}
	if out := mustOK(t, srv.URL, "type", "list"); !strings.Contains(out, "decision") {
		t.Fatalf("type list output: %s", out)
	}

	if out := mustOK(t, srv.URL, "db", "stats"); !strings.Contains(out, "entities=") || !strings.Contains(out, "traces=") {
		t.Fatalf("db stats output: %s", out)
	}
	if out := mustOK(t, srv.URL, "db", "check"); !strings.Contains(out, "ok") {
		t.Fatalf("db check output: %s", out)
	}

	if out := mustOK(t, srv.URL, "export", "--format", "json"); !strings.Contains(out, `"entities"`) || !strings.Contains(out, `"traces"`) {
		t.Fatalf("export json output: %s", out)
	}
	if out := mustOK(t, srv.URL, "export", "--format", "markdown"); !strings.Contains(out, "# hideas export") {
		t.Fatalf("export markdown output: %s", out)
	}
}

func TestSearchTerms(t *testing.T) {
	got := searchTerms("Skill Q2 规划 记忆系统", false)
	want := []string{"Skill Q2 规划 记忆系统", "规划", "记忆系统"}
	if !equalStrings(got, want) {
		t.Fatalf("search terms = %#v, want %#v", got, want)
	}
	if got := searchTerms("Skill Q2 规划", true); !equalStrings(got, []string{"Skill Q2 规划"}) {
		t.Fatalf("literal search terms = %#v", got)
	}
	if searchTokenEligible("SQLite") || searchTokenEligible("2026") || searchTokenEligible("Q2") {
		t.Fatal("pure ascii tokens should not be eligible")
	}
	if !searchTokenEligible("记忆系统") || !searchTokenEligible("Skill平台") {
		t.Fatal("non-ascii tokens should be eligible")
	}
}

func TestServerModeHTTPAPI(t *testing.T) {
	store := newTestStore(t)
	auth, err := newServerAuth(ServerAuthConfig{StaticToken: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHTTPHandler(store, auth, "/hideas/")
	server := httptest.NewServer(handler)
	defer server.Close()

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/hideas/api/v1/health", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	version := apiDo[VersionInfo](t, server.URL, "", http.MethodGet, "/hideas/api/v1/version", nil)
	if version.Version == "" || version.BuildTime == "" {
		t.Fatalf("version response: %+v", version)
	}

	e := apiDo[Entity](t, server.URL, "secret", http.MethodPost, "/hideas/api/v1/entities", map[string]string{"Name": "Alice", "Type": "person"})
	tr := apiDo[Trace](t, server.URL, "secret", http.MethodPost, "/hideas/api/v1/traces", AddTraceInput{Content: "Alice mentioned SQLite", TypeName: "event", EntityIDs: []int64{e.ID}})
	tr2 := apiDo[Trace](t, server.URL, "secret", http.MethodPost, "/hideas/api/v1/traces", AddTraceInput{Content: "Alice mentioned SQLite again", TypeName: "event", EntityIDs: []int64{e.ID}})
	happened := mustMillis(t, "2026-04-19")
	updated := apiDo[Trace](t, server.URL, "secret", http.MethodPatch, "/hideas/api/v1/traces/"+strconvFormat(tr.ID), UpdateTraceInput{HappenedAt: &happened})
	if updated.HappenedAt == nil || *updated.HappenedAt != happened {
		t.Fatalf("trace update response: %+v", updated)
	}
	search := apiDo[SearchResult](t, server.URL, "secret", http.MethodGet, "/hideas/api/v1/search?q=SQLite&limit=1", nil)
	if len(search.Traces) != 1 || search.Traces[0].ID != tr2.ID || !search.TracesHasMore {
		t.Fatalf("search response: %+v", search)
	}
	profile := apiDo[Trace](t, server.URL, "secret", http.MethodPut, "/hideas/api/v1/profiles/"+strconvFormat(e.ID), map[string]string{"Content": "Alice profile"})
	if profile.Content != "Alice profile" {
		t.Fatalf("profile response: %+v", profile)
	}
	gotProfile := apiDo[Trace](t, server.URL, "secret", http.MethodGet, "/hideas/api/v1/profiles/"+strconvFormat(e.ID), nil)
	if gotProfile.ID != profile.ID {
		t.Fatalf("profile get response: %+v", gotProfile)
	}
	rel := apiDo[Relation](t, server.URL, "secret", http.MethodPost, "/hideas/api/v1/relations", map[string]interface{}{"FromKind": "trace", "FromID": tr.ID, "ToKind": "trace", "ToID": profile.ID, "Type": "supports"})
	if rel.TypeName != "supports" {
		t.Fatalf("relation response: %+v", rel)
	}
	_ = apiDo[ShowResult](t, server.URL, "secret", http.MethodGet, "/hideas/api/v1/relations/"+strconvFormat(rel.ID), nil)
	blockedDelete := apiDoRaw(t, server.URL, "secret", http.MethodDelete, "/hideas/api/v1/traces/"+strconvFormat(profile.ID), nil)
	if blockedDelete.OK || blockedDelete.Error == nil || blockedDelete.Error.Code != "delete_blocked" {
		t.Fatalf("expected delete_blocked, got %+v", blockedDelete)
	}
	deletedTrace := apiDo[DeleteResult](t, server.URL, "secret", http.MethodDelete, "/hideas/api/v1/traces/"+strconvFormat(profile.ID)+"?cascade=true", nil)
	if deletedTrace.ProfilesCleared != 1 || deletedTrace.RelationsDeleted == 0 {
		t.Fatalf("delete trace response: %+v", deletedTrace)
	}
	if types := apiDo[[]Type](t, server.URL, "secret", http.MethodGet, "/hideas/api/v1/types", nil); len(types) == 0 {
		t.Fatal("expected seeded types")
	}
	_ = apiDo[Type](t, server.URL, "secret", http.MethodPost, "/hideas/api/v1/types", map[string]string{"Domain": "entity", "Name": "team"})
	if stats := apiDo[Stats](t, server.URL, "secret", http.MethodGet, "/hideas/api/v1/db/stats", nil); stats.Entities == 0 || stats.Traces == 0 {
		t.Fatalf("stats response: %+v", stats)
	}
	if check := apiDo[CheckResult](t, server.URL, "secret", http.MethodGet, "/hideas/api/v1/db/check", nil); !check.OK {
		t.Fatalf("check response: %+v", check)
	}
	if exp := apiDo[map[string]string](t, server.URL, "secret", http.MethodGet, "/hideas/api/v1/export?format=json", nil); !strings.Contains(exp["content"], "Alice") {
		t.Fatalf("export response: %+v", exp)
	}
}

func TestCLIRequiresServer(t *testing.T) {
	t.Setenv("HIDEAS_SERVER", "")
	t.Setenv("HIDEAS_TOKEN", "")
	out, errOut, code := runCLI(t, "--config", filepath.Join(t.TempDir(), "missing"), "--credentials", filepath.Join(t.TempDir(), "creds.json"), "entity", "list")
	if code == 0 {
		t.Fatalf("expected failure without server, stdout=%s stderr=%s", out, errOut)
	}
	if !strings.Contains(errOut, "server is required") {
		t.Fatalf("unexpected error: %s", errOut)
	}
}

func TestCLIRequiresLogin(t *testing.T) {
	srv, _ := newCLIServer(t)
	out, errOut, code := runCLI(t, "--server", srv.URL, "--credentials", filepath.Join(t.TempDir(), "creds.json"), "entity", "list")
	if code == 0 {
		t.Fatalf("expected failure without token, stdout=%s stderr=%s", out, errOut)
	}
	if !strings.Contains(errOut, "not logged in") {
		t.Fatalf("unexpected error: %s", errOut)
	}
}

func TestCLIUsesConfigFile(t *testing.T) {
	srv, _ := newCLIServer(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	if err := os.WriteFile(configPath, []byte("server = \""+srv.URL+"\"\ntoken = \"secret\"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	out, errOut, code := runCLI(t, "--config", configPath, "entity", "add", "Config Server", "--type", "source")
	if code != 0 {
		t.Fatalf("config command failed stdout=%s stderr=%s", out, errOut)
	}
	if !strings.Contains(out, "entity") {
		t.Fatalf("config add output: %s", out)
	}
	if out := mustOK(t, srv.URL, "entity", "list"); !strings.Contains(out, "Config Server") {
		t.Fatalf("config did not target server: %s", out)
	}

	if out, errOut, code := runCLI(t, "--config", configPath, "status"); code != 0 {
		t.Fatalf("status failed stdout=%s stderr=%s", out, errOut)
	} else if !strings.Contains(out, "server: "+normalizeServerKey(srv.URL)) || !strings.Contains(out, "static token") {
		t.Fatalf("unexpected status: %s", out)
	}
}

func TestVersionCommands(t *testing.T) {
	out, errOut, code := runCLI(t, "--version")
	if code != 0 {
		t.Fatalf("--version failed stdout=%s stderr=%s", out, errOut)
	}
	if !strings.Contains(out, "Hideas ") || !strings.Contains(out, "build time:") {
		t.Fatalf("unexpected version output: %s", out)
	}

	out, errOut, code = runCLI(t)
	if code != 0 {
		t.Fatalf("no-args run failed stdout=%s stderr=%s", out, errOut)
	}
	if !strings.Contains(out, "Hideas ") || !strings.Contains(out, "build time:") || !strings.Contains(out, "Usage:") {
		t.Fatalf("unexpected no-args output: %s", out)
	}

	out, errOut, code = runCLI(t, "help")
	if code != 0 {
		t.Fatalf("help failed stdout=%s stderr=%s", out, errOut)
	}
	if !strings.Contains(out, "Hideas ") || !strings.Contains(out, "build time:") || !strings.Contains(out, "Usage:") {
		t.Fatalf("unexpected help output: %s", out)
	}

	srv, _ := newCLIServer(t)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	if err := os.WriteFile(configPath, []byte("server = \""+srv.URL+"\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	out, errOut, code = runCLI(t, "--config", configPath, "version")
	if code != 0 {
		t.Fatalf("remote version failed stdout=%s stderr=%s", out, errOut)
	}
	if !strings.Contains(out, "Hideas ") || !strings.Contains(out, "build time:") {
		t.Fatalf("unexpected remote version output: %s", out)
	}
}

func TestDefaultDBPathIsAppDataPath(t *testing.T) {
	path := defaultDBPath()
	if filepath.Base(path) != "hideas.sqlite" {
		t.Fatalf("unexpected default db filename: %s", path)
	}
	if path == "hideas.sqlite" {
		t.Fatalf("default db path should not be current-directory relative")
	}
	if !strings.Contains(path, "hideas") {
		t.Fatalf("default db path should include app directory: %s", path)
	}
}

func TestCLIHelp(t *testing.T) {
	out, errOut, code := runCLI(t, "--help")
	if code != 0 {
		t.Fatalf("root help failed stdout=%s stderr=%s", out, errOut)
	}
	if !strings.Contains(errOut, "Hideas ") || !strings.Contains(errOut, "build time:") || !strings.Contains(errOut, "Usage:") || !strings.Contains(errOut, "hideas help COMMAND") {
		t.Fatalf("unexpected root help stderr=%s", errOut)
	}

	out, errOut, code = runCLI(t, "help", "entity")
	if code != 0 {
		t.Fatalf("help entity failed stdout=%s stderr=%s", out, errOut)
	}
	if !strings.Contains(out, "Hideas ") || !strings.Contains(out, "build time:") || !strings.Contains(out, "hideas entity add|list|show|rename") {
		t.Fatalf("unexpected entity help output: %s", out)
	}

	out, errOut, code = runCLI(t, "entity", "add", "--help")
	if code != 0 {
		t.Fatalf("entity add help failed stdout=%s stderr=%s", out, errOut)
	}
	if !strings.Contains(errOut, "Hideas ") || !strings.Contains(errOut, "build time:") || !strings.Contains(errOut, "Usage: hideas entity add NAME") {
		t.Fatalf("unexpected entity add help stderr=%s", errOut)
	}

	out, errOut, code = runCLI(t, "show", "--help")
	if code != 0 {
		t.Fatalf("show help failed stdout=%s stderr=%s", out, errOut)
	}
	if !strings.Contains(out, "Hideas ") || !strings.Contains(out, "build time:") || !strings.Contains(out, "Usage: hideas show entity|trace|relation ID") {
		t.Fatalf("unexpected show help output: %s", out)
	}
}

// mockOIDCServer implements just enough of an OIDC provider for SSO login
// tests. Calling the authorize endpoint immediately redirects back to the
// hideas callback with a deterministic code, mimicking a user who consents
// without interaction.
type mockOIDCServer struct {
	server   *httptest.Server
	clientID string
	secret   string
	t        *testing.T
}

func newMockOIDC(t *testing.T, clientID, secret string) *mockOIDCServer {
	t.Helper()
	m := &mockOIDCServer{clientID: clientID, secret: secret, t: t}
	mux := http.NewServeMux()
	var baseURL string
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 baseURL,
			"authorization_endpoint": baseURL + "/authorize",
			"token_endpoint":         baseURL + "/token",
			"userinfo_endpoint":      baseURL + "/userinfo",
		})
	})
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		redirect := q.Get("redirect_uri")
		state := q.Get("state")
		u, err := url.Parse(redirect)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		v := u.Query()
		v.Set("state", state)
		v.Set("code", "test-code")
		u.RawQuery = v.Encode()
		http.Redirect(w, r, u.String(), http.StatusFound)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if r.PostForm.Get("client_id") != clientID || r.PostForm.Get("client_secret") != secret {
			http.Error(w, "bad client", http.StatusUnauthorized)
			return
		}
		if r.PostForm.Get("code") != "test-code" {
			http.Error(w, "bad code", http.StatusBadRequest)
			return
		}
		if r.PostForm.Get("code_verifier") == "" {
			http.Error(w, "missing code_verifier", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "access-token-for-alice",
			"token_type":   "Bearer",
			"expires_in":   1800,
		})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-token-for-alice" {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"sub": "alice"})
	})
	m.server = httptest.NewServer(mux)
	baseURL = m.server.URL
	t.Cleanup(m.server.Close)
	return m
}

// newSSOHideasServer returns a hideas HTTP server wired to a mock OIDC
// provider. The browser-side redirect is performed by the test directly,
// using the returned hideas server URL as both base and redirect target.
func newSSOHideasServer(t *testing.T) (*httptest.Server, *mockOIDCServer) {
	t.Helper()
	store := newTestStore(t)

	// We need the final hideas server URL to set redirect_uri before
	// constructing the handler, so use an httptest.Server placeholder.
	srv := httptest.NewUnstartedServer(nil)
	srv.Start()
	t.Cleanup(srv.Close)
	redirectURL := srv.URL + "/api/v1/auth/callback"

	mock := newMockOIDC(t, "hideas-client", "hideas-secret")
	auth, err := newServerAuth(ServerAuthConfig{
		SSO: SSOConfig{
			Issuer:       mock.server.URL,
			ClientID:     "hideas-client",
			ClientSecret: "hideas-secret",
			RedirectURL:  redirectURL,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.Config.Handler = NewHTTPHandler(store, auth, "/")
	return srv, mock
}

func TestSSOLoginWait(t *testing.T) {
	hideasSrv, _ := newSSOHideasServer(t)

	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials.json")
	configPath := filepath.Join(dir, "config")

	// Run hideas login --wait in the background; once the CLI surfaces the
	// authorize URL by writing the pending session and we walk that URL via
	// the SSO mock, the wait loop should observe ready and exit 0.
	type result struct {
		out, err string
		code     int
	}
	done := make(chan result, 1)
	var stdoutBuf, stderrBuf bytes.Buffer
	go func() {
		code := Run([]string{
			"--config", configPath,
			"--credentials", credPath,
			"login",
			"--server", hideasSrv.URL,
			"--wait",
			"--timeout", "10s",
		}, &stdoutBuf, &stderrBuf)
		done <- result{stdoutBuf.String(), stderrBuf.String(), code}
	}()

	deadline := time.Now().Add(5 * time.Second)
	var authorizeURL string
	for time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		out := stdoutBuf.String()
		idx := strings.Index(out, "http")
		if idx >= 0 {
			end := strings.IndexByte(out[idx:], '\n')
			if end > 0 {
				authorizeURL = strings.TrimSpace(out[idx : idx+end])
				break
			}
		}
	}
	if authorizeURL == "" {
		t.Fatal("CLI never printed an authorize URL")
	}
	// Walk the authorize URL to complete the SSO flow for the CLI's session.
	if _, err := http.DefaultClient.Get(authorizeURL); err != nil {
		t.Fatalf("follow authorize url: %v", err)
	}
	res := <-done
	if res.code != 0 {
		t.Fatalf("login --wait failed stdout=%s stderr=%s", res.out, res.err)
	}
	if !strings.Contains(res.out, "logged in to") {
		t.Fatalf("unexpected stdout: %s", res.out)
	}
	entry, ok, err := credentialForServer(credPath, hideasSrv.URL)
	if err != nil || !ok || entry.Token == "" {
		t.Fatalf("token not stored: ok=%v entry=%+v err=%v", ok, entry, err)
	}
}

func TestSSOLoginAutoResumePoll(t *testing.T) {
	hideasSrv, _ := newSSOHideasServer(t)
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials.json")
	configPath := filepath.Join(dir, "config")

	// Start a login session directly to obtain a session ID and authorize URL.
	client := NewHTTPStore(hideasSrv.URL, "")
	start, err := client.AuthLoginStart()
	if err != nil {
		t.Fatal(err)
	}
	if err := storeCredential(credPath, hideasSrv.URL, CredentialEntry{PendingSessionID: start.SessionID}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("server = \""+hideasSrv.URL+"\"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Simulate the user completing the browser-side authorization.
	if _, err := http.DefaultClient.Get(start.AuthorizeURL); err != nil {
		t.Fatal(err)
	}

	// Running any command should opportunistically finish the login.
	out, errOut, code := runCLI(t,
		"--config", configPath,
		"--credentials", credPath,
		"db", "stats",
	)
	if code != 0 {
		t.Fatalf("command failed stdout=%s stderr=%s", out, errOut)
	}
	if !strings.Contains(errOut, "login completed") {
		t.Fatalf("expected login completion notice, stderr=%s", errOut)
	}
	if !strings.Contains(out, "entities=") {
		t.Fatalf("unexpected db stats output: %s", out)
	}

	entry, ok, err := credentialForServer(credPath, hideasSrv.URL)
	if err != nil || !ok || entry.Token == "" || entry.PendingSessionID != "" {
		t.Fatalf("credentials should be promoted to a token: ok=%v entry=%+v err=%v", ok, entry, err)
	}
}

func TestSSOLoginCallbackErrors(t *testing.T) {
	hideasSrv, _ := newSSOHideasServer(t)

	// Unknown state should produce a 400 HTML page.
	resp, err := http.Get(hideasSrv.URL + "/api/v1/auth/callback?state=bogus&code=test-code")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "授权失败") {
		t.Fatalf("expected failure HTML, got %s", body)
	}
}

func TestValidateServerSSOConfig(t *testing.T) {
	good := SSOConfig{
		Issuer: "https://x", ClientID: "id", ClientSecret: "s",
		RedirectURL: "https://example.com/hideas/api/v1/auth/callback",
	}
	if err := validateServerSSOConfig(good, "/hideas/"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The validator does not require base_path to match the redirect URL
	// prefix, because reverse proxies often strip the public prefix before
	// forwarding to the container. base_path "/" with a public-prefixed
	// redirect URL is a legitimate setup.
	if err := validateServerSSOConfig(good, "/"); err != nil {
		t.Fatalf("public-prefixed redirect with base_path / should be allowed: %v", err)
	}
	bad := good
	bad.RedirectURL = "https://example.com/wrong"
	if err := validateServerSSOConfig(bad, "/hideas/"); err == nil {
		t.Fatal("expected error for redirect_url that does not end with /api/v1/auth/callback")
	}
	if err := validateServerSSOConfig(SSOConfig{}, "/"); err != nil {
		t.Fatalf("empty config (static-token-only) should be allowed: %v", err)
	}
	partial := SSOConfig{Issuer: "https://x"}
	if err := validateServerSSOConfig(partial, "/"); err == nil {
		t.Fatal("partial config should fail")
	}
}

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "hideas.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	return store
}

func apiDo[T any](t *testing.T, base, token, method, path string, body interface{}) T {
	t.Helper()
	env := apiDoRaw(t, base, token, method, path, body)
	if !env.OK {
		t.Fatalf("api error error=%+v", env.Error)
	}
	var out T
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func apiDoRaw(t *testing.T, base, token, method, path string, body interface{}) struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *apiError       `json:"error"`
} {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, base+path, r)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var env struct {
		OK    bool            `json:"ok"`
		Data  json.RawMessage `json:"data"`
		Error *apiError       `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	return env
}

func strconvFormat(v int64) string {
	return strconv.FormatInt(v, 10)
}

func mustMillis(t *testing.T, v string) int64 {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", v)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.UTC().UnixMilli()
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
