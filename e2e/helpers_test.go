package e2e_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// ── binary resolution ─────────────────────────────────────────────────────────

// repoBin returns an absolute path to ../bin/<name> next to this package (repo
// root), or name for lookup on $PATH if that file does not exist. MODELSRV_BIN
// / SENSOR_BIN still override when set (e.g. CI, make e2e).
func repoBin(name string) string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return name
	}
	dir := filepath.Dir(thisFile)
	relBin := filepath.Join(dir, "..", "bin")
	var names []string
	if runtime.GOOS == "windows" {
		names = []string{name + ".exe", name}
	} else {
		names = []string{name}
	}
	for _, n := range names {
		p, err := filepath.Abs(filepath.Join(relBin, n))
		if err != nil {
			continue
		}
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return name
}

func modelsrvBin() string {
	if b := os.Getenv("MODELSRV_BIN"); b != "" {
		return b
	}
	return repoBin("modelsrv")
}

func sensorBin() string {
	if b := os.Getenv("SENSOR_BIN"); b != "" {
		return b
	}
	return repoBin("modelsrv-git-sensor")
}

// ── networking ────────────────────────────────────────────────────────────────

func freePort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("freePort: %v", err))
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// ── process management ────────────────────────────────────────────────────────

func logPath(name string) string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("e2e-%s.log", name))
}

func startProcess(logFile, name string, args ...string) *exec.Cmd {
	return startProcessInDir("", logFile, name, args...)
}

// startProcessInDir launches a binary with stdout/stderr redirected to logFile.
// When dir is non-empty it becomes the child process's working directory.
func startProcessInDir(dir, logFile, name string, args ...string) *exec.Cmd {
	f, err := os.Create(logFile)
	if err != nil {
		panic(fmt.Sprintf("create log %s: %v", logFile, err))
	}
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdout = f
	cmd.Stderr = f
	if err := cmd.Start(); err != nil {
		_ = f.Close()
		panic(fmt.Sprintf("start %s: %v\n  Is the binary built? Set MODELSRV_BIN / SENSOR_BIN or run `make build-bins`.", name, err))
	}
	return cmd
}

func mustRun(name string, args ...string) {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("%s %v: %v\n%s", name, args, err, strings.TrimSpace(string(out))))
	}
}

func killProc(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// ── readiness ─────────────────────────────────────────────────────────────────

func checkReady(url string) error {
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// ── sensor config ─────────────────────────────────────────────────────────────

type sensorRepoCfg struct {
	Type        string   `yaml:"type"`
	Repo        string   `yaml:"repo"`
	Branch      string   `yaml:"branch"`
	CheckoutDir string   `yaml:"checkoutDir"`
	DeployKey   string   `yaml:"deployKey,omitempty"`
	Paths       []string `yaml:"paths"`
}

type sensorCfg struct {
	Subscribers []string        `yaml:"subscribers"`
	Watch       bool            `yaml:"watch"`
	Repos       []sensorRepoCfg `yaml:"repos"`
}

// loadBaseConfig reads testdata/sensor-e2e.yaml (repo URL, branch, watch,
// paths, etc.). BeforeSuite clones the remote repo; buildRuntimeSensorConfig
// turns that into a temp sensor config pointing at the local clone.
func loadBaseConfig() sensorCfg {
	b, err := os.ReadFile("testdata/sensor-e2e.yaml")
	if err != nil {
		panic(fmt.Sprintf("read base config: %v\n  Make sure tests run from the e2e/ directory (go test ./e2e/...)", err))
	}
	var cfg sensorCfg
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		panic(fmt.Sprintf("parse base config: %v", err))
	}
	return cfg
}

// buildRuntimeSensorConfig merges base YAML (watch, repo type, branch, paths)
// with the local clone path and subscriber URL, then writes a temp file.
// pathsOverride: when non-nil, replaces paths for every repo (tests use this
// to scan only watchedDir/no_findings vs watchedDir/findings). When nil, keeps
// paths from the YAML (e.g. watchedDir).
func buildRuntimeSensorConfig(base sensorCfg, localRepoDir, subscriberURL string, pathsOverride []string) string {
	cfg := sensorCfg{
		Subscribers: []string{subscriberURL},
		Watch:       base.Watch,
		Repos:       make([]sensorRepoCfg, len(base.Repos)),
	}
	for i, r := range base.Repos {
		paths := r.Paths
		if pathsOverride != nil {
			paths = pathsOverride
		}
		cfg.Repos[i] = sensorRepoCfg{
			Type:        r.Type,
			Repo:        localRepoDir,
			Branch:      r.Branch,
			CheckoutDir: localRepoDir,
			Paths:       paths,
		}
	}
	return writeConfigToTempFile(cfg)
}

func writeConfigToTempFile(cfg sensorCfg) string {
	out, err := yaml.Marshal(&cfg)
	if err != nil {
		panic(fmt.Sprintf("marshal sensor config: %v", err))
	}
	f, err := os.CreateTemp("", "sensor-e2e-*.yaml")
	if err != nil {
		panic(fmt.Sprintf("create temp config: %v", err))
	}
	_, _ = f.Write(out)
	_ = f.Close()
	return f.Name()
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

func httpGetStatus(url string) int {
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return 0
	}
	resp.Body.Close()
	return resp.StatusCode
}

func httpGetList(url string) ([]map[string]any, error) {
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var out []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func registerSubscriber(serverURL, callbackURL string) error {
	body := fmt.Sprintf(`{"callbackUrl": %q}`, callbackURL)
	resp, err := http.Post( //nolint:noctx
		serverURL+"/api/events/register",
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("want 201, got %d: %s", resp.StatusCode, b)
	}
	return nil
}

// ── event collector ───────────────────────────────────────────────────────────

type collectedEvent struct {
	Kind      string `json:"kind"`
	Operation string `json:"operation"`
}

// eventCollector is a minimal HTTP server that captures events pushed to
// POST /events/push, matching the modelsrv subscriber callback contract.
type eventCollector struct {
	mu     sync.Mutex
	events []collectedEvent
	srv    *http.Server
	addr   string
}

func newEventCollector() *eventCollector {
	c := &eventCollector{}
	mux := http.NewServeMux()
	mux.HandleFunc("/events/push", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}
		var ev collectedEvent
		var m map[string]any
		if err := json.Unmarshal(body, &m); err != nil {
			ev = collectedEvent{Kind: "?", Operation: "?"}
		} else {
			if k, ok := m["kind"].(string); ok {
				ev.Kind = k
			}
			switch opv := m["operation"].(type) {
			case string:
				ev.Operation = opv
			default:
				ev.Operation = fmt.Sprint(opv)
			}
		}
		c.mu.Lock()
		c.events = append(c.events, ev)
		c.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/test", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	port := freePort()
	c.addr = fmt.Sprintf("127.0.0.1:%d", port)
	c.srv = &http.Server{Addr: c.addr, Handler: mux}
	go func() { _ = c.srv.ListenAndServe() }()
	return c
}

func (c *eventCollector) URL() string { return "http://" + c.addr }
func (c *eventCollector) stop()       { _ = c.srv.Close() }

func (c *eventCollector) hasEvent(kind, operation string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, ev := range c.events {
		if ev.Kind == kind && ev.Operation == operation {
			return true
		}
	}
	return false
}
