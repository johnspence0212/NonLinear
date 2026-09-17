const state = {
  filter: localStorage.getItem("nl-filter") || "home",
  mapChildFilter: localStorage.getItem("nl-map-filter") || "open",
  issues: [],
  selected: 0,
  error: "",
  stats: { all: 0, open: 0, closed: 0, maps: 0, frontier: 0, blocked: 0 },
  labels: [],
  version: "",
  dataPath: "",
  issueBackHref: "#/",
};

const $ = (sel, el = document) => el.querySelector(sel);
const main = $("#main");
const rail = $("#rail");

function route() {
  const hash = location.hash.replace(/^#/, "") || "/";
  const map = hash.match(/^\/map\/(\d+)$/);
  if (map) return { name: "map", id: Number(map[1]) };
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
  return (issue.labels || []).includes("wayfinder:map");
}

function hrefFor(issue) {
  return isMap(issue) ? `#/map/${issue.id}` : `#/${issue.id}`;
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

function stamp(label, kind = "") {
  return `<span class="stamp ${kind}">${esc(label)}</span>`;
}

function statusStamp(issue, lg = "") {
  const size = lg ? ` ${lg}` : "";
  if (issue.state === "closed") return stamp("closed", `closed${size}`);
  if (issue.blocked) return stamp("blocked", `blocked${size}`);
  if (issue.assignee) return stamp("claimed", `claim${size}`);
  if (issue.frontier) return stamp("open", `take${size}`);
  return stamp("open", `open${size}`);
}

function rowHTML(issue, selected, nav = true) {
  const m = mark(issue);
  const closed = issue.state === "closed";
  const extra = closed
    ? stamp("closed", "closed")
    : issue.blocked
      ? stamp("blocked", "blocked")
      : issue.assignee
        ? `@${issue.assignee}`
        : m.ch;
  return `<div class="row ${selected ? "selected" : ""} ${closed ? "is-closed" : ""}" ${nav ? "data-nav" : ""} data-id="${issue.id}">
    <a class="id" href="${hrefFor(issue)}">${issue.identifier}</a>
    <a class="title" href="${hrefFor(issue)}">${esc(issue.title)}</a>
    <span class="meta">${tagButtons(issue.labels)}</span>
    <span class="mark ${m.cls}">${extra}</span>
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

function esc(s) {
  return String(s ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

function backHref(issue) {
  if (issue.parent && isMap(issue.parent)) return `#/map/${issue.parent.id}`;
  if (issue.parent) return `#/${issue.parent.id}`;
  return "#/";
}

function box(titleRight, inner, footer = "") {
  return `<section class="box"><div class="box-h">${titleRight}</div><div class="box-b">${inner}</div>${footer}</section>`;
}

function composeBar(placeholder) {
  return `<form class="banner" id="compose"><span>+</span><input name="title" placeholder="${placeholder}" autocomplete="off" /></form>`;
}

function bindCompose(extra) {
  const form = $("#compose");
  if (!form) return;
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const title = form.title.value.trim();
    if (!title) return;
    try {
      const created = await api("/api/issues", {
        method: "POST",
        body: JSON.stringify({ title, ...extra }),
      });
      location.hash = hrefFor(created);
    } catch (err) {
      state.error = err.message;
      paint();
    }
  });
}

