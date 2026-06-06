package hideas

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const testAuthorizedKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOCC/YQBOu03vEyad+jYolX7kYuacb2ZHB0KUM3eLZHv han@Huge-Han.local\n"

const testPrivateKey = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACDggv2EATrtN7xMmnfo2KJV+5GLmnG9mRwdClDN3i2R7wAAAJj1b8sw9W/L
MAAAAAtzc2gtZWQyNTUxOQAAACDggv2EATrtN7xMmnfo2KJV+5GLmnG9mRwdClDN3i2R7w
AAAEC3U2RzUJh6B+/hlxX1PC5RZUuu0YfwLhnfeWFWePoifOCC/YQBOu03vEyad+jYolX7
kYuacb2ZHB0KUM3eLZHvAAAAEmhhbkBIdWdlLUhhbi5sb2NhbAECAw==
-----END OPENSSH PRIVATE KEY-----
`

func runCLI(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, err bytes.Buffer
	code := Run(args, &out, &err)
	return out.String(), err.String(), code
}

func mustOK(t *testing.T, db string, args ...string) string {
	t.Helper()
	full := append([]string{"--db", db}, args...)
	out, errOut, code := runCLI(t, full...)
	if code != 0 {
		t.Fatalf("command failed: %v\nstdout=%s\nstderr=%s", full, out, errOut)
	}
	return out
}

func mustRemoteOK(t *testing.T, server string, args ...string) string {
	t.Helper()
	full := append([]string{"--server", server}, args...)
	out, errOut, code := runCLI(t, full...)
	if code != 0 {
		t.Fatalf("remote command failed: %v\nstdout=%s\nstderr=%s", full, out, errOut)
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

func TestLocalCLIAllCommands(t *testing.T) {
	db := filepath.Join(t.TempDir(), "hideas.sqlite")

	if out := mustOK(t, db, "init"); !strings.Contains(out, "initialized") {
		t.Fatalf("init output: %s", out)
	}

	e1 := firstID(t, mustOK(t, db, "entity", "add", "李雷", "--type", "person"), "entity")
	e2 := firstID(t, mustOK(t, db, "entity", "add", "李雷", "--type", "person"), "entity")
	if e1 == e2 {
		t.Fatal("duplicate entity names should still create distinct IDs")
	}

	if out := mustOK(t, db, "entity", "list", "--type", "person"); strings.Count(out, "李雷") != 2 {
		t.Fatalf("entity list should include duplicate names: %s", out)
	}

	traceOut := mustOK(t, db, "add", "今天和李雷讨论 SQLite 记忆库", "--type", "thought", "--at", "2026-06-05", "--entity-id", strconvFormat(e1))
	traceID := firstID(t, traceOut, "trace")

	out, errOut, code := runCLI(t, "--db", db, "add", "这条会歧义", "--entity", "李雷")
	if code == 0 || !strings.Contains(errOut, "ambiguous entity name") {
		t.Fatalf("expected ambiguous entity failure, code=%d stdout=%s stderr=%s", code, out, errOut)
	}

	if out := mustOK(t, db, "search", "SQLite", "--entity-id", strconvFormat(e1), "--type", "thought", "--format", "json"); !strings.Contains(out, "SQLite") {
		t.Fatalf("search json output: %s", out)
	}
	if out := mustOK(t, db, "show", "trace", strconvFormat(traceID)); !strings.Contains(out, "entity "+strconvFormat(e1)) {
		t.Fatalf("show trace output: %s", out)
	}

	relID := firstID(t, mustOK(t, db, "link", "entity", strconvFormat(e1), "entity", strconvFormat(e2), "--type", "related_to"), "relation")
	if out := mustOK(t, db, "show", "relation", strconvFormat(relID)); !strings.Contains(out, "related_to") {
		t.Fatalf("show relation output: %s", out)
	}

	if out := mustOK(t, db, "profile", "set", strconvFormat(e1), "前同事，做后端"); !strings.Contains(out, "profile") {
		t.Fatalf("profile set output: %s", out)
	}
	if out := mustOK(t, db, "profile", "show", strconvFormat(e1)); !strings.Contains(out, "前同事") {
		t.Fatalf("profile show output: %s", out)
	}
	if out := mustOK(t, db, "entity", "show", strconvFormat(e1)); !strings.Contains(out, "profile: 前同事") {
		t.Fatalf("entity show output: %s", out)
	}

	if out := mustOK(t, db, "entity", "rename", strconvFormat(e2), "李雷-设计师"); !strings.Contains(out, "renamed") {
		t.Fatalf("rename output: %s", out)
	}

	if out := mustOK(t, db, "type", "add", "trace", "decision"); !strings.Contains(out, "type") {
		t.Fatalf("type add output: %s", out)
	}
	if out := mustOK(t, db, "type", "list"); !strings.Contains(out, "decision") {
		t.Fatalf("type list output: %s", out)
	}

	if out := mustOK(t, db, "db", "path"); !strings.Contains(out, db) {
		t.Fatalf("db path output: %s", out)
	}
	if out := mustOK(t, db, "db", "stats"); !strings.Contains(out, "entities=") || !strings.Contains(out, "traces=") {
		t.Fatalf("db stats output: %s", out)
	}
	if out := mustOK(t, db, "db", "check"); !strings.Contains(out, "ok") {
		t.Fatalf("db check output: %s", out)
	}

	if out := mustOK(t, db, "export", "--format", "json"); !strings.Contains(out, `"entities"`) || !strings.Contains(out, `"traces"`) {
		t.Fatalf("export json output: %s", out)
	}
	if out := mustOK(t, db, "export", "--format", "markdown"); !strings.Contains(out, "# hideas export") {
		t.Fatalf("export markdown output: %s", out)
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

	e := apiDo[Entity](t, server.URL, "secret", http.MethodPost, "/hideas/api/v1/entities", map[string]string{"Name": "Alice", "Type": "person"})
	tr := apiDo[Trace](t, server.URL, "secret", http.MethodPost, "/hideas/api/v1/traces", AddTraceInput{Content: "Alice mentioned SQLite", TypeName: "event", EntityIDs: []int64{e.ID}})
	search := apiDo[SearchResult](t, server.URL, "secret", http.MethodGet, "/hideas/api/v1/search?q=SQLite", nil)
	if len(search.Traces) != 1 || search.Traces[0].ID != tr.ID {
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

func TestRemoteCLIAllCommands(t *testing.T) {
	store := newTestStore(t)
	server := httptest.NewServer(NewHTTPHandler(store, nil, "/"))
	defer server.Close()

	e1 := firstID(t, mustRemoteOK(t, server.URL, "entity", "add", "Remote 李雷", "--type", "person"), "entity")
	e2 := firstID(t, mustRemoteOK(t, server.URL, "entity", "add", "远端项目", "--type", "project"), "entity")
	traceID := firstID(t, mustRemoteOK(t, server.URL, "add", "远端 SQLite 记录", "--type", "thought", "--entity-id", strconvFormat(e1)), "trace")

	if out := mustRemoteOK(t, server.URL, "search", "SQLite", "--entity-id", strconvFormat(e1)); !strings.Contains(out, "远端 SQLite") {
		t.Fatalf("remote search output: %s", out)
	}
	if out := mustRemoteOK(t, server.URL, "show", "trace", strconvFormat(traceID)); !strings.Contains(out, "Remote 李雷") {
		t.Fatalf("remote show trace output: %s", out)
	}
	relID := firstID(t, mustRemoteOK(t, server.URL, "link", "entity", strconvFormat(e1), "entity", strconvFormat(e2), "--type", "related_to"), "relation")
	if out := mustRemoteOK(t, server.URL, "show", "relation", strconvFormat(relID)); !strings.Contains(out, "related_to") {
		t.Fatalf("remote show relation output: %s", out)
	}
	if out := mustRemoteOK(t, server.URL, "profile", "set", strconvFormat(e1), "远端 profile"); !strings.Contains(out, "profile") {
		t.Fatalf("remote profile set output: %s", out)
	}
	if out := mustRemoteOK(t, server.URL, "profile", "show", strconvFormat(e1)); !strings.Contains(out, "远端 profile") {
		t.Fatalf("remote profile show output: %s", out)
	}
	if out := mustRemoteOK(t, server.URL, "entity", "list"); !strings.Contains(out, "Remote 李雷") {
		t.Fatalf("remote entity list output: %s", out)
	}
	if out := mustRemoteOK(t, server.URL, "entity", "rename", strconvFormat(e2), "远端项目2"); !strings.Contains(out, "renamed") {
		t.Fatalf("remote rename output: %s", out)
	}
	if out := mustRemoteOK(t, server.URL, "type", "add", "relation", "inspired_by"); !strings.Contains(out, "type") {
		t.Fatalf("remote type add output: %s", out)
	}
	if out := mustRemoteOK(t, server.URL, "type", "list"); !strings.Contains(out, "inspired_by") {
		t.Fatalf("remote type list output: %s", out)
	}
	if out := mustRemoteOK(t, server.URL, "db", "stats"); !strings.Contains(out, "entities=") {
		t.Fatalf("remote stats output: %s", out)
	}
	if out := mustRemoteOK(t, server.URL, "db", "check"); !strings.Contains(out, "ok") {
		t.Fatalf("remote check output: %s", out)
	}
	if out := mustRemoteOK(t, server.URL, "export", "--format", "json"); !strings.Contains(out, "Remote 李雷") {
		t.Fatalf("remote export output: %s", out)
	}
}

func TestRemoteCLIUsesConfigFile(t *testing.T) {
	store := newTestStore(t)
	server := httptest.NewServer(NewHTTPHandler(store, nil, "/"))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(configPath, []byte("server = \""+server.URL+"\"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	out, errOut, code := runCLI(t, "--config", configPath, "entity", "add", "Config Server", "--type", "source")
	if code != 0 {
		t.Fatalf("config remote command failed stdout=%s stderr=%s", out, errOut)
	}
	if !strings.Contains(out, "entity") {
		t.Fatalf("config remote add output: %s", out)
	}
	if out := mustRemoteOK(t, server.URL, "entity", "list"); !strings.Contains(out, "Config Server") {
		t.Fatalf("config remote did not use server: %s", out)
	}
}

func TestRemoteCLILoginAndAuthStatus(t *testing.T) {
	store := newTestStore(t)
	auth := mustSSHAuth(t)
	server := httptest.NewServer(NewHTTPHandler(store, auth, "/"))
	defer server.Close()

	dir := t.TempDir()
	identityPath := filepath.Join(dir, "id_ed25519")
	credentialsPath := filepath.Join(dir, "credentials.json")
	if err := os.WriteFile(identityPath, []byte(testPrivateKey), 0600); err != nil {
		t.Fatal(err)
	}

	out, errOut, code := runCLI(t, "--credentials", credentialsPath, "login", "--server", server.URL, "--identity", identityPath)
	if code != 0 {
		t.Fatalf("login failed stdout=%s stderr=%s", out, errOut)
	}
	if !strings.Contains(out, "logged in") {
		t.Fatalf("unexpected login output: %s", out)
	}

	if out := mustRemoteOK(t, server.URL, "--credentials", credentialsPath, "entity", "add", "SSH Login", "--type", "person"); !strings.Contains(out, "entity") {
		t.Fatalf("remote add after login output: %s", out)
	}

	out, errOut, code = runCLI(t, "--server", server.URL, "--credentials", credentialsPath, "auth", "status")
	if code != 0 {
		t.Fatalf("auth status failed stdout=%s stderr=%s", out, errOut)
	}
	if !strings.Contains(out, "logged in") {
		t.Fatalf("unexpected auth status output: %s", out)
	}

	out, errOut, code = runCLI(t, "--server", server.URL, "--credentials", credentialsPath, "logout")
	if code != 0 {
		t.Fatalf("logout failed stdout=%s stderr=%s", out, errOut)
	}
	if !strings.Contains(out, "logged out") {
		t.Fatalf("unexpected logout output: %s", out)
	}
	out, errOut, code = runCLI(t, "--server", server.URL, "--credentials", credentialsPath, "auth", "status")
	if code != 0 {
		t.Fatalf("post logout auth status failed stdout=%s stderr=%s", out, errOut)
	}
	if !strings.Contains(out, "not logged in") {
		t.Fatalf("unexpected auth status output after logout: %s", out)
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

func mustSSHAuth(t *testing.T) *serverAuth {
	t.Helper()
	path := filepath.Join(t.TempDir(), "authorized_keys")
	if err := os.WriteFile(path, []byte(testAuthorizedKey), 0600); err != nil {
		t.Fatal(err)
	}
	auth, err := newServerAuth(ServerAuthConfig{AuthorizedKeysPath: path})
	if err != nil {
		t.Fatal(err)
	}
	return auth
}

func apiDo[T any](t *testing.T, base, token, method, path string, body interface{}) T {
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
	if !env.OK {
		t.Fatalf("api error status=%d error=%+v", resp.StatusCode, env.Error)
	}
	var out T
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func strconvFormat(v int64) string {
	return strconv.FormatInt(v, 10)
}
