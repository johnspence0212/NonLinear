package cursor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/johnspence0212/NonLinear/internal/model"
)

var ErrInvalid = errors.New("invalid request")

type Settings struct {
	Model string `json:"model"`
}

type RunResult struct {
	Action    string `json:"action"`
	Mode      string `json:"mode"`
	Model     string `json:"model,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Prompt    string `json:"prompt"`
	PID       int    `json:"pid,omitempty"`
}

type Status struct {
	Model     string   `json:"model,omitempty"`
	Models    []string `json:"models"`
	Workspace string   `json:"workspace,omitempty"`
	CLI       bool     `json:"cli"`
	CLIError  string   `json:"cliError,omitempty"`
}

// Service starts the Cursor CLI. The workspace is the git repo the process
// was started in. The model list comes from `agent models`; a chosen model
// is stored locally and passed as --model.
type Service struct {
	Dir        string
	Path       string
	LookPath   func(string) (string, error)
	Command    func(name string, args ...string) *exec.Cmd
	Getwd      func() (string, error)
	ConfigPath string
}

func New(dataDir string) *Service {
	return &Service{Dir: dataDir, Path: filepath.Join(dataDir, "cursor.json")}
}

func (s *Service) lookPath(name string) (string, error) {
	if s.LookPath != nil {
		return s.LookPath(name)
	}
	return exec.LookPath(name)
}

func (s *Service) command(name string, args ...string) *exec.Cmd {
	if s.Command != nil {
		return s.Command(name, args...)
	}
	return exec.Command(name, args...)
}

func (s *Service) getwd() (string, error) {
	if s.Getwd != nil {
		return s.Getwd()
	}
	return os.Getwd()
}

func (s *Service) Load() (Settings, error) {
	raw, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, err
	}
	var cfg Settings
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Settings{}, err
	}
	cfg.Model = strings.TrimSpace(cfg.Model)
	return cfg, nil
}

func (s *Service) Save(in Settings) (Settings, error) {
	cfg := Settings{Model: strings.TrimSpace(in.Model)}
	if cfg.Model != "" && !modelOK(cfg.Model) {
		return Settings{}, fmt.Errorf("%w: model %q", ErrInvalid, cfg.Model)
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return Settings{}, err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return Settings{}, err
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(s.Path, raw, 0o644); err != nil {
		return Settings{}, err
	}
	return cfg, nil
}

func (s *Service) Status(ctx context.Context) Status {
	cfg, err := s.Load()
	out := Status{Workspace: s.workspace(), Models: []string{}, Model: cfg.Model}
	if err != nil {
		out.CLIError = err.Error()
		return out
	}
	bin, err := s.lookPath("agent")
	if err != nil {
		if out.Model == "" {
			out.Model = s.cliModel()
		}
		out.CLIError = "cursor cli not found (agent)"
		return out
	}
	out.CLI = true
	models, err := s.listModels(ctx, bin)
	if err != nil {
		out.CLIError = err.Error()
		if out.Model == "" {
			out.Model = s.cliModel()
		}
		return out
	}
	out.Models = models
	if out.Model == "" {
		out.Model = s.cliModel()
	}
	return out
}

func (s *Service) listModels(ctx context.Context, bin string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cmd := s.command(bin, "models")
	cmd.Env = os.Environ()
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("listing models timed out")
	case err := <-done:
		if err != nil && buf.Len() == 0 {
			return nil, err
		}
	}
	return ParseModels(buf.String()), nil
}

func (s *Service) Run(ctx context.Context, action string, issue model.IssueView) (RunResult, error) {
	prompt, err := Prompt(action, issue)
	if err != nil {
		return RunResult{}, err
	}
	dir, err := s.workspaceFor(issue)
	if err != nil {
		return RunResult{}, err
	}
	bin, err := s.lookPath("agent")
	if err != nil {
		return RunResult{}, fmt.Errorf("%w: cursor cli not found (agent)", ErrInvalid)
	}
	cfg, err := s.Load()
	if err != nil {
		return RunResult{}, err
	}
	model := cfg.Model
	if model == "" {
		model = s.cliModel()
	}
	_ = ctx
	args := agentArgs(cfg.Model, prompt, false)
	if term := findTerminal(s.lookPath); term != "" {
		pid, err := s.start(term, gnomeArgs(dir, bin, args), dir)
		if err != nil {
			return RunResult{}, err
		}
		return RunResult{Action: action, Mode: "terminal", Model: model, Workspace: dir, Prompt: prompt, PID: pid}, nil
	}
	pid, err := s.start(bin, agentArgs(cfg.Model, prompt, true), dir)
	if err != nil {
		return RunResult{}, err
	}
	return RunResult{Action: action, Mode: "print", Model: model, Workspace: dir, Prompt: prompt, PID: pid}, nil
}

func (s *Service) start(name string, args []string, dir string) (int, error) {
	cmd := s.command(name, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	logPath := filepath.Join(s.Dir, "cursor-run.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	fmt.Fprintf(f, "\n--- %s %s\n", time.Now().UTC().Format(time.RFC3339), name)
	cmd.Stdout = f
	cmd.Stderr = f
	if err := cmd.Start(); err != nil {
		f.Close()
		return 0, err
	}
	go func() {
		_ = cmd.Wait()
		f.Close()
	}()
	if cmd.Process == nil {
		return 0, nil
	}
	return cmd.Process.Pid, nil
}

func findTerminal(look func(string) (string, error)) string {
	path, err := look("gnome-terminal")
	if err != nil || path == "" {
		return ""
	}
	return path
}

func gnomeArgs(dir, bin string, args []string) []string {
	out := []string{}
	if dir != "" {
		out = append(out, "--working-directory="+dir)
	}
	out = append(out, "--", bin)
	out = append(out, args...)
	return out
}

func agentArgs(model, prompt string, print bool) []string {
	args := []string{}
	if print {
		args = append(args, "-p", "--force", "--trust", "--approve-mcps")
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, prompt)
	return args
}

func (s *Service) DefaultWorkspace() string {
	return s.workspace()
}

func (s *Service) workspaceFor(issue model.IssueView) (string, error) {
	if issue.ProjectRef != nil {
		if repo := strings.TrimSpace(issue.ProjectRef.Repo); repo != "" {
			info, err := os.Stat(repo)
			if err != nil || !info.IsDir() {
				return "", fmt.Errorf("%w: project repo is not a directory", ErrInvalid)
			}
			return repo, nil
		}
	}
	dir := s.workspace()
	if dir == "" {
		return "", fmt.Errorf("%w: could not find the repo", ErrInvalid)
	}
	return dir, nil
}

func (s *Service) workspace() string {
	wd, err := s.getwd()
	if err != nil || strings.TrimSpace(wd) == "" {
		return ""
	}
	return gitRoot(wd)
}

func gitRoot(start string) string {
	dir := filepath.Clean(start)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Clean(start)
		}
		dir = parent
	}
}

func (s *Service) cliModel() string {
	path := s.ConfigPath
	if path == "" {
		path = cliConfigPath()
	}
	if path == "" {
		return ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return modelFromConfig(raw)
}

func cliConfigPath() string {
	if dir := strings.TrimSpace(os.Getenv("CURSOR_CONFIG_DIR")); dir != "" {
		return filepath.Join(dir, "cli-config.json")
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, "cursor", "cli-config.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cursor", "cli-config.json")
}

func modelFromConfig(raw []byte) string {
	var doc struct {
		Model json.RawMessage `json:"model"`
	}
	if json.Unmarshal(raw, &doc) != nil || len(doc.Model) == 0 {
		return ""
	}
	var asString string
	if json.Unmarshal(doc.Model, &asString) == nil && modelOK(asString) {
		return asString
	}
	var obj map[string]any
	if json.Unmarshal(doc.Model, &obj) != nil {
		return ""
	}
	for _, key := range []string{"modelId", "id", "slug", "name"} {
		v, _ := obj[key].(string)
		if modelOK(v) {
			return v
		}
	}
	return ""
}

func Prompt(action string, issue model.IssueView) (string, error) {
	switch action {
	case "issue":
		if model.IsArtifact(issue.Issue) {
			return "", fmt.Errorf("%w: not a ticket", ErrInvalid)
		}
		if issue.State != model.StateOpen {
			return "", fmt.Errorf("%w: ticket is not open", ErrInvalid)
		}
		if issue.Blocked {
			return "", fmt.Errorf("%w: ticket is blocked", ErrInvalid)
		}
		return fmt.Sprintf("This ticket is unblocked. Work it in this workspace using the nonlinear issue tracker. get_issue id %d. %s: %s", issue.ID, issue.Identifier, issue.Title), nil
	case "to-spec":
		if !model.IsMap(issue.Issue) {
			return "", fmt.Errorf("%w: not a map", ErrInvalid)
		}
		return "/to-spec #" + issue.Identifier, nil
	case "to-plan":
		if !model.IsMap(issue.Issue) {
			return "", fmt.Errorf("%w: not a map", ErrInvalid)
		}
		return "/to-tickets #" + issue.Identifier, nil
	case "to-tickets":
		if !model.IsSpec(issue.Issue) {
			return "", fmt.Errorf("%w: not a spec", ErrInvalid)
		}
		return "/to-tickets #" + issue.Identifier, nil
	default:
		return "", fmt.Errorf("%w: unknown action", ErrInvalid)
	}
}

func ParseModels(out string) []string {
	seen := map[string]bool{}
	var models []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		if i := strings.Index(line, " ("); i > 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" || strings.HasSuffix(line, ":") || strings.Contains(line, " ") || !modelOK(line) || seen[line] {
			continue
		}
		seen[line] = true
		models = append(models, line)
	}
	if models == nil {
		models = []string{}
	}
	return models
}

func modelOK(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-' || r == ':' || r == '@' || r == '/':
		default:
			return false
		}
	}
	return true
}
