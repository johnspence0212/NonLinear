package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/johnspence0212/NonLinear/internal/cursor"
	"github.com/johnspence0212/NonLinear/internal/store"
	"github.com/johnspence0212/NonLinear/internal/version"
)

func TestIssueLifecycle(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	(&Handler{Store: st}).Register(mux)

	mapIssue := postJSON(t, mux, "/api/issues", map[string]any{
		"title":  "Find the way to a local tracker",
		"labels": []string{"wayfinder:map"},
		"body":   "## Destination\n\nA tracker Cursor can drive.\n",
	})
	if mapIssue["identifier"] != "NL-1" {
		t.Fatalf("%v", mapIssue)
	}
	parentID := int(mapIssue["id"].(float64))

	a := postJSON(t, mux, "/api/issues", map[string]any{
		"title":    "What store?",
		"labels":   []string{"wayfinder:grilling"},
		"parentId": parentID,
	})
	b := postJSON(t, mux, "/api/issues", map[string]any{
		"title":    "How to expose MCP?",
		"labels":   []string{"wayfinder:grilling"},
		"parentId": parentID,
	})
	putJSON(t, mux, "/api/issues/"+itoa(b["id"])+"/blocked-by", map[string]any{
		"issueIds": []any{a["id"]},
	})

	front := getJSON(t, mux, "/api/frontier?parentId="+itoa(mapIssue["id"]))
	issues := front["issues"].([]any)
	if len(issues) != 1 {
		t.Fatalf("frontier: %v", front)
	}
	if issues[0].(map[string]any)["id"] != a["id"] {
		t.Fatalf("expected A, got %v", issues[0])
	}

	postJSON(t, mux, "/api/issues/"+itoa(a["id"])+"/resolve", map[string]any{
		"author": "cursor",
		"answer": "One JSON file, atomic rename.",
	})
	front = getJSON(t, mux, "/api/frontier?parentId="+itoa(mapIssue["id"]))
	issues = front["issues"].([]any)
	if len(issues) != 1 || issues[0].(map[string]any)["id"] != b["id"] {
		t.Fatalf("after resolve, expected B: %v", front)
	}

	health := getJSON(t, mux, "/api/health")
	if health["version"] != version.Version || health["ok"] != true {
		t.Fatalf("health: %v", health)
	}

	deleted := deleteJSON(t, mux, "/api/issues/"+itoa(mapIssue["id"]))
	ids, _ := deleted["deleted"].([]any)
	if len(ids) != 3 {
		t.Fatalf("cascade delete: %v", deleted)
	}

	wipe := postJSON(t, mux, "/api/wipe", map[string]any{"confirm": true})
	if wipe["deleted"] != float64(0) {
		t.Fatalf("wipe after delete: %v", wipe)
	}
	again := postJSON(t, mux, "/api/issues", map[string]any{"title": "Fresh"})
	if again["identifier"] != "NL-1" {
		t.Fatalf("%v", again)
	}
}

func TestUpdateComment(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	(&Handler{Store: st}).Register(mux)

	issue := postJSON(t, mux, "/api/issues", map[string]any{"title": "Ticket"})
	id := itoa(issue["id"])
	withComment := postJSON(t, mux, "/api/issues/"+id+"/comments", map[string]any{
		"author": "me",
		"body":   "first draft",
	})
	comments, _ := withComment["comments"].([]any)
	if len(comments) != 1 {
		t.Fatalf("comments: %v", withComment)
	}
	cid := comments[0].(map[string]any)["id"].(string)
	updated := patchJSON(t, mux, "/api/issues/"+id+"/comments/"+cid, map[string]any{
		"body": "now markdown **preview**",
	})
	got := updated["comments"].([]any)[0].(map[string]any)
	if got["body"] != "now markdown **preview**" {
		t.Fatalf("body: %v", got)
	}
}

