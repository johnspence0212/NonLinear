package model

import (
	"strings"
	"time"
	"unicode"
)

const (
	MapBundleKind    = "nonlinear.map"
	MapBundleVersion = 1
)

// MapBundle is a portable single-map document: the map issue plus every
// descendant, comments included. IDs inside the bundle are local to the
// file; importers allocate new tracker ids.
type MapBundle struct {
	Kind       string    `json:"kind"`
	Version    int       `json:"version"`
	ExportedAt time.Time `json:"exportedAt"`
	RootID     int       `json:"rootId"`
	Issues     []Issue   `json:"issues"`
}

func (b MapBundle) Filename() string {
	name := "map"
	for _, issue := range b.Issues {
		if issue.ID != b.RootID {
			continue
		}
		name = issue.Identifier
		if slug := SlugTitle(issue.Title); slug != "" {
			name += "-" + slug
		}
		break
	}
	if name == "" {
		name = "map"
	}
	return name + ".nlmap.json"
}

func SlugTitle(title string) string {
	var b strings.Builder
	dash := false
	n := 0
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		if n >= 40 {
			break
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			dash = false
			n++
			continue
		}
		if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
			n++
		}
	}
	return strings.Trim(b.String(), "-")
}
