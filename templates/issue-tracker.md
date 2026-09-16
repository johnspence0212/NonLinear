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
- **Read an issue**: MCP `get_issue` with `id`. Returns body, comments, children, blockers, and `frontier` / `blocked` flags.
- **List issues**: MCP `list_issues`. Filters: `state` (`open`/`closed`), `labels` (AND), `parentId`, `assignee` (`unassigned` for unclaimed), `project`, `query`, `frontier`.
- **Comment**: MCP `add_comment` with `id` and `body`.
- **Labels**: pass `labels` on `create_issue` / `update_issue`. Canonical triage strings: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`.
- **Close**: MCP `update_issue` with `state: "closed"`, or `resolve_issue` (comment + close).

## When a skill says "publish to the issue tracker"

Call `create_issue`.

## When a skill says "fetch the relevant ticket"

Call `get_issue`.

## Wayfinding operations

Used by `/wayfinder`. The **map** is a single issue with **child** issues as tickets. Blocking is a native `blockedBy` relation on the issue, rendered in the NonLinear UI as a child tree on the map.

- **Map**: `create_issue` with `labels: ["wayfinder:map"]`. Body holds Destination / Notes / Decisions so far / Not yet specified / Out of scope.
- **Child ticket**: `create_issue` with `parentId` set to the map's `id`, `labels: ["wayfinder:<type>"]` where type is `research`, `prototype`, `grilling`, or `task`. Create tickets first, then wire blocking (issues need ids before they can reference each other).
- **Blocking**: `set_blocked_by` with the child `id` and `issueIds` of the issues that block it. Canonical, UI-visible. A ticket is unblocked when every blocker is `closed`.
- **Frontier query**: `list_frontier` with `parentId` equal to the map's `id`. Returns open, unblocked, unassigned children, ordered by id. First result is next. Equivalent: `list_issues` with `parentId`, `state: "open"`, `frontier: true`.
- **Claim**: `claim_issue` with the ticket `id` (optional `assignee`, default `cursor`). The session's first write. An open unassigned ticket is unclaimed.
- **Resolve**: `resolve_issue` with `id` and `answer` (posts a resolution comment and closes). Then `update_issue` the map body to append a context pointer under Decisions so far: ticket **title** as the link text, one-line gist of the answer. Do not restate the full decision on the map.
- **Delete a map**: `delete_issue` with the map `id`. Cascades to every child ticket and strips leftover blocked-by edges.
- **Wipe the tracker**: `wipe_db` with `confirm: true`. Resets ids so the next issue is NL-1. Irreversible.
