const state = {
  filter: localStorage.getItem("nl-filter") || "home",
  mapChildFilter: localStorage.getItem("nl-map-filter") || "open",
  issues: [],
  projects: [],
  selected: 0,
  error: "",
  notice: "",
  busy: "",
  stats: { all: 0, open: 0, closed: 0, maps: 0, projects: 0, frontier: 0, blocked: 0 },
  labels: [],
  version: "",
  dataPath: "",
  defaultRepo: "",
  viewRepo: "",
  issueBackHref: "#/",
};

const $ = (sel, el = document) => el.querySelector(sel);
const main = $("#main");
const rail = $("#rail");

function route() {
  const hash = location.hash.replace(/^#/, "") || "/";
  const map = hash.match(/^\/map\/(\d+)$/);
  if (map) return { name: "map", id: Number(map[1]) };
  const spec = hash.match(/^\/spec\/(\d+)$/);
  if (spec) return { name: "spec", id: Number(spec[1]) };
  const plan = hash.match(/^\/plan\/(\d+)$/);
  if (plan) return { name: "plan", id: Number(plan[1]) };
  const project = hash.match(/^\/project\/(\d+)$/);
  if (project) return { name: "project", id: Number(project[1]) };
  const tag = hash.match(/^\/tag\/(.+)$/);
  if (tag) return { name: "tag", label: decodeURIComponent(tag[1]) };
  const search = hash.match(/^\/search\/(.+)$/);
  if (search) return { name: "search", query: decodeURIComponent(search[1]) };
  if (hash === "/settings") return { name: "settings" };
  const issue = hash.match(/^\/(\d+)$/);
  if (issue) return { name: "issue", id: Number(issue[1]) };
  return { name: "list" };
}

function activeTag() {
  const r = route();
  return r.name === "tag" ? r.label : "";
}

function tagHref(label) {
  return "#/tag/" + encodeURIComponent(label);
}

function tagButtons(labels) {
  const on = activeTag();
  return (labels || [])
    .map(
      (l) =>
        `<button type="button" class="tag ${on === l ? "active" : ""}" data-tag="${esc(l)}">#${esc(l)}</button>`
    )
    .join(" ");
}

function isMap(issue) {
  return issue.kind === "decision-map" || (issue.labels || []).includes("wayfinder:map");
}

function isSpec(issue) {
  return issue.kind === "spec";
}

function isPlan(issue) {
  return issue.kind === "plan";
}

function isArtifact(issue) {
  return isMap(issue) || isSpec(issue) || isPlan(issue);
}

function hrefFor(issue) {
  if (isMap(issue)) return `#/map/${issue.id}`;
  if (isSpec(issue)) return `#/spec/${issue.id}`;
  if (isPlan(issue)) return `#/plan/${issue.id}`;
  return `#/${issue.id}`;
}

function listCursor() {
  const r = route();
  if (r.name === "list" && state.filter === "projects") return state.projects;
  return state.issues;
}

function listHref(item) {
  if (item && item.stage && (item.identifier || "").startsWith("P-")) return projectHref(item);
  return hrefFor(item);
}

async function api(path, opts = {}) {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json", ...(opts.headers || {}) },
    ...opts,
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : {};
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

function mark(issue) {
  if (issue.state === "closed") return { cls: "closed", ch: "x" };
  if (issue.frontier) return { cls: "take", ch: "*" };
  if (issue.assignee) return { cls: "claim", ch: "@" };
  if (issue.blocked) return { cls: "blocked", ch: "." };
  return { cls: "", ch: "·" };
}

function sortRelations(items) {
  return [...(items || [])].sort((a, b) => {
    if (a.state !== b.state) return a.state === "open" ? -1 : 1;
    return a.id - b.id;
  });
}

function relCounts(items) {
  const open = items.filter((i) => i.state !== "closed").length;
  const closed = items.length - open;
  if (!items.length) return "";
  return [open ? `${open} open` : "", closed ? `${closed} closed` : ""].filter(Boolean).join(" · ");
}

function takeability(issue) {
  if (isMap(issue)) return issue.state === "closed" ? "closed" : "map";
  if (isSpec(issue)) return issue.state === "closed" ? "closed" : "spec";
  if (isPlan(issue)) return issue.state === "closed" ? "closed" : "plan";
  if (issue.state === "closed") return "closed";
  if (issue.frontier) return "frontier";
  if (issue.assignee) return "claimed";
  return "blocked";
}

function stamp(label, kind = "") {
  return `<span class="stamp ${kind}">${esc(label)}</span>`;
}

function statusStamp(issue, lg = "") {
  const size = lg ? ` ${lg}` : "";
  const k = takeability(issue);
  if (k === "closed") return stamp("closed", `closed${size}`);
  if (k === "map") return stamp("map", `open${size}`);
  if (k === "spec") return stamp(issue.lifecycle || "spec", `open${size}`);
  if (k === "plan") return stamp(issue.lifecycle || "plan", `open${size}`);
  if (k === "blocked") return stamp("blocked", `blocked${size}`);
  if (k === "claimed") return stamp(issue.assignee ? `@${issue.assignee}` : "claimed", `claim${size}`);
  return stamp("frontier", `take${size}`);
}

function rowHTML(issue, selected, nav = true) {
  const m = mark(issue);
  const closed = issue.state === "closed";
  return `<div class="row ${selected ? "selected" : ""} ${closed ? "is-closed" : ""}" ${nav ? "data-nav" : ""} data-id="${issue.id}">
    <a class="id" href="${hrefFor(issue)}">${issue.identifier}</a>
    <a class="title" href="${hrefFor(issue)}">${esc(issue.title)}</a>
    <span class="meta">${tagButtons(issue.labels)}</span>
    <span class="mark ${m.cls}">${statusStamp(issue)}</span>
  </div>`;
}

function ticketHTML(issue, selected, nav = true) {
  return rowHTML(issue, selected, nav);
}

function relationBox(title, items, empty) {
  const list = sortRelations(items);
  return box(
    `<strong>${title}</strong><span>${relCounts(list)}</span>`,
    list.map((i) => rowHTML(i, false, false)).join("") || `<div class="empty">${empty}</div>`
  );
}

function flash() {
  const err = state.error ? `<div class="error">${esc(state.error)}</div>` : "";
  const note = state.notice ? `<div class="notice">${esc(state.notice)}</div>` : "";
  state.notice = "";
  return err + note;
}

function busyLine(msg) {
  return `<div class="busy" role="status"><span class="spin" aria-hidden="true"></span>${esc(msg)}</div>`;
}

function setBusy(msg) {
  state.busy = msg || "";
  if (!state.busy) {
    document.body.classList.remove("is-busy");
    main.querySelector(".busy")?.remove();
    return;
  }
  document.body.classList.add("is-busy");
  const el = main.querySelector(".busy");
  if (el) {
    el.outerHTML = busyLine(state.busy);
    return;
  }
  main.insertAdjacentHTML("afterbegin", busyLine(state.busy));
}

function armBusyButton(kind, msg) {
  const btn = main.querySelector(`[data-act="${kind}"]`);
  if (btn) {
    btn.innerHTML = `<span class="spin" aria-hidden="true"></span>${esc(msg)}`;
    btn.classList.add("is-waiting");
  }
  setBusy(msg);
}

function showSettingsLoading() {
  if (main.querySelector(".settings-meta")) {
    setBusy("asking the cursor cli…");
    return;
  }
  document.body.classList.add("is-busy");
  main.innerHTML = box("<strong>settings</strong>", `<div class="box-b pad">${busyLine("asking the cursor cli…")}</div>`);
  syncChrome();
}

async function holdBusy(started, minMs = 700) {
  const wait = minMs - (Date.now() - started);
  if (wait > 0) await new Promise((r) => setTimeout(r, wait));
}

async function sendCursor(action, id) {
  const wait = {
    issue: "sending to cursor…",
    "to-spec": "starting to spec…",
    "to-plan": "starting to plan…",
    "to-tickets": "starting to tickets…",
  };
  const started = Date.now();
  setBusy(wait[action] || "talking to cursor…");
  try {
    const res = await api("/api/cursor/run", {
      method: "POST",
      body: JSON.stringify({ action, id }),
    });
    await holdBusy(started);
    const model = res.model ? ` · ${res.model}` : "";
    state.notice = res.mode === "terminal" ? `opened cursor${model}` : `sent to cursor${model}`;
  } catch (err) {
    await holdBusy(started);
    throw err;
  }
}

function esc(s) {
  return String(s ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

function backHref(issue) {
  if (issue.parent && isMap(issue.parent)) return `#/map/${issue.parent.id}`;
  if (issue.parent && isPlan(issue.parent)) return `#/plan/${issue.parent.id}`;
  if (issue.parent) return hrefFor(issue.parent);
  if (issue.projectRef) return `#/project/${issue.projectRef.id}`;
  return "#/";
}

function box(titleRight, inner, footer = "") {
  return `<section class="box"><div class="box-h">${titleRight}</div><div class="box-b">${inner}</div>${footer}</section>`;
}

function composeBar(label, formId = "compose", kind = "issue") {
  const fields =
    kind === "project"
      ? `<label class="edit-label">title<input name="title" autocomplete="off" /></label>
        <label class="edit-label">destination<textarea name="destination" placeholder="optional product writeup"></textarea></label>
        <label class="edit-label">repo<input name="repo" placeholder="optional folder for cursor" autocomplete="off" /></label>`
      : `<label class="edit-label">title<input name="title" autocomplete="off" /></label>
        <label class="edit-label">body${mdEditorHTML(formId + "-md", "markdown body")}</label>`;
  return `<div class="compose" data-compose-root="${esc(formId)}">
    <button type="button" class="banner compose-open" data-compose-open>+ ${esc(label)}</button>
    <form class="compose-form" id="${esc(formId)}" hidden>
      <div class="pad">${fields}
        <div class="actions"><button type="submit">create</button><button type="button" data-compose-cancel>cancel</button></div>
      </div>
    </form>
  </div>`;
}

function bindCompose(extra, formId = "compose") {
  const wrap = document.querySelector(`[data-compose-root="${formId}"]`);
  const form = document.getElementById(formId);
  if (!wrap || !form) return;
  const openBtn = wrap.querySelector("[data-compose-open]");
  bindMdEditor(form.querySelector(".md-editor"));
  const show = (on) => {
    openBtn.hidden = on;
    form.hidden = !on;
    if (on) form.querySelector("[name=title]")?.focus();
  };
  openBtn.addEventListener("click", () => show(true));
  form.querySelector("[data-compose-cancel]")?.addEventListener("click", () => {
    form.reset();
    const preview = form.querySelector(".md-preview");
    if (preview) preview.innerHTML = mdPreviewHTML("");
    show(false);
  });
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const title = (form.querySelector("[name=title]")?.value || "").trim();
    if (!title) return;
    try {
      if (extra && extra._project) {
        const created = await api("/api/projects", {
          method: "POST",
          body: JSON.stringify({
            title,
            destination: form.querySelector("[name=destination]")?.value || "",
            repo: form.querySelector("[name=repo]")?.value || "",
          }),
        });
        location.hash = `#/project/${created.id}`;
        return;
      }
      const body = form.querySelector("[name=body]")?.value || "";
      const { _project, ...payload } = extra || {};
      const created = await api("/api/issues", {
        method: "POST",
        body: JSON.stringify({ title, body, ...payload }),
      });
      location.hash = hrefFor(created);
    } catch (err) {
      state.error = err.message;
      paint();
    }
  });
}

function bindProjectCompose(formId = "compose") {
  bindCompose({ _project: true }, formId);
}

function setViewRepo(repo) {
  state.viewRepo = String(repo || "").trim();
}

function syncFootRepo() {
  const el = $("#foot-repo");
  if (!el) return;
  const repo = state.viewRepo || state.defaultRepo || "";
  el.hidden = !repo;
  el.textContent = repo;
  el.title = repo;
}

async function refreshStats() {
  const [allData, labelData, health, projectData] = await Promise.all([
    api("/api/issues"),
    api("/api/labels"),
    api("/api/health"),
    api("/api/projects"),
  ]);
  state.version = health.version || "";
  state.dataPath = health.data || "";
  state.defaultRepo = health.workspace || "";
  const ver = $("#version");
  if (ver) ver.textContent = state.version ? "v" + state.version : "";
  const all = allData.issues || [];
  const tickets = all.filter((i) => !isArtifact(i));
  state.projects = projectData.projects || [];
  state.stats = {
    all: tickets.length,
    total: all.length,
    open: tickets.filter((i) => i.state === "open").length,
    closed: tickets.filter((i) => i.state === "closed").length,
    maps: all.filter(isMap).length,
    projects: state.projects.length,
    frontier: tickets.filter((i) => i.frontier).length,
    blocked: tickets.filter((i) => i.blocked && i.state === "open").length,
  };
  state.labels = labelData.labels || [];
  document.querySelectorAll("[data-count]").forEach((el) => {
    const n = state.stats[el.dataset.count];
    el.textContent = n || "";
  });
  $("#counts").innerHTML = `<b>${state.stats.open}</b> open · <b class="take">${state.stats.frontier}</b> frontier`;
}

function railLines(rows) {
  return `<div class="pad">${rows
    .map(([k, v]) => `<div class="rail-line">${k} <b>${v}</b></div>`)
    .join("")}</div>`;
}

function renderRail(extraHTML = "") {
  const s = state.stats;
  rail.innerHTML =
    box(
      "<strong>this tracker</strong>",
      railLines([
        ["open", s.open],
        ["frontier", s.frontier],
        ["blocked", s.blocked],
        ["projects", s.projects],
        ["maps", s.maps],
        ["closed", s.closed],
      ])
    ) +
    box(
      "<strong>labels</strong>",
      `<div class="tags">${tagButtons(state.labels) || `<span class="muted">none</span>`}</div>`
    ) +
    extraHTML;
}

async function loadList() {
  if (state.filter === "projects") {
    const data = await api("/api/projects");
    state.projects = data.projects || [];
    if (state.selected >= state.projects.length) state.selected = 0;
    return;
  }
  const q = new URLSearchParams();
  if (state.filter === "open" || state.filter === "closed") q.set("state", state.filter);
  if (state.filter === "frontier") q.set("frontier", "1");
  if (state.filter === "maps") q.set("labels", "wayfinder:map");
  const data = await api("/api/issues?" + q.toString());
  const fetched = data.issues || [];
  state.issues = state.filter === "maps" ? fetched : fetched.filter((i) => !isArtifact(i));
  if (state.selected >= state.issues.length) state.selected = 0;
}

async function loadMapChildren(mapId) {
  const q = new URLSearchParams({ parentId: String(mapId) });
  if (state.mapChildFilter === "open" || state.mapChildFilter === "closed") q.set("state", state.mapChildFilter);
  if (state.mapChildFilter === "frontier") q.set("frontier", "1");
  const data = await api("/api/issues?" + q.toString());
  state.issues = data.issues || [];
  if (state.selected >= state.issues.length) state.selected = 0;
}

function ticketHint() {
  return `<div class="hint">tickets live on maps — open a map to add one</div>`;
}

function groupTicketsByMap(issues) {
  const groups = new Map();
  for (const t of issues || []) {
    const p = t.parent || null;
    const key = p ? `p${p.id}` : "inbox";
    if (!groups.has(key)) groups.set(key, { parent: p, tickets: [] });
    groups.get(key).tickets.push(t);
  }
  return [...groups.values()].sort((a, b) => {
    if (!a.parent) return 1;
    if (!b.parent) return -1;
    return a.parent.id - b.parent.id;
  });
}

function flattenGroups(groups, split = false) {
  if (!split) return groups.flatMap((g) => g.tickets);
  return groups.flatMap((g) => {
    const buckets = bucketTickets(g.tickets);
    return TAKE_SECTIONS.flatMap(([key]) => buckets[key]);
  });
}

const TAKE_SECTIONS = [
  ["frontier", "frontier"],
  ["claimed", "claimed"],
  ["blocked", "waiting on a blocker"],
  ["closed", "closed"],
];

function bucketTickets(tickets) {
  const buckets = { frontier: [], claimed: [], blocked: [], closed: [], map: [] };
  for (const t of tickets || []) buckets[takeability(t)].push(t);
  return buckets;
}

function ticketRowsHTML(tickets, startIndex) {
  return tickets
    .map((t, n) => ticketHTML(t, startIndex + n === state.selected))
    .join("");
}

function sectionedTicketsHTML(tickets, startIndex, split) {
  if (!split) return { html: ticketRowsHTML(tickets, startIndex), next: startIndex + tickets.length };
  const buckets = bucketTickets(tickets);
  let i = startIndex;
  const html = TAKE_SECTIONS.map(([key, label]) => {
    const list = buckets[key];
    if (!list.length) return "";
    const rows = ticketRowsHTML(list, i);
    i += list.length;
    return `<div class="group-sub">${label}</div>${rows}`;
  }).join("");
  return { html, next: i };
}

function groupedTicketHTML(groups, split = false) {
  let i = 0;
  return groups
    .map((g) => {
      const head = g.parent
        ? `<a class="group-map" href="${hrefFor(g.parent)}">${esc(g.parent.title)}</a>`
        : `<span class="group-map">inbox · no map</span>`;
      const n = g.tickets.length;
      const { html: rows, next } = sectionedTicketsHTML(g.tickets, i, split);
      i = next;
      return `<div class="group"><div class="group-h">${head}<span>${n} ticket${n === 1 ? "" : "s"}</span></div><div class="group-rows">${rows}</div></div>`;
    })
    .join("");
}

function projectHref(project) {
  return `#/project/${project.id}`;
}

function projectCounts(project) {
  const maps = (project.maps || []).length;
  const specs = (project.specs || []).length;
  const plans = (project.plans || []).length;
  return `${maps} map${maps === 1 ? "" : "s"} · ${specs} spec${specs === 1 ? "" : "s"} · ${plans} plan${plans === 1 ? "" : "s"}`;
}

function projectRowHTML(project, selected = false, nav = false) {
  return `<div class="row project-row ${selected ? "selected" : ""}" ${nav ? "data-nav" : ""} data-id="${project.id}">
    <span class="map-mark">${esc(project.identifier || "P")}</span>
    <a class="title" href="${projectHref(project)}">${esc(project.title)}</a>
    <span class="mark">${projectCounts(project)}</span>
  </div>`;
}

function mapRowHTML(map, selected = false, nav = false) {
  const kids = map.children || [];
  const total = kids.length;
  const done = kids.filter((c) => c.state === "closed").length;
  const open = total - done;
  const pct = total ? Math.round((done / total) * 100) : 0;
  return `<div class="row map-row ${selected ? "selected" : ""}" ${nav ? "data-nav" : ""} data-id="${map.id}">
    <span class="map-mark">map</span>
    <a class="title" href="#/map/${map.id}">${esc(map.title)}</a>
    <span class="meta">${tagButtons(map.labels)}</span>
    <span class="mark">${open} open · ${done} done</span>
  </div>
  <div class="progress"><i style="width:${pct}%"></i></div>`;
}

function statStrip() {
  const s = state.stats;
  const total = s.all || 0;
  const done = s.closed || 0;
  const pct = total ? Math.round((done / total) * 100) : 0;
  return `<div class="stats">
    <div class="stat"><div class="stat-v">${s.projects || 0}</div><div class="stat-l">projects</div></div>
    <div class="stat"><div class="stat-v">${s.maps || 0}</div><div class="stat-l">maps</div></div>
    <div class="stat"><div class="stat-v">${s.open || 0}</div><div class="stat-l">open tickets</div></div>
    <div class="stat"><div class="stat-v">${done}</div><div class="stat-l">done</div></div>
    <div class="stat"><div class="stat-v">${pct}<span class="stat-u">%</span></div><div class="stat-l">complete</div><div class="progress slim"><i style="width:${pct}%"></i></div></div>
  </div>`;
}

function renderTagList(label) {
  if (label === "wayfinder:map") {
    const rows = state.issues.map((issue, i) => ticketHTML(issue, i === state.selected)).join("");
    main.innerHTML = `${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}${box(
      `<strong>#${esc(label)}</strong><a href="#/" class="muted">clear</a>`,
      rows || `<div class="empty">no maps with this tag</div>`,
      composeBar("new map with this tag")
    )}`;
    bindCompose({ labels: [label] });
    syncChrome();
    return;
  }
  const groups = groupTicketsByMap(state.issues);
  state.issues = flattenGroups(groups, true);
  main.innerHTML = `${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}${box(
    `<strong>#${esc(label)}</strong><a href="#/" class="muted">clear</a>`,
    groupedTicketHTML(groups, true) || `<div class="empty">no tickets with this tag</div>`,
    ticketHint()
  )}`;
  syncChrome();
}

async function loadTag(label) {
  const data = await api("/api/issues?" + new URLSearchParams({ labels: label }).toString());
  const fetched = data.issues || [];
  state.issues = label === "wayfinder:map" ? fetched : fetched.filter((i) => !isArtifact(i));
  if (state.selected >= state.issues.length) state.selected = 0;
}

function renderList() {
  if (state.filter === "projects") {
    const rows = state.projects.map((p, i) => projectRowHTML(p, i === state.selected, true)).join("");
    main.innerHTML = `${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}${box(
      `<strong>projects</strong><span>decision map → spec → plan</span>`,
      rows || `<div class="empty">no projects — compose one or create a map</div>`,
      composeBar("new project", "compose", "project")
    )}`;
    bindProjectCompose();
    syncChrome();
    return;
  }
  if (state.filter === "maps") {
    const rows = state.issues.map((issue, i) => mapRowHTML(issue, i === state.selected, true)).join("");
    main.innerHTML = `${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}${box(
      `<strong>maps</strong><button type="button" data-import-map>import</button>`,
      rows || `<div class="empty">none</div>`,
      composeBar("new map")
    )}`;
    bindCompose({ labels: ["wayfinder:map"] });
    syncChrome();
    return;
  }
  const split = state.filter === "open" || state.filter === "all";
  const title =
    state.filter === "frontier" ? "frontier" : state.filter === "open" ? "open" : state.filter === "closed" ? "closed" : "all";
  const caption =
    state.filter === "frontier"
      ? "open, unblocked, and unclaimed"
      : state.filter === "open"
        ? "all unfinished · frontier, claimed, and waiting on a blocker"
        : state.filter === "closed"
          ? "resolved tickets by map"
          : "every ticket by map";
  const empty =
    state.filter === "frontier"
      ? "nothing on the frontier — blocked and claimed tickets are under open"
      : "no tickets — open a map to add one";
  const groups = groupTicketsByMap(state.issues);
  state.issues = flattenGroups(groups, split);
  main.innerHTML = `${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}${box(
    `<strong>${title}</strong><span>${caption}</span>`,
    groupedTicketHTML(groups, split) || `<div class="empty">${empty}</div>`,
    ticketHint()
  )}`;
  syncChrome();
}

function foldBox(id, title, extra, inner, footer = "") {
  const open = homeFoldOpen(id);
  const actions = extra && extra.includes("box-actions");
  return `<section class="box fold${open ? "" : " is-closed"}">
    <div class="box-h fold-h">
      <button type="button" class="fold-bar" data-fold="${id}" aria-expanded="${open ? "true" : "false"}" aria-label="${open ? "collapse" : "expand"} ${title}">
        <span class="fold-ch" aria-hidden="true">${open ? "▾" : "▸"}</span>
        <strong>${title}</strong>
        ${actions ? "" : extra || ""}
      </button>
      ${actions ? extra : ""}
    </div>
    <div class="fold-body"${open ? "" : " hidden"}>
      <div class="box-b">${inner}</div>
      ${footer}
    </div>
  </section>`;
}

function homeFolds() {
  const defaults = { projects: true, maps: false, frontier: false };
  try {
    return { ...defaults, ...JSON.parse(localStorage.getItem("nl-home-fold-v2") || "{}") };
  } catch {
    return defaults;
  }
}

function homeFoldOpen(id) {
  return homeFolds()[id] !== false;
}

function setHomeFold(id, open) {
  let saved = {};
  try {
    saved = JSON.parse(localStorage.getItem("nl-home-fold-v2") || "{}") || {};
  } catch {
    saved = {};
  }
  saved[id] = open;
  localStorage.setItem("nl-home-fold-v2", JSON.stringify(saved));
}

function bindFolds() {
  main.querySelectorAll(".fold").forEach((boxEl) => {
    const btn = boxEl.querySelector("[data-fold]");
    if (!btn) return;
    const ch = btn.querySelector(".fold-ch");
    const toggle = () => {
      const body = boxEl.querySelector(".fold-body");
      const open = body.hidden;
      body.hidden = !open;
      boxEl.classList.toggle("is-closed", !open);
      btn.setAttribute("aria-expanded", String(open));
      btn.setAttribute("aria-label", `${open ? "collapse" : "expand"} ${btn.dataset.fold}`);
      if (ch) ch.textContent = open ? "▾" : "▸";
      setHomeFold(btn.dataset.fold, open);
    };
    btn.addEventListener("click", (e) => {
      e.preventDefault();
      toggle();
    });
  });
}

async function renderHome() {
  const [front, maps, projects] = await Promise.all([
    api("/api/issues?frontier=1"),
    api("/api/issues?labels=wayfinder:map"),
    api("/api/projects"),
  ]);
  const frontier = (front.issues || []).filter((i) => !isArtifact(i));
  const mapList = maps.issues || [];
  const projectList = projects.projects || [];
  state.projects = projectList;
  const groups = groupTicketsByMap(frontier);
  state.issues = flattenGroups(groups);
  if (state.selected >= state.issues.length) state.selected = 0;
  main.innerHTML = `
    ${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}
    ${statStrip()}
    ${foldBox(
      "projects",
      "projects",
      `<span class="box-actions"><a href="#/" data-jump="projects">open projects view</a></span>`,
      projectList.map((p) => projectRowHTML(p)).join("") || `<div class="empty">no projects</div>`,
      composeBar("new project", "compose-project", "project")
    )}
    ${foldBox(
      "maps",
      "maps",
      `<span class="box-actions"><button type="button" data-import-map>import</button><a href="#/" data-jump="maps">open maps view</a></span>`,
      mapList.map((m) => mapRowHTML(m)).join("") || `<div class="empty">no maps</div>`,
      composeBar("new map")
    )}
    ${foldBox(
      "frontier",
      "frontier",
      "<span>open, unblocked, and unclaimed</span>",
      groupedTicketHTML(groups) || `<div class="empty">nothing on the frontier</div>`,
      ticketHint()
    )}`;
  bindCompose({ labels: ["wayfinder:map"] });
  bindProjectCompose("compose-project");
  bindJump("maps");
  bindJump("projects");
  bindFolds();
  syncChrome();
}

function bindJump(name) {
  const jump = main.querySelector(`[data-jump=${name}]`);
  if (!jump) return;
  jump.addEventListener("click", (e) => {
    e.preventDefault();
    state.filter = name;
    localStorage.setItem("nl-filter", name);
    location.hash = "#/";
    paint();
  });
}

function goBack() {
  const r = route();
  if (r.name === "issue" || r.name === "spec" || r.name === "plan") {
    location.hash = state.issueBackHref || "#/";
    return;
  }
  if (r.name === "project") {
    state.filter = "projects";
    localStorage.setItem("nl-filter", "projects");
    if (location.hash.replace(/^#/, "") === "/") paint();
    else location.hash = "#/";
    return;
  }
  if (r.name === "map") {
    state.filter = "maps";
    localStorage.setItem("nl-filter", "maps");
    if (location.hash.replace(/^#/, "") === "/") paint();
    else location.hash = "#/";
    return;
  }
  if (r.name === "list") {
    state.filter = "home";
    localStorage.setItem("nl-filter", "home");
    paint();
    return;
  }
  location.hash = "#/";
}

function pageBackHTML() {
  return `<div class="page-bar"><button type="button" class="back-btn" data-back>← back</button></div>`;
}

function ensurePageBack() {
  const r = route();
  const home = r.name === "list" && state.filter === "home";
  const existing = main.querySelector(":scope > .page-bar");
  if (home) {
    existing?.remove();
    return;
  }
  if (!existing) main.insertAdjacentHTML("afterbegin", pageBackHTML());
}

function issuesNavOpen() {
  try {
    return localStorage.getItem("nl-nav-issues") === "1";
  } catch {
    return false;
  }
}

function setIssuesNavOpen(open) {
  localStorage.setItem("nl-nav-issues", open ? "1" : "0");
}

function renderNav() {
  const open = issuesNavOpen();
  const btn = document.querySelector("[data-nav-toggle=issues]");
  const chev = btn?.querySelector(".nav-chev");
  const kids = document.querySelector("#nav-issues .nav-kids");
  if (btn) {
    btn.setAttribute("aria-expanded", String(open));
    btn.setAttribute("aria-label", open ? "collapse issues" : "expand issues");
  }
  if (chev) chev.textContent = open ? "▾" : "▸";
  if (kids) kids.hidden = !open;
}

function syncChrome() {
  syncFootRepo();
  const r = route();
  renderNav();
  document.querySelectorAll("nav [data-filter]").forEach((b) => {
    const onMap = r.name === "map" && b.dataset.filter === "maps";
    const onProject = r.name === "project" && b.dataset.filter === "projects";
    b.classList.toggle("active", onMap || onProject || (r.name === "list" && b.dataset.filter === state.filter));
  });
  const settings = document.querySelector("nav > [data-go=settings]");
  if (settings) settings.classList.toggle("active", r.name === "settings");
  ensurePageBack();
}

async function renderMap(id) {
  let map;
  let project = null;
  try {
    map = await api("/api/issues/" + id);
    if (map.projectRef) {
      try {
        project = await api("/api/projects/" + map.projectRef.id);
      } catch {
        project = null;
      }
    }
    await loadMapChildren(id);
    state.error = "";
  } catch (err) {
    main.innerHTML = `<div class="error">${esc(err.message)}</div>`;
    return;
  }
  setViewRepo((project && project.repo) || (map.projectRef && map.projectRef.repo));
  const filters = ["open", "frontier", "closed", "all"]
    .map((f) => `<button data-map-filter="${f}" class="${f === state.mapChildFilter ? "active" : ""}">${f}</button>`)
    .join("");
  const split = state.mapChildFilter === "open" || state.mapChildFilter === "all";
  const { html: rows } = sectionedTicketsHTML(state.issues, 0, split);
  state.issues = flattenGroups([{ parent: null, tickets: state.issues }], split);
  if (state.selected >= state.issues.length) state.selected = 0;
  const empty =
    state.mapChildFilter === "frontier"
      ? "nothing on the frontier — blocked and claimed tickets are under open"
      : "no tickets on this map";
  const kids = (map.children || []).filter((c) => !isArtifact(c));
  const specs = project ? project.specs || [] : [];
  const hasSpec = specs.length > 0;
  const life = map.lifecycle || "active";
  const actions = [
    !hasSpec && life !== "cleared" ? `<button data-act="advance-spec">make spec</button>` : "",
    `<button data-act="to-spec">to spec</button>`,
    `<button data-act="to-plan">to plan</button>`,
    life !== "cleared" ? `<button data-act="clear-route">route is clear</button>` : "",
    `<button data-act="edit">edit map</button>`,
    `<button data-act="export">export</button>`,
    `<button data-act="delete" class="danger">delete map</button>`,
  ]
    .filter(Boolean)
    .join("");
  main.innerHTML = `
    ${flash()}
    ${projectCrumb(map)}
    ${box(
      `<strong>${esc(map.identifier)}</strong>${stamp(life, map.state === "closed" ? "closed lg" : "open lg")}`,
      `<div class="box-b pad" id="issue-head">
        <h1>${esc(map.title)}</h1>
        <div class="chips">${tagButtons(map.labels) || `<span class="muted">no tags</span>`}</div>
        <div class="body map-body">${map.body ? renderMarkdown(map.body) : `<span class="muted">empty map body</span>`}</div>
        <div class="actions">${actions}</div>
      </div>`
    )}
    ${
      hasSpec
        ? box(
            "<strong>spec</strong>",
            specs.map(artifactRowHTML).join("")
          )
        : ""
    }
    ${box(
      `<strong>tickets</strong><div class="subnav">${filters}</div>`,
      rows || `<div class="empty">${empty}</div>`,
      composeBar("new ticket on this map")
    )}`;
  bindCompose({ parentId: map.id });
  main.querySelectorAll("[data-act]").forEach((btn) => btn.addEventListener("click", () => act(map, btn.dataset.act)));
  main.querySelectorAll("[data-map-filter]").forEach((b) => {
    b.addEventListener("click", () => {
      state.mapChildFilter = b.dataset.mapFilter;
      localStorage.setItem("nl-map-filter", state.mapChildFilter);
      paint();
    });
  });
  renderRail(
    box(
      "<strong>this map</strong>",
      railLines([
        ["children", kids.length],
        ["open", kids.filter((c) => c.state === "open").length],
        ["frontier", kids.filter((c) => c.frontier).length],
        ["blocked", kids.filter((c) => c.blocked).length],
      ])
    )
  );
  syncChrome();
}

function projectCrumb(issue) {
  const p = issue.projectRef;
  if (!p) return "";
  return `<div class="hint"><a href="#/project/${p.id}">${esc(p.identifier)}</a> ${esc(p.title)} · ${esc(p.stage || "")}</div>`;
}

function artifactRowHTML(issue) {
  const src = issue.derivedFrom
    ? `from <a href="${hrefFor(issue.derivedFrom)}">${esc(issue.derivedFrom.identifier)}</a>`
    : "";
  let detail = "";
  let progress = "";
  if (isPlan(issue)) {
    const kids = (issue.children || []).filter((c) => !isArtifact(c));
    const total = kids.length;
    const done = kids.filter((c) => c.state === "closed").length;
    const pct = total ? Math.round((done / total) * 100) : 0;
    detail = `${total - done} open · ${done} done`;
    progress = `<div class="progress"><i style="width:${pct}%"></i></div>`;
  }
  const meta = [src, detail].filter(Boolean).join(" · ");
  return `<div class="row">
    <a class="id" href="${hrefFor(issue)}">${issue.identifier}</a>
    <a class="title" href="${hrefFor(issue)}">${esc(issue.title)}</a>
    <span class="meta">${meta}</span>
    <span class="mark">${statusStamp(issue)}</span>
  </div>${progress}`;
}

async function renderProject(id) {
  let project;
  try {
    project = await api("/api/projects/" + id);
    state.error = "";
  } catch (err) {
    main.innerHTML = `<div class="error">${esc(err.message)}</div>`;
    return;
  }
  setViewRepo(project.repo);
  const dest = project.destination || "";
  const repo = project.repo || "";
  const others = (state.projects || []).filter((p) => p.id !== project.id);
  const moveOpts = others
    .map((p) => `<option value="${p.id}">${esc(p.identifier)} ${esc(p.title)}</option>`)
    .join("");
  const move = moveOpts
    ? `<select data-move-to>${moveOpts}</select><button type="button" data-act="move">move all to…</button>`
    : "";
  main.innerHTML = `
    ${flash()}
    ${box(
      `<strong>${esc(project.identifier)}</strong>${stamp(project.stage || "wayfinding", "open lg")}`,
      `<div class="box-b pad" id="project-head">
        <h1>${esc(project.title)}</h1>
        <div class="body">${dest ? renderMarkdown(dest) : `<span class="muted">no destination</span>`}</div>
        <p class="muted">${repo ? `repo ${esc(repo)}` : "no repo — send to cursor uses the server default"}</p>
        <div class="actions">${move}<button type="button" data-act="edit">edit</button><button type="button" data-act="delete" class="danger">delete project</button></div>
      </div>`
    )}
    ${box(
      "<strong>decision maps</strong>",
      (project.maps || []).map((m) => mapRowHTML(m)).join("") || `<div class="empty">no maps</div>`,
      composeBar("new map on this project")
    )}
    ${box(
      "<strong>spec</strong>",
      (project.specs || []).map(artifactRowHTML).join("") || `<div class="empty">none — make the spec from the map</div>`
    )}
    ${box(
      "<strong>implementation plan</strong>",
      (project.plans || []).map(artifactRowHTML).join("") || `<div class="empty">none — approve a spec first</div>`
    )}`;
  renderRail(
    box(
      "<strong>this project</strong>",
      railLines([
        ["stage", project.stage || ""],
        ["repo", repo || "server default"],
        ["maps", (project.maps || []).length],
        ["specs", (project.specs || []).length],
        ["plans", (project.plans || []).length],
      ])
    )
  );
  const edit = main.querySelector("[data-act=edit]");
  if (edit) {
    edit.addEventListener("click", () => startProjectEdit(project));
  }
  const del = main.querySelector("[data-act=delete]");
  if (del) {
    del.addEventListener("click", async () => {
      if (!del.classList.contains("armed")) {
        del.classList.add("armed");
        del.textContent = "click again to delete";
        return;
      }
      try {
        await api("/api/projects/" + project.id, { method: "DELETE" });
        state.error = "";
        location.hash = "#/";
        await paint();
      } catch (err) {
        state.error = err.message;
        await renderProject(project.id);
      }
    });
  }
  const moveBtn = main.querySelector("[data-act=move]");
  if (moveBtn) {
    moveBtn.addEventListener("click", async () => {
      const sel = main.querySelector("[data-move-to]");
      const to = Number(sel && sel.value);
      if (!to) return;
      try {
        await api("/api/projects/move", {
          method: "POST",
          body: JSON.stringify({ fromProjectId: project.id, projectId: to }),
        });
        state.error = "";
        location.hash = "#/project/" + to;
        await paint();
      } catch (err) {
        state.error = err.message;
        await renderProject(project.id);
      }
    });
  }
  bindCompose({ labels: ["wayfinder:map"], projectId: project.id });
  syncChrome();
}

async function renderSpec(id) {
  let issue;
  let project = null;
  try {
    issue = await api("/api/issues/" + id);
    if (issue.projectRef) {
      try {
        project = await api("/api/projects/" + issue.projectRef.id);
      } catch {
        project = null;
      }
    }
    state.error = "";
  } catch (err) {
    main.innerHTML = `<div class="error">${esc(err.message)}</div>`;
    return;
  }
  if (!isSpec(issue)) {
    location.replace(hrefFor(issue));
    return;
  }
  setViewRepo((project && project.repo) || (issue.projectRef && issue.projectRef.repo));
  state.issueBackHref = issue.projectRef ? `#/project/${issue.projectRef.id}` : "#/";
  const plans = project ? project.plans || [] : [];
  const hasPlan = plans.length > 0;
  const actions = [
    issue.lifecycle === "draft" ? `<button data-act="approve">approve spec</button>` : "",
    issue.lifecycle === "approved" && !hasPlan ? `<button data-act="create-plan">create implementation plan</button>` : "",
    `<button data-act="edit">edit</button>`,
    `<button data-act="delete" class="danger">delete</button>`,
  ]
    .filter(Boolean)
    .join("");
  main.innerHTML = `
    ${flash()}
    ${projectCrumb(issue)}
    ${box(
      `<strong>${esc(issue.identifier)}</strong>${statusStamp(issue, "lg")}`,
      `<div class="box-b pad" id="issue-head">
        <h1>${esc(issue.title)}</h1>
        <div class="body">${issue.body ? renderMarkdown(issue.body) : `<span class="muted">empty spec — fill via /to-spec</span>`}</div>
        <div class="actions">${actions}</div>
      </div>`
    )}
    ${hasPlan ? box("<strong>implementation plan</strong>", plans.map(artifactRowHTML).join("")) : ""}`;
  main.querySelectorAll("[data-act]").forEach((btn) => btn.addEventListener("click", () => act(issue, btn.dataset.act)));
  renderRail(
    box(
      "<strong>this spec</strong>",
      railLines([
        ["lifecycle", issue.lifecycle || "draft"],
        ["from", issue.derivedFrom ? `<a href="${hrefFor(issue.derivedFrom)}">${esc(issue.derivedFrom.identifier)}</a>` : ""],
      ])
    )
  );
  syncChrome();
}

async function renderPlan(id) {
  let issue;
  try {
    issue = await api("/api/issues/" + id);
    await loadMapChildren(id);
    state.error = "";
  } catch (err) {
    main.innerHTML = `<div class="error">${esc(err.message)}</div>`;
    return;
  }
  if (!isPlan(issue)) {
    location.replace(hrefFor(issue));
    return;
  }
  setViewRepo(issue.projectRef && issue.projectRef.repo);
  state.issueBackHref = issue.projectRef ? `#/project/${issue.projectRef.id}` : "#/";
  const filters = ["open", "frontier", "closed", "all"]
    .map((f) => `<button data-map-filter="${f}" class="${f === state.mapChildFilter ? "active" : ""}">${f}</button>`)
    .join("");
  const split = state.mapChildFilter === "open" || state.mapChildFilter === "all";
  const { html: rows } = sectionedTicketsHTML(state.issues, 0, split);
  state.issues = flattenGroups([{ parent: null, tickets: state.issues }], split);
  if (state.selected >= state.issues.length) state.selected = 0;
  const kids = (issue.children || []).filter((c) => !isArtifact(c));
  const total = kids.length;
  const done = kids.filter((c) => c.state === "closed").length;
  const open = total - done;
  const pct = total ? Math.round((done / total) * 100) : 0;
  const life = issue.lifecycle || "draft";
  const actions = [
    life === "draft" ? `<button data-act="activate-plan">start implementation</button>` : "",
    life === "active" ? `<button data-act="deliver-plan">mark delivered</button>` : "",
    `<button data-act="edit">edit</button>`,
    `<button data-act="delete" class="danger">delete</button>`,
  ]
    .filter(Boolean)
    .join("");
  main.innerHTML = `
    ${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}
    ${projectCrumb(issue)}
    ${box(
      `<strong>${esc(issue.identifier)}</strong>${statusStamp(issue, "lg")}`,
      `<div class="box-b pad" id="issue-head">
        <h1>${esc(issue.title)}</h1>
        <div class="body">${issue.body ? renderMarkdown(issue.body) : `<span class="muted">empty plan — fill via /to-tickets</span>`}</div>
        <div class="actions">${actions}</div>
      </div>`
    )}
    ${relationBox("blocked by", sortRelations(issue.blockers), "not blocked")}
    ${box(
      `<strong>tickets</strong><span class="mark">${open} open · ${done} done</span><div class="subnav">${filters}</div>`,
      `<div class="progress"><i style="width:${pct}%"></i></div>` +
        (rows || `<div class="empty">no tickets on this plan</div>`),
      composeBar("new ticket on this plan")
    )}`;
  bindCompose({ parentId: issue.id });
  main.querySelectorAll("[data-act]").forEach((btn) => btn.addEventListener("click", () => act(issue, btn.dataset.act)));
  main.querySelectorAll("[data-map-filter]").forEach((b) => {
    b.addEventListener("click", () => {
      state.mapChildFilter = b.dataset.mapFilter;
      localStorage.setItem("nl-map-filter", state.mapChildFilter);
      paint();
    });
  });
  renderRail(
    box(
      "<strong>this plan</strong>",
      railLines([
        ["lifecycle", life],
        ["tickets", kids.length],
        ["frontier", kids.filter((c) => c.frontier).length],
        ["from", issue.derivedFrom ? `<a href="${hrefFor(issue.derivedFrom)}">${esc(issue.derivedFrom.identifier)}</a>` : ""],
      ])
    )
  );
  syncChrome();
}

function mdPreviewHTML(src) {
  const text = String(src ?? "").trim();
  return text ? renderMarkdown(text) : `<span class="muted">nothing to preview</span>`;
}

function mdEditorHTML(id, placeholder, value = "") {
  return `<div class="md-editor" id="${id}">
    <div class="subnav md-tabs">
      <button type="button" data-md-tab="write">write</button>
      <button type="button" data-md-tab="preview" class="active">preview</button>
    </div>
    <textarea name="body" placeholder="${placeholder}" hidden>${esc(value)}</textarea>
    <div class="body md-preview">${mdPreviewHTML(value)}</div>
  </div>`;
}

function bindMdEditor(root) {
  if (!root) return;
  const ta = root.querySelector("textarea");
  const preview = root.querySelector(".md-preview");
  const show = (tab) => {
    root.querySelectorAll("[data-md-tab]").forEach((b) => b.classList.toggle("active", b.dataset.mdTab === tab));
    const previewing = tab === "preview";
    ta.hidden = previewing;
    preview.hidden = !previewing;
    if (previewing) preview.innerHTML = mdPreviewHTML(ta.value);
    else ta.focus();
  };
  root.querySelectorAll("[data-md-tab]").forEach((btn) => {
    btn.addEventListener("click", () => show(btn.dataset.mdTab));
  });
  show("preview");
}

function commentEdited(c) {
  if (!c.updatedAt) return false;
  const t = Date.parse(c.updatedAt);
  return Number.isFinite(t) && new Date(t).getUTCFullYear() > 1970;
}

function commentStamp(c) {
  const created = esc(c.createdAt).slice(0, 19).replace("T", " ");
  const edited = commentEdited(c) ? " · edited" : "";
  return `${esc(c.author)} · ${created}${edited}`;
}

function commentHTML(c) {
  return `<div class="comment" data-comment-id="${esc(c.id)}">
    <div class="who">
      <span>${commentStamp(c)}</span>
      <button type="button" class="edit-c" data-edit-comment="${esc(c.id)}">edit</button>
    </div>
    <div class="text body">${c.body ? renderMarkdown(c.body) : `<span class="muted">empty</span>`}</div>
  </div>`;
}

function issueLinksBox(issue) {
  if (issue.parent && isMap(issue.parent)) {
    return box(
      "<strong>links</strong>",
      railLines([["map", `<a href="${hrefFor(issue.parent)}">${esc(issue.parent.title)}</a>`]])
    );
  }
  if (issue.parent) {
    return box(
      "<strong>links</strong>",
      railLines([["parent", `<a href="${hrefFor(issue.parent)}">${esc(issue.parent.title)}</a>`]])
    );
  }
  return box("<strong>links</strong>", railLines([["map", "inbox"]]));
}

function startCommentEdit(issue, comment) {
  const el = main.querySelector(`[data-comment-id="${comment.id}"]`);
  if (!el) return;
  el.dataset.commentEditing = "1";
  el.innerHTML = `${mdEditorHTML("edit-comment", "comment markdown", comment.body || "")}
    <div class="actions">
      <button type="button" data-save-comment>save</button>
      <button type="button" data-cancel-comment>cancel</button>
    </div>`;
  bindMdEditor(el.querySelector(".md-editor"));
  el.querySelector("[data-cancel-comment]").addEventListener("click", () => renderIssue(issue.id));
  el.querySelector("[data-save-comment]").addEventListener("click", async () => {
    const body = el.querySelector("textarea").value.trim();
    if (!body) return;
    try {
      await api(`/api/issues/${issue.id}/comments/${encodeURIComponent(comment.id)}`, {
        method: "PATCH",
        body: JSON.stringify({ body }),
      });
      state.error = "";
      await renderIssue(issue.id);
    } catch (err) {
      state.error = err.message;
      await renderIssue(issue.id);
    }
  });
}

async function renderIssue(id) {
  let issue;
  try {
    issue = await api("/api/issues/" + id);
    state.error = "";
  } catch (err) {
    main.innerHTML = `<div class="error">${esc(err.message)}</div>`;
    return;
  }
  if (isMap(issue)) {
    location.replace(`#/map/${issue.id}`);
    return;
  }
  if (isSpec(issue)) {
    location.replace(`#/spec/${issue.id}`);
    return;
  }
  if (isPlan(issue)) {
    location.replace(`#/plan/${issue.id}`);
    return;
  }
  setViewRepo(issue.projectRef && issue.projectRef.repo);
  state.issueBackHref = backHref(issue);
  const closed = issue.state === "closed";
  const comments = (issue.comments || []).map(commentHTML).join("");
  const blockers = sortRelations(issue.blockers);
  const blocks = sortRelations(issue.blocks);
  main.innerHTML = `
    ${flash()}
    ${box(
      `<strong>${esc(issue.identifier)}</strong>${statusStamp(issue, "lg")}`,
      `<div class="box-b pad ${closed ? "is-closed" : ""}" id="issue-head">
        <h1>${esc(issue.title)}</h1>
        <div class="chips">${tagButtons(issue.labels) || `<span class="muted">no labels</span>`}</div>
        <div class="body">${issue.body ? renderMarkdown(issue.body) : `<span class="muted">empty body</span>`}</div>
        <div class="actions">
          ${issue.state === "open" && !issue.blocked ? `<button data-act="send-cursor">send to cursor</button>` : ""}
          ${issue.state === "open" && !issue.assignee ? `<button data-act="claim">claim</button>` : ""}
          ${issue.assignee && issue.state === "open" ? `<button data-act="unclaim">unclaim</button>` : ""}
          ${issue.state === "open" ? `<button data-act="close">close</button>` : `<button data-act="reopen">reopen</button>`}
          <button data-act="edit">edit</button>
          <button data-act="delete" class="danger">delete</button>
        </div>
      </div>`
    )}
    ${relationBox("blocked by", blockers, "not blocked — frontier when unclaimed")}
    ${blocks.length ? relationBox("blocks", blocks, "") : ""}
    ${box(
      "<strong>comments</strong>",
      `${comments || `<div class="empty">none</div>`}
       <form id="comment" class="pad">
         ${mdEditorHTML("new-comment", "comment · markdown · ctrl+enter")}
         <div class="actions"><button type="submit">add comment</button></div>
       </form>`
    )}`;
  renderRail(issueLinksBox(issue));
  main.querySelectorAll("[data-act]").forEach((btn) => btn.addEventListener("click", () => act(issue, btn.dataset.act)));
  main.querySelectorAll("[data-edit-comment]").forEach((btn) => {
    btn.addEventListener("click", () => {
      const comment = (issue.comments || []).find((c) => c.id === btn.dataset.editComment);
      if (comment) startCommentEdit(issue, comment);
    });
  });
  bindMdEditor($("#new-comment"));
  $("#comment").addEventListener("submit", async (e) => {
    e.preventDefault();
    const body = e.target.body.value.trim();
    if (!body) return;
    await api(`/api/issues/${issue.id}/comments`, { method: "POST", body: JSON.stringify({ author: "me", body }) });
    renderIssue(issue.id);
  });
  $("#comment textarea").addEventListener("keydown", (e) => {
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      $("#comment").requestSubmit();
    }
  });
  syncChrome();
}

