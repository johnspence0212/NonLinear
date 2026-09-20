# NonLinear

A local issue tracker for Cursor. JSON on disk, a Streamable HTTP MCP server, and a sparse UI.

Built so [Matt Pocock’s `/wayfinder`](https://github.com/mattpocock/skills/blob/main/skills/engineering/wayfinder/SKILL.md) can chart maps and decision tickets without GitHub, Linear, or markdown files in your repo. Agents are the primary user. You can type.

Deleting the data directory deletes the issues. That is expected.

## Why there is no Node

The server is a **single Go binary**. Work Node versions do not matter. If you can run a file (or Docker), you can run NonLinear.

## Run

```bash
go run ./cmd/nonlinear
```

Or build once and copy the binary wherever you need it:

```bash
go build -o nonlinear ./cmd/nonlinear
./nonlinear
```

Cross-compile without a local Go toolchain on the destination machine:

```bash
make dist
# dist/nonlinear-linux-amd64
# dist/nonlinear-darwin-arm64
# dist/nonlinear-windows-amd64.exe
```

Docker:

```bash
docker compose up --build
```

Then open the **app window** (not a browser tab):

```bash
./nonlinear -open
```

That starts the server and launches Chrome/Chromium/Edge in `--app` mode: its own window, no tabs. Same thing as installing the PWA (Chrome → ⊙ / Install nonlinear). After install, nonlinear lives in the dock like any other app.

Or visit [http://127.0.0.1:3333](http://127.0.0.1:3333) once and use Install.

| Flag / env | Default | Meaning |
| --- | --- | --- |
| `-addr` / `ADDR` / `PORT` | `:3333` | Listen address (`PORT` becomes `:<port>`) |
| `-data` / `DATA_DIR` | `./data` | Directory for `db.json` |
| `-token` / `NL_TOKEN` | empty | Optional bearer token for `/api` and `/mcp` |
| `-open` / `NL_OPEN=1` | off | Dedicated Chromium app window |

## Cursor MCP

Keep NonLinear running, then add to `~/.cursor/mcp.json` (global, all workspaces) or `.cursor/mcp.json` in a repo:

```json
{
  "mcpServers": {
    "nonlinear": {
      "url": "http://localhost:3333/mcp"
    }
  }
}
```

If `NL_TOKEN` is set, send it as `Authorization: Bearer …` in the MCP config `headers`.

## Wayfinder

Copy [`templates/issue-tracker.md`](templates/issue-tracker.md) to `docs/agents/issue-tracker.md` in the repo you are wayfinding. Point `AGENTS.md` / `CLAUDE.md` at it:

```markdown
## Agent skills

### Issue tracker

Issues live in local NonLinear over MCP. See `docs/agents/issue-tracker.md`.
```

Wayfinder operations on this tracker:

| Move | MCP tool |
| --- | --- |
| Create map | `create_issue` labels `wayfinder:map` |
| Create child ticket | `create_issue` with `parentId` |
| Wire blocking | `set_blocked_by` |
| Next frontier ticket | `list_frontier` (`next` is the one to claim) |
| Claim | `claim_issue` |
| Resolve | `resolve_issue`, then `update_issue` on the map |
| Export a map | `export_map` |
| Import a map | `import_map` |
| Link maps | `create_issue` with `linkedMapId`, or `set_linked_maps` |

The map UI shows children as a tree: `*` frontier, `.` blocked, `@` claimed, `x` closed. Blocked tickets list what they wait on. Opening a ticket shows **blocked by** (and **blocks**) as clickable rows.

## Project lifecycle

A **Project** (`P-{id}`) is the parent of a Decision Map → Spec → Implementation Plan. Stage is derived from those artifacts; tags stay classification-only. Existing `db.json` files load unchanged (`schemaVersion` missing/`0`). The first save writes `schemaVersion: 1`. Maps still open at `#/map/{id}`. Project view is `#/project/{id}`. Specs and plans are `#/spec/{id}` and `#/plan/{id}`.

| Move | MCP tool |
| --- | --- |
| List / get / create project | `list_projects`, `get_project`, `create_project` |
| Move issues onto another project | `move_to_project` (`id` + descendants, or `fromProjectId` for the whole project) |
| Delete a project | `delete_project` (cascades to maps, specs, plans, tickets) |
| Map ready for spec | `ready_for_spec` |
| Create spec (empty to-spec skeleton; does not run `/to-spec`) | `create_spec` |
| Make the spec in one step (ready + draft spec) | `advance_to_spec` |
| Approve spec | `approve_spec` |
| Create implementation plan (does not run `/to-tickets`; parent each ticket to the plan id) | `create_plan` |
| Start / deliver plan | `activate_plan`, `deliver_plan` |
| Route is clear | `clear_route` |

## MCP tools

`list_issues`, `get_issue`, `create_issue`, `update_issue`, `add_comment`, `update_comment`, `set_blocked_by`, `set_linked_maps`, `list_frontier`, `claim_issue`, `resolve_issue`, `delete_issue` (map + all children), `export_map`, `import_map`, `list_labels`, `create_label`, `add_label`, `list_projects`, `get_project`, `create_project`, `move_to_project`, `delete_project`, `ready_for_spec`, `create_spec`, `advance_to_spec`, `approve_spec`, `create_plan`, `activate_plan`, `deliver_plan`, `clear_route`, `wipe_db` (`confirm: true`).

`GET /api/health` returns `{ ok, version, data, issues }` so a later client can detect an update. The UI footer and **settings** show the same version. Wipe the database from settings (two clicks). Delete a map from the map view; children go with it. Delete a Project from the project view (two clicks); every issue on it goes with it. Move a Project's issues onto another Project from the project view. Export a map from the map view; import a `.nlmap.json` from maps, home, or settings.

## Views

Left nav is global. **home** is the boxed dashboard (frontier tickets grouped by map, plus projects and maps). A **Project** is the parent of Decision Map → Spec → Plan. Maps still open at `#/map/12`. From a map you can start another map as a **linked map**: both stay top-level (delete does not cascade) and each shows the other.

| View | Shows |
| --- | --- |
| `home` | Stats strip, then collapsible **projects**, **maps**, and **frontier** (open/unclaimed tickets grouped by map) |
| `projects` | Every Project (identifier `P-{id}`, derived stage, destination). Compose creates a Project. Open `#/project/{id}` |
| `maps` | Wayfinder maps only (with a new-map box and import) |
| `open` | All unfinished tickets, grouped by map, split into frontier / claimed / waiting on a blocker |
| `frontier` | Only frontier tickets: open, unblocked, unclaimed (not a map, spec, or plan) |
| `closed` / `all` | Tickets only, never maps, grouped by parent map; map-less tickets land under inbox. No compose here — add tickets from inside a map |
| `#/project/{id}` | Project destination, derived stage, Decision Map / Spec / Plan rows |
| `#/spec/{id}` | Spec body, approve, create implementation plan |
| `#/plan/{id}` | Plan body plus ticket list / compose / blockers (same as a map's tickets) |
| `settings` | Version, data path, import a map file, wipe the database |

Click a `#tag` in the right rail, on a row, or on an issue chip to filter (`#/tag/wayfinder:grilling`). Click it again or **clear** to drop the filter. New maps composed on the `#wayfinder:map` tag view get the tag. MCP `create_label` adds a tag to the catalog so it shows in the rail before any issue uses it; `add_label` puts a tag on an issue.

Export a map from the map view (map + tickets + comments + in-map blockers) as a `.nlmap.json` file. Import that file from **maps**, **home**, or **settings** — ids are remapped so it lands as a new map, even on the same tracker.

The header search (`/`) matches identifier, title, and body across maps and tickets, split into maps + tickets-by-map boxes.

## Keyboard (UI)

`j` / `k` move, `Enter` open, `Esc` back, `r` refresh, `/` focus search.
