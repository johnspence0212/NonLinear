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
  homePage: { claimed: 0, frontier: 0, blocked: 0, events: 0 },
};

function navFilter(name) {
  return name === "projects" ? "projects" : "home";
}

function setFilter(name) {
  state.filter = navFilter(name);
  localStorage.setItem("nl-filter", state.filter);
}

setFilter(state.filter);

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

function knownLabel(q) {
  const raw = String(q || "")
    .trim()
    .replace(/^#+/, "");
  if (!raw) return "";
  const hit = (state.labels || []).find((l) => l.toLowerCase() === raw.toLowerCase());
  return hit || "";
}

function searchHash(q) {
  const trimmed = String(q || "").trim();
  if (!trimmed) return "#/";
  const tag = knownLabel(trimmed);
  if (tag) return tagHref(tag);
  return "#/search/" + encodeURIComponent(trimmed);
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

const WAYFINDER_MAP_TAG = "wayfinder:map";
const WAYFINDER_TICKET_TAGS = ["wayfinder:research", "wayfinder:prototype", "wayfinder:grilling", "wayfinder:task"];

function uniqueLabels(list) {
  const seen = new Set();
  const out = [];
  for (const raw of list || []) {
    const label = String(raw || "").trim();
    if (!label || seen.has(label)) continue;
    seen.add(label);
    out.push(label);
  }
  return out;
}

function catalogTags() {
  return uniqueLabels(state.labels);
}

function availableTagsFor(issue) {
  const catalog = catalogTags();
  const current = uniqueLabels(issue && issue.labels);
  let pool;
  if (isMap(issue)) {
    pool = catalog.filter((l) => !WAYFINDER_TICKET_TAGS.includes(l));
  } else if (isSpec(issue) || isPlan(issue) || isBug(issue)) {
    pool = catalog.filter((l) => l !== WAYFINDER_MAP_TAG && !WAYFINDER_TICKET_TAGS.includes(l));
  } else {
    pool = catalog.filter((l) => l !== WAYFINDER_MAP_TAG);
  }
  return uniqueLabels([...pool, ...current]);
}

function lockedTags(issue) {
  return isMap(issue) ? [WAYFINDER_MAP_TAG] : [];
}

function initialPickedTags(issue) {
  const picked = uniqueLabels(issue && issue.labels);
  for (const label of lockedTags(issue)) {
    if (!picked.includes(label)) picked.push(label);
  }
  return picked;
}

function tagPickerHTML(issue) {
  return `<label class="edit-label">tags
    <div class="tag-picker" data-tag-picker></div>
  </label>`;
}

function tagPickerInnerHTML(issue, picked) {
  const locked = new Set(lockedTags(issue));
  const available = availableTagsFor(issue);
  const pickedSet = new Set(picked);
  const chips = picked
    .map((l) => {
      if (locked.has(l)) return `<span class="tag pinned">#${esc(l)}</span>`;
      return `<button type="button" class="tag" data-remove-tag="${esc(l)}">#${esc(l)} ×</button>`;
    })
    .join("");
  const remaining = available.filter((l) => !pickedSet.has(l));
  const opts = remaining.map((l) => `<option value="${esc(l)}">#${esc(l)}</option>`).join("");
  const prompt = picked.length ? "add tag…" : "choose a tag…";
  const select = remaining.length
    ? `<select id="edit-add-tag" aria-label="available tags"><option value="">${esc(prompt)}</option>${opts}</select>`
    : `<span class="muted">${picked.length ? "all available tags applied" : "no tags in catalog"}</span>`;
  return `${select}<div class="chips tag-picked">${chips}</div>`;
}

function bindTagPicker(root, issue) {
  const picked = initialPickedTags(issue);
  const paintPicker = () => {
    root.innerHTML = tagPickerInnerHTML(issue, picked);
    const select = root.querySelector("#edit-add-tag");
    if (select) {
      select.addEventListener("change", () => {
        const value = select.value;
        if (!value || picked.includes(value)) return;
        picked.push(value);
        paintPicker();
      });
    }
    root.querySelectorAll("[data-remove-tag]").forEach((btn) => {
      btn.addEventListener("click", (e) => {
        e.preventDefault();
        e.stopPropagation();
        const tag = btn.dataset.removeTag;
        if (lockedTags(issue).includes(tag)) return;
        const i = picked.indexOf(tag);
        if (i >= 0) picked.splice(i, 1);
        paintPicker();
      });
    });
  };
  paintPicker();
  return () => picked.slice();
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

function isBug(issue) {
  return issue.kind === "bug";
}

function isArtifact(issue) {
  return isMap(issue) || isSpec(issue) || isPlan(issue);
}

function derivedFrom(issue, sourceId) {
  return !!(issue && issue.derivedFrom && issue.derivedFrom.id === sourceId);
}

function ownArtifacts(projectItems, issue, kind) {
  const fromProject = (projectItems || []).filter((item) => derivedFrom(item, issue.id));
  if (fromProject.length) return fromProject;
  return (issue.derived || []).filter((item) => !kind || item.kind === kind);
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

async function holdBusy(started, minMs = 900) {
  const wait = minMs - (Date.now() - started);
  if (wait > 0) await new Promise((r) => setTimeout(r, wait));
}

async function sendCursor(action, id) {
  const wait = {
    issue: "sending to cursor…",
    "to-spec": "starting to spec…",
    "to-plan": "starting to tickets…",
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
    return res;
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
        <label class="edit-label">body${mdEditorHTML(formId + "-md", "markdown body", "", "write")}</label>`;
  return `<span class="compose" data-compose-root="${esc(formId)}">
    <button type="button" class="compose-open" data-compose-open>+ ${esc(label)}</button>
    <div class="compose-layer" hidden>
      <button type="button" class="compose-scrim" data-compose-cancel aria-label="close"></button>
      <form class="compose-card" id="${esc(formId)}" role="dialog" aria-modal="true" aria-label="${esc(label)}">
        <div class="compose-card-h"><strong>${esc(label)}</strong></div>
        <div class="compose-card-b">${fields}
          <div class="actions"><button type="submit">create</button><button type="button" data-compose-cancel>cancel</button></div>
        </div>
      </form>
    </div>
  </span>`;
}

function stripComposeLayers() {
  document.querySelectorAll("body > .compose-layer").forEach((el) => el.remove());
  document.body.classList.remove("compose-open");
}

function resetComposeForm(form) {
  if (!form) return;
  form.reset();
  const editor = form.querySelector(".md-editor");
  if (editor) showMdTab(editor, "write", { focus: false });
}

function closeOpenCompose() {
  const layer = document.querySelector(".compose-layer:not([hidden])");
  if (!layer) return false;
  resetComposeForm(layer.querySelector("form"));
  layer.hidden = true;
  document.body.classList.remove("compose-open");
  return true;
}

function bindCompose(extra, formId = "compose") {
  const wrap = document.querySelector(`[data-compose-root="${formId}"]`);
  const form = document.getElementById(formId);
  if (!wrap || !form) return;
  const openBtn = wrap.querySelector("[data-compose-open]");
  const layer = wrap.querySelector(".compose-layer");
  if (layer) document.body.appendChild(layer);
  bindMdEditor(form.querySelector(".md-editor"), "write");
  const show = (on) => {
    if (!layer) return;
    if (on) {
      document.querySelectorAll("body > .compose-layer").forEach((el) => {
        if (el !== layer) {
          resetComposeForm(el.querySelector("form"));
          el.hidden = true;
        }
      });
    }
    layer.hidden = !on;
    document.body.classList.toggle("compose-open", on);
    if (on) {
      showMdTab(form.querySelector(".md-editor"), "write", { focus: false });
      form.querySelector("[name=title]")?.focus();
    }
  };
  const hide = () => {
    resetComposeForm(form);
    show(false);
  };
  openBtn.addEventListener("click", () => show(true));
  layer?.querySelectorAll("[data-compose-cancel]").forEach((btn) => {
    btn.addEventListener("click", hide);
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
      if (extra && extra._bug && extra.projectId) {
        const body = form.querySelector("[name=body]")?.value || "";
        await api("/api/projects/" + extra.projectId + "/bugs", {
          method: "POST",
          body: JSON.stringify({ title, body }),
        });
        await paint();
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

function labelsBox() {
  return box(
    "<strong>labels</strong>",
    `<div class="tags">${tagButtons(state.labels) || `<span class="muted">none</span>`}</div>`
  );
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
    labelsBox() +
    extraHTML;
}

async function loadList() {
  const data = await api("/api/projects");
  state.projects = data.projects || [];
  if (state.selected >= state.projects.length) state.selected = 0;
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
  const bugs = (project.bugs || []).length;
  const parts = [
    `${maps} map${maps === 1 ? "" : "s"}`,
    `${specs} spec${specs === 1 ? "" : "s"}`,
    `${plans} tickets`,
  ];
  if (bugs) parts.push(`${bugs} bug${bugs === 1 ? "" : "s"}`);
  return parts.join(" · ");
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
  const maps = state.issues.filter(isMap);
  const extras = state.issues.filter((i) => isSpec(i) || isPlan(i) || isBug(i));
  const groups = groupTicketsByMap(state.issues.filter((i) => !isArtifact(i) && !isBug(i)));
  state.issues = flattenGroups(groups, true);
  if (state.selected >= state.issues.length) state.selected = 0;
  const n = maps.length + extras.length + state.issues.length;
  const mapCompose = label === "wayfinder:map" ? composeBar("new map with this tag") : "";
  main.innerHTML = `${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}${box(
    `<strong>#${esc(label)}</strong><span>${n} match${n === 1 ? "" : "es"}</span><a href="#/" class="muted">clear</a>${mapCompose}`,
    [...maps.map((m) => mapRowHTML(m)), ...extras.map(artifactRowHTML)].join("") || `<div class="empty">no maps or specs with this tag</div>`
  )}${box(
    `<strong>tickets</strong><span>by map</span>`,
    groupedTicketHTML(groups, true) || `<div class="empty">no tickets with this tag</div>`,
    label === "wayfinder:map" ? "" : ticketHint()
  )}`;
  if (label === "wayfinder:map") bindCompose({ labels: [label] });
  syncChrome();
}

async function loadTag(label) {
  const data = await api("/api/issues?" + new URLSearchParams({ labels: label }).toString());
  state.issues = data.issues || [];
  if (state.selected >= state.issues.length) state.selected = 0;
}

function renderList() {
  const rows = state.projects.map((p, i) => projectRowHTML(p, i === state.selected, true)).join("");
  main.innerHTML = `${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}${box(
    `<strong>projects</strong><span>decision map → spec → tickets</span>${composeBar("new project", "compose", "project")}`,
    rows || `<div class="empty">no projects — compose one or create a map</div>`
  )}`;
  bindProjectCompose();
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

const HOME_PAGE_SIZE = 5;

function homePaged(items, key) {
  const list = items || [];
  const pages = Math.max(1, Math.ceil(list.length / HOME_PAGE_SIZE) || 1);
  let page = state.homePage[key] || 0;
  page = ((page % pages) + pages) % pages;
  state.homePage[key] = page;
  return {
    rows: list.slice(page * HOME_PAGE_SIZE, page * HOME_PAGE_SIZE + HOME_PAGE_SIZE),
    page,
    pages,
    total: list.length,
  };
}

function homePager(key, paged) {
  if (paged.total <= HOME_PAGE_SIZE) return "";
  return `<button type="button" data-home-next="${key}">next ${paged.page + 1}/${paged.pages}</button>`;
}

function eventHref(ev) {
  if (ev.targetKind === "project" && ev.projectId) return `#/project/${ev.projectId}`;
  if (ev.targetKind === "map" && ev.issueId) return `#/map/${ev.issueId}`;
  if (ev.targetKind === "spec" && ev.issueId) return `#/spec/${ev.issueId}`;
  if (ev.targetKind === "plan" && ev.issueId) return `#/plan/${ev.issueId}`;
  if (ev.issueId) return `#/${ev.issueId}`;
  if (ev.projectId) return `#/project/${ev.projectId}`;
  return "#/";
}

function eventStampKind(kind) {
  if (kind === "resolved" || kind === "closed") return "closed";
  if (kind === "claimed" || kind === "unclaimed" || kind === "ready_for_spec" || kind === "blocked") return "claim";
  if (kind === "created" || kind === "reopened" || kind === "spec_draft" || kind === "plan_draft") return "open";
  return "";
}

function eventLabel(kind) {
  const labels = {
    spec_draft: "spec",
    spec_approved: "approved",
    plan_draft: "tickets",
    plan_active: "active",
    plan_delivered: "delivered",
  };
  return labels[kind] || kind;
}

function eventVerb(kind) {
  const verbs = {
    created: "created",
    claimed: "claimed",
    unclaimed: "unclaimed",
    resolved: "resolved",
    closed: "closed",
    reopened: "reopened",
    commented: "commented on",
    blocked: "blocked",
    ready_for_spec: "marked ready for spec",
    cleared: "cleared",
    spec_draft: "drafted spec",
    spec_approved: "approved",
    plan_draft: "created tickets",
    plan_active: "started",
    plan_delivered: "delivered",
    deleted: "deleted",
  };
  return verbs[kind] || kind;
}

function eventDot(kind) {
  if (["claimed", "resolved", "closed", "reopened"].includes(kind)) return "work";
  if (["commented"].includes(kind)) return "note";
  if (["created"].includes(kind)) return "create";
  return "life";
}

function eventTime(iso) {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toISOString().slice(11, 16);
}

function eventDay(iso) {
  if (!iso) return "";
  const day = new Date(iso).toISOString().slice(0, 10);
  const today = new Date().toISOString().slice(0, 10);
  const yest = new Date(Date.now() - 86400000).toISOString().slice(0, 10);
  if (day === today) return "today";
  if (day === yest) return "yesterday";
  return day;
}

function eventRowsHTML(events) {
  let lastDay = "";
  return events
    .map((ev) => {
      const day = eventDay(ev.at);
      const head = day && day !== lastDay ? `<div class="day-h">${esc(day)}</div>` : "";
      lastDay = day;
      const name = [ev.identifier, ev.title].filter(Boolean).join(" ");
      return `${head}
        <div class="event">
          <div class="event-time">${esc(eventTime(ev.at))}</div>
          <div class="event-rail"><i class="event-dot ${eventDot(ev.kind)}"></i></div>
          <div class="event-main">
            <div class="event-line"><span class="event-who">${esc(ev.actor || "cursor")}</span> ${esc(eventVerb(ev.kind))} <a href="${eventHref(ev)}">${esc(name)}</a></div>
            ${ev.gist ? `<div class="event-gist">${esc(ev.gist)}</div>` : ""}
          </div>
          <div class="event-meta">${stamp(eventLabel(ev.kind), eventStampKind(ev.kind))}${ev.projectRef ? `<a class="muted" href="#/project/${ev.projectId || ""}">${esc(ev.projectRef)}</a>` : ""}</div>
        </div>`;
    })
    .join("");
}

function nowListHTML(items, empty) {
  if (!items.length) return `<div class="empty">${empty}</div>`;
  return items
    .map(
      (issue) => `<div class="row">
      <a class="id" href="${hrefFor(issue)}">${esc(issue.identifier)}</a>
      <a class="title" href="${hrefFor(issue)}">${esc(issue.title)}</a>
      <span class="mark">${statusStamp(issue)}</span>
    </div>`
    )
    .join("");
}

async function renderHome() {
  const home = await api("/api/home");
  const claimed = homePaged(home.claimed, "claimed");
  const frontier = homePaged(home.frontier, "frontier");
  const blocked = homePaged(home.blocked, "blocked");
  const events = homePaged(home.events, "events");
  const today = home.today || {};
  state.issues = (home.frontier || []).concat(home.claimed || []);
  if (state.selected >= state.issues.length) state.selected = 0;
  main.innerHTML = `
    ${flash()}
    <div class="now-grid">
      ${box(`<strong>claimed</strong>${homePager("claimed", claimed)}`, nowListHTML(claimed.rows, "nothing claimed"))}
      ${box(`<strong>frontier</strong>${homePager("frontier", frontier)}`, nowListHTML(frontier.rows, "nothing on the frontier"))}
      ${box(`<strong>waiting</strong>${homePager("blocked", blocked)}`, nowListHTML(blocked.rows, "nothing blocked"))}
    </div>
    ${box(
      `<strong>activity</strong>${homePager("events", events)}`,
      eventRowsHTML(events.rows) || `<div class="empty">no activity yet — create, claim, or resolve something</div>`
    )}`;
  main.querySelectorAll("[data-home-next]").forEach((btn) => {
    btn.addEventListener("click", () => {
      const key = btn.dataset.homeNext;
      state.homePage[key] = (state.homePage[key] || 0) + 1;
      paint();
    });
  });
  rail.innerHTML =
    box(
      "<strong>today</strong>",
      railLines([
        ["resolved", today.resolved || 0],
        ["claimed", today.claimed || 0],
        ["created", today.created || 0],
        ["lifecycle", today.lifecycle || 0],
      ])
    ) +
    labelsBox();
  syncChrome();
}

function bindJump(name) {
  const jump = main.querySelector(`[data-jump=${name}]`);
  if (!jump) return;
  jump.addEventListener("click", (e) => {
    e.preventDefault();
    setFilter(name);
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
    setFilter("projects");
    if (location.hash.replace(/^#/, "") === "/") paint();
    else location.hash = "#/";
    return;
  }
  if (r.name === "map") {
    location.hash = state.issueBackHref || "#/";
    return;
  }
  if (r.name === "list") {
    setFilter("home");
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

function syncChrome() {
  syncFootRepo();
  const r = route();
  document.querySelectorAll("nav [data-filter]").forEach((b) => {
    const f = b.dataset.filter;
    const onHome = f === "home" && r.name === "list" && state.filter === "home";
    const onProjects =
      f === "projects" &&
      (r.name === "project" ||
        r.name === "map" ||
        r.name === "spec" ||
        r.name === "plan" ||
        (r.name === "list" && state.filter === "projects"));
    b.classList.toggle("active", onHome || onProjects);
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
  state.issueBackHref = map.projectRef ? `#/project/${map.projectRef.id}` : "#/";
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
  const specs = ownArtifacts(project && project.specs, map, "spec");
  const hasSpec = specs.length > 0;
  const life = map.lifecycle || "active";
  const actions = [
    !hasSpec && life !== "cleared" ? `<button data-act="advance-spec">make spec</button>` : "",
    `<button data-act="to-spec">to spec</button>`,
    `<button data-act="to-tickets">to tickets</button>`,
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
      `<strong>tickets</strong><div class="subnav">${filters}</div>${composeBar("new ticket on this map")}`,
      rows || `<div class="empty">${empty}</div>`
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

function otherProjects(id) {
  return (state.projects || []).filter((p) => p.id !== id);
}

function projectMoveBox(project) {
  const others = otherProjects(project.id);
  if (!others.length) return "";
  const opts = others
    .map((p) => `<option value="${p.id}">${esc(p.identifier)} ${esc(p.title)}</option>`)
    .join("");
  return box(
    "<strong>move to</strong>",
    `<div class="pad">
      <p class="muted">Send every map, spec, ticket, and bug on this project to another project.</p>
      <label class="edit-label">destination<select data-move-to>${opts}</select></label>
      <div class="actions"><button type="button" data-act="move">move all</button></div>
    </div>`
  );
}

function bindProjectMove(project) {
  const moveBtn = rail.querySelector("[data-act=move]");
  if (!moveBtn) return;
  moveBtn.addEventListener("click", async () => {
    const sel = rail.querySelector("[data-move-to]");
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
  main.innerHTML = `
    ${flash()}
    ${box(
      `<strong>${esc(project.identifier)}</strong>${stamp(project.stage || "wayfinding", "open lg")}`,
      `<div class="box-b pad" id="project-head">
        <h1>${esc(project.title)}</h1>
        <div class="body">${dest ? renderMarkdown(dest) : `<span class="muted">no destination</span>`}</div>
        <p class="muted">${repo ? `repo ${esc(repo)}` : "no repo — send to cursor uses the server default"}</p>
        <div class="actions"><button type="button" data-act="edit">edit</button><button type="button" data-act="delete" class="danger">delete project</button></div>
      </div>`
    )}
    ${box(
      `<strong>decision maps</strong>${composeBar("new map on this project")}`,
      (project.maps || []).map((m) => mapRowHTML(m)).join("") || `<div class="empty">no maps</div>`
    )}
    ${box(
      "<strong>spec</strong>",
      (project.specs || []).map(artifactRowHTML).join("") || `<div class="empty">none — make the spec from the map</div>`
    )}
    ${box(
      "<strong>tickets</strong>",
      (project.plans || []).map(artifactRowHTML).join("") || `<div class="empty">none — make tickets from the spec</div>`
    )}
    ${box(
      `<strong>bugs</strong>${composeBar("new bug", "compose-bug")}`,
      (project.bugs || []).map((b) => ticketHTML(b, false, true)).join("") || `<div class="empty">no bugs</div>`
    )}`;
  renderRail(
    box(
      "<strong>this project</strong>",
      railLines([
        ["stage", project.stage || ""],
        ["repo", repo || "server default"],
        ["maps", (project.maps || []).length],
        ["specs", (project.specs || []).length],
        ["tickets", (project.plans || []).length],
        ["bugs", (project.bugs || []).length],
      ])
    ) + projectMoveBox(project)
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
  bindProjectMove(project);
  bindCompose({ labels: ["wayfinder:map"], projectId: project.id });
  bindCompose({ _bug: true, projectId: project.id }, "compose-bug");
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
  const plans = ownArtifacts(project && project.plans, issue, "plan");
  const hasPlan = plans.length > 0;
  const actions = [
    issue.lifecycle === "draft" ? `<button data-act="approve">approve spec</button>` : "",
    issue.lifecycle === "approved" ? `<button data-act="to-tickets">to tickets</button>` : "",
    issue.lifecycle === "approved" && !hasPlan ? `<button data-act="create-plan">create tickets</button>` : "",
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
        <div class="chips">${tagButtons(issue.labels) || `<span class="muted">no tags</span>`}</div>
        <div class="body">${issue.body ? renderMarkdown(issue.body) : `<span class="muted">empty spec — fill via /to-spec</span>`}</div>
        <div class="actions">${actions}</div>
      </div>`
    )}
    ${hasPlan ? box("<strong>tickets</strong>", plans.map(artifactRowHTML).join("")) : ""}`;
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
    `<button data-act="to-tickets">to tickets</button>`,
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
        <div class="chips">${tagButtons(issue.labels) || `<span class="muted">no tags</span>`}</div>
        <div class="body">${issue.body ? renderMarkdown(issue.body) : `<span class="muted">empty — fill via /to-tickets</span>`}</div>
        <div class="actions">${actions}</div>
      </div>`
    )}
    ${relationBox("blocked by", sortRelations(issue.blockers), "not blocked")}
    ${box(
      `<strong>tickets</strong><span class="mark">${open} open · ${done} done</span><div class="subnav">${filters}</div>${composeBar("new ticket")}`,
      `<div class="progress"><i style="width:${pct}%"></i></div>` + (rows || `<div class="empty">no tickets</div>`)
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
      "<strong>tickets</strong>",
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

function mdEditorHTML(id, placeholder, value = "", tab = "preview") {
  const previewing = tab !== "write";
  return `<div class="md-editor" id="${id}">
    <div class="subnav md-tabs">
      <button type="button" data-md-tab="write"${previewing ? "" : ` class="active"`}>write</button>
      <button type="button" data-md-tab="preview"${previewing ? ` class="active"` : ""}>preview</button>
    </div>
    <textarea name="body" placeholder="${placeholder}"${previewing ? " hidden" : ""}>${esc(value)}</textarea>
    <div class="body md-preview"${previewing ? "" : " hidden"}>${mdPreviewHTML(value)}</div>
  </div>`;
}

function showMdTab(root, tab, opts = {}) {
  if (!root) return;
  const ta = root.querySelector("textarea");
  const preview = root.querySelector(".md-preview");
  if (!ta || !preview) return;
  root.querySelectorAll("[data-md-tab]").forEach((b) => b.classList.toggle("active", b.dataset.mdTab === tab));
  const previewing = tab === "preview";
  ta.hidden = previewing;
  preview.hidden = !previewing;
  if (previewing) preview.innerHTML = mdPreviewHTML(ta.value);
  else if (opts.focus !== false) ta.focus();
}

function bindMdEditor(root, tab = "preview") {
  if (!root) return;
  root.querySelectorAll("[data-md-tab]").forEach((btn) => {
    btn.addEventListener("click", () => showMdTab(root, btn.dataset.mdTab));
  });
  showMdTab(root, tab, { focus: false });
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
    ${
      isBug(issue)
        ? ""
        : `${relationBox("blocked by", blockers, "not blocked — frontier when unclaimed")}
    ${blocks.length ? relationBox("blocks", blocks, "") : ""}`
    }
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

function editFormHTML(issue) {
  return `<label class="edit-label">title<input id="edit-title" value="${esc(issue.title).replaceAll('"', "&quot;")}" /></label>
  ${tagPickerHTML(issue)}
  <label class="edit-label">body<textarea id="edit-body">${esc(issue.body || "")}</textarea></label>
  <div class="actions"><button data-save>save</button><button data-cancel>cancel</button></div>`;
}

function bindEditForm(issue) {
  const save = main.querySelector("[data-save]");
  const cancel = main.querySelector("[data-cancel]");
  const picker = main.querySelector("[data-tag-picker]");
  const getTags = picker ? bindTagPicker(picker, issue) : () => [];
  cancel.addEventListener("click", () => paint());
  save.addEventListener("click", async () => {
    const title = main.querySelector("#edit-title").value.trim();
    const body = main.querySelector("#edit-body").value;
    const labels = getTags();
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
    if (e.key === "Escape") {
      if (e.target.matches("select")) return;
      paint();
    }
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
  const slow = kind === "approve" || kind === "to-spec" || kind === "to-plan" || kind === "to-tickets" || kind === "send-cursor";
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
        const target = await api(`/api/issues/${issue.id}/advance-to-plan`, { method: "POST", body: "{}" });
        location.hash = hrefFor(target);
        await sendCursor("to-tickets", target.id);
      } catch (err) {
        state.error = "approved. " + err.message;
      }
      await paint();
      return;
    }
    if (kind === "to-spec" || kind === "to-plan" || kind === "to-tickets" || kind === "send-cursor") {
      const action = kind === "send-cursor" ? "issue" : kind === "to-plan" ? "to-tickets" : kind;
      const wait = {
        issue: "sending to cursor…",
        "to-spec": "starting to spec…",
        "to-tickets": "starting to tickets…",
      };
      armBusyButton(kind, wait[action] || "talking to cursor…");
      let target = issue;
      if (kind === "to-spec") {
        target = await api(`/api/issues/${issue.id}/ensure-spec`, { method: "POST", body: "{}" });
        location.hash = hrefFor(target);
      }
      if (kind === "to-plan" || (kind === "to-tickets" && !isPlan(issue))) {
        target = await api(`/api/issues/${issue.id}/advance-to-plan`, { method: "POST", body: "{}" });
        location.hash = hrefFor(target);
      }
      try {
        await sendCursor(action, target.id);
      } catch (err) {
        if (kind === "to-spec" || kind === "to-plan" || kind === "to-tickets") {
          state.error = `${kind === "to-spec" ? "spec ready." : "tickets ready."} ${err.message}`;
          await paint();
          return;
        }
        throw err;
      }
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
      const label = isMap(issue) ? "map" : isSpec(issue) ? "spec" : isPlan(issue) ? "tickets" : "issue";
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
  stripComposeLayers();
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
    if (si && document.activeElement !== si) {
      si.value = r.name === "search" ? r.query : r.name === "tag" ? "#" + r.label : "";
    }
    if (r.name === "list" && state.filter === "home") {
      try {
        state.error = "";
        await renderHome();
      } catch (err) {
        state.error = err.message;
        main.innerHTML = `<div class="error">${esc(err.message)}</div>`;
        renderRail();
      }
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
    extras = all.filter((i) => isSpec(i) || isPlan(i) || isBug(i));
    groups = groupTicketsByMap(all.filter((i) => !isArtifact(i) && !isBug(i)));
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
  const settings = e.target.closest("[data-go=settings]");
  if (settings) {
    showSettingsLoading();
    location.hash = "#/settings";
    return;
  }
  const b = e.target.closest("[data-filter]");
  if (!b) return;
  setFilter(b.dataset.filter);
  location.hash = "#/";
  paint();
});

document.getElementById("search-form").addEventListener("submit", (e) => {
  e.preventDefault();
  const q = document.getElementById("global-search").value.trim();
  location.hash = searchHash(q);
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
  if (e.key === "Escape" && closeOpenCompose()) {
    e.preventDefault();
    return;
  }
  if (e.target.matches("input, textarea, select")) return;
  if (document.querySelector(".compose-layer:not([hidden])")) return;
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
