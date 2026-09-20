# NonLinear

Local JSON issue tracker. Agents are the primary user.

## Glossary

**Project** — parent entity (`P-{id}`). Owns a Decision Map, Spec, and Implementation Plan. Stage is derived, never stored. A Project id space is separate from issues: `P-6` and `NL-6` can coexist.

**Decision Map** — a Wayfinder map. `kind` is `decision-map`, or the issue still carries the `wayfinder:map` label. Maps still open at `#/map/{id}`. Decision tickets are children (`parentId`).

**Spec** — an issue with `kind=spec`, derived from a map (`derivedFromArtifactId`). Lifecycle: `draft` → `approved` (or `superseded`). Open at `#/spec/{id}`.

**Plan** — an issue with `kind=plan`, derived from an approved spec. Lifecycle: `draft` → `active` → `delivered`. Implementation tickets are children of the plan. Open at `#/plan/{id}`.

**Stage** — derived from map / spec / plan lifecycle, never persisted: `wayfinding`, `ready_for_spec`, `spec_review`, `ready_for_tickets`, `implementing`, `complete`.

**Tags** — classification only (`wayfinder:*`, triage, catalog). They do not encode lifecycle or stage.

**Frontier** — open + unblocked + unclaimed tickets that are not a map, spec, or plan.

**`project` vs `projectId`** — `project` is the existing string slug (default `inbox`). `projectId` points at the Project parent.

## Persistence

`schemaVersion` 1. Missing or `0` is legacy: Open loads without writing. The first mutating save upgrades the whole file atomically (`schemaVersion: 1` plus `projects`). Unknown top-level JSON keys are kept and re-emitted. Schema `>1` is an error and does not write.

Standalone maps with no `projectId` wrap in an implicit Project whose id equals the map id (`P-6` for map `NL-6`). Same map, same parent across loads. The wrap is in memory until the next save.

## Lifecycle actions

Explicit. Not inferred from “zero open tickets”. Skills (`/to-spec`, `/to-tickets`) are a handoff; NonLinear does not run them or fabricate a completed document.

1. Map **ready for spec** → map `ready_for_spec`
2. **Create spec** → draft spec, empty to-spec skeleton, destination copied from the map if present
3. **Approve spec** → spec `approved`
4. **Create implementation plan** (approved spec only) → draft plan
5. **Start implementation** / **mark delivered** → plan `active` / `delivered`
6. Map **route is clear** → map `cleared` (does not invent a spec)
7. **Move to project** → set `projectId` on an issue and descendants, or on every issue currently on a source Project
8. **Delete project** → remove the Project and every issue on it

`advance_to_spec` combines 1+2 in one call: marks the map ready and creates the draft spec. Prefer it for "make the spec".
