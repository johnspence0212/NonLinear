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
	Model     string `json:"model"`
	Workspace string `json:"workspace"`
}

type RunResult struct {
	Action string `json:"action"`
	Mode   string `json:"mode"`
	Model  string `json:"model,omitempty"`
	Prompt string `json:"prompt"`
	PID    int    `json:"pid,omitempty"`
}

type Status struct {
	Settings
	Models   []string `json:"models"`
	CLI      bool     `json:"cli"`
	CLIError string   `json:"cliError,omitempty"`
}

// Service stores the default model and workspace, and starts the Cursor CLI.
type Service struct {
	Path     string
	LookPath func(string) (string, error)
	Command  func(name string, args ...string) *exec.Cmd
}

func New(dataDir string) *Service {
	return &Service{Path: filepath.Join(dataDir, "cursor.json")}
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
	cfg.Workspace = strings.TrimSpace(cfg.Workspace)
	return cfg, nil
}

func (s *Service) Save(in Settings) (Settings, error) {
	cfg := Settings{
		Model:     strings.TrimSpace(in.Model),
		Workspace: strings.TrimSpace(in.Workspace),
	}
	if cfg.Model != "" && !modelOK(cfg.Model) {
		return Settings{}, fmt.Errorf("%w: model %q", ErrInvalid, cfg.Model)
	}
	if cfg.Workspace != "" {
		abs, err := filepath.Abs(cfg.Workspace)
		if err != nil {
			return Settings{}, fmt.Errorf("%w: workspace", ErrInvalid)
		}
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			return Settings{}, fmt.Errorf("%w: workspace is not a directory", ErrInvalid)
		}
		cfg.Workspace = abs
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
	out := Status{Settings: cfg, Models: []string{}}
	if err != nil {
		out.CLIError = err.Error()
		return out
	}
	bin, err := s.lookPath("agent")
	if err != nil {
		out.CLIError = "cursor cli not found (agent)"
		return out
	}
	out.CLI = true
	models, err := s.listModels(ctx, bin)
	if err != nil {
		out.CLIError = err.Error()
		return out
	}
	out.Models = models
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
	cfg, err := s.Load()
	if err != nil {
		return RunResult{}, err
	}
	if cfg.Workspace == "" {
		return RunResult{}, fmt.Errorf("%w: set a workspace in settings", ErrInvalid)
	}
	bin, err := s.lookPath("agent")
	if err != nil {
		return RunResult{}, fmt.Errorf("%w: cursor cli not found (agent)", ErrInvalid)
	}
	_ = ctx
	args := agentArgs(cfg, prompt, false)
	if term := findTerminal(s.lookPath); term != "" {
		pid, err := s.start(term, gnomeArgs(cfg.Workspace, bin, args), cfg.Workspace)
		if err != nil {
			return RunResult{}, err
		}
		return RunResult{Action: action, Mode: "terminal", Model: cfg.Model, Prompt: prompt, PID: pid}, nil
	}
	printArgs := agentArgs(cfg, prompt, true)
	pid, err := s.start(bin, printArgs, cfg.Workspace)
	if err != nil {
		return RunResult{}, err
	}
	return RunResult{Action: action, Mode: "print", Model: cfg.Model, Prompt: prompt, PID: pid}, nil
}

func (s *Service) start(name string, args []string, dir string) (int, error) {
	cmd := s.command(name, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	logPath := filepath.Join(filepath.Dir(s.Path), "cursor-run.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
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

func agentArgs(cfg Settings, prompt string, print bool) []string {
	args := []string{}
	if print {
		args = append(args, "-p", "--force", "--trust", "--approve-mcps")
	}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	if cfg.Workspace != "" {
		args = append(args, "--workspace", cfg.Workspace)
	}
	args = append(args, prompt)
	return args
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
