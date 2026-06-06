package hideas

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type GlobalSettings struct {
	DB              string
	Server          string
	Token           string
	Identity        string
	CredentialsPath string
	AuthorizedKeys  string
}

func Run(args []string, stdout, stderr io.Writer) int {
	if err := run(args, stdout, stderr); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}

func run(args []string, stdout, stderr io.Writer) error {
	global := flag.NewFlagSet("hideas", flag.ContinueOnError)
	global.SetOutput(stderr)
	dbPath := global.String("db", "", "SQLite database path")
	serverURL := global.String("server", "", "hideas server URL")
	configPath := global.String("config", "", "config file path")
	token := global.String("token", "", "HTTP bearer token")
	identity := global.String("identity", "", "SSH private key path for login")
	credentialsPath := global.String("credentials", "", "credentials file path")
	if err := global.Parse(args); err != nil {
		return err
	}
	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	settings, err := resolveGlobalConfig(*dbPath, *serverURL, *token, *identity, *credentialsPath, cfg)
	if err != nil {
		return err
	}
	rest := global.Args()
	if len(rest) == 0 {
		return errors.New("command is required")
	}
	cmd := rest[0]
	cmdArgs := rest[1:]

	if cmd == "serve" {
		return runServe(cmdArgs, settings, stdout, stderr)
	}
	if cmd == "login" {
		return cmdLogin(cmdArgs, settings, stdout, stderr)
	}
	if cmd == "logout" {
		return cmdLogout(cmdArgs, settings, stdout, stderr)
	}
	if cmd == "auth" {
		return cmdAuth(cmdArgs, settings, stdout, stderr)
	}

	store, err := openStore(settings)
	if err != nil {
		return err
	}
	defer store.Close()

	switch cmd {
	case "init":
		if err := store.Init(); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "initialized %s\n", store.Path())
	case "add":
		return cmdAdd(store, cmdArgs, stdout, stderr)
	case "search":
		return cmdSearch(store, cmdArgs, stdout, stderr)
	case "show":
		return cmdShow(store, cmdArgs, stdout)
	case "link":
		return cmdLink(store, cmdArgs, stdout)
	case "entity":
		return cmdEntity(store, cmdArgs, stdout, stderr)
	case "profile":
		return cmdProfile(store, cmdArgs, stdout)
	case "type":
		return cmdType(store, cmdArgs, stdout)
	case "db":
		return cmdDB(store, cmdArgs, stdout)
	case "export":
		return cmdExport(store, cmdArgs, stdout)
	default:
		return fmt.Errorf("unknown command: %s", cmd)
	}
	return nil
}

func resolveGlobalConfig(cliDB, cliServer, cliToken, cliIdentity, cliCredentials string, cfg Config) (GlobalSettings, error) {
	settings := GlobalSettings{
		DB:              cfg.DB,
		Server:          cfg.Server,
		Token:           cfg.Token,
		Identity:        cfg.Identity,
		CredentialsPath: cfg.CredentialsPath,
		AuthorizedKeys:  cfg.AuthorizedKeys,
	}
	if v := os.Getenv("HIDEAS_DB"); v != "" {
		settings.DB = v
	}
	if v := os.Getenv("HIDEAS_SERVER"); v != "" {
		settings.Server = v
	}
	if v := os.Getenv("HIDEAS_TOKEN"); v != "" {
		settings.Token = v
	}
	if v := os.Getenv("HIDEAS_IDENTITY"); v != "" {
		settings.Identity = v
	}
	if v := os.Getenv("HIDEAS_CREDENTIALS"); v != "" {
		settings.CredentialsPath = v
	}
	if v := os.Getenv("HIDEAS_AUTHORIZED_KEYS"); v != "" {
		settings.AuthorizedKeys = v
	}
	if cliDB != "" {
		settings.DB = cliDB
	}
	if cliServer != "" {
		settings.Server = cliServer
	}
	if cliToken != "" {
		settings.Token = cliToken
	}
	if cliIdentity != "" {
		settings.Identity = cliIdentity
	}
	if cliCredentials != "" {
		settings.CredentialsPath = cliCredentials
	}
	if settings.CredentialsPath == "" {
		settings.CredentialsPath = defaultCredentialsPath()
	}
	if settings.Server != "" && settings.Token == "" {
		if entry, ok, err := credentialForServer(settings.CredentialsPath, settings.Server); err != nil {
			return GlobalSettings{}, err
		} else if ok {
			settings.Token = entry.Token
		}
	}
	return settings, nil
}

