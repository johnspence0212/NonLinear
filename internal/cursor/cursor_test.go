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

func TestParseModels(t *testing.T) {
	got := ParseModels("Available models:\n\n- composer-2 (current)\n- gpt-5.4\n* claude-4.5\nnot a model\n")
	want := []string{"composer-2", "gpt-5.4", "claude-4.5"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("models %v", got)
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
	spec := model.IssueView{Issue: model.Issue{ID: 2, Identifier: "NL-2", Kind: model.KindSpec}}
	specPrompt, err := Prompt("to-spec", spec)
	if err != nil || !strings.Contains(specPrompt, "/to-spec #NL-2") || !strings.Contains(specPrompt, "id 2") {
		t.Fatalf("to-spec: %q %v", specPrompt, err)
	}
	if _, err := Prompt("to-spec", m); err == nil {
		t.Fatal("to-spec on a map")
	}
	plan := model.IssueView{Issue: model.Issue{ID: 3, Identifier: "NL-3", Kind: model.KindPlan}}
	planPrompt, err := Prompt("to-plan", plan)
	if err != nil || !strings.Contains(planPrompt, "/to-tickets #NL-3") || !strings.Contains(planPrompt, "parentId=3") {
		t.Fatalf("to-plan: %q %v", planPrompt, err)
	}
	tickets, err := Prompt("to-tickets", plan)
	if err != nil || !strings.Contains(tickets, "parentId=3") {
		t.Fatalf("to-tickets: %q %v", tickets, err)
	}
	if _, err := Prompt("to-plan", m); err == nil {
		t.Fatal("to-plan on a map")
	}
	if _, err := Prompt("to-tickets", spec); err == nil {
		t.Fatal("to-tickets on a spec")
	}
}

func TestStatusListsCLIModels(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := New(t.TempDir())
	s.Getwd = func() (string, error) { return root, nil }
	s.LookPath = func(name string) (string, error) {
		if name == "agent" {
			return "/bin/agent", nil
		}
		return "", os.ErrNotExist
	}
	s.Command = func(name string, args ...string) *exec.Cmd {
		if len(args) == 1 && args[0] == "models" {
			return exec.Command("sh", "-c", "printf '%s\\n' 'composer-2 (current)' 'gpt-5.4'")
		}
		return exec.Command("true")
	}
	st := s.Status(t.Context())
	if !st.CLI || strings.Join(st.Models, ",") != "composer-2,gpt-5.4" || st.Workspace != root {
		t.Fatalf("status %+v", st)
	}
}

func TestRunUsesSavedModelAndGitRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := New(t.TempDir())
	s.Getwd = func() (string, error) { return filepath.Join(root, "cmd"), nil }
	if err := os.MkdirAll(filepath.Join(root, "cmd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(Settings{Model: "gpt-5.4"}); err != nil {
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
	if res.Mode != "print" || res.Model != "gpt-5.4" || res.Workspace != root {
		t.Fatalf("result %+v", res)
	}
	if gotName != "/bin/agent" || !contains(gotArgs, "--model") || !contains(gotArgs, "gpt-5.4") || contains(gotArgs, "--workspace") {
		t.Fatalf("cmd %s %v", gotName, gotArgs)
	}
	if cmd.Dir != root {
		t.Fatalf("dir %s", cmd.Dir)
	}
}

func TestRunUsesProjectRepo(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	if err := os.Mkdir(filepath.Join(other, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := New(t.TempDir())
	s.Getwd = func() (string, error) { return root, nil }
	var cmd *exec.Cmd
	s.LookPath = func(name string) (string, error) {
		if name == "agent" {
			return "/bin/agent", nil
		}
		return "", os.ErrNotExist
	}
	s.Command = func(name string, args ...string) *exec.Cmd {
		cmd = exec.Command("true")
		return cmd
	}
	issue := model.IssueView{
		Issue:      model.Issue{ID: 3, Identifier: "NL-3", Title: "Ship it", State: model.StateOpen},
		ProjectRef: &model.ProjectSummary{ID: 1, Identifier: "P-1", Title: "Other", Repo: other},
	}
	res, err := s.Run(t.Context(), "issue", issue)
	if err != nil {
		t.Fatal(err)
	}
	if res.Workspace != other {
		t.Fatalf("workspace %s want %s", res.Workspace, other)
	}
	if cmd.Dir != other {
		t.Fatalf("dir %s", cmd.Dir)
	}
}

func TestRunOmitsModelFlagWhenUnset(t *testing.T) {
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
	s.Getwd = func() (string, error) { return root, nil }
	var gotArgs []string
	s.LookPath = func(name string) (string, error) {
		if name == "agent" {
			return "/bin/agent", nil
		}
		return "", os.ErrNotExist
	}
	s.Command = func(name string, args ...string) *exec.Cmd {
		gotArgs = append([]string(nil), args...)
		return exec.Command("true")
	}
	issue := model.IssueView{Issue: model.Issue{ID: 3, Identifier: "NL-3", Title: "Ship it", State: model.StateOpen}}
	res, err := s.Run(t.Context(), "issue", issue)
	if err != nil {
		t.Fatal(err)
	}
	if res.Model != "composer-2" || contains(gotArgs, "--model") || contains(gotArgs, "--workspace") {
		t.Fatalf("result %+v args %v", res, gotArgs)
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
