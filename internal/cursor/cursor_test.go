package cursor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/johnspence0212/NonLinear/internal/model"
)

func TestGitRootAndModelConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "web", "app")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := gitRoot(nested); got != root {
		t.Fatalf("git root %s", got)
	}
	raw := []byte(`{"model":{"modelId":"composer-2","max":true}}`)
	if got := modelFromConfig(raw); got != "composer-2" {
		t.Fatalf("model %q", got)
	}
}

func TestPrompt(t *testing.T) {
	ticket := model.IssueView{Issue: model.Issue{ID: 7, Identifier: "NL-7", Title: "Lock the door", State: model.StateOpen}}
	got, err := Prompt("issue", ticket)
	if err != nil || !strings.Contains(got, "NL-7") || !strings.Contains(got, "id 7") {
		t.Fatalf("issue prompt: %q %v", got, err)
	}
	ticket.Blocked = true
	if _, err := Prompt("issue", ticket); err == nil {
		t.Fatal("blocked ticket should not send")
	}
	m := model.IssueView{Issue: model.Issue{Identifier: "NL-1", Kind: model.KindDecisionMap, Labels: []string{"wayfinder:map"}}}
	specPrompt, err := Prompt("to-spec", m)
	if err != nil || specPrompt != "/to-spec #NL-1" {
		t.Fatalf("to-spec: %q %v", specPrompt, err)
	}
	planPrompt, err := Prompt("to-plan", m)
	if err != nil || planPrompt != "/to-tickets #NL-1" {
		t.Fatalf("to-plan: %q %v", planPrompt, err)
	}
	spec := model.IssueView{Issue: model.Issue{Identifier: "NL-2", Kind: model.KindSpec}}
	tickets, err := Prompt("to-tickets", spec)
	if err != nil || tickets != "/to-tickets #NL-2" {
		t.Fatalf("to-tickets: %q %v", tickets, err)
	}
	if _, err := Prompt("to-tickets", m); err == nil {
		t.Fatal("to-tickets on a map")
	}
}

func TestRunLeavesModelAndWorkspaceToCLI(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(t.TempDir(), "cli-config.json")
	if err := os.WriteFile(cfg, []byte(`{"model":{"id":"composer-2"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(t.TempDir())
	s.ConfigPath = cfg
	s.Getwd = func() (string, error) { return filepath.Join(root, "cmd"), nil }
	if err := os.MkdirAll(filepath.Join(root, "cmd"), 0o755); err != nil {
		t.Fatal(err)
	}
	var gotName string
	var gotArgs []string
	var cmd *exec.Cmd
	s.LookPath = func(name string) (string, error) {
		if name == "agent" {
			return "/bin/agent", nil
		}
		return "", os.ErrNotExist
	}
	s.Command = func(name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = append([]string(nil), args...)
		cmd = exec.Command("true")
		return cmd
	}
	issue := model.IssueView{Issue: model.Issue{ID: 3, Identifier: "NL-3", Title: "Ship it", State: model.StateOpen}}
	res, err := s.Run(t.Context(), "issue", issue)
	if err != nil {
		t.Fatal(err)
	}
	if res.Mode != "print" || res.Model != "composer-2" || res.Workspace != root {
		t.Fatalf("result %+v", res)
	}
	if gotName != "/bin/agent" || contains(gotArgs, "--model") || contains(gotArgs, "--workspace") {
		t.Fatalf("cmd %s %v", gotName, gotArgs)
	}
	if cmd.Dir != root {
		t.Fatalf("dir %s", cmd.Dir)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
