package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/johnspence0212/NonLinear/internal/cursor"
	"github.com/johnspence0212/NonLinear/internal/model"
	"github.com/johnspence0212/NonLinear/internal/store"
	"github.com/johnspence0212/NonLinear/internal/version"
)

type Handler struct {
	Store  *store.Store
	Cursor *cursor.Service
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", h.health)
	mux.HandleFunc("GET /api/labels", h.labels)
	mux.HandleFunc("POST /api/labels", h.createLabel)
	mux.HandleFunc("GET /api/projects", h.listProjects)
	mux.HandleFunc("POST /api/projects", h.createProject)
	mux.HandleFunc("POST /api/projects/move", h.moveToProject)
	mux.HandleFunc("GET /api/projects/{id}", h.getProject)
	mux.HandleFunc("PATCH /api/projects/{id}", h.updateProject)
	mux.HandleFunc("DELETE /api/projects/{id}", h.deleteProject)
	mux.HandleFunc("GET /api/issues", h.list)
	mux.HandleFunc("POST /api/issues", h.create)
	mux.HandleFunc("GET /api/frontier", h.frontier)
	mux.HandleFunc("GET /api/issues/{id}", h.get)
	mux.HandleFunc("PATCH /api/issues/{id}", h.update)
	mux.HandleFunc("POST /api/issues/{id}/comments", h.comment)
	mux.HandleFunc("PATCH /api/issues/{id}/comments/{cid}", h.updateComment)
	mux.HandleFunc("PUT /api/issues/{id}/blocked-by", h.blockedBy)
	mux.HandleFunc("PUT /api/issues/{id}/linked-maps", h.linkedMaps)
	mux.HandleFunc("POST /api/issues/{id}/labels", h.addLabel)
	mux.HandleFunc("POST /api/issues/{id}/claim", h.claim)
	mux.HandleFunc("POST /api/issues/{id}/resolve", h.resolve)
	mux.HandleFunc("POST /api/issues/{id}/ready-for-spec", h.readyForSpec)
	mux.HandleFunc("POST /api/issues/{id}/clear-route", h.clearRoute)
	mux.HandleFunc("POST /api/issues/{id}/create-spec", h.createSpec)
	mux.HandleFunc("POST /api/issues/{id}/advance-to-spec", h.advanceToSpec)
	mux.HandleFunc("POST /api/issues/{id}/approve", h.approveSpec)
	mux.HandleFunc("POST /api/issues/{id}/create-plan", h.createPlan)
	mux.HandleFunc("POST /api/issues/{id}/activate-plan", h.activatePlan)
	mux.HandleFunc("POST /api/issues/{id}/deliver-plan", h.deliverPlan)
	mux.HandleFunc("DELETE /api/issues/{id}", h.delete)
	mux.HandleFunc("GET /api/issues/{id}/export", h.exportMap)
	mux.HandleFunc("POST /api/import", h.importMap)
	mux.HandleFunc("POST /api/wipe", h.wipe)
	mux.HandleFunc("GET /api/cursor", h.cursorStatus)
	mux.HandleFunc("PUT /api/cursor", h.cursorSave)
	mux.HandleFunc("POST /api/cursor/run", h.cursorRun)
}

func (h *Handler) cursorSvc() *cursor.Service {
	if h.Cursor == nil && h.Store != nil {
		h.Cursor = cursor.New(filepath.Dir(h.Store.Path()))
	}
	return h.Cursor
}

func (h *Handler) cursorStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.cursorSvc().Status(r.Context()))
}

func (h *Handler) cursorSave(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Model string `json:"model"`
	}
	if !decode(w, r, &body) {
		return
	}
	cfg, err := h.cursorSvc().Save(cursor.Settings{Model: body.Model})
	if err != nil {
		writeCursorError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (h *Handler) cursorRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string `json:"action"`
		ID     int    `json:"id"`
	}
	if !decode(w, r, &body) {
		return
	}
	issue, err := h.Store.Get(body.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	result, err := h.cursorSvc().Run(r.Context(), body.Action, issue)
	if err != nil {
		writeCursorError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeCursorError(w http.ResponseWriter, err error) {
	if errors.Is(err, cursor.ErrInvalid) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	workspace := ""
	if svc := h.cursorSvc(); svc != nil {
		workspace = svc.DefaultWorkspace()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"version":   version.Version,
		"data":      h.Store.Path(),
		"issues":    h.Store.Count(),
		"workspace": workspace,
	})
}

func (h *Handler) labels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"labels": h.Store.Labels()})
}

func (h *Handler) createLabel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Label string `json:"label"`
	}
	if !decode(w, r, &body) {
		return
	}
	result, err := h.Store.CreateLabel(body.Label)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, result)
}