function formatTags(labels) {
  return (labels || []).join(" ");
}

function parseTags(raw) {
  return String(raw || "")
    .split(/[,\s]+/)
    .map((s) => s.replace(/^#+/, "").trim())
    .filter(Boolean);
}

function editFormHTML(issue) {
  return `<label class="edit-label">title<input id="edit-title" value="${esc(issue.title).replaceAll('"', "&quot;")}" /></label>
  <label class="edit-label">tags<input id="edit-labels" value="${esc(formatTags(issue.labels)).replaceAll('"', "&quot;")}" placeholder="space or comma · # optional" /></label>
  <label class="edit-label">body<textarea id="edit-body">${esc(issue.body || "")}</textarea></label>
  <div class="actions"><button data-save>save</button><button data-cancel>cancel</button></div>`;
}

function bindEditForm(issue) {
  const save = main.querySelector("[data-save]");
  const cancel = main.querySelector("[data-cancel]");
  cancel.addEventListener("click", () => paint());
  save.addEventListener("click", async () => {
    const title = main.querySelector("#edit-title").value.trim();
    const body = main.querySelector("#edit-body").value;
    const labels = parseTags(main.querySelector("#edit-labels")?.value || "");
    if (!title) return;
    try {
      await api(`/api/issues/${issue.id}`, { method: "PATCH", body: JSON.stringify({ title, body, labels }) });
      await paint();
    } catch (err) {
      state.error = err.message;
      await paint();
    }
  });
  main.querySelector("#issue-head").addEventListener("keydown", (e) => {
    if (e.key === "Escape") paint();
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      save.click();
    }
  });
  const t = main.querySelector("#edit-title");
  if (t) {
    t.focus();
    t.select();
  }
}

function startInlineEdit(issue) {
  const head = main.querySelector("#issue-head");
  if (!head) return;
  head.innerHTML = editFormHTML(issue);
  bindEditForm(issue);
}

function startProjectEdit(project) {
  const head = main.querySelector("#project-head");
  if (!head) return;
  head.innerHTML = `<label class="edit-label">title<input id="edit-title" value="${esc(project.title).replaceAll('"', "&quot;")}" /></label>
  <label class="edit-label">destination<textarea id="edit-destination">${esc(project.destination || "")}</textarea></label>
  <label class="edit-label">repo<input id="edit-repo" value="${esc(project.repo || "").replaceAll('"', "&quot;")}" placeholder="folder for cursor" autocomplete="off" /></label>
  <div class="actions"><button type="button" data-save>save</button><button type="button" data-cancel>cancel</button></div>`;
  const save = head.querySelector("[data-save]");
  const cancel = head.querySelector("[data-cancel]");
  cancel.addEventListener("click", () => renderProject(project.id));
  save.addEventListener("click", async () => {
    const title = head.querySelector("#edit-title").value.trim();
    if (!title) return;
    try {
      await api("/api/projects/" + project.id, {
        method: "PATCH",
        body: JSON.stringify({
          title,
          destination: head.querySelector("#edit-destination").value,
          repo: head.querySelector("#edit-repo").value,
        }),
      });
      state.error = "";
      state.notice = "saved project";
      await renderProject(project.id);
    } catch (err) {
      state.error = err.message;
      main.querySelector(".error")?.remove();
      main.insertAdjacentHTML("afterbegin", `<div class="error">${esc(err.message)}</div>`);
    }
  });
  const t = head.querySelector("#edit-title");
  if (t) {
    t.focus();
    t.select();
  }
}

function slugTitle(title) {
  return String(title || "")
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 40)
    .replace(/-+$/g, "");
}

