package hideas

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type GlobalSettings struct {
	ConfigPath      string
	Server          string
	Token           string
	CredentialsPath string
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
	global.Usage = func() {
		writeRootHelp(stderr)
	}
	showVersion := global.Bool("version", false, "show version")
	serverURL := global.String("server", "", "hideas server URL")
	configPath := global.String("config", "", "config file path")
	token := global.String("token", "", "HTTP bearer token")
	credentialsPath := global.String("credentials", "", "credentials file path")
	if err := global.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *showVersion {
		fmt.Fprint(stdout, formatVersionInfo(localVersionInfo()))
		return nil
	}
	resolvedConfigPath := *configPath
	if resolvedConfigPath == "" {
		resolvedConfigPath = defaultConfigPath()
	}
	rest := global.Args()
	if len(rest) == 0 {
		writeRootHelp(stdout)
		return nil
	}
	cmd := rest[0]
	cmdArgs := rest[1:]
	if cmd == "help" {
		if len(cmdArgs) == 0 {
			writeRootHelp(stdout)
			return nil
		}
		writeCommandHelp(stdout, cmdArgs)
		return nil
	}
	if isHelpArg(cmd) {
		writeRootHelp(stdout)
		return nil
	}

	cfg, err := loadConfig(resolvedConfigPath)
	if err != nil {
		return err
	}
	settings, err := resolveGlobalConfig(resolvedConfigPath, *serverURL, *token, *credentialsPath, cfg)
	if err != nil {
		return err
	}

	if cmd == "serve" {
		return runServe(cmdArgs, resolvedConfigPath, cfg, stdout, stderr)
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
	if cmd == "status" {
		return cmdStatus(settings, stdout)
	}
	if cmd == "version" {
		return cmdVersion(settings, stdout)
	}

	// Help requests for any data-source-bound subcommand must short-circuit
	// before we try to open a remote store: otherwise `hideas entity --help`
	// fails simply because there is no token configured yet. Help requests
	// targeting a specific subcommand (e.g. `entity add --help`) are routed
	// through the subcommand handler with a nil store so its flag.Usage can
	// surface the precise usage line.
	if firstArgIsHelp(cmdArgs) || (len(cmdArgs) == 0 && wantsHelp(cmdArgs)) {
		writeCommandHelp(stdout, []string{cmd})
		return nil
	}
	if wantsHelp(cmdArgs) {
		return dispatchCommand(cmd, nil, cmdArgs, stdout, stderr)
	}

	store, err := openStore(&settings, stderr)
	if err != nil {
		return err
	}
	defer store.Close()

	return dispatchCommand(cmd, store, cmdArgs, stdout, stderr)
}

func dispatchCommand(cmd string, store Store, cmdArgs []string, stdout, stderr io.Writer) error {
	switch cmd {
	case "add":
		return cmdAdd(store, cmdArgs, stdout, stderr)
	case "search":
		return cmdSearch(store, cmdArgs, stdout, stderr)
	case "show":
		return cmdShow(store, cmdArgs, stdout)
	case "delete":
		return cmdDelete(store, cmdArgs, stdout)
	case "link":
		return cmdLink(store, cmdArgs, stdout)
	case "trace":
		return cmdTrace(store, cmdArgs, stdout, stderr)
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
}

func isHelpArg(v string) bool {
	return v == "-h" || v == "--help"
}

func wantsHelp(args []string) bool {
	if len(args) == 0 {
		return false
	}
	for _, arg := range args {
		if isHelpArg(arg) || arg == "help" {
			return true
		}
	}
	return false
}

func firstArgIsHelp(args []string) bool {
	return len(args) > 0 && (isHelpArg(args[0]) || args[0] == "help")
}

func writeRootHelp(w io.Writer) {
	writeVersionInfo(w, localVersionInfo())
	fmt.Fprint(w, `Hideas is a personal memory system. The CLI is a thin client over a hideas server.

Usage:
  hideas [global options] COMMAND [command options] [arguments]
  hideas help [COMMAND]

Core commands:
  version         Show the current version and build time
  status          Show the configured server and login state
  login           Start SSO login against the configured server
  logout          Remove the stored token for a remote server
  auth status     Verify the stored remote token
  add             Add a trace
  search          Search traces and entities
  show            Show an entity, trace, or relation by ID
  delete          Delete an entity, trace, or relation by ID
  link            Create a relation
  trace           Manage traces
  entity          Manage entities
  profile         Show or set an entity profile
  type            List or add types
  db              Inspect remote database stats
  export          Export data

Server commands:
  serve           Run the HTTP server (reads ~/.hideas/config)

Global options:
  --version             Show version and build time
  --server URL          Hideas server URL
  --config PATH         Config file path
  --token TOKEN         Static HTTP bearer token
  --credentials PATH    Credentials file path

Examples:
  hideas login --server https://example.com/hideas/
  hideas login --wait
  hideas add "Discussed SQLite indexing" --type thought
  hideas search "SQLite" --format json
  hideas entity add "Li Lei" --type person

Use "hideas help COMMAND" for command-specific help.
`)
}

func writeCommandHelp(w io.Writer, args []string) {
	if len(args) == 0 {
		writeRootHelp(w)
		return
	}
	writeVersionInfo(w, localVersionInfo())
	switch args[0] {
	case "serve":
		fmt.Fprint(w, "Usage: hideas serve [--config PATH]\n  All other options are read from the config file or HIDEAS_SSO_* env vars.\n")
	case "status":
		fmt.Fprint(w, "Usage: hideas status\n")
	case "login":
		fmt.Fprint(w, "Usage: hideas login [--server URL] [--credentials PATH] [--wait] [--timeout DURATION]\n")
	case "logout":
		fmt.Fprint(w, "Usage: hideas logout [--server URL] [--credentials PATH]\n")
	case "auth":
		fmt.Fprint(w, "Usage: hideas auth status [--server URL] [--credentials PATH]\n")
	case "add":
		fmt.Fprint(w, "Usage: hideas add CONTENT [--type TYPE] [--at TIME] [--entity NAME] [--entity-id ID]\n")
	case "search":
		fmt.Fprint(w, "Usage: hideas search [QUERY] [--entity NAME] [--entity-id ID] [--type TYPE] [--since TIME] [--until TIME] [--recent DURATION] [--literal] [--limit N] [--format text|json]\n")
	case "show":
		fmt.Fprint(w, "Usage: hideas show entity|trace|relation ID\n")
	case "delete":
		fmt.Fprint(w, "Usage: hideas delete entity|trace|relation ID [--cascade]\n")
	case "link":
		fmt.Fprint(w, "Usage: hideas link FROM_KIND FROM_ID TO_KIND TO_ID --type TYPE\n")
	case "trace":
		fmt.Fprint(w, "Usage: hideas trace update ID [--happened-at TIME] [--created-at TIME] [--updated-at TIME]\n")
	case "entity":
		fmt.Fprint(w, "Usage: hideas entity add|list|show|rename ...\n")
		fmt.Fprint(w, "  add NAME [--type TYPE]\n  list [--type TYPE]\n  show ID\n  rename ID NAME\n")
	case "profile":
		fmt.Fprint(w, "Usage: hideas profile show ENTITY_ID\n       hideas profile set ENTITY_ID CONTENT\n")
	case "type":
		fmt.Fprint(w, "Usage: hideas type list\n       hideas type add entity|trace|relation NAME\n")
	case "db":
		fmt.Fprint(w, "Usage: hideas db stats|check\n")
	case "export":
		fmt.Fprint(w, "Usage: hideas export [--format json|markdown]\n")
	default:
		fmt.Fprintf(w, "unknown help topic: %s\n", args[0])
	}
}

func writeVersionInfo(w io.Writer, info VersionInfo) {
	fmt.Fprint(w, formatVersionInfo(info))
}

func resolveGlobalConfig(configPath, cliServer, cliToken, cliCredentials string, cfg Config) (GlobalSettings, error) {
	settings := GlobalSettings{
		ConfigPath:      configPath,
		Server:          cfg.Server,
		Token:           cfg.Token,
		CredentialsPath: cfg.CredentialsPath,
	}
	if v := os.Getenv("HIDEAS_SERVER"); v != "" {
		settings.Server = v
	}
	if v := os.Getenv("HIDEAS_TOKEN"); v != "" {
		settings.Token = v
	}
	if v := os.Getenv("HIDEAS_CREDENTIALS"); v != "" {
		settings.CredentialsPath = v
	}
	if cliServer != "" {
		settings.Server = cliServer
	}
	if cliToken != "" {
		settings.Token = cliToken
	}
	if cliCredentials != "" {
		settings.CredentialsPath = cliCredentials
	}
	if settings.CredentialsPath == "" {
		settings.CredentialsPath = defaultCredentialsPath()
	}
	return settings, nil
}

// openStore returns the HTTP-backed Store for command execution. If the stored
// credentials carry a pending SSO session, it polls once to opportunistically
// finish the login before issuing the command.
func openStore(settings *GlobalSettings, stderr io.Writer) (Store, error) {
	if strings.TrimSpace(settings.Server) == "" {
		return nil, errors.New("server is required: configure `server` in the config file, set HIDEAS_SERVER, or pass --server")
	}
	if settings.Token == "" {
		entry, ok, err := credentialForServer(settings.CredentialsPath, settings.Server)
		if err != nil {
			return nil, err
		}
		if ok && entry.Token == "" && entry.PendingSessionID != "" {
			entry, err = tryFinishPendingLogin(settings.CredentialsPath, settings.Server, entry, stderr)
			if err != nil {
				return nil, err
			}
		}
		if ok && entry.Token != "" {
			settings.Token = entry.Token
		}
	}
	if settings.Token == "" {
		return nil, errors.New("not logged in: run `hideas login` first")
	}
	return NewHTTPStore(settings.Server, settings.Token), nil
}

// tryFinishPendingLogin polls the server once for a pending login session and
// updates the credentials file accordingly. The caller's settings.Token is set
// when the poll resolves to ready.
func tryFinishPendingLogin(credentialsPath, server string, entry CredentialEntry, stderr io.Writer) (CredentialEntry, error) {
	client := NewHTTPStore(server, "")
	res, err := client.AuthLoginPoll(entry.PendingSessionID)
	if err != nil {
		return entry, nil
	}
	switch res.Status {
	case loginStatusReady:
		entry.Token = res.Token
		entry.ExpiresAt = res.ExpiresAt
		entry.PendingSessionID = ""
		if err := storeCredential(credentialsPath, server, entry); err != nil {
			return entry, err
		}
		fmt.Fprintf(stderr, "login completed for %s\n", normalizeServerKey(server))
	case loginStatusExpired:
		entry.PendingSessionID = ""
		if err := storeCredential(credentialsPath, server, entry); err != nil {
			return entry, err
		}
		return entry, errors.New("pending login session expired: run `hideas login` again")
	}
	return entry, nil
}

// runServe starts the hideas HTTP server. All configuration is read from the
// config file or HIDEAS_SSO_* environment variables; no CLI flags besides
// --config are accepted, by design.
func runServe(args []string, configPath string, cfg Config, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { writeCommandHelp(stderr, []string{"serve"}) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	_ = configPath
	cfg.applySSOEnv()
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	port := cfg.Port
	if port == 0 {
		port = 8765
	}
	basePath := cfg.BasePath
	if basePath == "" {
		basePath = "/"
	}
	dbPath := strings.TrimSpace(cfg.DB)
	if dbPath == "" {
		dbPath = defaultDBPath()
	}
	if err := validateServerSSOConfig(cfg.SSO, basePath); err != nil {
		return err
	}
	store, err := OpenSQLite(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Init(); err != nil {
		return err
	}
	auth, err := newServerAuth(ServerAuthConfig{StaticToken: cfg.Token, SSO: cfg.SSO})
	if err != nil {
		return err
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	fmt.Fprintf(stdout, "serving %s on http://%s%s\n", store.Path(), addr, normalizeBasePath(basePath))
	return http.ListenAndServe(addr, NewHTTPHandler(store, auth, basePath))
}

// validateServerSSOConfig verifies that, when SSO is configured, the redirect
// URL ends with the API callback path under the configured base path.
func validateServerSSOConfig(sso SSOConfig, basePath string) error {
	missing := []string{}
	if strings.TrimSpace(sso.Issuer) == "" {
		missing = append(missing, "issuer")
	}
	if strings.TrimSpace(sso.ClientID) == "" {
		missing = append(missing, "client_id")
	}
	if strings.TrimSpace(sso.ClientSecret) == "" {
		missing = append(missing, "client_secret")
	}
	if strings.TrimSpace(sso.RedirectURL) == "" {
		missing = append(missing, "redirect_url")
	}
	// Allow running with no SSO at all (static-token only) for tests/CI.
	if len(missing) == 4 {
		return nil
	}
	if len(missing) > 0 {
		return fmt.Errorf("incomplete sso configuration: missing %s", strings.Join(missing, ", "))
	}
	expectedSuffix := normalizeBasePath(basePath) + "api/v1/auth/callback"
	parsed, err := url.Parse(strings.TrimSpace(sso.RedirectURL))
	if err != nil {
		return fmt.Errorf("invalid redirect_url: %w", err)
	}
	if parsed.Path != expectedSuffix {
		return fmt.Errorf("redirect_url path must end with %s (got %s)", expectedSuffix, parsed.Path)
	}
	return nil
}

func cmdLogin(args []string, settings GlobalSettings, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { writeCommandHelp(stderr, []string{"login"}) }
	server := fs.String("server", settings.Server, "hideas server URL")
	credentialsPath := fs.String("credentials", settings.CredentialsPath, "credentials file path")
	wait := fs.Bool("wait", false, "block until the browser-side authorization completes")
	timeout := fs.Duration("timeout", 5*time.Minute, "maximum wait time when --wait is used")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if strings.TrimSpace(*server) == "" {
		return errors.New("server is required: pass --server or configure `server` in the config file")
	}
	client := NewHTTPStore(*server, "")
	start, err := client.AuthLoginStart()
	if err != nil {
		return err
	}
	entry := CredentialEntry{PendingSessionID: start.SessionID}
	if err := storeCredential(*credentialsPath, *server, entry); err != nil {
		return err
	}
	if err := validateCredentialFileMode(*credentialsPath); err != nil {
		return err
	}
	if err := persistRemoteServer(settings.ConfigPath, *server, *credentialsPath); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Open the following URL in your browser to complete login:\n\n  %s\n\n", start.AuthorizeURL)
	fmt.Fprintf(stdout, "Session expires at %s.\n", time.UnixMilli(start.ExpiresAt).UTC().Format(time.RFC3339))
	if !*wait {
		fmt.Fprintln(stdout, "After authorizing, run any hideas command (or `hideas auth status`) to finish login.")
		return nil
	}
	fmt.Fprintln(stdout, "Waiting for browser-side authorization...")
	deadline := time.Now().Add(*timeout)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		res, err := client.AuthLoginPoll(start.SessionID)
		if err != nil {
			return err
		}
		switch res.Status {
		case loginStatusReady:
			entry.Token = res.Token
			entry.ExpiresAt = res.ExpiresAt
			entry.PendingSessionID = ""
			if err := storeCredential(*credentialsPath, *server, entry); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "logged in to %s until %s\n", normalizeServerKey(*server), time.UnixMilli(res.ExpiresAt).UTC().Format(time.RFC3339))
			return nil
		case loginStatusExpired:
			_ = removeCredential(*credentialsPath, *server)
			return errors.New("login session expired before the browser completed authorization")
		}
	}
	return errors.New("login timed out: rerun `hideas login --wait` or finish in browser and run any hideas command")
}

func cmdLogout(args []string, settings GlobalSettings, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("logout", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { writeCommandHelp(stderr, []string{"logout"}) }
	server := fs.String("server", settings.Server, "hideas server URL")
	credentialsPath := fs.String("credentials", settings.CredentialsPath, "credentials file path")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if strings.TrimSpace(*server) == "" {
		return errors.New("server is required")
	}
	if err := removeCredential(*credentialsPath, *server); err != nil {
		return err
	}
	if err := clearRemoteServer(settings.ConfigPath, *server); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "logged out from %s\n", normalizeServerKey(*server))
	return nil
}

func cmdStatus(settings GlobalSettings, stdout io.Writer) error {
	server := normalizeServerKey(settings.Server)
	if server == "" {
		fmt.Fprintln(stdout, "server: (none)")
		fmt.Fprintln(stdout, "login: not logged in")
		return nil
	}
	fmt.Fprintf(stdout, "server: %s\n", server)
	if settings.Token != "" {
		fmt.Fprintln(stdout, "login: authenticated with static token")
		return nil
	}
	entry, ok, err := credentialForServer(settings.CredentialsPath, settings.Server)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(stdout, "login: not logged in")
		return nil
	}
	if entry.Token != "" {
		fmt.Fprintf(stdout, "login: logged in until %s\n", time.UnixMilli(entry.ExpiresAt).UTC().Format(time.RFC3339))
		return nil
	}
	if entry.PendingSessionID != "" {
		fmt.Fprintln(stdout, "login: pending browser authorization")
	}
	return nil
}

func cmdVersion(settings GlobalSettings, stdout io.Writer) error {
	if strings.TrimSpace(settings.Server) != "" {
		store := NewHTTPStore(settings.Server, settings.Token)
		info, err := store.Version()
		if err == nil {
			writeVersionInfo(stdout, info)
			return nil
		}
	}
	writeVersionInfo(stdout, localVersionInfo())
	return nil
}

func persistRemoteServer(configPath, server, credentialsPath string) error {
	if strings.TrimSpace(configPath) == "" {
		return errors.New("config path is not available")
	}
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	cfg.Server = normalizeServerKey(server)
	cfg.Token = ""
	cfg.CredentialsPath = strings.TrimSpace(credentialsPath)
	return saveConfig(configPath, cfg)
}

func clearRemoteServer(configPath, server string) error {
	if strings.TrimSpace(configPath) == "" {
		return errors.New("config path is not available")
	}
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	if normalizeServerKey(cfg.Server) != normalizeServerKey(server) {
		return nil
	}
	cfg.Server = ""
	return saveConfig(configPath, cfg)
}

func cmdAuth(args []string, settings GlobalSettings, stdout, stderr io.Writer) error {
	if len(args) == 0 || firstArgIsHelp(args) {
		writeCommandHelp(stdout, []string{"auth"})
		return nil
	}
	switch args[0] {
	case "status":
		fs := flag.NewFlagSet("auth status", flag.ContinueOnError)
		fs.SetOutput(stderr)
		fs.Usage = func() { writeCommandHelp(stderr, []string{"auth"}) }
		server := fs.String("server", settings.Server, "hideas server URL")
		credentialsPath := fs.String("credentials", settings.CredentialsPath, "credentials file path")
		if err := fs.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if strings.TrimSpace(*server) == "" {
			return errors.New("server is required")
		}
		entry, ok, err := credentialForServer(*credentialsPath, *server)
		if err != nil {
			return err
		}
		if ok && entry.Token == "" && entry.PendingSessionID != "" {
			entry, err = tryFinishPendingLogin(*credentialsPath, *server, entry, stderr)
			if err != nil {
				return err
			}
		}
		if entry.Token == "" {
			if entry.PendingSessionID != "" {
				fmt.Fprintf(stdout, "pending login for %s: open the authorization URL in your browser\n", normalizeServerKey(*server))
				return nil
			}
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
	fs.Usage = func() { writeCommandHelp(stderr, []string{"add"}) }
	typeName := fs.String("type", "", "trace type")
	at := fs.String("at", "", "happened time")
	entities := multiFlag{}
	entityIDs := multiIntFlag{}
	fs.Var(&entities, "entity", "entity name")
	fs.Var(&entityIDs, "entity-id", "entity id")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
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
	fs.Usage = func() { writeCommandHelp(stderr, []string{"search"}) }
	entity := fs.String("entity", "", "entity name")
	entityID := fs.Int64("entity-id", 0, "entity id")
	typeName := fs.String("type", "", "trace type")
	sinceStr := fs.String("since", "", "since")
	untilStr := fs.String("until", "", "until")
	recentStr := fs.String("recent", "", "recent window, e.g. 24h")
	limit := fs.Int("limit", 20, "limit")
	format := fs.String("format", "text", "text|json")
	literal := fs.Bool("literal", false, "match the query as a single literal phrase")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if strings.TrimSpace(*recentStr) != "" && (strings.TrimSpace(*sinceStr) != "" || strings.TrimSpace(*untilStr) != "") {
		return errors.New("--recent cannot be combined with --since or --until")
	}
	since, err := parseOptionalSearchTime(*sinceStr, false)
	if err != nil {
		return err
	}
	until, err := parseOptionalSearchTime(*untilStr, true)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*recentStr) != "" {
		since, until, err = parseRecentWindow(*recentStr)
		if err != nil {
			return err
		}
	}
	in := SearchInput{Query: strings.Join(pos, " "), EntityName: *entity, TypeName: *typeName, Since: since, Until: until, Limit: *limit, Literal: *literal}
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
		fmt.Fprintf(stdout, "trace %d [%s] %s\n", t.ID, t.TypeName, summarizeText(t.Content, 100))
	}
	if res.EntitiesHasMore || res.TracesHasMore {
		fmt.Fprintf(stdout, "more results available: traces=%t entities=%t\n", res.TracesHasMore, res.EntitiesHasMore)
	}
	return nil
}

func summarizeText(s string, maxRunes int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if maxRunes <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

func cmdShow(store Store, args []string, stdout io.Writer) error {
	if wantsHelp(args) {
		writeCommandHelp(stdout, []string{"show"})
		return nil
	}
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

func cmdTrace(store Store, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || firstArgIsHelp(args) {
		writeCommandHelp(stdout, []string{"trace"})
		return nil
	}
	switch args[0] {
	case "update":
		fs := flag.NewFlagSet("trace update", flag.ContinueOnError)
		fs.SetOutput(stderr)
		fs.Usage = func() { writeCommandHelp(stderr, []string{"trace"}) }
		happenedAtStr := fs.String("happened-at", "", "happened time")
		createdAtStr := fs.String("created-at", "", "created time")
		updatedAtStr := fs.String("updated-at", "", "updated time")
		pos, err := parseInterspersed(fs, args[1:])
		if err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		if len(pos) != 1 {
			return errors.New("usage: trace update ID [--happened-at TIME] [--created-at TIME] [--updated-at TIME]")
		}
		id, err := strconv.ParseInt(pos[0], 10, 64)
		if err != nil {
			return err
		}
		in := UpdateTraceInput{}
		if strings.TrimSpace(*happenedAtStr) != "" {
			t, err := parseOptionalTime(*happenedAtStr)
			if err != nil {
				return err
			}
			in.HappenedAt = t
		}
		if strings.TrimSpace(*createdAtStr) != "" {
			t, err := parseOptionalTime(*createdAtStr)
			if err != nil {
				return err
			}
			in.CreatedAt = t
		}
		if strings.TrimSpace(*updatedAtStr) != "" {
			t, err := parseOptionalTime(*updatedAtStr)
			if err != nil {
				return err
			}
			in.UpdatedAt = t
		}
		t, err := store.UpdateTrace(id, in)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "trace %d updated", t.ID)
		if t.HappenedAt != nil {
			fmt.Fprintf(stdout, " happened_at=%d", *t.HappenedAt)
		}
		fmt.Fprintf(stdout, " created_at=%d updated_at=%d\n", t.CreatedAt, t.UpdatedAt)
		return nil
	default:
		return fmt.Errorf("unknown trace subcommand: %s", args[0])
	}
}

func cmdDelete(store Store, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	fs.Usage = func() { writeCommandHelp(stdout, []string{"delete"}) }
	cascade := fs.Bool("cascade", false, "delete related relations and clear profile references")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if len(pos) != 2 {
		return errors.New("usage: delete KIND ID [--cascade]")
	}
	id, err := strconv.ParseInt(pos[1], 10, 64)
	if err != nil {
		return err
	}
	res, err := store.Delete(pos[0], id, *cascade)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "deleted %s %d", res.Kind, res.ID)
	if res.RelationsDeleted > 0 {
		fmt.Fprintf(stdout, " relations_deleted=%d", res.RelationsDeleted)
	}
	if res.ProfilesCleared > 0 {
		fmt.Fprintf(stdout, " profiles_cleared=%d", res.ProfilesCleared)
	}
	fmt.Fprintln(stdout)
	return nil
}

func cmdLink(store Store, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("link", flag.ContinueOnError)
	fs.Usage = func() { writeCommandHelp(stdout, []string{"link"}) }
	typeName := fs.String("type", "", "relation type")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
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
	if len(args) == 0 || firstArgIsHelp(args) {
		writeCommandHelp(stdout, []string{"entity"})
		return nil
	}
	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("entity add", flag.ContinueOnError)
		fs.SetOutput(stderr)
		fs.Usage = func() {
			writeVersionInfo(stderr, localVersionInfo())
			fmt.Fprint(stderr, "Usage: hideas entity add NAME [--type TYPE]\n")
		}
		typeName := fs.String("type", "", "entity type")
		pos, err := parseInterspersed(fs, args[1:])
		if err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
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
		fs.Usage = func() {
			writeVersionInfo(stderr, localVersionInfo())
			fmt.Fprint(stderr, "Usage: hideas entity list [--type TYPE]\n")
		}
		typeName := fs.String("type", "", "entity type")
		if err := fs.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
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
		if len(args) > 1 && wantsHelp(args[1:]) {
			writeVersionInfo(stdout, localVersionInfo())
			fmt.Fprint(stdout, "Usage: hideas entity show ID\n")
			return nil
		}
		if len(args) != 2 {
			return errors.New("usage: entity show ID")
		}
		return cmdShow(store, []string{"entity", args[1]}, stdout)
	case "rename":
		if len(args) > 1 && wantsHelp(args[1:]) {
			writeVersionInfo(stdout, localVersionInfo())
			fmt.Fprint(stdout, "Usage: hideas entity rename ID NAME\n")
			return nil
		}
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
	if len(args) == 0 || firstArgIsHelp(args) {
		writeCommandHelp(stdout, []string{"profile"})
		return nil
	}
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
	if len(args) == 0 || firstArgIsHelp(args) {
		writeCommandHelp(stdout, []string{"type"})
		return nil
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
	if len(args) == 0 || firstArgIsHelp(args) {
		writeCommandHelp(stdout, []string{"db"})
		return nil
	}
	if len(args) != 1 {
		return errors.New("usage: db stats|check")
	}
	switch args[0] {
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
	fs.Usage = func() { writeCommandHelp(stdout, []string{"export"}) }
	format := fs.String("format", "json", "json|markdown")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	b, err := store.Export(*format)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, string(b))
	return nil
}

func parseOptionalSearchTime(v string, upperBound bool) (*int64, error) {
	if strings.TrimSpace(v) == "" {
		return nil, nil
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		return &n, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", v, time.Local); err == nil {
		if upperBound {
			t = t.Add(24*time.Hour - time.Millisecond)
		}
		ms := t.UTC().UnixMilli()
		return &ms, nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		ms := t.UTC().UnixMilli()
		return &ms, nil
	}
	return nil, fmt.Errorf("invalid time: %s", v)
}

func parseOptionalTime(v string) (*int64, error) {
	return parseOptionalSearchTime(v, false)
}

func parseRecentWindow(v string) (*int64, *int64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil, nil
	}
	if len(v) < 2 {
		return nil, nil, fmt.Errorf("invalid recent window: %s", v)
	}
	unit := v[len(v)-1]
	if unit != 'h' && unit != 'w' && unit != 'y' {
		return nil, nil, fmt.Errorf("invalid recent window: %s", v)
	}
	n, err := strconv.ParseInt(strings.TrimSpace(v[:len(v)-1]), 10, 64)
	if err != nil || n <= 0 {
		return nil, nil, fmt.Errorf("invalid recent window: %s", v)
	}
	now := time.Now().UTC()
	var d time.Duration
	switch unit {
	case 'h':
		d = time.Duration(n) * time.Hour
	case 'w':
		d = time.Duration(n) * 7 * 24 * time.Hour
	case 'y':
		d = time.Duration(n) * 365 * 24 * time.Hour
	}
	since := now.Add(-d).UnixMilli()
	until := now.UnixMilli()
	return &since, &until, nil
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
		if isHelpArg(arg) {
			flagArgs = append(flagArgs, arg)
			continue
		}
		if strings.HasPrefix(arg, "--") {
			flagArgs = append(flagArgs, arg)
			if !strings.Contains(arg, "=") {
				name := strings.TrimLeft(arg, "-")
				if f := fs.Lookup(name); f != nil {
					if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
						continue
					}
				}
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