func (h *Handler) addLabel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Label string `json:"label"`
	}
	if !decode(w, r, &body) {
		return
	}
	issue, err := h.Store.AddLabel(id, body.Label)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, issue)
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
		Title       string   `json:"title"`
		Body        string   `json:"body"`
		Labels      []string `json:"labels"`
		ParentID    *int     `json:"parentId"`
		LinkedMapID *int     `json:"linkedMapId"`
		Project     string   `json:"project"`
		ProjectID   *int     `json:"projectId"`
		Assignee    *string  `json:"assignee"`
	}
	if !decode(w, r, &body) {
		return
	}
	issue, err := h.Store.Create(store.CreateIssue{
		Title:       body.Title,
		Body:        body.Body,
		Labels:      body.Labels,
		ParentID:    body.ParentID,
		LinkedMapID: body.LinkedMapID,
		Project:     body.Project,
		ProjectID:   body.ProjectID,
		Assignee:    body.Assignee,
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
		Lifecycle   *string   `json:"lifecycle"`
	}
	if !decode(w, r, &body) {
		return
	}
	in := store.UpdateIssue{
		Title:     body.Title,
		Body:      body.Body,
		Labels:    body.Labels,
		State:     body.State,
		Assignee:  body.Assignee,
		Project:   body.Project,
		Lifecycle: body.Lifecycle,
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

func (h *Handler) linkedMaps(w http.ResponseWriter, r *http.Request) {
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
	issue, err := h.Store.SetLinkedMaps(id, body.IssueIDs)
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

func (h *Handler) exportMap(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	bundle, err := h.Store.ExportMap(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	name := strings.ReplaceAll(bundle.Filename(), `"`, "")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	writeJSON(w, http.StatusOK, bundle)
}

func (h *Handler) importMap(w http.ResponseWriter, r *http.Request) {
	var bundle model.MapBundle
	if !decode(w, r, &bundle) {
		return
	}
	result, err := h.Store.ImportMap(bundle)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
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

func (h *Handler) listProjects(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"projects": h.Store.ListProjects()})
}

func (h *Handler) getProject(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	project, err := h.Store.GetProject(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (h *Handler) createProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title       string `json:"title"`
		Destination string `json:"destination"`
		Repo        string `json:"repo"`
	}
	if !decode(w, r, &body) {
		return
	}
	project, err := h.Store.CreateProject(store.CreateProject{Title: body.Title, Destination: body.Destination, Repo: body.Repo})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, project)
}

func (h *Handler) updateProject(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Title       *string `json:"title"`
		Destination *string `json:"destination"`
		Repo        *string `json:"repo"`
	}
	if !decode(w, r, &body) {
		return
	}
	project, err := h.Store.UpdateProject(id, store.UpdateProject{Title: body.Title, Destination: body.Destination, Repo: body.Repo})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (h *Handler) moveToProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID            *int `json:"id"`
		FromProjectID *int `json:"fromProjectId"`
		ProjectID     int  `json:"projectId"`
	}
	if !decode(w, r, &body) {
		return
	}
	result, err := h.Store.MoveToProject(store.MoveToProject{
		ID:            body.ID,
		FromProjectID: body.FromProjectID,
		ProjectID:     body.ProjectID,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) deleteProject(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	result, err := h.Store.DeleteProject(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) readyForSpec(w http.ResponseWriter, r *http.Request) {
	h.issueAction(w, r, h.Store.ReadyForSpec)
}

func (h *Handler) clearRoute(w http.ResponseWriter, r *http.Request) {
	h.issueAction(w, r, h.Store.ClearRoute)
}

func (h *Handler) createSpec(w http.ResponseWriter, r *http.Request) {
	h.issueActionCreated(w, r, h.Store.CreateSpec)
}

func (h *Handler) advanceToSpec(w http.ResponseWriter, r *http.Request) {
	h.issueActionCreated(w, r, h.Store.AdvanceToSpec)
}

func (h *Handler) approveSpec(w http.ResponseWriter, r *http.Request) {
	h.issueAction(w, r, h.Store.ApproveSpec)
}

func (h *Handler) createPlan(w http.ResponseWriter, r *http.Request) {
	h.issueActionCreated(w, r, h.Store.CreatePlan)
}

func (h *Handler) activatePlan(w http.ResponseWriter, r *http.Request) {
	h.issueAction(w, r, h.Store.ActivatePlan)
}

func (h *Handler) deliverPlan(w http.ResponseWriter, r *http.Request) {
	h.issueAction(w, r, h.Store.DeliverPlan)
}

func (h *Handler) issueAction(w http.ResponseWriter, r *http.Request, fn func(int) (model.IssueView, error)) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	issue, err := fn(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, issue)
}

func (h *Handler) issueActionCreated(w http.ResponseWriter, r *http.Request, fn func(int) (model.IssueView, error)) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	issue, err := fn(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, issue)
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
