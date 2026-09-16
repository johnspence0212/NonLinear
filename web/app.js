const state = {
  filter: localStorage.getItem("nl-filter") || "open",
  issues: [],
  selected: 0,
  error: "",
};

const $ = (sel, el = document) => el.querySelector(sel);
const main = $("#main");
const statusEl = $("#status");

function route() {
  const hash = location.hash.replace(/^#/, "") || "/";
  const m = hash.match(/^\/(\d+)$/);
  if (m) return { name: "issue", id: Number(m[1]) };
  return { name: "list" };
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
  return `<a class="row ${selected ? "selected" : ""}" href="#/${issue.id}" data-id="${issue.id}">
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

async function loadList() {
  const q = new URLSearchParams();
  if (state.filter === "open" || state.filter === "closed") q.set("state", state.filter);
  if (state.filter === "frontier") q.set("frontier", "1");
  if (state.filter === "maps") q.set("labels", "wayfinder:map");
  const data = await api("/api/issues?" + q.toString());
  state.issues = data.issues || [];
  if (state.selected >= state.issues.length) state.selected = 0;
}

function renderList() {
  const rows = state.issues.map((issue, i) => rowHTML(issue, i === state.selected)).join("");
  main.innerHTML = `
    ${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}
    ${rows || `<div class="empty">no issues</div>`}
    <form class="compose" id="compose">
      <span class="muted">&gt;</span>
      <input name="title" placeholder="new issue" autocomplete="off" />
    </form>`;
  $("#compose").addEventListener("submit", async (e) => {
    e.preventDefault();
    const input = e.target.title;
    const title = input.value.trim();
    if (!title) return;
    try {
      const created = await api("/api/issues", {
        method: "POST",
        body: JSON.stringify({ title }),
      });
      location.hash = "#/" + created.id;
    } catch (err) {
      state.error = err.message;
      renderList();
    }
  });
  $$nav();
}

function $$nav() {
  document.querySelectorAll("nav button").forEach((b) => {
    b.classList.toggle("active", b.dataset.filter === state.filter);
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
  const m = mark(issue);
  const tree = (issue.children || [])
    .map((c) => {
      const cm = mark(c);
      return `<a class="row" href="#/${c.id}">
        <span class="indent">└</span>
        <span class="id">${c.identifier}</span>
        <span class="title">${esc(c.title)}</span>
        <span class="mark ${cm.cls}">${cm.ch} ${c.assignee ? "@" + esc(c.assignee) : c.blocked ? "blocked" : ""}</span>
      </a>`;
    })
    .join("");
  const comments = (issue.comments || [])
    .map(
      (c) => `<div class="comment">
        <div class="who">${esc(c.author)} · ${esc(c.createdAt).slice(0, 19).replace("T", " ")}</div>
        <div class="text">${esc(c.body)}</div>
      </div>`
    )
    .join("");
  const parent = issue.parent
    ? `<a href="#/${issue.parent.id}">${esc(issue.parent.identifier)} ${esc(issue.parent.title)}</a>`
    : "—";
  const blockers = (issue.blockers || [])
    .map((b) => `<a href="#/${b.id}">${esc(b.identifier)}</a>`)
    .join(" ") || "—";
  statusEl.textContent = `${issue.identifier} ${issue.state}`;
  main.innerHTML = `
    <article class="issue">
      ${state.error ? `<div class="error">${esc(state.error)}</div>` : ""}
      <div class="kicker"><a href="#/">←</a>  ${esc(issue.identifier)}  <span class="mark ${m.cls}">${m.ch}</span>  ${issue.state}${issue.assignee ? "  @" + esc(issue.assignee) : ""}</div>
      <h1>${esc(issue.title)}</h1>
      <div class="chips">
        ${(issue.labels || []).map((l) => `<span class="chip">${esc(l)}</span>`).join("") || `<span class="muted">no labels</span>`}
      </div>
      <div class="body">${esc(issue.body) || `<span class="muted">empty body</span>`}</div>
      <div class="chips">
        <span>parent ${parent}</span>
        <span>blocked by ${blockers}</span>
        <span>${esc(issue.project)}</span>
      </div>
      <div class="actions">
        ${issue.state === "open" && !issue.assignee ? `<button data-act="claim">claim</button>` : ""}
        ${issue.assignee && issue.state === "open" ? `<button data-act="unclaim">unclaim</button>` : ""}
        ${issue.state === "open" ? `<button data-act="close">close</button>` : `<button data-act="reopen">reopen</button>`}
        <button data-act="edit">edit</button>
      </div>
      ${tree ? `<section class="tree"><h2>children / frontier</h2>${tree}</section>` : ""}
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
    await renderIssue(issue.id);
  } catch (err) {
    state.error = err.message;
    await renderIssue(issue.id);
  }
}

async function paint() {
  state.error = "";
  const r = route();
  document.querySelectorAll("nav button").forEach((b) => {
    b.classList.toggle("active", b.dataset.filter === state.filter);
  });
  if (r.name === "list") {
    statusEl.textContent = state.filter;
    try {
      await loadList();
    } catch (err) {
      state.error = err.message;
      state.issues = [];
    }
    renderList();
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

window.addEventListener("hashchange", paint);
window.addEventListener("keydown", (e) => {
  if (e.target.matches("input, textarea")) return;
  const r = route();
  if (r.name !== "list") {
    if (e.key === "Escape") location.hash = "#/";
    return;
  }
  if (e.key === "j") {
    state.selected = Math.min(state.issues.length - 1, state.selected + 1);
    renderList();
  }
  if (e.key === "k") {
    state.selected = Math.max(0, state.selected - 1);
    renderList();
  }
  if (e.key === "Enter" && state.issues[state.selected]) {
    location.hash = "#/" + state.issues[state.selected].id;
  }
  if (e.key === "/") {
    e.preventDefault();
    const input = document.querySelector(".compose input");
    if (input) input.focus();
  }
});

paint();
