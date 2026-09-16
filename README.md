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
| Next takeable ticket | `list_frontier` |
| Claim | `claim_issue` |
| Resolve | `resolve_issue`, then `update_issue` on the map |

The map UI shows children as a tree: `*` takeable, `.` blocked, `@` claimed, `x` closed.

## MCP tools

`list_issues`, `get_issue`, `create_issue`, `update_issue`, `add_comment`, `set_blocked_by`, `list_frontier`, `claim_issue`, `resolve_issue`.

## Views

Left nav is global. **home** is the boxed dashboard (frontier + maps). Opening a map is `#/map/12` with its own open/frontier/closed/all filters.

| View | Shows |
| --- | --- |
| `home` | Frontier and maps in separate boxes |
| `maps` | Wayfinder maps only |
| `open` | Every open issue |
| `frontier` | Takeable tickets (open, unclaimed, unblocked) |
| `closed` | Closed issues |
| `all` | Everything |

Click a `#tag` in the right rail, on a row, or on an issue chip to filter (`#/tag/wayfinder:grilling`). Click it again or **clear** to drop the filter. New issues composed on that view get the tag.

## Keyboard (UI)

`j` / `k` move, `Enter` open, `Esc` back, `/` focus new issue.
