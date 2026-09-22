# Issue tracker: NonLinear

Issues for this repo live in a local NonLinear instance (JSON on disk, MCP + UI). Use the **nonlinear** MCP server for all operations. Do not use GitHub issues, Linear, or `.scratch/` markdown files.

Connect Cursor with `~/.cursor/mcp.json` (or project `.cursor/mcp.json`):

```json
{
  "mcpServers": {
    "nonlinear": {
      "url": "http://localhost:3333/mcp"
    }
  }
}
```

The UI is `http://localhost:3333`. Issue identity is the numeric `id`. Display names look like `NL-12`; always refer to tickets by **title**, wrapping the identifier/link.

## Conventions

- **Create an issue**: MCP `create_issue` with `title` and markdown `body`. Optional `labels`, `parentId`, `project`.
- **Read an issue**: MCP `get_issue` with `id`. Returns body, comments, children, `blockers` (what this waits on), `blocks` (what waits on this), `linked` maps, and `frontier` / `blocked` flags.
- **List issues**: MCP `list_issues`. Filters: `state` (`open`/`closed`), `labels` (AND), `parentId`, `assignee` (`unassigned` for unclaimed), `project`, `query`, `frontier`.
- **Comment**: MCP `add_comment` with `id` and markdown `body`. Edit later with `update_comment` (`commentId` + `body`).
- **Labels**: `list_labels` to see seed + catalog + in-use tags. `create_label` with `label` adds a tag to the catalog (idempotent, strips a leading `#`) so it shows in the UI before any issue uses it. `add_label` with `id` + `label` appends a tag to an issue without replacing existing labels. You can still pass `labels` on `create_issue` / `update_issue`. Canonical triage strings: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`.
- **Close**: MCP `update_issue` with `state: "closed"`, or `resolve_issue` (comment + close).

## When a skill says "publish to the issue tracker"

Call `create_issue`. Parent the ticket structurally: implementation tickets get `parentId` set to the plan's id, Wayfinder tickets to the map's id. A `## Parent` text reference alone does not place the ticket.

## When a skill says "fetch the relevant ticket"

Call `get_issue`.

## Wayfinding operations

Used by `/wayfinder`. The **map** is a single issue with **child** issues as tickets. Blocking is a native `blockedBy` relation on the issue, rendered in the NonLinear UI as a child tree on the map.

- **Map**: `create_issue` with `labels: ["wayfinder:map"]`. Body holds Destination / Notes / Decisions so far / Not yet specified / Out of scope.
- **Child ticket**: `create_issue` with `parentId` set to the map's `id`, `labels: ["wayfinder:<type>"]` where type is `research`, `prototype`, `grilling`, or `task`. Create tickets first, then wire blocking (issues need ids before they can reference each other).
- **Another map on the same Project**: `create_issue` with `labels: ["wayfinder:map"]` and `projectId` set to the Project id. Sibling maps live on the Project; the map UI does not show linked-map edges. `linkedMapId` still creates a map, records an optional bidirectional edge, and attaches it to the source map's Project — prefer `projectId`. `set_linked_maps` (`id` + `mapIds`) only maintains those optional edges (pass `mapIds: []` to unlink). Linking does not nest, group, or cascade-delete. Export of a single map drops edges that pointed at maps outside the bundle.
- **Blocking**: `set_blocked_by` with the child `id` and `issueIds` of the issues that block it. Canonical, UI-visible. A ticket is unblocked when every blocker is `closed`.
- **Frontier query**: `list_frontier` with `parentId` equal to the map's `id`. Returns `{next, issues}` — open, unblocked, unassigned children, maps excluded, ordered by id. Use `next`. Do not pick from `get_issue` children (those include closed tickets). Equivalent: `list_issues` with `parentId`, `state: "open"`, `frontier: true`.
- **Claim**: `claim_issue` with the ticket `id` (optional `assignee`, default `cursor`). The session's first write. An open unassigned ticket is unclaimed.
- **Resolve**: `resolve_issue` with `id` and `answer` (posts a resolution comment and closes). Then `update_issue` the map body to append a context pointer under Decisions so far: ticket **title** as the link text, one-line gist of the answer. Do not restate the full decision on the map.
- **Delete a map**: `delete_issue` with the map `id`. Cascades to every child ticket and strips leftover blocked-by and linked-map edges. Linked maps themselves are not deleted.
- **Export a map**: `export_map` with the map `id`. Returns a `nonlinear.map` JSON bundle (map + descendants, comments, in-map `blockedBy`). Blocked-by edges that pointed outside the map are dropped.
- **Import a map**: `import_map` with that bundle. Allocates new ids and remaps parent / blocked-by edges. The imported map is always top-level. Importing twice creates two maps.
- **Wipe the tracker**: `wipe_db` with `confirm: true`. Resets ids so the next issue is NL-1. Irreversible.

## Project lifecycle

A **Project** (`P-{id}`) is a grouping for Decision Maps → Spec → Implementation Plan. Sibling maps belong to the Project. A Project has no tags. Readiness (`ready_for_spec`, `ready_for_tickets`) is edited on the map via `update_issue` `lifecycle` or **edit map**. Stage is still derived for agents (never stored). Tags are classification only on issues. The UI can launch `/to-spec` and `/to-tickets` through the Cursor CLI. MCP lifecycle calls still do not fill the document.

- **Project**: `list_projects`, `get_project` (`id` is the project id), `create_project` (`title`, optional `destination`, optional `repo`), `update_project` (`id`, optional `title` / `destination` / `repo`). `repo` is the folder Cursor uses for that Project; empty falls back to the server default. Creating a map does not create a Project.
- **Add a map to a Project**: `create_issue` with `labels: ["wayfinder:map"]` and `projectId`. Prefer this over `linkedMapId`.
- **Move onto a project**: `move_to_project` with `projectId` (destination) and either `id` (one issue + descendants) or `fromProjectId` (every issue currently on that Project). Use this to put a standalone map onto a Project.
- **Delete a project**: `delete_project` with the project `id`. Cascades to maps, specs, plans, and tickets on it. Empty projects can be deleted.
- **Ready for spec**: `ready_for_spec` with the map `id`, or `update_issue` with `lifecycle: "ready_for_spec"`. Explicit; not inferred from closed tickets. Same `lifecycle` field sets `ready_for_tickets`, `active`, `cleared`, or `archived` when you need to adjust readiness.
- **Create spec**: `create_spec` with the map `id` (map must be `ready_for_spec`). Draft spec, empty to-spec skeleton, `derivedFromArtifactId` = map. Destination copied from the map body if present.
- **Make the spec in one step**: `advance_to_spec` with the map `id`. Marks ready and creates the draft spec. Fill the SPEC body via `/to-spec`, not the map.
- **Approve spec**: `approve_spec` with the spec `id`. Also sets the source map to `ready_for_tickets`.
- **Create plan**: `create_plan` with an approved spec `id`. Draft plan; implementation tickets are children (`parentId` = plan id). Same `blockedBy` / claim / frontier rules as map tickets.
- **Activate / deliver**: `activate_plan` then `deliver_plan` with the plan `id`. Explicit complete; not inferred from zero open tickets.
- **Route is clear**: `clear_route` with the map `id`. Does not create a spec.