func openStore(settings GlobalSettings) (Store, error) {
	if settings.Server != "" {
		return NewHTTPStore(settings.Server, settings.Token), nil
	}
	if settings.DB == "" {
		settings.DB = defaultDBPath()
	}
	return OpenSQLite(settings.DB)
}

func runServe(args []string, settings GlobalSettings, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", settings.DB, "SQLite database path")
	host := fs.String("host", "127.0.0.1", "listen host")
	port := fs.Int("port", 8765, "listen port")
	basePath := fs.String("base-path", "/", "HTTP base path")
	localToken := fs.String("token", settings.Token, "HTTP bearer token")
	authorizedKeys := fs.String("authorized-keys", settings.AuthorizedKeys, "authorized SSH public keys file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" {
		*dbPath = defaultDBPath()
	}
	store, err := OpenSQLite(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Init(); err != nil {
		return err
	}
	addr := fmt.Sprintf("%s:%d", *host, *port)
	fmt.Fprintf(stdout, "serving %s on http://%s%s\n", store.Path(), addr, normalizeBasePath(*basePath))
	auth, err := newServerAuth(ServerAuthConfig{StaticToken: *localToken, AuthorizedKeysPath: *authorizedKeys})
	if err != nil {
		return err
	}
	return http.ListenAndServe(addr, NewHTTPHandler(store, auth, *basePath))
}

func cmdLogin(args []string, settings GlobalSettings, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.SetOutput(stderr)
	server := fs.String("server", settings.Server, "hideas server URL")
	identity := fs.String("identity", settings.Identity, "SSH private key path")
	credentialsPath := fs.String("credentials", settings.CredentialsPath, "credentials file path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*server) == "" {
		return errors.New("server is required")
	}
	if strings.TrimSpace(*identity) == "" {
		return errors.New("identity is required")
	}
	client := NewHTTPStore(*server, "")
	challenge, err := client.AuthChallenge()
	if err != nil {
		return err
	}
	publicKey, signature, err := signChallenge(*identity, challenge.Challenge)
	if err != nil {
		return err
	}
	login, err := client.AuthLogin(challenge.ChallengeID, publicKey, signature)
	if err != nil {
		return err
	}
	if err := storeCredential(*credentialsPath, *server, CredentialEntry{Token: login.Token, ExpiresAt: login.ExpiresAt}); err != nil {
		return err
	}
	if err := validateCredentialFileMode(*credentialsPath); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "logged in to %s until %s\n", normalizeServerKey(*server), time.UnixMilli(login.ExpiresAt).UTC().Format(time.RFC3339))
	return nil
}

func cmdLogout(args []string, settings GlobalSettings, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("logout", flag.ContinueOnError)
	fs.SetOutput(stderr)
	server := fs.String("server", settings.Server, "hideas server URL")
	credentialsPath := fs.String("credentials", settings.CredentialsPath, "credentials file path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*server) == "" {
		return errors.New("server is required")
	}
	if err := removeCredential(*credentialsPath, *server); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "logged out from %s\n", normalizeServerKey(*server))
	return nil
}

func cmdAuth(args []string, settings GlobalSettings, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("auth subcommand is required")
	}
	switch args[0] {
	case "status":
		fs := flag.NewFlagSet("auth status", flag.ContinueOnError)
		fs.SetOutput(stderr)
		server := fs.String("server", settings.Server, "hideas server URL")
		credentialsPath := fs.String("credentials", settings.CredentialsPath, "credentials file path")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*server) == "" {
			return errors.New("server is required")
		}
		entry, ok, err := credentialForServer(*credentialsPath, *server)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintf(stdout, "not logged in for %s\n", normalizeServerKey(*server))
			return nil
		}
		store := NewHTTPStore(*server, entry.Token)
		if _, err := store.Health(); err != nil {
			return fmt.Errorf("stored token exists but verification failed: %w", err)
		}
		fmt.Fprintf(stdout, "logged in for %s until %s\n", normalizeServerKey(*server), time.UnixMilli(entry.ExpiresAt).UTC().Format(time.RFC3339))
		return nil
	default:
		return fmt.Errorf("unknown auth subcommand: %s", args[0])
	}
}

