// This executable provides a one-click Linux development startup path for the existing CommerceOps services.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/platform/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	webURL    = "http://localhost:3000"
	healthURL = "http://localhost:8080/api/v1/health"
)

type processState struct {
	ServerPID     int    `json:"server_pid"`
	FrontendPID   int    `json:"frontend_pid"`
	ServerStart   string `json:"server_start"`
	FrontendStart string `json:"frontend_start"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		message := "CommerceOps could not start: " + err.Error()
		fmt.Fprintln(os.Stderr, message)
		notify("CommerceOps could not start", message)
		os.Exit(1)
	}
}

func run(args []string) error {
	root, err := findRepositoryRoot()
	if err != nil {
		return err
	}
	stateDir := filepath.Join(root, ".commerceops-local")
	if err = os.MkdirAll(filepath.Join(stateDir, "bin"), 0o700); err != nil {
		return fmt.Errorf("create launcher state: %w", err)
	}
	logFile, err := os.OpenFile(filepath.Join(stateDir, "launcher.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open launcher log: %w", err)
	}
	defer logFile.Close()

	lock, err := acquireLock(filepath.Join(stateDir, "launcher.lock"))
	if err != nil {
		return err
	}
	defer func() {
		_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		_ = lock.Close()
	}()

	if len(args) > 0 {
		switch args[0] {
		case "--stop":
			return stop(root, stateDir, logFile)
		default:
			return fmt.Errorf("unknown option %q", args[0])
		}
	}

	if recordedProcessesRunning(filepath.Join(stateDir, "processes.json")) {
		if err = waitForURL(healthURL, 90*time.Second); err != nil {
			return err
		}
		if err = waitForURL(webURL, 90*time.Second); err != nil {
			return err
		}
		if err = installDesktopEntry(root); err != nil {
			fmt.Fprintf(logFile, "desktop entry: %v\n", err)
		}
		return openBrowser()
	}

	for _, dependency := range []string{"docker", "go", "pnpm", "xdg-open", "pdfinfo", "pdftotext", "pdftoppm", "pdftocairo", "pdfseparate", "pdfunite", "tesseract"} {
		if _, lookupErr := exec.LookPath(dependency); lookupErr != nil {
			return fmt.Errorf("required program %q is not installed", dependency)
		}
	}
	notify("Starting CommerceOps", "Preparing the local app. The browser will open when it is ready.")
	if err = ensureEnvironment(root); err != nil {
		return err
	}
	values, err := readEnvironment(filepath.Join(root, ".env"))
	if err != nil {
		return err
	}
	if err = validateLocalConfiguration(values); err != nil {
		return err
	}
	values["HTTP_ADDR"] = "127.0.0.1:8080"
	values["FILE_STORAGE_DIR"] = filepath.Join(stateDir, "uploads")
	environment := mergeEnvironment(values)

	// Stop only launcher-owned stale processes before replacing them. Unknown processes are never killed.
	if err = stopRecordedProcesses(filepath.Join(stateDir, "processes.json"), logFile); err != nil {
		return err
	}
	for _, address := range []string{"127.0.0.1:3000", "127.0.0.1:8080"} {
		listener, e := net.Listen("tcp", address)
		if e != nil {
			return fmt.Errorf("port %s is occupied; stop the other application first", address)
		}
		listener.Close()
	}
	if err = runCommand(root, environment, logFile, "docker", "compose", "up", "-d", "--wait", "postgres"); err != nil {
		return err
	}
	migrationURL, _ := url.Parse(values["DATABASE_URL"])
	migrationURL.Host = "postgres:5432"
	if err = runCommand(root, environment, logFile, "docker", "compose", "--profile", "tools", "run", "--rm", "migrate", "-path=/migrations", "-database="+migrationURL.String(), "up"); err != nil {
		return err
	}
	credentialsPath := filepath.Join(stateDir, "credentials.txt")
	_, created, err := bootstrapLocalAdministrator(values["DATABASE_URL"], credentialsPath)
	if err != nil {
		return err
	}
	if err = refreshLocalAdministratorAccess(values["DATABASE_URL"], credentialsPath); err != nil {
		return err
	}
	if _, statErr := os.Stat(filepath.Join(root, "apps", "web", "node_modules")); errors.Is(statErr, os.ErrNotExist) {
		if err = runCommand(filepath.Join(root, "apps", "web"), environment, logFile, "pnpm", "install", "--frozen-lockfile"); err != nil {
			return err
		}
	} else if statErr != nil {
		return fmt.Errorf("inspect frontend dependencies: %w", statErr)
	}
	serverBinary := filepath.Join(stateDir, "bin", "commerceops-server")
	if err = runCommand(filepath.Join(root, "services", "api"), environment, logFile, "go", "build", "-o", serverBinary, "./cmd/server"); err != nil {
		return err
	}

	server, err := startProcess(filepath.Join(root, "services", "api"), environment, logFile, serverBinary)
	if err != nil {
		return err
	}
	frontend, err := startProcess(filepath.Join(root, "apps", "web"), environment, logFile, "pnpm", "dev", "--hostname", "127.0.0.1", "--port", "3000")
	if err != nil {
		terminateProcessGroup(server.Process.Pid)
		return err
	}
	state := processState{ServerPID: server.Process.Pid, FrontendPID: frontend.Process.Pid, ServerStart: processStart(server.Process.Pid), FrontendStart: processStart(frontend.Process.Pid)}
	if err = writeProcessState(filepath.Join(stateDir, "processes.json"), state); err != nil {
		terminateProcessGroup(server.Process.Pid)
		terminateProcessGroup(frontend.Process.Pid)
		return err
	}
	if err = waitForURL(healthURL, 90*time.Second); err != nil {
		_ = stopRecordedProcesses(filepath.Join(stateDir, "processes.json"), logFile)
		return err
	}
	if err = waitForURL(webURL, 90*time.Second); err != nil {
		_ = stopRecordedProcesses(filepath.Join(stateDir, "processes.json"), logFile)
		return err
	}
	if err = installDesktopEntry(root); err != nil {
		fmt.Fprintf(logFile, "desktop entry: %v\n", err)
	}
	if created {
		notify("CommerceOps is ready", "Local login details are in "+credentialsPath)
		if e := exec.Command("xdg-open", credentialsPath).Start(); e != nil {
			fmt.Fprintf(logFile, "open credentials: %v\n", e)
		}
	} else {
		notify("CommerceOps is ready", webURL)
	}
	return openBrowser()
}

func findRepositoryRoot() (string, error) {
	candidates := make([]string, 0, 2)
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(executable))
	}
	if working, err := os.Getwd(); err == nil {
		candidates = append(candidates, working)
	}
	for _, candidate := range candidates {
		for current := candidate; ; current = filepath.Dir(current) {
			if fileExists(filepath.Join(current, "services", "api", "go.mod")) && fileExists(filepath.Join(current, "apps", "web", "package.json")) {
				return current, nil
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
		}
	}
	return "", errors.New("CommerceOps repository root was not found")
}

func ensureEnvironment(root string) error {
	path := filepath.Join(root, ".env")
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect .env: %w", err)
	}
	password, err := randomSecret(24)
	if err != nil {
		return err
	}
	contents := strings.Join([]string{
		"COMMERCEOPS_POSTGRES_DB=commerceops",
		"COMMERCEOPS_POSTGRES_USER=commerceops",
		"COMMERCEOPS_POSTGRES_PASSWORD=" + password,
		"DATABASE_URL=postgres://commerceops:" + password + "@localhost:5432/commerceops?sslmode=disable",
		"HTTP_ADDR=127.0.0.1:8080",
		"APP_ENV=development",
		"CORS_ALLOWED_ORIGINS=http://localhost:3000",
		"NEXT_PUBLIC_API_BASE_URL=http://localhost:8080",
		"OBJECT_STORAGE_DRIVER=local",
		"FILE_STORAGE_DIR=./data/uploads",
		"OBJECT_STORAGE_ENDPOINT=",
		"OBJECT_STORAGE_BUCKET=",
		"OBJECT_STORAGE_REGION=",
		"OBJECT_STORAGE_ACCESS_KEY=",
		"OBJECT_STORAGE_SECRET_KEY=",
		"OBJECT_STORAGE_PATH_STYLE=false",
		"",
	}, "\n")
	if err = os.WriteFile(path, []byte(contents), 0o600); err != nil {
		return fmt.Errorf("create .env: %w", err)
	}
	return nil
}

func readEnvironment(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read .env: %w", err)
	}
	values := make(map[string]string)
	for number, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			return nil, fmt.Errorf("invalid .env line %d", number+1)
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
			value = value[1 : len(value)-1]
		}
		values[key] = value
	}
	return values, nil
}

func validateLocalConfiguration(values map[string]string) error {
	if values["APP_ENV"] != "development" {
		return errors.New("launcher requires APP_ENV=development")
	}
	databaseURL, err := url.Parse(values["DATABASE_URL"])
	if err != nil || (databaseURL.Scheme != "postgres" && databaseURL.Scheme != "postgresql") {
		return errors.New("DATABASE_URL is invalid")
	}
	for key := range databaseURL.Query() {
		if key != "sslmode" {
			return errors.New("launcher only supports sslmode in DATABASE_URL query parameters")
		}
	}
	host := strings.ToLower(databaseURL.Hostname())
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return errors.New("launcher refuses a non-local DATABASE_URL")
	}
	if databaseURL.Port() != "5432" {
		return errors.New("launcher requires PostgreSQL port 5432")
	}
	if databaseURL.User == nil {
		return errors.New("DATABASE_URL must include local credentials")
	}
	password, _ := databaseURL.User.Password()
	if values["COMMERCEOPS_POSTGRES_USER"] == "" || values["COMMERCEOPS_POSTGRES_DB"] == "" || password == "" || databaseURL.User.Username() != values["COMMERCEOPS_POSTGRES_USER"] || password != values["COMMERCEOPS_POSTGRES_PASSWORD"] || strings.TrimPrefix(databaseURL.Path, "/") != values["COMMERCEOPS_POSTGRES_DB"] {
		return errors.New("DATABASE_URL must match the local Compose database, user and password")
	}
	if values["NEXT_PUBLIC_API_BASE_URL"] != "http://localhost:8080" || (values["HTTP_ADDR"] != ":8080" && values["HTTP_ADDR"] != "127.0.0.1:8080") {
		return errors.New("launcher requires API port 8080 and http://localhost:8080")
	}
	if values["OBJECT_STORAGE_DRIVER"] != "local" {
		return errors.New("launcher requires local object storage")
	}
	if strings.Contains(values["COMMERCEOPS_POSTGRES_PASSWORD"], "replace-with") {
		return errors.New("replace the example PostgreSQL password in .env or remove .env so the launcher can generate one")
	}
	return nil
}

func bootstrapLocalAdministrator(databaseURL, credentialsPath string) (string, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return "", false, fmt.Errorf("connect for local bootstrap: %w", err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", false, fmt.Errorf("begin local bootstrap: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err = tx.Exec(ctx, `LOCK TABLE users, companies IN EXCLUSIVE MODE`); err != nil {
		return "", false, fmt.Errorf("lock local bootstrap: %w", err)
	}
	var users int
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM users) + (SELECT count(*) FROM companies)`).Scan(&users); err != nil {
		return "", false, fmt.Errorf("inspect local users: %w", err)
	}
	if users != 0 {
		return "", false, nil
	}
	password, err := randomSecret(18)
	if err != nil {
		return "", false, err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return "", false, fmt.Errorf("hash local administrator password: %w", err)
	}
	const email = "admin@commerceops.local"
	var companyID, userID, roleID string
	if err = tx.QueryRow(ctx, `INSERT INTO companies(name) VALUES('CommerceOps Local') RETURNING id`).Scan(&companyID); err != nil {
		return "", false, fmt.Errorf("create local company: %w", err)
	}
	if err = tx.QueryRow(ctx, `INSERT INTO users(email,password_hash) VALUES($1,$2) RETURNING id`, email, hash).Scan(&userID); err != nil {
		return "", false, fmt.Errorf("create local administrator: %w", err)
	}
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO company_users(company_id,user_id) VALUES($1,$2)`, []any{companyID, userID}},
		{`INSERT INTO employees(company_id,user_id,display_name) VALUES($1,$2,'Local Administrator')`, []any{companyID, userID}},
	}
	for _, statement := range statements {
		if _, err = tx.Exec(ctx, statement.query, statement.args...); err != nil {
			return "", false, fmt.Errorf("create local administrator access: %w", err)
		}
	}
	if err = tx.QueryRow(ctx, `INSERT INTO roles(company_id,name) VALUES($1,'Local Administrator') RETURNING id`, companyID).Scan(&roleID); err != nil {
		return "", false, fmt.Errorf("create local administrator role: %w", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO role_permissions(company_id,role_id,permission_key) SELECT $1,$2,key FROM permissions`, companyID, roleID); err != nil {
		return "", false, fmt.Errorf("grant local permissions: %w", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO company_user_roles(company_id,user_id,role_id) VALUES($1,$2,$3)`, companyID, userID, roleID); err != nil {
		return "", false, fmt.Errorf("assign local administrator role: %w", err)
	}
	modules := []string{"amazon", "consignments", "flipkart", "inventory", "meesho", "myntra", "returns", "snapdeal", "traceability"}
	if _, err = tx.Exec(ctx, `INSERT INTO module_entitlements(company_id,module_key,enabled) SELECT $1,unnest($2::text[]),true`, companyID, modules); err != nil {
		return "", false, fmt.Errorf("enable local modules: %w", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO business_identities(company_id,display_name,legal_name) VALUES($1,'CommerceOps Local','CommerceOps Local')`, companyID); err != nil {
		return "", false, fmt.Errorf("create local business identity: %w", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(company_id,actor_user_id,action,target_type,target_id,metadata) VALUES($1,$2,'local.bootstrap','company',$1::uuid::text,jsonb_build_object('development_only',true))`, companyID, userID); err != nil {
		return "", false, fmt.Errorf("audit local bootstrap: %w", err)
	}
	credentials := "Email: " + email + "\nPassword: " + password + "\n"
	if err = os.WriteFile(credentialsPath, []byte(credentials), 0o600); err != nil {
		return "", false, fmt.Errorf("save local login details: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return "", false, fmt.Errorf("commit local bootstrap: %w", err)
	}
	return credentials, true, nil
}

// refreshLocalAdministratorAccess keeps only the launcher-created development administrator
// aligned with permissions and modules added by later migrations.
func refreshLocalAdministratorAccess(databaseURL, credentialsPath string) error {
	if _, err := os.Stat(credentialsPath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect local login details: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect for local access refresh: %w", err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin local access refresh: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var companyID, userID, roleID string
	err = tx.QueryRow(ctx, `SELECT c.id,u.id,r.id FROM companies c JOIN company_users cu ON cu.company_id=c.id JOIN users u ON u.id=cu.user_id JOIN company_user_roles cur ON cur.company_id=c.id AND cur.user_id=u.id JOIN roles r ON r.company_id=cur.company_id AND r.id=cur.role_id WHERE c.name='CommerceOps Local' AND u.email='admin@commerceops.local' AND r.name='Local Administrator'`).Scan(&companyID, &userID, &roleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return tx.Commit(ctx)
	}
	if err != nil {
		return fmt.Errorf("locate local administrator: %w", err)
	}
	permissionResult, err := tx.Exec(ctx, `INSERT INTO role_permissions(company_id,role_id,permission_key) SELECT $1,$2,key FROM permissions ON CONFLICT DO NOTHING`, companyID, roleID)
	if err != nil {
		return fmt.Errorf("refresh local permissions: %w", err)
	}
	modules := []string{"amazon", "consignments", "flipkart", "inventory", "meesho", "myntra", "returns", "snapdeal", "traceability"}
	moduleResult, err := tx.Exec(ctx, `INSERT INTO module_entitlements(company_id,module_key,enabled) SELECT $1,unnest($2::text[]),true ON CONFLICT DO NOTHING`, companyID, modules)
	if err != nil {
		return fmt.Errorf("refresh local modules: %w", err)
	}
	if permissionResult.RowsAffected()+moduleResult.RowsAffected() > 0 {
		if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(company_id,actor_user_id,action,target_type,target_id,metadata) VALUES($1,$2,'local.access_refreshed','user',$2::uuid::text,jsonb_build_object('permissions_added',$3::bigint,'modules_added',$4::bigint))`, companyID, userID, permissionResult.RowsAffected(), moduleResult.RowsAffected()); err != nil {
			return fmt.Errorf("audit local access refresh: %w", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit local access refresh: %w", err)
	}
	return nil
}

func mergeEnvironment(values map[string]string) []string {
	environment := append([]string{}, os.Environ()...)
	for key, value := range values {
		environment = append(environment, key+"="+value)
	}
	return environment
}

func runCommand(directory string, environment []string, output io.Writer, name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Dir = directory
	command.Env = environment
	command.Stdout = output
	command.Stderr = output
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s failed; see .commerceops-local/launcher.log: %w", name, err)
	}
	return nil
}

func startProcess(directory string, environment []string, output io.Writer, name string, args ...string) (*exec.Cmd, error) {
	command := exec.Command(name, args...)
	command.Dir = directory
	command.Env = environment
	command.Stdout = output
	command.Stderr = output
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	return command, nil
}

func waitForURL(address string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if reachable(address) {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", address)
}

func reachable(address string) bool {
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(address)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == http.StatusOK
}

func stop(root, stateDir string, logFile io.Writer) error {
	if err := stopRecordedProcesses(filepath.Join(stateDir, "processes.json"), logFile); err != nil {
		return err
	}
	values, err := readEnvironment(filepath.Join(root, ".env"))
	if err != nil {
		return err
	}
	if err = validateLocalConfiguration(values); err != nil {
		return err
	}
	if err = runCommand(root, mergeEnvironment(values), logFile, "docker", "compose", "stop", "postgres"); err != nil {
		return err
	}
	notify("CommerceOps stopped", "Local services have been stopped; database data was preserved.")
	return nil
}

func stopRecordedProcesses(path string, logFile io.Writer) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read launcher process state: %w", err)
	}
	var state processState
	if err = json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("decode launcher process state: %w", err)
	}
	if state.ServerStart != "" && processStart(state.ServerPID) == state.ServerStart {
		terminateProcessGroup(state.ServerPID)
	}
	if state.FrontendStart != "" && processStart(state.FrontendPID) == state.FrontendStart {
		terminateProcessGroup(state.FrontendPID)
	}
	if err = os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove launcher process state: %w", err)
	}
	fmt.Fprintln(logFile, "stopped launcher-owned application processes")
	return nil
}

