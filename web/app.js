const state = {
  filter: localStorage.getItem("nl-filter") || "home",
  mapChildFilter: localStorage.getItem("nl-map-filter") || "open",
  issues: [],
  selected: 0,
  error: "",
  stats: { all: 0, open: 0, closed: 0, maps: 0, frontier: 0, blocked: 0 },
  labels: [],
};

const $ = (sel, el = document) => el.querySelector(sel);
const main = $("#main");
const rail = $("#rail");

function route() {
  const hash = location.hash.replace(/^#/, "") || "/";
  const map = hash.match(/^\/map\/(\d+)$/);
  if (map) return { name: "map", id: Number(map[1]) };
  const issue = hash.match(/^\/(\d+)$/);
  if (issue) return { name: "issue", id: Number(issue[1]) };
  return { name: "list" };
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

function rowHTML(issue, selected, nav = true) {
  const m = mark(issue);
  const extra = issue.assignee
    ? `@${issue.assignee}`
    : issue.openBlockers
      ? `blocked×${issue.openBlockers}`
      : "";
  return `<a class="row ${selected ? "selected" : ""}" ${nav ? "data-nav" : ""} href="${hrefFor(issue)}" data-id="${issue.id}">
    <span class="id">${issue.identifier}</span>
    <span class="title">${esc(issue.title)}</span>
    <span class="meta">${esc((issue.labels || []).join(" "))}</span>
    <span class="mark ${m.cls}">${extra || m.ch}</span>
  </a>`;
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
  const [allData, labelData] = await Promise.all([api("/api/issues"), api("/api/labels")]);
  const all = allData.issues || [];
  state.stats = {
    all: all.length,
    open: all.filter((i) => i.state === "open").length,
    closed: all.filter((i) => i.state === "closed").length,
    maps: all.filter(isMap).length,
    frontier: all.filter((i) => i.frontier).length,
    blocked: all.filter((i) => i.blocked && i.state === "open").length,
  };
  state.labels = labelData.labels || [];
  document.querySelectorAll("[data-count]").forEach((el) => {
    const n = state.stats[el.dataset.count];
    el.textContent = n || "";
  });
  $("#counts").innerHTML = `<b>${state.stats.open}</b> open · <b class="take">${state.stats.frontier}</b> frontier`;
}

function renderRail(extraHTML = "") {
  const s = state.stats;
  rail.innerHTML =
    box(
      "<strong>this tracker</strong>",
      `<div class="box-b pad">
        <div class="rail-line">open <b>${s.open}</b></div>
        <div class="rail-line">frontier <b>${s.frontier}</b></div>
        <div class="rail-line">blocked <b>${s.blocked}</b></div>
        <div class="rail-line">maps <b>${s.maps}</b></div>
        <div class="rail-line">closed <b>${s.closed}</b></div>
      </div>`
    ) +
    box(
      "<strong>labels</strong>",
      `<div class="tags">${state.labels.map((l) => `<span>#${esc(l)}</span>`).join(" ")}</div>`
    ) +
    extraHTML;
}

async function loadList() {
  const q = new URLSearchParams();
  if (state.filter === "open" || state.filter === "closed") q.set("state", state.filter);
  if (state.filter === "frontier") q.set("frontier", "1");
  if (state.filter === "maps") q.set("labels", "wayfinder:map");
  const data = await api("/api/issues?" + q.toString());
  state.issues = data.issues || [];
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

function renderList() {
  const title =
    state.filter === "maps" ? "maps" : state.filter === "frontier" ? "frontier" : state.filter === "open" ? "open" : state.filter === "closed" ? "closed" : "all";
  const hint = state.filter === "maps" ? "new map" : "new issue";
  const extra = state.filter === "maps" ? { labels: ["wayfinder:map"] } : {};
  const rows = state.issues.map((issue, i) => rowHTML(issue, i === state.selected)).join("");
  main.innerHTML = `${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}${listBox(title, hint, rows)}`;
  bindCompose(extra);
  syncChrome();
}

async function renderHome() {
  const [front, maps] = await Promise.all([
    api("/api/issues?frontier=1"),
    api("/api/issues?labels=wayfinder:map"),
  ]);
  const frontier = front.issues || [];
  const mapList = maps.issues || [];
  state.issues = frontier;
  main.innerHTML = `
    ${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}
    ${box(
      "<strong>frontier</strong><span>takeable now</span>",
      frontier.map((issue, i) => rowHTML(issue, i === state.selected)).join("") || `<div class="empty">nothing takeable</div>`,
      composeBar("new issue")
    )}
    ${box(
      `<strong>maps</strong><a href="#/" data-jump="maps">open maps view</a>`,
      mapList.map((issue) => rowHTML(issue, false, false)).join("") || `<div class="empty">no maps</div>`
    )}`;
  bindCompose({});
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

function syncChrome() {
  const r = route();
  $("#tab-home").classList.toggle("active", r.name === "list" && state.filter === "home");
  document.querySelectorAll("nav button").forEach((b) => {
    const onMap = r.name === "map" && b.dataset.filter === "maps";
    b.classList.toggle("active", onMap || (r.name === "list" && b.dataset.filter === state.filter));
  });
  if (r.name === "list") $("#crumb").textContent = state.filter;
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
  const rows = state.issues.map((issue, i) => rowHTML(issue, i === state.selected)).join("");
  const kids = map.children || [];
  $("#crumb").textContent = `maps / ${map.identifier}`;
  main.innerHTML = `
    ${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}
    ${box(
      `<strong>${esc(map.identifier)}</strong><span>${map.state}</span>`,
      `<div class="box-b pad">
        <div class="kicker"><a href="#/">← maps</a></div>
        <h1>${esc(map.title)}</h1>
        <div class="body map-body">${esc(map.body) || `<span class="muted">empty map body</span>`}</div>
        <div class="actions"><button data-act="edit">edit map</button></div>
      </div>`
    )}
    ${box(
      `<strong>tickets</strong><div class="subnav">${filters}</div>`,
      rows || `<div class="empty">no tickets on this map</div>`,
      composeBar("new ticket on this map")
    )}`;
  bindCompose({ parentId: map.id });
  main.querySelector("[data-act=edit]").addEventListener("click", () => act(map, "edit"));
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
      `<div class="box-b pad">
        <div class="rail-line">children <b>${kids.length}</b></div>
        <div class="rail-line">open <b>${kids.filter((c) => c.state === "open").length}</b></div>
        <div class="rail-line">frontier <b>${kids.filter((c) => c.frontier).length}</b></div>
        <div class="rail-line">blocked <b>${kids.filter((c) => c.blocked).length}</b></div>
      </div>`
    )
  );
  syncChrome();
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
  const m = mark(issue);
  const comments = (issue.comments || [])
    .map(
      (c) => `<div class="comment">
        <div class="who">${esc(c.author)} · ${esc(c.createdAt).slice(0, 19).replace("T", " ")}</div>
        <div class="text">${esc(c.body)}</div>
      </div>`
    )
    .join("");
  const parent = issue.parent
    ? `<a href="${hrefFor(issue.parent)}">${esc(issue.parent.identifier)} ${esc(issue.parent.title)}</a>`
    : "—";
  const blockers = (issue.blockers || []).map((b) => `<a href="${hrefFor(b)}">${esc(b.identifier)}</a>`).join(" ") || "—";
  $("#crumb").textContent = issue.identifier;
  main.innerHTML = `
    ${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}
    ${box(
      `<strong>${esc(issue.identifier)}</strong><span class="mark ${m.cls}">${m.ch} ${issue.state}${issue.assignee ? " @" + esc(issue.assignee) : ""}</span>`,
      `<div class="box-b pad">
        <div class="kicker"><a href="${backHref(issue)}">←</a></div>
        <h1>${esc(issue.title)}</h1>
        <div class="chips">${(issue.labels || []).map((l) => `<span class="chip">${esc(l)}</span>`).join("") || `<span class="muted">no labels</span>`}</div>
        <div class="body">${esc(issue.body) || `<span class="muted">empty body</span>`}</div>
        <div class="actions">
          ${issue.state === "open" && !issue.assignee ? `<button data-act="claim">claim</button>` : ""}
          ${issue.assignee && issue.state === "open" ? `<button data-act="unclaim">unclaim</button>` : ""}
          ${issue.state === "open" ? `<button data-act="close">close</button>` : `<button data-act="reopen">reopen</button>`}
          <button data-act="edit">edit</button>
        </div>
      </div>`
    )}
    ${box(
      "<strong>comments</strong>",
      `${comments || `<div class="empty">none</div>`}
       <form id="comment" class="box-b pad">
         <textarea name="body" placeholder="comment · ctrl+enter"></textarea>
         <div class="actions"><button type="submit">add comment</button></div>
       </form>`
    )}`;
  renderRail(
    box(
      "<strong>links</strong>",
      `<div class="box-b pad">
        <div class="rail-line">parent <b>${parent}</b></div>
        <div class="rail-line">blocked by <b>${blockers}</b></div>
        <div class="rail-line">project <b>${esc(issue.project)}</b></div>
      </div>`
    )
  );
  main.querySelectorAll("[data-act]").forEach((btn) => btn.addEventListener("click", () => act(issue, btn.dataset.act)));
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

async function act(issue, kind) {
  try {
    if (kind === "claim") await api(`/api/issues/${issue.id}/claim`, { method: "POST", body: "{}" });
    if (kind === "unclaim") await api(`/api/issues/${issue.id}`, { method: "PATCH", body: JSON.stringify({ assignee: "" }) });
    if (kind === "close") await api(`/api/issues/${issue.id}`, { method: "PATCH", body: JSON.stringify({ state: "closed" }) });
    if (kind === "reopen") await api(`/api/issues/${issue.id}`, { method: "PATCH", body: JSON.stringify({ state: "open" }) });
    if (kind === "edit") {
      const title = prompt("title", issue.title);
      if (title == null) return;
      const body = prompt("body", issue.body || "");
      if (body == null) return;
      await api(`/api/issues/${issue.id}`, { method: "PATCH", body: JSON.stringify({ title, body }) });
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
  if (r.name === "map") {
    await renderMap(r.id);
    return;
  }
  await renderIssue(r.id);
}

document.querySelector("nav").addEventListener("click", (e) => {
  const b = e.target.closest("[data-filter]");
  if (!b) return;
  state.filter = b.dataset.filter;
  localStorage.setItem("nl-filter", state.filter);
  location.hash = "#/";
  paint();
});

function highlightSelected() {
  document.querySelectorAll("main .row[data-nav]").forEach((el, i) => el.classList.toggle("selected", i === state.selected));
}

window.addEventListener("hashchange", paint);
window.addEventListener("keydown", (e) => {
  if (e.target.matches("input, textarea")) return;
  const r = route();
  if (r.name === "issue") {
    if (e.key === "Escape") history.back();
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
  if (e.key === "Enter" && state.issues[state.selected]) location.hash = hrefFor(state.issues[state.selected]);
  if (e.key === "/") {
    e.preventDefault();
    const input = document.querySelector(".banner input, .compose input");
    if (input) input.focus();
  }
  if (e.key === "Escape" && r.name === "map") location.hash = "#/";
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
