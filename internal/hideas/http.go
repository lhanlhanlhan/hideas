package hideas

// HTTP API contract:
//
// All handlers and client methods in this file must stay consistent with
// docs/http-api-v1.md. Endpoint paths, request shapes, response envelope,
// JSON field names, status/error semantics, authentication behavior, and base
// path handling are part of that documented contract. Any intentional API
// change must update docs/http-api-v1.md and the corresponding tests in the
// same change.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type apiResponse struct {
	OK    bool        `json:"ok"`
	Data  interface{} `json:"data"`
	Error *apiError   `json:"error"`
}

type apiError struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

func NewHTTPHandler(store Store, auth *serverAuth, basePath string) http.Handler {
	mux := http.NewServeMux()
	api := &apiServer{store: store, auth: auth}
	prefix := normalizeBasePath(basePath) + "api/v1"
	mux.Handle(prefix+"/auth/challenge", api.wrapPublic(api.authChallenge))
	mux.Handle(prefix+"/auth/login", api.wrapPublic(api.authLogin))
	mux.Handle(prefix+"/health", api.wrap(api.health))
	mux.Handle(prefix+"/version", api.wrapPublic(api.version))
	mux.Handle(prefix+"/traces", api.wrap(api.traces))
	mux.Handle(prefix+"/traces/", api.wrap(api.traceByID))
	mux.Handle(prefix+"/search", api.wrap(api.search))
	mux.Handle(prefix+"/entities", api.wrap(api.entities))
	mux.Handle(prefix+"/entities/", api.wrap(api.entityByID))
	mux.Handle(prefix+"/relations", api.wrap(api.relations))
	mux.Handle(prefix+"/relations/", api.wrap(api.relationByID))
	mux.Handle(prefix+"/profiles/", api.wrap(api.profileByEntityID))
	mux.Handle(prefix+"/types", api.wrap(api.types))
	mux.Handle(prefix+"/db/stats", api.wrap(api.stats))
	mux.Handle(prefix+"/db/check", api.wrap(api.check))
	mux.Handle(prefix+"/export", api.wrap(api.export))
	return mux
}

func normalizeBasePath(base string) string {
	if base == "" || base == "/" {
		return "/"
	}
	if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	return base
}

type apiServer struct {
	store Store
	auth  *serverAuth
}

type handlerFunc func(http.ResponseWriter, *http.Request) (interface{}, error)

func (a *apiServer) wrap(fn handlerFunc) http.HandlerFunc {
	return a.wrapWithAuth(true, fn)
}

func (a *apiServer) wrapPublic(fn handlerFunc) http.HandlerFunc {
	return a.wrapWithAuth(false, fn)
}

func (a *apiServer) wrapWithAuth(requireAuth bool, fn handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if requireAuth && a.auth != nil && !a.auth.checkBearer(r.Header.Get("Authorization")) {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(apiResponse{OK: false, Error: &apiError{Code: "unauthorized", Message: "unauthorized"}})
			return
		}
		data, err := fn(w, r)
		if err != nil {
			w.WriteHeader(statusForError(err))
			_ = json.NewEncoder(w).Encode(apiResponse{OK: false, Error: &apiError{Code: codeForError(err), Message: err.Error()}})
			return
		}
		_ = json.NewEncoder(w).Encode(apiResponse{OK: true, Data: data})
	}
}

func statusForError(err error) int {
	if strings.Contains(err.Error(), "unauthorized") {
		return http.StatusUnauthorized
	}
	if strings.Contains(err.Error(), "not found") {
		return http.StatusNotFound
	}
	if strings.Contains(err.Error(), "ambiguous") || strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "unknown") || strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "expired") || strings.Contains(err.Error(), "delete blocked") {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func codeForError(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "unauthorized"):
		return "unauthorized"
	case strings.Contains(msg, "delete blocked"):
		return "delete_blocked"
	case strings.Contains(msg, "ambiguous entity"):
		return "ambiguous_entity"
	case strings.Contains(msg, "not found"):
		return "not_found"
	default:
		return "error"
	}
}