func TestMapExportImport(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	(&Handler{Store: st}).Register(mux)

	mapIssue := postJSON(t, mux, "/api/issues", map[string]any{
		"title":  "Find the way",
		"labels": []string{"wayfinder:map"},
	})
	parentID := int(mapIssue["id"].(float64))
	a := postJSON(t, mux, "/api/issues", map[string]any{
		"title":    "What store?",
		"parentId": parentID,
	})
	b := postJSON(t, mux, "/api/issues", map[string]any{
		"title":    "How to expose MCP?",
		"parentId": parentID,
	})
	putJSON(t, mux, "/api/issues/"+itoa(b["id"])+"/blocked-by", map[string]any{
		"issueIds": []any{a["id"]},
	})

	exported := getJSON(t, mux, "/api/issues/"+itoa(mapIssue["id"])+"/export")
	if exported["kind"] != "nonlinear.map" {
		t.Fatalf("kind: %v", exported)
	}
	issues, _ := exported["issues"].([]any)
	if len(issues) != 3 {
		t.Fatalf("export issues: %v", exported)
	}

	ticket := postJSON(t, mux, "/api/issues", map[string]any{"title": "Not a map"})
	req := httptest.NewRequest(http.MethodGet, "/api/issues/"+itoa(ticket["id"])+"/export", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("export ticket: %d %s", rec.Code, rec.Body.String())
	}

	imported := postJSON(t, mux, "/api/import", exported)
	gotMap, _ := imported["map"].(map[string]any)
	if gotMap["id"] == mapIssue["id"] {
		t.Fatalf("imported map should be new: %v", imported)
	}
	if gotMap["title"] != "Find the way" {
		t.Fatalf("title: %v", gotMap)
	}
}