async function refreshStats() {
  const [allData, labelData, health] = await Promise.all([
    api("/api/issues"),
    api("/api/labels"),
    api("/api/health"),
  ]);
  state.version = health.version || "";
  state.dataPath = health.data || "";
  const ver = $("#version");
  if (ver) ver.textContent = state.version ? "v" + state.version : "";
  const all = allData.issues || [];
  const tickets = all.filter((i) => !isMap(i));
  state.stats = {
    all: tickets.length,
    total: all.length,
    open: tickets.filter((i) => i.state === "open").length,
    closed: tickets.filter((i) => i.state === "closed").length,
    maps: all.filter(isMap).length,
    frontier: tickets.filter((i) => i.frontier).length,
    blocked: tickets.filter((i) => i.blocked && i.state === "open").length,
  };
  state.labels = labelData.labels || [];
  document.querySelectorAll("[data-count]").forEach((el) => {
    const n = state.stats[el.dataset.count];
    el.textContent = n || "";
  });
  $("#counts").innerHTML = `<b>${state.stats.open}</b> open tickets · <b class="take">${state.stats.frontier}</b> frontier`;
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
  const q = new URLSearchParams();
  if (state.filter === "open" || state.filter === "closed") q.set("state", state.filter);
  if (state.filter === "frontier") q.set("frontier", "1");
  if (state.filter === "maps") q.set("labels", "wayfinder:map");
  const data = await api("/api/issues?" + q.toString());
  const fetched = data.issues || [];
  state.issues = state.filter === "maps" ? fetched : fetched.filter((i) => !isMap(i));
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

function listBox(title, placeholder, rows) {
  return box(
    `<strong>${title}</strong>`,
    rows || `<div class="empty">none</div>`,
    composeBar(placeholder)
  );
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

function flattenGroups(groups) {
  return groups.flatMap((g) => g.tickets);
}

function groupedTicketHTML(groups) {
  let i = 0;
  return groups
    .map((g) => {
      const head = g.parent
        ? `<a class="group-map" href="${hrefFor(g.parent)}">${esc(g.parent.title)}</a>`
        : `<span class="group-map">inbox · no map</span>`;
      const n = g.tickets.length;
      const rows = g.tickets
        .map((t) => {
          const sel = i === state.selected;
          i++;
          return ticketHTML(t, sel);
        })
        .join("");
      return `<div class="group"><div class="group-h">${head}<span>${n} ticket${n === 1 ? "" : "s"}</span></div><div class="group-rows">${rows}</div></div>`;
    })
    .join("");
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
  state.issues = flattenGroups(groups);
  main.innerHTML = `${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}${box(
    `<strong>#${esc(label)}</strong><a href="#/" class="muted">clear</a>`,
    groupedTicketHTML(groups) || `<div class="empty">no tickets with this tag</div>`,
    ticketHint()
  )}`;
  syncChrome();
}

async function loadTag(label) {
  const data = await api("/api/issues?" + new URLSearchParams({ labels: label }).toString());
  const fetched = data.issues || [];
  state.issues = label === "wayfinder:map" ? fetched : fetched.filter((i) => !isMap(i));
  if (state.selected >= state.issues.length) state.selected = 0;
}

function renderList() {
  if (state.filter === "maps") {
    const rows = state.issues.map((issue, i) => mapRowHTML(issue, i === state.selected, true)).join("");
    main.innerHTML = `${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}${listBox("maps", "new map", rows)}`;
    bindCompose({ labels: ["wayfinder:map"] });
    syncChrome();
    return;
  }
  const title =
    state.filter === "frontier" ? "frontier" : state.filter === "open" ? "open" : state.filter === "closed" ? "closed" : "all";
  const groups = groupTicketsByMap(state.issues);
  state.issues = flattenGroups(groups);
  main.innerHTML = `${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}${box(
    `<strong>${title}</strong><span>tickets by map</span>`,
    groupedTicketHTML(groups) || `<div class="empty">no tickets — open a map to add one</div>`,
    ticketHint()
  )}`;
  syncChrome();
}

async function renderHome() {
  const [front, maps] = await Promise.all([
    api("/api/issues?frontier=1"),
    api("/api/issues?labels=wayfinder:map"),
  ]);
  const frontier = (front.issues || []).filter((i) => !isMap(i));
  const mapList = maps.issues || [];
  const groups = groupTicketsByMap(frontier);
  state.issues = flattenGroups(groups);
  if (state.selected >= state.issues.length) state.selected = 0;
  main.innerHTML = `
    ${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}
    ${statStrip()}
    ${box(
      "<strong>frontier</strong><span>takeable tickets by map</span>",
      groupedTicketHTML(groups) || `<div class="empty">nothing takeable</div>`,
      ticketHint()
    )}
    ${box(
      `<strong>maps</strong><a href="#/" data-jump="maps">open maps view</a>`,
      mapList.map((m) => mapRowHTML(m)).join("") || `<div class="empty">no maps</div>`,
      composeBar("new map")
    )}`;
  bindCompose({ labels: ["wayfinder:map"] });
  const jump = main.querySelector("[data-jump=maps]");
  if (jump) {
    jump.addEventListener("click", (e) => {
      e.preventDefault();
      state.filter = "maps";
      localStorage.setItem("nl-filter", "maps");
      location.hash = "#/";
      paint();
    });
  }
  syncChrome();
}

function goBack() {
  const r = route();
  if (r.name === "issue") {
    location.hash = state.issueBackHref || "#/";
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

function syncChrome() {
  const r = route();
  document.querySelectorAll("nav button").forEach((b) => {
    if (b.dataset.go === "settings") {
      b.classList.toggle("active", r.name === "settings");
      return;
    }
    const onMap = r.name === "map" && b.dataset.filter === "maps";
    b.classList.toggle("active", onMap || (r.name === "list" && b.dataset.filter === state.filter));
  });
  const back = $("#back");
  if (back) {
    const home = r.name === "list" && state.filter === "home";
    back.hidden = home;
  }
}

async function renderMap(id) {
  let map;
  try {
    map = await api("/api/issues/" + id);
    await loadMapChildren(id);
    state.error = "";
  } catch (err) {
    main.innerHTML = `<div class="error">${esc(err.message)}</div>`;
    return;
  }
  const filters = ["open", "frontier", "closed", "all"]
    .map((f) => `<button data-map-filter="${f}" class="${f === state.mapChildFilter ? "active" : ""}">${f}</button>`)
    .join("");
  const rows = state.issues.map((issue, i) => ticketHTML(issue, i === state.selected)).join("");
  const kids = map.children || [];
  main.innerHTML = `
    ${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}
    ${box(
      `<strong>${esc(map.identifier)}</strong>${stamp(map.state === "closed" ? "closed" : "open", map.state === "closed" ? "closed lg" : "open lg")}`,
      `<div class="box-b pad" id="issue-head">
        <h1>${esc(map.title)}</h1>
        <div class="body map-body">${map.body ? renderMarkdown(map.body) : `<span class="muted">empty map body</span>`}</div>
        <div class="actions">
          <button data-act="edit">edit map</button>
          <button data-act="delete" class="danger">delete map</button>
        </div>
      </div>`
    )}
    ${box(
      `<strong>tickets</strong><div class="subnav">${filters}</div>`,
      rows || `<div class="empty">no tickets on this map</div>`,
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

function mdEditorHTML(id, placeholder, value = "") {
  return `<div class="md-editor" id="${id}">
    <div class="subnav md-tabs">
      <button type="button" data-md-tab="write" class="active">write</button>
      <button type="button" data-md-tab="preview">preview</button>
    </div>
    <textarea name="body" placeholder="${placeholder}">${esc(value)}</textarea>
    <div class="body md-preview" hidden></div>
  </div>`;
}

function bindMdEditor(root) {
  if (!root) return;
  const ta = root.querySelector("textarea");
  const preview = root.querySelector(".md-preview");
  root.querySelectorAll("[data-md-tab]").forEach((btn) => {
    btn.addEventListener("click", () => {
      const tab = btn.dataset.mdTab;
      root.querySelectorAll("[data-md-tab]").forEach((b) => b.classList.toggle("active", b === btn));
      const previewing = tab === "preview";
      ta.hidden = previewing;
      preview.hidden = !previewing;
      if (previewing) {
        const src = ta.value.trim();
        preview.innerHTML = src ? renderMarkdown(src) : `<span class="muted">nothing to preview</span>`;
      } else {
        ta.focus();
      }
    });
  });
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
  const ta = el.querySelector("textarea");
  if (ta) {
    ta.focus();
    ta.setSelectionRange(ta.value.length, ta.value.length);
  }
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
  state.issueBackHref = backHref(issue);
  const closed = issue.state === "closed";
  const comments = (issue.comments || []).map(commentHTML).join("");
  const blockers = sortRelations(issue.blockers);
  const blocks = sortRelations(issue.blocks);
  main.innerHTML = `
    ${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}
    ${box(
      `<strong>${esc(issue.identifier)}</strong>${statusStamp(issue, "lg")}`,
      `<div class="box-b pad ${closed ? "is-closed" : ""}" id="issue-head">
        <h1>${esc(issue.title)}</h1>
        <div class="chips">${tagButtons(issue.labels) || `<span class="muted">no labels</span>`}</div>
        <div class="body">${issue.body ? renderMarkdown(issue.body) : `<span class="muted">empty body</span>`}</div>
        <div class="actions">
          ${issue.state === "open" && !issue.assignee ? `<button data-act="claim">claim</button>` : ""}
          ${issue.assignee && issue.state === "open" ? `<button data-act="unclaim">unclaim</button>` : ""}
          ${issue.state === "open" ? `<button data-act="close">close</button>` : `<button data-act="reopen">reopen</button>`}
          <button data-act="edit">edit</button>
          <button data-act="delete" class="danger">delete</button>
        </div>
      </div>`
    )}
    ${relationBox("blocked by", blockers, "not blocked — takeable when unassigned")}
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

function editFormHTML(issue) {
  return `<label class="edit-label">title<input id="edit-title" value="${esc(issue.title).replaceAll('"', "&quot;")}" /></label>
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
    if (!title) return;
    try {
      await api(`/api/issues/${issue.id}`, { method: "PATCH", body: JSON.stringify({ title, body }) });
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

async function act(issue, kind) {
  try {
    if (kind === "claim") await api(`/api/issues/${issue.id}/claim`, { method: "POST", body: "{}" });
    if (kind === "unclaim") await api(`/api/issues/${issue.id}`, { method: "PATCH", body: JSON.stringify({ assignee: "" }) });
    if (kind === "close") await api(`/api/issues/${issue.id}`, { method: "PATCH", body: JSON.stringify({ state: "closed" }) });
    if (kind === "reopen") await api(`/api/issues/${issue.id}`, { method: "PATCH", body: JSON.stringify({ state: "open" }) });
    if (kind === "edit") {
      startInlineEdit(issue);
      return;
    }
    if (kind === "delete") {
      const kids = (issue.children || []).length;
      const label = isMap(issue) ? "map" : "issue";
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
  try {
    await refreshStats();
  } catch (err) {
    state.error = err.message;
  }
  const r = route();
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
    renderSettings();
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
  await renderIssue(r.id);
}

async function renderSearch(query) {
  let mapList = [];
  let groups = [];
  try {
    const data = await api("/api/issues?" + new URLSearchParams({ query }).toString());
    const all = data.issues || [];
    mapList = all.filter(isMap);
    groups = groupTicketsByMap(all.filter((i) => !isMap(i)));
    state.issues = flattenGroups(groups);
    if (state.selected >= state.issues.length) state.selected = 0;
    state.error = "";
  } catch (err) {
    state.error = err.message;
    state.issues = [];
  }
  const n = mapList.length + state.issues.length;
  main.innerHTML = `
    ${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}
    ${box(
      `<strong>search</strong><span>${esc(query)} · ${n} match${n === 1 ? "" : "es"}</span><a href="#/" class="muted">clear</a>`,
      mapList.map((m) => mapRowHTML(m)).join("") || `<div class="empty">no maps match</div>`
    )}
    ${box(
      `<strong>tickets</strong><span>by map</span>`,
      groupedTicketHTML(groups) || `<div class="empty">no tickets match</div>`,
      ticketHint()
    )}`;
  syncChrome();
}

function renderSettings() {
  const s = state.stats;
  main.innerHTML = `${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}${box(
    "<strong>settings</strong>",
    `<div class="box-b pad">
      <div class="settings-meta">
        <div class="rail-line">version <b id="shown-version">${esc(state.version)}</b></div>
        <div class="rail-line">issues <b>${s.total ?? s.all}</b></div>
        <div class="rail-line">tickets <b>${s.all}</b></div>
        <div class="rail-line">maps <b>${s.maps}</b></div>
        <div class="rail-line">data <b>${esc(state.dataPath)}</b></div>
      </div>
      <p class="settings-warn">wipe deletes every issue. next create is NL-1. this cannot be undone.</p>
      <div class="actions">
        <button type="button" id="wipe" class="danger">wipe database</button>
      </div>
    </div>`
  )}`;
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
    location.hash = "#/settings";
    return;
  }
  const b = e.target.closest("[data-filter]");
  if (!b) return;
  state.filter = b.dataset.filter;
  localStorage.setItem("nl-filter", state.filter);
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

document.getElementById("back").addEventListener("click", (e) => {
  e.preventDefault();
  goBack();
});

document.getElementById("refresh").addEventListener("click", () => {
  paint();
});

document.getElementById("app").addEventListener("click", (e) => {
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
  if (r.name === "issue") {
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
    state.selected = Math.min(Math.max(state.issues.length - 1, 0), state.selected + 1);
    highlightSelected();
  }
  if (e.key === "k") {
    state.selected = Math.max(0, state.selected - 1);
    highlightSelected();
  }
  if (e.key === "Enter" && state.issues[state.selected]) {
    location.hash = hrefFor(state.issues[state.selected]);
  }
  if (e.key === "/") {
    e.preventDefault();
    const input = $("#global-search");
    if (input) input.focus();
  }
  if (e.key === "Escape" && (r.name === "map" || r.name === "tag" || r.name === "settings" || r.name === "search")) goBack();
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