func (a *apiServer) authChallenge(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	if r.Method != http.MethodPost {
		return nil, fmt.Errorf("unsupported method")
	}
	if a.auth == nil {
		return nil, errors.New("ssh login is not configured")
	}
	return a.auth.issueChallenge()
}

func (a *apiServer) authLogin(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	if r.Method != http.MethodPost {
		return nil, fmt.Errorf("unsupported method")
	}
	var in struct {
		ChallengeID string `json:"challenge_id"`
		PublicKey   string `json:"public_key"`
		Signature   string `json:"signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		return nil, err
	}
	sig, err := base64.StdEncoding.DecodeString(in.Signature)
	if err != nil {
		return nil, errors.New("invalid signature")
	}
	return a.auth.login(in.ChallengeID, in.PublicKey, sig)
}

func (a *apiServer) health(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	if r.Method != http.MethodGet {
		return nil, fmt.Errorf("unsupported method")
	}
	return map[string]string{"status": "ok"}, nil
}

func (a *apiServer) version(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	if r.Method != http.MethodGet {
		return nil, fmt.Errorf("unsupported method")
	}
	return localVersionInfo(), nil
}

func (a *apiServer) traces(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	if r.Method != http.MethodPost {
		return nil, fmt.Errorf("unsupported method")
	}
	var in AddTraceInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		return nil, err
	}
	return a.store.AddTrace(in)
}

func (a *apiServer) traceByID(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	id, err := tailID(r.URL.Path)
	if err != nil {
		return nil, err
	}
	switch r.Method {
	case http.MethodGet:
		return a.store.Show("trace", id)
	case http.MethodDelete:
		return a.store.Delete("trace", id, cascadeQuery(r))
	default:
		return nil, fmt.Errorf("unsupported method")
	}
}

func (a *apiServer) search(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	if r.Method != http.MethodGet {
		return nil, fmt.Errorf("unsupported method")
	}
	q := r.URL.Query()
	in := SearchInput{Query: q.Get("q"), EntityName: q.Get("entity"), TypeName: q.Get("type")}
	if v := q.Get("entity_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, err
		}
		in.EntityID = &id
	}
	if v := q.Get("since"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, err
		}
		in.Since = &n
	}
	if v := q.Get("until"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, err
		}
		in.Until = &n
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, err
		}
		in.Limit = n
	}
	return a.store.Search(in)
}

func (a *apiServer) entities(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	switch r.Method {
	case http.MethodPost:
		var in struct {
			Name string
			Type string
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			return nil, err
		}
		return a.store.AddEntity(in.Name, in.Type)
	case http.MethodGet:
		name := r.URL.Query().Get("name")
		if name != "" {
			return a.store.ResolveEntityName(name)
		}
		return a.store.ListEntities(r.URL.Query().Get("type"))
	default:
		return nil, fmt.Errorf("unsupported method")
	}
}

func (a *apiServer) entityByID(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	id, err := tailID(r.URL.Path)
	if err != nil {
		return nil, err
	}
	switch r.Method {
	case http.MethodGet:
		return a.store.Show("entity", id)
	case http.MethodPatch:
		var in struct{ Name string }
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			return nil, err
		}
		return a.store.RenameEntity(id, in.Name)
	case http.MethodDelete:
		return a.store.Delete("entity", id, cascadeQuery(r))
	default:
		return nil, fmt.Errorf("unsupported method")
	}
}

func (a *apiServer) relations(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	if r.Method != http.MethodPost {
		return nil, fmt.Errorf("unsupported method")
	}
	var in struct {
		FromKind string
		FromID   int64
		ToKind   string
		ToID     int64
		Type     string
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		return nil, err
	}
	return a.store.Link(in.FromKind, in.FromID, in.ToKind, in.ToID, in.Type)
}

func (a *apiServer) relationByID(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	id, err := tailID(r.URL.Path)
	if err != nil {
		return nil, err
	}
	switch r.Method {
	case http.MethodGet:
		return a.store.Show("relation", id)
	case http.MethodDelete:
		return a.store.Delete("relation", id, cascadeQuery(r))
	default:
		return nil, fmt.Errorf("unsupported method")
	}
}

func cascadeQuery(r *http.Request) bool {
	v := strings.ToLower(r.URL.Query().Get("cascade"))
	return v == "1" || v == "true" || v == "yes"
}

func (a *apiServer) profileByEntityID(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	id, err := tailID(r.URL.Path)
	if err != nil {
		return nil, err
	}
	switch r.Method {
	case http.MethodGet:
		return a.store.GetProfile(id)
	case http.MethodPut:
		var in struct{ Content string }
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			return nil, err
		}
		return a.store.SetProfile(id, in.Content)
	default:
		return nil, fmt.Errorf("unsupported method")
	}
}

func (a *apiServer) types(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	switch r.Method {
	case http.MethodGet:
		return a.store.ListTypes()
	case http.MethodPost:
		var in struct {
			Domain string
			Name   string
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			return nil, err
		}
		return a.store.AddType(in.Domain, in.Name)
	default:
		return nil, fmt.Errorf("unsupported method")
	}
}

func (a *apiServer) stats(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	return a.store.Stats()
}

func (a *apiServer) check(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	return a.store.Check()
}

func (a *apiServer) export(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	format := r.URL.Query().Get("format")
	b, err := a.store.Export(format)
	if err != nil {
		return nil, err
	}
	return map[string]string{"format": format, "content": string(b)}, nil
}

func tailID(path string) (int64, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	return strconv.ParseInt(parts[len(parts)-1], 10, 64)
}

type HTTPStore struct {
	base   string
	token  string
	client *http.Client
}

func NewHTTPStore(base, token string) *HTTPStore {
	return &HTTPStore{base: strings.TrimRight(base, "/") + "/api/v1", token: token, client: http.DefaultClient}
}

func (h *HTTPStore) Init() error  { return nil }
func (h *HTTPStore) Close() error { return nil }
func (h *HTTPStore) Path() string { return h.base }

func (h *HTTPStore) do(method, path string, body interface{}, out interface{}) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, h.base+path, r)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if h.token != "" {
		req.Header.Set("Authorization", "Bearer "+h.token)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var env struct {
		OK    bool            `json:"ok"`
		Data  json.RawMessage `json:"data"`
		Error *apiError       `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return err
	}
	if !env.OK {
		if env.Error != nil {
			return errorsFromAPI(env.Error.Message)
		}
		return fmt.Errorf("remote request failed")
	}
	if out != nil {
		return json.Unmarshal(env.Data, out)
	}
	return nil
}

