const state = {
  filter: localStorage.getItem("nl-filter") || "maps",
  mapChildFilter: localStorage.getItem("nl-map-filter") || "open",
  issues: [],
  selected: 0,
  error: "",
};

const $ = (sel, el = document) => el.querySelector(sel);
const main = $("#main");
const statusEl = $("#status");

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

function labels(issue) {
  return (issue.labels || []).join(" ");
}

function rowHTML(issue, selected) {
  const m = mark(issue);
  const extra = issue.assignee
    ? `@${issue.assignee}`
    : issue.openBlockers
      ? `blocked×${issue.openBlockers}`
      : "";
  return `<a class="row ${selected ? "selected" : ""}" href="${hrefFor(issue)}" data-id="${issue.id}">
    <span class="id">${issue.identifier}</span>
    <span class="title">${esc(issue.title)}</span>
    <span class="meta">${esc(labels(issue))}</span>
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

function bindCompose(form, extra) {
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const input = e.target.title;
    const title = input.value.trim();
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

function renderList() {
  const rows = state.issues.map((issue, i) => rowHTML(issue, i === state.selected)).join("");
  const hint = state.filter === "maps" ? "new map" : "new issue";
  main.innerHTML = `
    ${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}
    ${rows || `<div class="empty">no issues</div>`}
    <form class="compose" id="compose">
      <span class="muted">&gt;</span>
      <input name="title" placeholder="${hint}" autocomplete="off" />
    </form>`;
  const extra = state.filter === "maps" ? { labels: ["wayfinder:map"] } : {};
  bindCompose($("#compose"), extra);
  syncNav();
}

function syncNav() {
  const r = route();
  document.querySelectorAll("nav button").forEach((b) => {
    const onMap = r.name === "map" && b.dataset.filter === "maps";
    b.classList.toggle("active", onMap || (r.name === "list" && b.dataset.filter === state.filter));
  });
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
    .map(
      (f) =>
        `<button data-map-filter="${f}" class="${f === state.mapChildFilter ? "active" : ""}">${f}</button>`
    )
    .join("");
  const rows = state.issues.map((issue, i) => rowHTML(issue, i === state.selected)).join("");
  statusEl.textContent = `map ${map.identifier}`;
  main.innerHTML = `
    <div class="map-head">
      ${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}
      <div class="kicker"><a href="#/">← maps</a>  ${esc(map.identifier)}  ${map.state}  <a class="muted" href="#/${map.id}">issue</a></div>
      <h1>${esc(map.title)}</h1>
      <div class="body map-body">${esc(map.body) || `<span class="muted">empty map body</span>`}</div>
      <div class="actions">
        <button data-act="edit">edit map</button>
      </div>
      <div class="subnav">${filters}</div>
    </div>
    ${rows || `<div class="empty">no tickets on this map</div>`}
    <form class="compose" id="compose">
      <span class="muted">&gt;</span>
      <input name="title" placeholder="new ticket on this map" autocomplete="off" />
    </form>`;
  bindCompose($("#compose"), { parentId: map.id });
  main.querySelector("[data-act=edit]").addEventListener("click", () => act(map, "edit"));
  main.querySelectorAll("[data-map-filter]").forEach((b) => {
    b.addEventListener("click", () => {
      state.mapChildFilter = b.dataset.mapFilter;
      localStorage.setItem("nl-map-filter", state.mapChildFilter);
      paint();
    });
  });
  syncNav();
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
  const blockers = (issue.blockers || [])
    .map((b) => `<a href="${hrefFor(b)}">${esc(b.identifier)}</a>`)
    .join(" ") || "—";
  statusEl.textContent = `${issue.identifier} ${issue.state}`;
  main.innerHTML = `
    <article class="issue">
      ${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}
      <div class="kicker"><a href="${backHref(issue)}">←</a>  ${esc(issue.identifier)}  <span class="mark ${m.cls}">${m.ch}</span>  ${issue.state}${issue.assignee ? "  @" + esc(issue.assignee) : ""}</div>
      <h1>${esc(issue.title)}</h1>
      <div class="chips">
        ${(issue.labels || []).map((l) => `<span class="chip">${esc(l)}</span>`).join("") || `<span class="muted">no labels</span>`}
      </div>
      <div class="body">${esc(issue.body) || `<span class="muted">empty body</span>`}</div>
      <div class="chips">
        <span class="chip">parent ${parent}</span>
        <span class="chip">blocked by ${blockers}</span>
        <span class="chip">${esc(issue.project)}</span>
      </div>
      <div class="actions">
        ${issue.state === "open" && !issue.assignee ? `<button data-act="claim">claim</button>` : ""}
        ${issue.assignee && issue.state === "open" ? `<button data-act="unclaim">unclaim</button>` : ""}
        ${issue.state === "open" ? `<button data-act="close">close</button>` : `<button data-act="reopen">reopen</button>`}
        <button data-act="edit">edit</button>
      </div>
      <section class="comments">
        <h2>comments</h2>
        ${comments || `<div class="muted">none</div>`}
        <form id="comment">
          <textarea name="body" placeholder="comment · ctrl+enter"></textarea>
          <div class="actions"><button type="submit">add comment</button></div>
        </form>
      </section>
    </article>`;

  main.querySelectorAll("[data-act]").forEach((btn) => {
    btn.addEventListener("click", () => act(issue, btn.dataset.act));
  });
  $("#comment").addEventListener("submit", async (e) => {
    e.preventDefault();
    const body = e.target.body.value.trim();
    if (!body) return;
    await api(`/api/issues/${issue.id}/comments`, {
      method: "POST",
      body: JSON.stringify({ author: "me", body }),
    });
    renderIssue(issue.id);
  });
  $("#comment textarea").addEventListener("keydown", (e) => {
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      $("#comment").requestSubmit();
    }
  });
  syncNav();
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
  const r = route();
  if (r.name === "list") {
    statusEl.textContent = state.filter;
    try {
      await loadList();
      state.error = "";
    } catch (err) {
      state.error = err.message;
      state.issues = [];
    }
    renderList();
    return;
  }
  if (r.name === "map") {
    await renderMap(r.id);
    return;
  }
  await renderIssue(r.id);
}

document.querySelectorAll("nav button").forEach((b) => {
  b.addEventListener("click", () => {
    state.filter = b.dataset.filter;
    localStorage.setItem("nl-filter", state.filter);
    location.hash = "#/";
    paint();
  });
});

function highlightSelected() {
  document.querySelectorAll("main .row").forEach((el, i) => {
    el.classList.toggle("selected", i === state.selected);
  });
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
    state.selected = Math.min(state.issues.length - 1, state.selected + 1);
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
    const input = document.querySelector(".compose input");
    if (input) input.focus();
  }
  if (e.key === "Escape" && r.name === "map") location.hash = "#/";
});

paint();