func cmdAdd(store Store, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(stderr)
	typeName := fs.String("type", "", "trace type")
	at := fs.String("at", "", "happened time")
	entities := multiFlag{}
	entityIDs := multiIntFlag{}
	fs.Var(&entities, "entity", "entity name")
	fs.Var(&entityIDs, "entity-id", "entity id")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 1 {
		return errors.New("content is required")
	}
	happened, err := parseOptionalTime(*at)
	if err != nil {
		return err
	}
	t, err := store.AddTrace(AddTraceInput{Content: strings.Join(pos, " "), TypeName: *typeName, Happened: happened, Entities: entities, EntityIDs: entityIDs})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "trace %d added\n", t.ID)
	return nil
}

func cmdSearch(store Store, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(stderr)
	entity := fs.String("entity", "", "entity name")
	entityID := fs.Int64("entity-id", 0, "entity id")
	typeName := fs.String("type", "", "trace type")
	sinceStr := fs.String("since", "", "since")
	untilStr := fs.String("until", "", "until")
	limit := fs.Int("limit", 20, "limit")
	format := fs.String("format", "text", "text|json")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	since, err := parseOptionalTime(*sinceStr)
	if err != nil {
		return err
	}
	until, err := parseOptionalTime(*untilStr)
	if err != nil {
		return err
	}
	in := SearchInput{Query: strings.Join(pos, " "), EntityName: *entity, TypeName: *typeName, Since: since, Until: until, Limit: *limit}
	if *entityID != 0 {
		in.EntityID = entityID
	}
	res, err := store.Search(in)
	if err != nil {
		return err
	}
	if *format == "json" {
		return writeJSON(stdout, res)
	}
	for _, e := range res.Entities {
		fmt.Fprintf(stdout, "entity %d %s [%s]\n", e.ID, e.Name, e.TypeName)
	}
	for _, t := range res.Traces {
		fmt.Fprintf(stdout, "trace %d [%s] %s\n", t.ID, t.TypeName, t.Content)
	}
	return nil
}

func cmdShow(store Store, args []string, stdout io.Writer) error {
	if len(args) != 2 {
		return errors.New("usage: show KIND ID")
	}
	id, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return err
	}
	res, err := store.Show(args[0], id)
	if err != nil {
		return err
	}
	switch args[0] {
	case "entity":
		fmt.Fprintf(stdout, "entity %d %s [%s]\n", res.Entity.ID, res.Entity.Name, res.Entity.TypeName)
		if res.Entity.Profile != "" {
			fmt.Fprintf(stdout, "profile: %s\n", res.Entity.Profile)
		}
	case "trace":
		fmt.Fprintf(stdout, "trace %d [%s] %s\n", res.Trace.ID, res.Trace.TypeName, res.Trace.Content)
		for _, e := range res.Entities {
			fmt.Fprintf(stdout, "entity %d %s [%s]\n", e.ID, e.Name, e.TypeName)
		}
	case "relation":
		fmt.Fprintf(stdout, "relation %d %s %d -> %s %d [%s]\n", res.Relation.ID, kindName(res.Relation.FromKind), res.Relation.FromID, kindName(res.Relation.ToKind), res.Relation.ToID, res.Relation.TypeName)
	}
	for _, r := range res.Relations {
		fmt.Fprintf(stdout, "relation %d %s %d -> %s %d [%s]\n", r.ID, kindName(r.FromKind), r.FromID, kindName(r.ToKind), r.ToID, r.TypeName)
	}
	for _, t := range res.Traces {
		fmt.Fprintf(stdout, "trace %d [%s] %s\n", t.ID, t.TypeName, t.Content)
	}
	return nil
}

func cmdLink(store Store, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("link", flag.ContinueOnError)
	typeName := fs.String("type", "", "relation type")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 4 {
		return errors.New("usage: link FROM_KIND FROM_ID TO_KIND TO_ID --type TYPE")
	}
	fromID, err := strconv.ParseInt(pos[1], 10, 64)
	if err != nil {
		return err
	}
	toID, err := strconv.ParseInt(pos[3], 10, 64)
	if err != nil {
		return err
	}
	r, err := store.Link(pos[0], fromID, pos[2], toID, *typeName)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "relation %d added\n", r.ID)
	return nil
}

