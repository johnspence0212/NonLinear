package store

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/johnspence0212/NonLinear/internal/model"
)

var knownDocumentKeys = map[string]bool{
	"schemaVersion": true,
	"nextId":        true,
	"nextProjectId": true,
	"prefix":        true,
	"issues":        true,
	"labels":        true,
	"projects":      true,
}

type leftover map[string]json.RawMessage

func DecodeDocument(raw []byte) (model.DB, leftover, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return emptyDB(), nil, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return model.DB{}, nil, fmt.Errorf("parse db: %w", err)
	}
	version, err := schemaVersionOf(obj)
	if err != nil {
		return model.DB{}, nil, err
	}
	if version > model.SchemaVersion {
		return model.DB{}, nil, fmt.Errorf("unsupported schema version %d", version)
	}
	var db model.DB
	if err := json.Unmarshal(raw, &db); err != nil {
		return model.DB{}, nil, fmt.Errorf("parse db: %w", err)
	}
	extra := leftover{}
	for k, v := range obj {
		if !knownDocumentKeys[k] {
			extra[k] = v
		}
	}
	if len(extra) == 0 {
		extra = nil
	}
	NormalizeDocument(&db)
	return db, extra, nil
}

func EncodeDocument(db model.DB, extra leftover) ([]byte, error) {
	db.SchemaVersion = model.SchemaVersion
	if db.Issues == nil {
		db.Issues = []model.Issue{}
	}
	if db.Labels == nil {
		db.Labels = []string{}
	}
	if db.Projects == nil {
		db.Projects = []model.Project{}
	}
	if db.Prefix == "" {
		db.Prefix = model.DefaultPrefix
	}
	if db.NextID < 1 {
		db.NextID = 1
	}
	if db.NextProjectID < 1 {
		db.NextProjectID = 1
	}
	typed, err := json.Marshal(db)
	if err != nil {
		return nil, err
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(typed, &obj); err != nil {
		return nil, err
	}
	for k, v := range extra {
		if _, exists := obj[k]; !exists {
			obj[k] = v
		}
	}
	return json.MarshalIndent(obj, "", "  ")
}

func NormalizeDocument(db *model.DB) {
	if db.Prefix == "" {
		db.Prefix = model.DefaultPrefix
	}
	if db.NextID < 1 {
		db.NextID = 1
	}
	if db.Issues == nil {
		db.Issues = []model.Issue{}
	}
	if db.Labels == nil {
		db.Labels = []string{}
	}
	if db.Projects == nil {
		db.Projects = []model.Project{}
	}
	for i, p := range db.Projects {
		if p.Identifier == "" {
			db.Projects[i].Identifier = model.ProjectIdentifier(p.ID)
		}
	}
	for i := range db.Issues {
		issue := &db.Issues[i]
		if !model.IsMap(*issue) {
			continue
		}
		if issue.Kind == "" {
			issue.Kind = model.KindDecisionMap
		}
		if issue.Lifecycle == "" {
			issue.Lifecycle = model.MapLifecycleActive
		}
	}
	byID := map[int]model.Issue{}
	for _, issue := range db.Issues {
		byID[issue.ID] = issue
	}
	for i := range db.Issues {
		issue := &db.Issues[i]
		if issue.ProjectID != nil || issue.ParentID == nil {
			continue
		}
		if parent, ok := byID[*issue.ParentID]; ok && parent.ProjectID != nil {
			pid := *parent.ProjectID
			issue.ProjectID = &pid
		}
	}
	maxPID := 0
	for _, p := range db.Projects {
		if p.ID > maxPID {
			maxPID = p.ID
		}
	}
	if db.NextProjectID <= maxPID {
		db.NextProjectID = maxPID + 1
	}
	if db.NextProjectID < 1 {
		db.NextProjectID = 1
	}
	sort.SliceStable(db.Projects, func(i, j int) bool { return db.Projects[i].ID < db.Projects[j].ID })
}

func emptyDB() model.DB {
	return model.DB{
		NextID:        1,
		NextProjectID: 1,
		Prefix:        model.DefaultPrefix,
		Issues:        []model.Issue{},
		Labels:        []string{},
		Projects:      []model.Project{},
	}
}

func schemaVersionOf(obj map[string]json.RawMessage) (int, error) {
	raw, ok := obj["schemaVersion"]
	if !ok || strings.TrimSpace(string(raw)) == "" || string(raw) == "null" {
		return 0, nil
	}
	var n int
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, fmt.Errorf("parse db: schemaVersion: %w", err)
	}
	return n, nil
}