func errorsFromAPI(msg string) error { return fmt.Errorf("%s", msg) }

func (h *HTTPStore) AuthChallenge() (authChallengeResponse, error) {
	var out authChallengeResponse
	err := h.do(http.MethodPost, "/auth/challenge", map[string]string{"client": "hideas-cli"}, &out)
	return out, err
}

func (h *HTTPStore) AuthLogin(challengeID, publicKey, signature string) (authLoginResponse, error) {
	var out authLoginResponse
	err := h.do(http.MethodPost, "/auth/login", map[string]string{
		"challenge_id": challengeID,
		"public_key":   publicKey,
		"signature":    signature,
	}, &out)
	return out, err
}

func (h *HTTPStore) Health() (map[string]string, error) {
	var out map[string]string
	err := h.do(http.MethodGet, "/health", nil, &out)
	return out, err
}

func (h *HTTPStore) Version() (VersionInfo, error) {
	var out VersionInfo
	err := h.do(http.MethodGet, "/version", nil, &out)
	return out, err
}

func (h *HTTPStore) AddTrace(in AddTraceInput) (Trace, error) {
	var out Trace
	err := h.do(http.MethodPost, "/traces", in, &out)
	return out, err
}

func (h *HTTPStore) Search(in SearchInput) (SearchResult, error) {
	q := fmt.Sprintf("/search?q=%s&entity=%s&type=%s&limit=%d", url.QueryEscape(in.Query), url.QueryEscape(in.EntityName), url.QueryEscape(in.TypeName), in.Limit)
	if in.EntityID != nil {
		q += fmt.Sprintf("&entity_id=%d", *in.EntityID)
	}
	if in.Since != nil {
		q += fmt.Sprintf("&since=%d", *in.Since)
	}
	if in.Until != nil {
		q += fmt.Sprintf("&until=%d", *in.Until)
	}
	var out SearchResult
	err := h.do(http.MethodGet, q, nil, &out)
	return out, err
}