func TestWipeRequiresConfirm(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	(&Handler{Store: st}).Register(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/wipe", bytes.NewReader([]byte(`{"confirm":false}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestCreateAndAddLabelHTTP(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	(&Handler{Store: st}).Register(mux)

	created := postJSON(t, mux, "/api/labels", map[string]any{"label": "#focus"})
	if created["label"] != "focus" || created["created"] != true {
		t.Fatalf("create: %v", created)
	}
	listed := getJSON(t, mux, "/api/labels")
	found := false
	for _, v := range listed["labels"].([]any) {
		if v == "focus" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("list: %v", listed)
	}

	issue := postJSON(t, mux, "/api/issues", map[string]any{"title": "Ticket"})
	tagged := postJSON(t, mux, "/api/issues/"+itoa(issue["id"])+"/labels", map[string]any{"label": "focus"})
	labels, _ := tagged["labels"].([]any)
	if len(labels) != 1 || labels[0] != "focus" {
		t.Fatalf("add: %v", tagged)
	}
}

func TestLinkedMapsHTTP(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	(&Handler{Store: st}).Register(mux)

	origin := postJSON(t, mux, "/api/issues", map[string]any{
		"title":  "Origin",
		"labels": []string{"wayfinder:map"},
	})
	spawned := postJSON(t, mux, "/api/issues", map[string]any{
		"title":       "Spawned",
		"linkedMapId": origin["id"],
	})
	linked, _ := spawned["linked"].([]any)
	if len(linked) != 1 {
		t.Fatalf("spawned linked: %v", spawned)
	}
	got := getJSON(t, mux, "/api/issues/"+itoa(origin["id"]))
	if len(got["linked"].([]any)) != 1 {
		t.Fatalf("origin linked: %v", got)
	}
	putJSON(t, mux, "/api/issues/"+itoa(origin["id"])+"/linked-maps", map[string]any{
		"issueIds": []any{},
	})
	got = getJSON(t, mux, "/api/issues/"+itoa(origin["id"]))
	if linked, ok := got["linked"].([]any); ok && len(linked) != 0 {
		t.Fatalf("unlinked: %v", got)
	}
}

func TestCreateMapDoesNotCreateProjectHTTP(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	(&Handler{Store: st}).Register(mux)

	m := postJSON(t, mux, "/api/issues", map[string]any{
		"title":  "First session",
		"labels": []string{"wayfinder:map"},
	})
	if m["kind"] != "decision-map" {
		t.Fatalf("kind: %v", m)
	}
	if _, ok := m["projectId"]; ok && m["projectId"] != nil {
		t.Fatalf("standalone map created a project: %v", m["projectId"])
	}
	projects := getJSON(t, mux, "/api/projects")
	if list, _ := projects["projects"].([]any); len(list) != 0 {
		t.Fatalf("projects: %v", projects)
	}
}

func TestCreateMapOnProjectHTTP(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	(&Handler{Store: st}).Register(mux)

	p := postJSON(t, mux, "/api/projects", map[string]any{"title": "Idle Frontier"})
	pid := p["id"]
	first := postJSON(t, mux, "/api/issues", map[string]any{
		"title":     "First session",
		"labels":    []string{"wayfinder:map"},
		"projectId": pid,
	})
	second := postJSON(t, mux, "/api/issues", map[string]any{
		"title":     "Second session",
		"labels":    []string{"wayfinder:map"},
		"projectId": pid,
	})
	if first["projectId"] != pid || second["projectId"] != pid {
		t.Fatalf("projectId first=%v second=%v want %v", first["projectId"], second["projectId"], pid)
	}
	got := getJSON(t, mux, "/api/projects/"+itoa(pid))
	maps, _ := got["maps"].([]any)
	if len(maps) != 2 {
		t.Fatalf("maps: %v", got)
	}
}

func TestSecondMapDoesNotInheritSpecHTTP(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	(&Handler{Store: st}).Register(mux)

	p := postJSON(t, mux, "/api/projects", map[string]any{
		"title":       "Idle Frontier",
		"destination": "Project destination",
	})
	first := postJSON(t, mux, "/api/issues", map[string]any{
		"title":     "First session",
		"labels":    []string{"wayfinder:map"},
		"body":      "## Destination\n\nFirst map dest.\n",
		"projectId": p["id"],
	})
	spec := postJSON(t, mux, "/api/issues/"+itoa(first["id"])+"/advance-to-spec", map[string]any{})
	if spec["kind"] != "spec" {
		t.Fatalf("spec: %v", spec)
	}
	second := postJSON(t, mux, "/api/issues", map[string]any{
		"title":     "Second session",
		"labels":    []string{"wayfinder:map"},
		"projectId": p["id"],
	})
	if second["derivedFromArtifactId"] != nil {
		t.Fatalf("new map inherited derivedFrom: %v", second)
	}
	if derived, ok := second["derived"].([]any); ok && len(derived) != 0 {
		t.Fatalf("new map inherited derived: %v", second["derived"])
	}
	gotFirst := getJSON(t, mux, "/api/issues/"+itoa(first["id"]))
	derived, _ := gotFirst["derived"].([]any)
	if len(derived) != 1 {
		t.Fatalf("first map derived: %v", gotFirst["derived"])
	}
	own := postJSON(t, mux, "/api/issues/"+itoa(second["id"])+"/advance-to-spec", map[string]any{})
	if own["kind"] != "spec" || own["derivedFromArtifactId"] != second["id"] {
		t.Fatalf("second spec: %v", own)
	}
	got := getJSON(t, mux, "/api/projects/"+itoa(p["id"]))
	specs, _ := got["specs"].([]any)
	if len(specs) != 2 {
		t.Fatalf("project specs: %v", got["specs"])
	}
	firstPlan := postJSON(t, mux, "/api/issues/"+itoa(first["id"])+"/advance-to-plan", map[string]any{})
	secondPlan := postJSON(t, mux, "/api/issues/"+itoa(second["id"])+"/advance-to-plan", map[string]any{})
	if firstPlan["kind"] != "plan" || secondPlan["kind"] != "plan" || firstPlan["id"] == secondPlan["id"] {
		t.Fatalf("each map should get its own plan: %v %v", firstPlan, secondPlan)
	}
	if firstPlan["derivedFromArtifactId"] == secondPlan["derivedFromArtifactId"] {
		t.Fatalf("plans inherited the same spec: %v %v", firstPlan, secondPlan)
	}
}

func TestProjectLifecycle(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	(&Handler{Store: st}).Register(mux)

	p := postJSON(t, mux, "/api/projects", map[string]any{"title": "Ship"})
	m := postJSON(t, mux, "/api/issues", map[string]any{
		"title":     "Decision map",
		"labels":    []string{"wayfinder:map"},
		"body":      "## Destination\n\nShip it.\n",
		"projectId": p["id"],
	})
	ready := postJSON(t, mux, "/api/issues/"+itoa(m["id"])+"/ready-for-spec", map[string]any{})
	if ready["lifecycle"] != "ready_for_spec" {
		t.Fatalf("ready: %v", ready)
	}
	spec := postJSON(t, mux, "/api/issues/"+itoa(m["id"])+"/create-spec", map[string]any{})
	if spec["kind"] != "spec" || spec["lifecycle"] != "draft" {
		t.Fatalf("spec: %v", spec)
	}
	approved := postJSON(t, mux, "/api/issues/"+itoa(spec["id"])+"/approve", map[string]any{})
	if approved["lifecycle"] != "approved" {
		t.Fatalf("approve: %v", approved)
	}
	plan := postJSON(t, mux, "/api/issues/"+itoa(spec["id"])+"/create-plan", map[string]any{})
	if plan["kind"] != "plan" || plan["lifecycle"] != "draft" {
		t.Fatalf("plan: %v", plan)
	}
	projects := getJSON(t, mux, "/api/projects")
	list, _ := projects["projects"].([]any)
	if len(list) != 1 {
		t.Fatalf("projects: %v", projects)
	}
	pid := int(list[0].(map[string]any)["id"].(float64))
	got := getJSON(t, mux, fmt.Sprintf("/api/projects/%d", pid))
	if got["stage"] != "ready_for_tickets" {
		t.Fatalf("stage: %v", got)
	}
	status := getJSON(t, mux, fmt.Sprintf("/api/projects/%d/status", pid))
	if status["kind"] != "nonlinear.project-status" || status["stage"] != "ready_for_tickets" {
		t.Fatalf("status: %v", status)
	}
	byQuery := getJSON(t, mux, "/api/projects/status?query=Ship")
	if byQuery["id"] != status["id"] {
		t.Fatalf("query status: %v", byQuery)
	}
}

func TestAdvanceToSpecHTTP(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	(&Handler{Store: st}).Register(mux)

	m := postJSON(t, mux, "/api/issues", map[string]any{
		"title":  "Decision map",
		"labels": []string{"wayfinder:map"},
		"body":   "## Destination\n\nShip it.\n",
	})
	spec := postJSON(t, mux, "/api/issues/"+itoa(m["id"])+"/advance-to-spec", map[string]any{})
	if spec["kind"] != "spec" || spec["lifecycle"] != "draft" {
		t.Fatalf("spec: %v", spec)
	}
	again := postJSON(t, mux, "/api/issues/"+itoa(m["id"])+"/ensure-spec", map[string]any{})
	if again["id"] != spec["id"] {
		t.Fatalf("ensure-spec should reuse: %v vs %v", again, spec)
	}
	plan := postJSON(t, mux, "/api/issues/"+itoa(m["id"])+"/advance-to-plan", map[string]any{})
	if plan["kind"] != "plan" || plan["derivedFromArtifactId"] != spec["id"] {
		t.Fatalf("advance-to-plan: %v", plan)
	}
	got := getJSON(t, mux, "/api/issues/"+itoa(m["id"]))
	if got["lifecycle"] != "ready_for_spec" {
		t.Fatalf("map lifecycle: %v", got["lifecycle"])
	}
}

func TestMoveAndDeleteProjectHTTP(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	(&Handler{Store: st}).Register(mux)

	src := postJSON(t, mux, "/api/projects", map[string]any{"title": "Solo"})
	srcID := int(src["id"].(float64))
	postJSON(t, mux, "/api/issues", map[string]any{
		"title":     "Solo loop",
		"labels":    []string{"wayfinder:map"},
		"projectId": srcID,
	})
	dest := postJSON(t, mux, "/api/projects", map[string]any{"title": "Idle Frontier"})
	destID := int(dest["id"].(float64))
	moved := postJSON(t, mux, "/api/projects/move", map[string]any{
		"fromProjectId": srcID,
		"projectId":     destID,
	})
	ids, _ := moved["moved"].([]any)
	if len(ids) != 1 {
		t.Fatalf("moved: %v", moved)
	}
	got := getJSON(t, mux, "/api/projects/"+itoa(destID))
	maps, _ := got["maps"].([]any)
	if len(maps) != 1 {
		t.Fatalf("dest maps: %v", got)
	}
	deleted := deleteJSON(t, mux, "/api/projects/"+itoa(srcID))
	if n, _ := deleted["deleted"].([]any); len(n) != 0 {
		t.Fatalf("empty delete: %v", deleted)
	}
}

func TestCursorRunHTTP(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "db.json"))
	if err != nil {
		t.Fatal(err)
	}
	svc := cursor.New(dir)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	svc.Getwd = func() (string, error) { return root, nil }
	svc.LookPath = func(name string) (string, error) {
		if name == "agent" {
			return "/bin/agent", nil
		}
		return "", os.ErrNotExist
	}
	svc.Command = func(name string, args ...string) *exec.Cmd {
		if len(args) == 1 && args[0] == "models" {
			return exec.Command("sh", "-c", "printf '%s\\n' 'composer-2 (current)' 'gpt-5.4'")
		}
		return exec.Command("true")
	}
	mux := http.NewServeMux()
	(&Handler{Store: st, Cursor: svc}).Register(mux)

	status := getJSON(t, mux, "/api/cursor")
	if status["workspace"] != root || fmt.Sprint(status["models"]) != "[composer-2 gpt-5.4]" {
		t.Fatalf("status: %v", status)
	}
	other := t.TempDir()
	if err := os.Mkdir(filepath.Join(other, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	proj := postJSON(t, mux, "/api/projects", map[string]any{"title": "Other tree", "repo": other})
	if proj["repo"] != other {
		t.Fatalf("project repo: %v", proj)
	}
	cleared := patchJSON(t, mux, "/api/projects/"+itoa(proj["id"]), map[string]any{"repo": ""})
	if cleared["repo"] != nil && cleared["repo"] != "" {
		t.Fatalf("cleared: %v", cleared)
	}
	restored := patchJSON(t, mux, "/api/projects/"+itoa(proj["id"]), map[string]any{"repo": other})
	if restored["repo"] != other {
		t.Fatalf("restored: %v", restored)
	}
	saved := putJSON(t, mux, "/api/cursor", map[string]any{"model": "gpt-5.4"})
	if saved["model"] != "gpt-5.4" {
		t.Fatalf("saved: %v", saved)
	}

	parent := postJSON(t, mux, "/api/issues", map[string]any{
		"title":  "Map",
		"labels": []string{"wayfinder:map"},
	})
	openTicket := postJSON(t, mux, "/api/issues", map[string]any{
		"title":    "Unblocked",
		"parentId": parent["id"],
	})
	blocked := postJSON(t, mux, "/api/issues", map[string]any{
		"title":    "Waiting",
		"parentId": parent["id"],
	})
	putJSON(t, mux, "/api/issues/"+itoa(blocked["id"])+"/blocked-by", map[string]any{
		"issueIds": []any{openTicket["id"]},
	})

	sent := postJSON(t, mux, "/api/cursor/run", map[string]any{"action": "issue", "id": openTicket["id"]})
	if sent["mode"] != "print" || sent["workspace"] != root || sent["model"] != "gpt-5.4" {
		t.Fatalf("sent: %v", sent)
	}
	reqRaw, _ := json.Marshal(map[string]any{"action": "issue", "id": blocked["id"]})
	req := httptest.NewRequest(http.MethodPost, "/api/cursor/run", bytes.NewReader(reqRaw))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("blocked send: %d %s", rec.Code, rec.Body.String())
	}

	specd := postJSON(t, mux, "/api/cursor/run", map[string]any{"action": "to-spec", "id": parent["id"]})
	specIssue, _ := specd["issue"].(map[string]any)
	if specIssue["kind"] != "spec" {
		t.Fatalf("to-spec should create a spec: %v", specd)
	}
	if !strings.Contains(fmt.Sprint(specd["prompt"]), "/to-spec #"+fmt.Sprint(specIssue["identifier"])) {
		t.Fatalf("to-spec prompt should point at the spec: %v", specd)
	}
	planned := postJSON(t, mux, "/api/cursor/run", map[string]any{"action": "to-plan", "id": parent["id"]})
	planIssue, _ := planned["issue"].(map[string]any)
	if planIssue["kind"] != "plan" {
		t.Fatalf("to-plan should create a plan: %v", planned)
	}
	if !strings.Contains(fmt.Sprint(planned["prompt"]), fmt.Sprintf("parentId=%d", int(planIssue["id"].(float64)))) {
		t.Fatalf("to-plan prompt should parent tickets to the plan: %v", planned)
	}
	if planIssue["derivedFromArtifactId"] != specIssue["id"] {
		t.Fatalf("plan should derive from this map's spec: plan=%v spec=%v", planIssue, specIssue)
	}

	onProj := postJSON(t, mux, "/api/issues", map[string]any{
		"title":     "Project map",
		"labels":    []string{"wayfinder:map"},
		"projectId": proj["id"],
	})
	ticket := postJSON(t, mux, "/api/issues", map[string]any{
		"title":    "Work the other tree",
		"parentId": onProj["id"],
	})
	otherSent := postJSON(t, mux, "/api/cursor/run", map[string]any{"action": "issue", "id": ticket["id"]})
	if otherSent["workspace"] != other {
		t.Fatalf("project workspace: %v", otherSent)
	}
}

func postJSON(t *testing.T, h http.Handler, path string, body any) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code >= 300 {
		t.Fatalf("%s %s -> %d %s", http.MethodPost, path, rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func patchJSON(t *testing.T, h http.Handler, path string, body any) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code >= 300 {
		t.Fatalf("PATCH %s -> %d %s", path, rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func putJSON(t *testing.T, h http.Handler, path string, body any) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code >= 300 {
		t.Fatalf("%s -> %d %s", path, rec.Code, rec.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func deleteJSON(t *testing.T, h http.Handler, path string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code >= 300 {
		t.Fatalf("DELETE %s -> %d %s", path, rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func getJSON(t *testing.T, h http.Handler, path string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code >= 300 {
		t.Fatalf("%s -> %d %s", path, rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func itoa(v any) string {
	switch n := v.(type) {
	case float64:
		return fmt.Sprintf("%.0f", n)
	default:
		return fmt.Sprint(v)
	}
}