func cmdEntity(store Store, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("entity subcommand is required")
	}
	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("entity add", flag.ContinueOnError)
		fs.SetOutput(stderr)
		typeName := fs.String("type", "", "entity type")
		pos, err := parseInterspersed(fs, args[1:])
		if err != nil {
			return err
		}
		if len(pos) < 1 {
			return errors.New("entity name is required")
		}
		e, err := store.AddEntity(strings.Join(pos, " "), *typeName)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "entity %d added\n", e.ID)
	case "list":
		fs := flag.NewFlagSet("entity list", flag.ContinueOnError)
		fs.SetOutput(stderr)
		typeName := fs.String("type", "", "entity type")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		list, err := store.ListEntities(*typeName)
		if err != nil {
			return err
		}
		for _, e := range list {
			fmt.Fprintf(stdout, "entity %d %s [%s]", e.ID, e.Name, e.TypeName)
			if e.Profile != "" {
				fmt.Fprintf(stdout, " profile: %s", e.Profile)
			}
			fmt.Fprintln(stdout)
		}
	case "show":
		if len(args) != 2 {
			return errors.New("usage: entity show ID")
		}
		return cmdShow(store, []string{"entity", args[1]}, stdout)
	case "rename":
		if len(args) < 3 {
			return errors.New("usage: entity rename ID NAME")
		}
		id, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return err
		}
		e, err := store.RenameEntity(id, strings.Join(args[2:], " "))
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "entity %d renamed to %s\n", e.ID, e.Name)
	default:
		return fmt.Errorf("unknown entity subcommand: %s", args[0])
	}
	return nil
}

func cmdProfile(store Store, args []string, stdout io.Writer) error {
	if len(args) < 2 {
		return errors.New("usage: profile show|set ENTITY_ID [content]")
	}
	id, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return err
	}
	switch args[0] {
	case "show":
		t, err := store.GetProfile(id)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "profile %d %s\n", t.ID, t.Content)
	case "set":
		if len(args) < 3 {
			return errors.New("profile content is required")
		}
		t, err := store.SetProfile(id, strings.Join(args[2:], " "))
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "profile %d set\n", t.ID)
	default:
		return fmt.Errorf("unknown profile subcommand: %s", args[0])
	}
	return nil
}

func cmdType(store Store, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("type subcommand is required")
	}
	switch args[0] {
	case "list":
		types, err := store.ListTypes()
		if err != nil {
			return err
		}
		for _, t := range types {
			fmt.Fprintf(stdout, "type %d domain=%d %s\n", t.ID, t.Domain, t.Name)
		}
	case "add":
		if len(args) != 3 {
			return errors.New("usage: type add DOMAIN NAME")
		}
		t, err := store.AddType(args[1], args[2])
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "type %d added\n", t.ID)
	default:
		return fmt.Errorf("unknown type subcommand: %s", args[0])
	}
	return nil
}

func cmdDB(store Store, args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: db path|stats|check")
	}
	switch args[0] {
	case "path":
		fmt.Fprintln(stdout, store.Path())
	case "stats":
		st, err := store.Stats()
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "entities=%d traces=%d relations=%d types=%d\n", st.Entities, st.Traces, st.Relations, st.Types)
	case "check":
		res, err := store.Check()
		if err != nil {
			return err
		}
		if res.OK {
			fmt.Fprintln(stdout, "ok")
		} else {
			for _, e := range res.Errors {
				fmt.Fprintln(stdout, e)
			}
		}
	default:
		return fmt.Errorf("unknown db subcommand: %s", args[0])
	}
	return nil
}

func cmdExport(store Store, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	format := fs.String("format", "json", "json|markdown")
	if err := fs.Parse(args); err != nil {
		return err
	}
	b, err := store.Export(*format)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, string(b))
	return nil
}

func parseOptionalTime(v string) (*int64, error) {
	if strings.TrimSpace(v) == "" {
		return nil, nil
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		return &n, nil
	}
	for _, layout := range []string{"2006-01-02", time.RFC3339} {
		if t, err := time.Parse(layout, v); err == nil {
			ms := t.UTC().UnixMilli()
			return &ms, nil
		}
	}
	return nil, fmt.Errorf("invalid time: %s", v)
}

func writeJSON(w io.Writer, v interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

type multiIntFlag []int64

func (m *multiIntFlag) String() string {
	var out []string
	for _, v := range *m {
		out = append(out, strconv.FormatInt(v, 10))
	}
	return strings.Join(out, ",")
}
func (m *multiIntFlag) Set(v string) error {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return err
	}
	*m = append(*m, n)
	return nil
}

func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var flagArgs []string
	var pos []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") {
			flagArgs = append(flagArgs, arg)
			if !strings.Contains(arg, "=") {
				if i+1 >= len(args) {
					return nil, fmt.Errorf("missing value for %s", arg)
				}
				flagArgs = append(flagArgs, args[i+1])
				i++
			}
			continue
		}
		pos = append(pos, arg)
	}
	if err := fs.Parse(flagArgs); err != nil {
		return nil, err
	}
	return pos, nil
}
