package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/johnspence0212/NonLinear/internal/store"
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
