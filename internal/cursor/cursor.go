package cursor

import (
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

type RunResult struct {
	Action    string `json:"action"`
	Mode      string `json:"mode"`
	Model     string `json:"model,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Prompt    string `json:"prompt"`
	PID       int    `json:"pid,omitempty"`
}

type Status struct {
	Model     string `json:"model,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	CLI       bool   `json:"cli"`
	CLIError  string `json:"cliError,omitempty"`
}

// Service starts the Cursor CLI. The model comes from the CLI's own config.
// The workspace is the git repo the process was started in.
type Service struct {
	Dir        string
	LookPath   func(string) (string, error)
	Command    func(name string, args ...string) *exec.Cmd
	Getwd      func() (string, error)
	ConfigPath string
}

func New(dataDir string) *Service {
	return &Service{Dir: dataDir}
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

func (s *Service) Status(ctx context.Context) Status {
	_ = ctx
	out := Status{Workspace: s.workspace(), Model: s.cliModel()}
	if _, err := s.lookPath("agent"); err != nil {
		out.CLIError = "cursor cli not found (agent)"
		return out
	}
	out.CLI = true
	return out
}

func (s *Service) Run(ctx context.Context, action string, issue model.IssueView) (RunResult, error) {
	prompt, err := Prompt(action, issue)
	if err != nil {
		return RunResult{}, err
	}
	dir := s.workspace()
	if dir == "" {
		return RunResult{}, fmt.Errorf("%w: could not find the repo", ErrInvalid)
	}
	bin, err := s.lookPath("agent")
	if err != nil {
		return RunResult{}, fmt.Errorf("%w: cursor cli not found (agent)", ErrInvalid)
	}
	model := s.cliModel()
	_ = ctx
	args := agentArgs(prompt, false)
	if term := findTerminal(s.lookPath); term != "" {
		pid, err := s.start(term, gnomeArgs(dir, bin, args), dir)
		if err != nil {
			return RunResult{}, err
		}
		return RunResult{Action: action, Mode: "terminal", Model: model, Workspace: dir, Prompt: prompt, PID: pid}, nil
	}
	pid, err := s.start(bin, agentArgs(prompt, true), dir)
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

func agentArgs(prompt string, print bool) []string {
	args := []string{}
	if print {
		args = append(args, "-p", "--force", "--trust", "--approve-mcps")
	}
	args = append(args, prompt)
	return args
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