async function downloadMap(issue) {
  const bundle = await api(`/api/issues/${issue.id}/export`);
  const slug = slugTitle(issue.title);
  const name = `${issue.identifier}${slug ? "-" + slug : ""}.nlmap.json`;
  const blob = new Blob([JSON.stringify(bundle, null, 2) + "\n"], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  URL.revokeObjectURL(url);
}

async function importMapFile(file) {
  const bundle = JSON.parse(await file.text());
  const result = await api("/api/import", { method: "POST", body: JSON.stringify(bundle) });
  state.error = "";
  location.hash = `#/map/${result.map.id}`;
  await paint();
}

async function act(issue, kind) {
  const slow = kind === "approve" || kind === "to-spec" || kind === "to-plan" || kind === "send-cursor";
  if (slow && document.body.classList.contains("is-busy")) return;
  try {
    if (kind === "claim") await api(`/api/issues/${issue.id}/claim`, { method: "POST", body: "{}" });
    if (kind === "unclaim") await api(`/api/issues/${issue.id}`, { method: "PATCH", body: JSON.stringify({ assignee: "" }) });
    if (kind === "close") await api(`/api/issues/${issue.id}`, { method: "PATCH", body: JSON.stringify({ state: "closed" }) });
    if (kind === "reopen") await api(`/api/issues/${issue.id}`, { method: "PATCH", body: JSON.stringify({ state: "open" }) });
    if (kind === "edit") {
      startInlineEdit(issue);
      return;
    }
    if (kind === "export") {
      await downloadMap(issue);
      return;
    }
    if (kind === "advance-spec") {
      const spec = await api(`/api/issues/${issue.id}/advance-to-spec`, { method: "POST", body: "{}" });
      location.hash = hrefFor(spec);
      await paint();
      return;
    }
    if (kind === "clear-route") {
      await api(`/api/issues/${issue.id}/clear-route`, { method: "POST", body: "{}" });
    }
    if (kind === "approve") {
      armBusyButton("approve", "approving…");
      await api(`/api/issues/${issue.id}/approve`, { method: "POST", body: "{}" });
      try {
        await sendCursor("to-tickets", issue.id);
      } catch (err) {
        state.error = "approved. " + err.message;
      }
      await paint();
      return;
    }
    if (kind === "to-spec" || kind === "to-plan" || kind === "send-cursor") {
      const action = kind === "send-cursor" ? "issue" : kind;
      const wait = {
        issue: "sending to cursor…",
        "to-spec": "starting to spec…",
        "to-plan": "starting to plan…",
      };
      armBusyButton(kind, wait[action] || "talking to cursor…");
      await sendCursor(action, issue.id);
      await paint();
      return;
    }
    if (kind === "create-plan") {
      const plan = await api(`/api/issues/${issue.id}/create-plan`, { method: "POST", body: "{}" });
      location.hash = hrefFor(plan);
      await paint();
      return;
    }
    if (kind === "activate-plan") {
      await api(`/api/issues/${issue.id}/activate-plan`, { method: "POST", body: "{}" });
    }
    if (kind === "deliver-plan") {
      await api(`/api/issues/${issue.id}/deliver-plan`, { method: "POST", body: "{}" });
    }
    if (kind === "delete") {
      const kids = (issue.children || []).length;
      const label = isMap(issue) ? "map" : isSpec(issue) ? "spec" : isPlan(issue) ? "plan" : "issue";
      const extra = kids ? " and all children" : "";
      if (!confirm(`delete ${label} ${issue.identifier}${extra}?`)) return;
      await api(`/api/issues/${issue.id}`, { method: "DELETE" });
      location.hash = isMap(issue) ? "#/" : backHref(issue);
      await paint();
      return;
    }
    await paint();
  } catch (err) {
    state.error = err.message;
    await paint();
  }
}

async function paint() {
  state.viewRepo = "";
  const r = route();
  if (r.name === "settings") showSettingsLoading();
  else setBusy("");
  try {
    await refreshStats();
  } catch (err) {
    state.error = err.message;
  }
  try {
    const si = $("#global-search");
    if (si && document.activeElement !== si) si.value = r.name === "search" ? r.query : "";
    if (r.name === "list" && state.filter === "home") {
      try {
        state.error = "";
        await renderHome();
      } catch (err) {
        state.error = err.message;
        main.innerHTML = `<div class="error">${esc(err.message)}</div>`;
      }
      renderRail();
      return;
    }
    if (r.name === "list") {
      try {
        await loadList();
        state.error = "";
      } catch (err) {
        state.error = err.message;
        state.issues = [];
      }
      renderList();
      renderRail();
      return;
    }
    if (r.name === "tag") {
      try {
        await loadTag(r.label);
        state.error = "";
      } catch (err) {
        state.error = err.message;
        state.issues = [];
      }
      renderTagList(r.label);
      renderRail();
      return;
    }
    if (r.name === "settings") {
      await renderSettings();
      renderRail();
      return;
    }
    if (r.name === "search") {
      await renderSearch(r.query);
      renderRail();
      return;
    }
    if (r.name === "map") {
      await renderMap(r.id);
      return;
    }
    if (r.name === "project") {
      await renderProject(r.id);
      return;
    }
    if (r.name === "spec") {
      await renderSpec(r.id);
      return;
    }
    if (r.name === "plan") {
      await renderPlan(r.id);
      return;
    }
    await renderIssue(r.id);
  } finally {
    ensurePageBack();
  }
}

async function renderSearch(query) {
  let mapList = [];
  let extras = [];
  let groups = [];
  try {
    const data = await api("/api/issues?" + new URLSearchParams({ query }).toString());
    const all = data.issues || [];
    mapList = all.filter(isMap);
    extras = all.filter((i) => isSpec(i) || isPlan(i));
    groups = groupTicketsByMap(all.filter((i) => !isArtifact(i)));
    state.issues = flattenGroups(groups, true);
    if (state.selected >= state.issues.length) state.selected = 0;
    state.error = "";
  } catch (err) {
    state.error = err.message;
    state.issues = [];
  }
  const n = mapList.length + extras.length + state.issues.length;
  main.innerHTML = `
    ${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}
    ${box(
      `<strong>search</strong><span>${esc(query)} · ${n} match${n === 1 ? "" : "es"}</span><a href="#/" class="muted">clear</a>`,
      [...mapList.map((m) => mapRowHTML(m)), ...extras.map(artifactRowHTML)].join("") || `<div class="empty">no maps match</div>`
    )}
    ${box(
      `<strong>tickets</strong><span>by map</span>`,
      groupedTicketHTML(groups, true) || `<div class="empty">no tickets match</div>`,
      ticketHint()
    )}`;
  syncChrome();
}

async function renderSettings() {
  const s = state.stats;
  showSettingsLoading();
  let cursor = { model: "", workspace: "", models: [], cli: false, cliError: "" };
  try {
    cursor = await api("/api/cursor");
  } catch (err) {
    state.error = err.message;
  }
  document.body.classList.remove("is-busy");
  state.busy = "";
  const models = cursor.models || [];
  const known = models.includes(cursor.model);
  const modelField = cursor.cli
    ? `<select id="cursor-model">
        <option value="">cli default</option>
        ${cursor.model && !known ? `<option value="${esc(cursor.model)}" selected>${esc(cursor.model)}</option>` : ""}
        ${models.map((m) => `<option value="${esc(m)}" ${m === cursor.model ? "selected" : ""}>${esc(m)}</option>`).join("")}
      </select>`
    : "";
  const cli = cursor.cli ? "found" : esc(cursor.cliError || "not found");
  main.innerHTML = `${flash()}${box(
    "<strong>settings</strong>",
    `<div class="box-b pad">
      <div class="settings-meta">
        <div class="rail-line">version <b id="shown-version">${esc(state.version)}</b></div>
        <div class="rail-line">issues <b>${s.total ?? s.all}</b></div>
        <div class="rail-line">projects <b>${s.projects || 0}</b></div>
        <div class="rail-line">tickets <b>${s.all}</b></div>
        <div class="rail-line">maps <b>${s.maps}</b></div>
        <div class="rail-line">data <b>${esc(state.dataPath)}</b></div>
        <div class="rail-line">cursor cli <b>${cli}</b></div>
        <div class="rail-line">default repo <b>${esc(cursor.workspace || "")}</b></div>
      </div>
      ${modelField ? `<form id="cursor-settings" class="pad">
        <label class="edit-label">model${modelField}</label>
        <div class="actions"><button type="submit">save model</button></div>
      </form>` : ""}
      <p class="muted">models come from the cursor cli. default repo is where nonlinear was started. a project can set its own.</p>
      <p class="settings-warn">wipe deletes every issue. next create is NL-1. this cannot be undone.</p>
      <div class="actions">
        <button type="button" data-import-map>import map</button>
        <button type="button" id="wipe" class="danger">wipe database</button>
      </div>
    </div>`
  )}`;
  const form = $("#cursor-settings");
  if (form) {
    form.addEventListener("submit", async (e) => {
      e.preventDefault();
      if (document.body.classList.contains("is-busy")) return;
      setBusy("saving model…");
      try {
        await api("/api/cursor", {
          method: "PUT",
          body: JSON.stringify({ model: form.querySelector("#cursor-model")?.value || "" }),
        });
        state.error = "";
        state.notice = "saved model";
        await renderSettings();
      } catch (err) {
        state.error = err.message;
        await renderSettings();
      }
    });
  }
  const btn = $("#wipe");
  btn.addEventListener("click", async () => {
    if (!btn.classList.contains("armed")) {
      btn.classList.add("armed");
      btn.textContent = "click again to confirm";
      return;
    }
    try {
      await api("/api/wipe", { method: "POST", body: JSON.stringify({ confirm: true }) });
      state.error = "";
      location.hash = "#/";
      await paint();
    } catch (err) {
      state.error = err.message;
      renderSettings();
    }
  });
  syncChrome();
}

document.querySelector("nav").addEventListener("click", (e) => {
  const tog = e.target.closest("[data-nav-toggle]");
  if (tog) {
    e.preventDefault();
    const open = !issuesNavOpen();
    setIssuesNavOpen(open);
    renderNav();
    return;
  }
  const settings = e.target.closest("[data-go=settings]");
  if (settings) {
    showSettingsLoading();
    location.hash = "#/settings";
    return;
  }
  const b = e.target.closest("[data-filter]");
  if (!b) return;
  state.filter = b.dataset.filter;
  localStorage.setItem("nl-filter", state.filter);
  if (state.filter !== "home") setIssuesNavOpen(true);
  location.hash = "#/";
  paint();
});

document.getElementById("search-form").addEventListener("submit", (e) => {
  e.preventDefault();
  const q = document.getElementById("global-search").value.trim();
  location.hash = q ? "#/search/" + encodeURIComponent(q) : "#/";
});

document.getElementById("global-search").addEventListener("keydown", (e) => {
  if (e.key === "Escape") {
    e.target.blur();
    location.hash = "#/";
  }
});

document.getElementById("refresh").addEventListener("click", () => {
  paint();
});

$("#import-map-file").addEventListener("change", async (e) => {
  const file = e.target.files && e.target.files[0];
  e.target.value = "";
  if (!file) return;
  try {
    await importMapFile(file);
  } catch (err) {
    state.error = err.message;
    await paint();
  }
});

main.addEventListener("click", (e) => {
  if (!e.target.closest("[data-back]")) return;
  e.preventDefault();
  goBack();
});

document.getElementById("app").addEventListener("click", (e) => {
  const importBtn = e.target.closest("[data-import-map]");
  if (importBtn) {
    e.preventDefault();
    const input = $("#import-map-file");
    if (input) input.click();
    return;
  }
  const t = e.target.closest("[data-tag]");
  if (!t) return;
  e.preventDefault();
  e.stopPropagation();
  const label = t.dataset.tag;
  const cur = route();
  location.hash = cur.name === "tag" && cur.label === label ? "#/" : tagHref(label);
});

function highlightSelected() {
  document.querySelectorAll("main .row[data-nav]").forEach((el, i) => el.classList.toggle("selected", i === state.selected));
}

window.addEventListener("hashchange", paint);
window.addEventListener("keydown", (e) => {
  if (e.target.matches("input, textarea")) return;
  const r = route();
  if (e.key === "r") {
    e.preventDefault();
    paint();
    return;
  }
  if (r.name === "issue" || r.name === "spec" || r.name === "plan") {
    if (e.key === "Escape") {
      if (main.querySelector("[data-comment-editing], [data-save]")) {
        paint();
        return;
      }
      goBack();
    }
    return;
  }
  if (e.key === "j") {
    const items = listCursor();
    state.selected = Math.min(Math.max(items.length - 1, 0), state.selected + 1);
    highlightSelected();
  }
  if (e.key === "k") {
    state.selected = Math.max(0, state.selected - 1);
    highlightSelected();
  }
  if (e.key === "Enter") {
    const items = listCursor();
    const item = items[state.selected];
    if (item) location.hash = listHref(item);
  }
  if (e.key === "/") {
    e.preventDefault();
    const input = $("#global-search");
    if (input) input.focus();
  }
  if (e.key === "Escape" && (r.name === "map" || r.name === "tag" || r.name === "settings" || r.name === "search" || r.name === "project")) goBack();
});

const THEMES = { orange: "#e85d04", matrix: "#00e64d", cool: "#5ba8e8" };

function applyTheme(name) {
  if (name === "coop") name = "cool";
  if (!THEMES[name]) name = "matrix";
  document.documentElement.dataset.theme = name;
  localStorage.setItem("nl-theme", name);
  const meta = document.querySelector('meta[name="theme-color"]');
  if (meta) meta.content = THEMES[name];
  document.querySelectorAll(".swatch").forEach((b) => {
    b.setAttribute("aria-checked", b.dataset.theme === name ? "true" : "false");
  });
}

applyTheme(localStorage.getItem("nl-theme") || "matrix");
document.querySelector(".swatches").addEventListener("click", (e) => {
  const b = e.target.closest("[data-theme]");
  if (b) applyTheme(b.dataset.theme);
});

if ("serviceWorker" in navigator) {
  navigator.serviceWorker.register("/sw.js").catch(() => {});
}

let deferredPrompt;
window.addEventListener("beforeinstallprompt", (e) => {
  e.preventDefault();
  deferredPrompt = e;
  const btn = $("#install");
  btn.hidden = false;
  btn.onclick = async () => {
    deferredPrompt.prompt();
    await deferredPrompt.userChoice;
    btn.hidden = true;
  };
});

paint();
