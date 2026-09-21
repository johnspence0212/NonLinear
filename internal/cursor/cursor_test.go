package cursor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/johnspence0212/NonLinear/internal/model"
)

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

func TestSaveAndRunUsesModel(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	ws := t.TempDir()
	if _, err := s.Save(Settings{Model: "composer-2", Workspace: ws}); err != nil {
		t.Fatal(err)
	}
	var gotName string
	var gotArgs []string
	s.LookPath = func(name string) (string, error) {
		if name == "agent" {
			return "/bin/agent", nil
		}
		return "", os.ErrNotExist
	}
	s.Command = func(name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return exec.Command("true")
	}
	issue := model.IssueView{Issue: model.Issue{ID: 3, Identifier: "NL-3", Title: "Ship it", State: model.StateOpen}}
	res, err := s.Run(t.Context(), "issue", issue)
	if err != nil {
		t.Fatal(err)
	}
	if res.Mode != "print" || res.Model != "composer-2" {
		t.Fatalf("result %+v", res)
	}
	if gotName != "/bin/agent" || !contains(gotArgs, "--model") || !contains(gotArgs, "composer-2") || !contains(gotArgs, "--workspace") {
		t.Fatalf("cmd %s %v", gotName, gotArgs)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "cursor.json"))
	if err != nil || !strings.Contains(string(raw), "composer-2") {
		t.Fatalf("settings: %s %v", raw, err)
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