func (h *HTTPStore) Show(kind string, id int64) (ShowResult, error) {
	var out ShowResult
	plural := map[string]string{"entity": "entities", "trace": "traces", "relation": "relations"}[kind]
	if plural == "" {
		return ShowResult{}, fmt.Errorf("unknown kind: %s", kind)
	}
	err := h.do(http.MethodGet, "/"+plural+"/"+strconv.FormatInt(id, 10), nil, &out)
	return out, err
}

func (h *HTTPStore) Delete(kind string, id int64, cascade bool) (DeleteResult, error) {
	var out DeleteResult
	plural := map[string]string{"entity": "entities", "trace": "traces", "relation": "relations"}[kind]
	if plural == "" {
		return DeleteResult{}, fmt.Errorf("unknown kind: %s", kind)
	}
	path := "/" + plural + "/" + strconv.FormatInt(id, 10)
	if cascade {
		path += "?cascade=true"
	}
	err := h.do(http.MethodDelete, path, nil, &out)
	return out, err
}

func (h *HTTPStore) Link(fromKind string, fromID int64, toKind string, toID int64, typeName string) (Relation, error) {
	var out Relation
	err := h.do(http.MethodPost, "/relations", map[string]interface{}{"FromKind": fromKind, "FromID": fromID, "ToKind": toKind, "ToID": toID, "Type": typeName}, &out)
	return out, err
}

func (h *HTTPStore) AddEntity(name, typeName string) (Entity, error) {
	var out Entity
	err := h.do(http.MethodPost, "/entities", map[string]string{"Name": name, "Type": typeName}, &out)
	return out, err
}

func (h *HTTPStore) ListEntities(typeName string) ([]Entity, error) {
	var out []Entity
	err := h.do(http.MethodGet, "/entities?type="+url.QueryEscape(typeName), nil, &out)
	return out, err
}

func (h *HTTPStore) GetEntity(id int64) (Entity, error) {
	res, err := h.Show("entity", id)
	if err != nil {
		return Entity{}, err
	}
	return *res.Entity, nil
}

func (h *HTTPStore) RenameEntity(id int64, name string) (Entity, error) {
	var out Entity
	err := h.do(http.MethodPatch, "/entities/"+strconv.FormatInt(id, 10), map[string]string{"Name": name}, &out)
	return out, err
}

func (h *HTTPStore) ResolveEntityName(name string) ([]Entity, error) {
	var out []Entity
	err := h.do(http.MethodGet, "/entities?name="+url.QueryEscape(name), nil, &out)
	return out, err
}

func (h *HTTPStore) SetProfile(entityID int64, content string) (Trace, error) {
	var out Trace
	err := h.do(http.MethodPut, "/profiles/"+strconv.FormatInt(entityID, 10), map[string]string{"Content": content}, &out)
	return out, err
}

func (h *HTTPStore) GetProfile(entityID int64) (Trace, error) {
	var out Trace
	err := h.do(http.MethodGet, "/profiles/"+strconv.FormatInt(entityID, 10), nil, &out)
	return out, err
}

func (h *HTTPStore) ListTypes() ([]Type, error) {
	var out []Type
	err := h.do(http.MethodGet, "/types", nil, &out)
	return out, err
}

func (h *HTTPStore) AddType(domainName, name string) (Type, error) {
	var out Type
	err := h.do(http.MethodPost, "/types", map[string]string{"Domain": domainName, "Name": name}, &out)
	return out, err
}

func (h *HTTPStore) Stats() (Stats, error) {
	var out Stats
	err := h.do(http.MethodGet, "/db/stats", nil, &out)
	return out, err
}

func (h *HTTPStore) Check() (CheckResult, error) {
	var out CheckResult
	err := h.do(http.MethodGet, "/db/check", nil, &out)
	return out, err
}

func (h *HTTPStore) Export(format string) ([]byte, error) {
	var out map[string]string
	err := h.do(http.MethodGet, "/export?format="+url.QueryEscape(format), nil, &out)
	if err != nil {
		return nil, err
	}
	return []byte(out["content"]), nil
}