func terminateProcessGroup(pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	for attempts := 0; attempts < 20; attempts++ {
		if syscall.Kill(pid, 0) != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}

func writeProcessState(path string, state processState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err = os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write launcher process state: %w", err)
	}
	return nil
}

func acquireLock(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open launcher lock: %w", err)
	}
	if err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, errors.New("CommerceOps is already starting or stopping")
	}
	return file, nil
}

func installDesktopEntry(root string) error {
	executable, err := os.Executable()
	if err != nil || filepath.Clean(filepath.Dir(executable)) != filepath.Clean(root) {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	directory := filepath.Join(home, ".local", "share", "applications")
	if err = os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	quotedExecutable := "\"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "`", "\\`", "$", "\\$", "%", "%%").Replace(executable) + "\""
	contents := "[Desktop Entry]\nName=CommerceOps\nComment=Open the local CommerceOps web application\nType=Application\nTerminal=false\nExec=" + quotedExecutable + "\nIcon=applications-internet\nActions=Stop;\n\n[Desktop Action Stop]\nName=Stop CommerceOps\nExec=" + quotedExecutable + " --stop\n"
	return os.WriteFile(filepath.Join(directory, "commerceops-local.desktop"), []byte(contents), 0o644)
}

func openBrowser() error {
	command := exec.Command("xdg-open", webURL)
	if err := command.Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}

func notify(title, body string) {
	if _, err := exec.LookPath("notify-send"); err == nil {
		_ = exec.Command("notify-send", title, body).Start()
	}
}

func randomSecret(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate local secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// processStart binds a PID to its Linux process start time, preventing PID-reuse mistakes.
func processStart(pid int) string {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return ""
	}
	end := strings.LastIndex(string(data), ")")
	if end < 0 {
		return ""
	}
	fields := strings.Fields(string(data)[end+1:])
	if len(fields) < 20 {
		return ""
	}
	return fields[19]
}

// A running launcher instance can still be compiling a page; wait instead of restarting it.
func recordedProcessesRunning(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var state processState
	if json.Unmarshal(data, &state) != nil {
		return false
	}
	return state.ServerStart != "" && state.FrontendStart != "" && processStart(state.ServerPID) == state.ServerStart && processStart(state.FrontendPID) == state.FrontendStart
}
