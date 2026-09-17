package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/johnspence0212/NonLinear/internal/store"
	"github.com/johnspence0212/NonLinear/internal/version"
)

type Handler struct {
	Store *store.Store
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", h.health)
	mux.HandleFunc("GET /api/labels", h.labels)
	mux.HandleFunc("GET /api/issues", h.list)
	mux.HandleFunc("POST /api/issues", h.create)
	mux.HandleFunc("GET /api/frontier", h.frontier)
	mux.HandleFunc("GET /api/issues/{id}", h.get)
	mux.HandleFunc("PATCH /api/issues/{id}", h.update)
	mux.HandleFunc("POST /api/issues/{id}/comments", h.comment)
	mux.HandleFunc("PATCH /api/issues/{id}/comments/{cid}", h.updateComment)
	mux.HandleFunc("PUT /api/issues/{id}/blocked-by", h.blockedBy)
	mux.HandleFunc("POST /api/issues/{id}/claim", h.claim)
	mux.HandleFunc("POST /api/issues/{id}/resolve", h.resolve)
	mux.HandleFunc("DELETE /api/issues/{id}", h.delete)
	mux.HandleFunc("POST /api/wipe", h.wipe)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"version": version.Version,
		"data":    h.Store.Path(),
		"issues":  h.Store.Count(),
	})
}

func (h *Handler) labels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"labels": h.Store.Labels()})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := store.ListFilter{
		State:        q.Get("state"),
		Project:      q.Get("project"),
		Query:        q.Get("query"),
		FrontierOnly: q.Get("frontier") == "1" || q.Get("frontier") == "true",
	}
	if labels := q.Get("labels"); labels != "" {
		filter.Labels = splitCSV(labels)
	}
	if v := q.Get("parentId"); v != "" {
		id, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid parentId")
			return
		}
		filter.ParentID = &id
	}
	if q.Has("assignee") {
		a := q.Get("assignee")
		filter.Assignee = &a
	}
	writeJSON(w, http.StatusOK, map[string]any{"issues": h.Store.List(filter)})
}

func (h *Handler) frontier(w http.ResponseWriter, r *http.Request) {
	var parent *int
	if v := r.URL.Query().Get("parentId"); v != "" {
		id, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid parentId")
			return
		}
		parent = &id
	}
	writeJSON(w, http.StatusOK, map[string]any{"issues": h.Store.Frontier(parent)})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title    string   `json:"title"`
		Body     string   `json:"body"`
		Labels   []string `json:"labels"`
		ParentID *int     `json:"parentId"`
		Project  string   `json:"project"`
		Assignee *string  `json:"assignee"`
	}
	if !decode(w, r, &body) {
		return
	}
	issue, err := h.Store.Create(store.CreateIssue{
		Title:    body.Title,
		Body:     body.Body,
		Labels:   body.Labels,
		ParentID: body.ParentID,
		Project:  body.Project,
		Assignee: body.Assignee,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, issue)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	issue, err := h.Store.Get(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, issue)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Title       *string   `json:"title"`
		Body        *string   `json:"body"`
		Labels      *[]string `json:"labels"`
		State       *string   `json:"state"`
		Assignee    *string   `json:"assignee"`
		ParentID    *int      `json:"parentId"`
		ClearParent bool      `json:"clearParent"`
		Project     *string   `json:"project"`
	}
	if !decode(w, r, &body) {
		return
	}
	in := store.UpdateIssue{
		Title:    body.Title,
		Body:     body.Body,
		Labels:   body.Labels,
		State:    body.State,
		Assignee: body.Assignee,
		Project:  body.Project,
	}
	if body.ClearParent {
		var none *int
		in.ParentID = &none
	} else if body.ParentID != nil {
		p := body.ParentID
		in.ParentID = &p
	}
	issue, err := h.Store.Update(id, in)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, issue)
}

func (h *Handler) comment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Author string `json:"author"`
		Body   string `json:"body"`
	}
	if !decode(w, r, &body) {
		return
	}
	issue, err := h.Store.AddComment(id, body.Author, body.Body)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, issue)
}

func (h *Handler) updateComment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Body string `json:"body"`
	}
	if !decode(w, r, &body) {
		return
	}
	issue, err := h.Store.UpdateComment(id, r.PathValue("cid"), body.Body)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, issue)
}

func (h *Handler) blockedBy(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		IssueIDs []int `json:"issueIds"`
	}
	if !decode(w, r, &body) {
		return
	}
	issue, err := h.Store.SetBlockedBy(id, body.IssueIDs)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, issue)
}

func (h *Handler) claim(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Assignee string `json:"assignee"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	issue, err := h.Store.Claim(id, body.Assignee)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, issue)
}

func (h *Handler) resolve(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Author string `json:"author"`
		Answer string `json:"answer"`
	}
	if !decode(w, r, &body) {
		return
	}
	issue, err := h.Store.Resolve(id, body.Author, body.Answer)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, issue)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	result, err := h.Store.Delete(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) wipe(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm bool `json:"confirm"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !body.Confirm {
		writeError(w, http.StatusBadRequest, "confirm must be true")
		return
	}
	n, err := h.Store.Wipe()
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": n, "version": version.Version})
}

func pathID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, dest any) bool {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dest); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return false
	}
	return true
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := []string{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, store.ErrInvalid), errors.Is(err, store.ErrParentCycle), errors.Is(err, store.ErrSelfRelation):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
