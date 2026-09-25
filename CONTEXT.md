# NonLinear

Local JSON issue tracker. Agents are the primary user.

## Glossary

**Project** — parent entity (`P-{id}`). Owns Decision Maps, Spec, and Tickets. Sibling maps belong here; that is the grouping, not linked-map edges. Stage is derived, never stored. A Project id space is separate from issues: `P-6` and `NL-6` can coexist.

**Decision Map** — a Wayfinder map. `kind` is `decision-map`, or the issue still carries the `wayfinder:map` label. Maps still open at `#/map/{id}`. Decision tickets are children (`parentId`). A Project can have several maps.

**Linked maps** — optional bidirectional edges (`linkedMaps`). They do not nest, group, or cascade. The map UI does not show them. Related maps share a `projectId`.

**Spec** — an issue with `kind=spec`, derived from a map (`derivedFromArtifactId`). Lifecycle: `draft` → `approved` (or `superseded`). Open at `#/spec/{id}`.

**Tickets** — an issue with `kind=plan`, derived from an approved spec. Lifecycle: `draft` → `active` → `delivered`. Implementation tickets are children of this list. Open at `#/plan/{id}`. The UI calls this Tickets; **to tickets** creates it and runs `/to-tickets`.

**Stage** — derived from map / spec / plan lifecycle, never persisted: `wayfinding`, `ready_for_spec`, `spec_review`, `ready_for_tickets`, `implementing`, `complete`.

**Project status** — compact JSON snapshot (`kind: nonlinear.project-status`) from MCP `get_project_status` (`project: "P-8"`) or `GET /api/projects/{id}/status`. Stage, counts, frontier/claimed/blocked, `next`, `nextAction`. No bodies or comments. “Give me the status of P-8” is this tool.

**Activity** — append-only event log on `db.json` (`events`, newest first, capped at 200). Written on create / claim / resolve / comment / block / lifecycle. Home is `GET /api/home`: claimed, frontier, blocked, events. The UI pages each list 5 at a time.

**Tags** — classification only (`wayfinder:*`, triage, catalog). They do not encode lifecycle or stage.

**Frontier** — open + unblocked + unclaimed tickets that are not a map, spec, or plan.

**`project` vs `projectId`** — `project` is the existing string slug (default `inbox`). `projectId` points at the Project parent.

## Persistence

`schemaVersion` 1. Missing or `0` is legacy: Open loads without writing. The first mutating save upgrades the whole file atomically (`schemaVersion: 1` plus `projects`). Unknown top-level JSON keys are kept and re-emitted. Schema `>1` is an error and does not write.

A map with no `projectId` stays off every Project. Creating a map does not invent a Project. Attach it with `projectId` on create, or `move_to_project`.

## Lifecycle actions

Explicit. Not inferred from “zero open tickets”. MCP lifecycle actions do not run skills or fabricate a completed document.

The UI can hand work to the Cursor CLI (`agent`). Settings lists models from `agent models` and stores the chosen one. A Project can set an optional `repo` (the folder Cursor uses for that project). Otherwise the git root of the directory NonLinear was started in is the default. The footer shows the repo in play. **to spec** on a map creates that map's spec if needed, then runs `/to-spec` on the spec. **to tickets** creates that map's spec and Tickets list if needed, then runs `/to-tickets` so tickets parent to that list. **approve spec** approves, creates Tickets, then runs `/to-tickets`. An open unblocked ticket can be **sent to cursor**.

1. Map **ready for spec** → map `ready_for_spec`
2. **Create spec** → draft spec, empty to-spec skeleton, destination copied from the map if present
3. **Approve spec** → spec `approved`
4. **Create tickets** (approved spec only) → draft Tickets list (`kind=plan`)
5. **Start implementation** / **mark delivered** → plan `active` / `delivered`
6. Map **route is clear** → map `cleared` (does not invent a spec)
7. **Move to project** → set `projectId` on an issue and descendants, or on every issue currently on a source Project
8. **Delete project** → remove the Project and every issue on it

`advance_to_spec` combines 1+2 in one call: marks the map ready and creates the draft spec. Prefer it for "make the spec". `advance_to_plan` creates that map's spec if needed, approves it, and creates a new plan. A second map does not inherit another map's spec or plan.
