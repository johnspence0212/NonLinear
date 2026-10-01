function paintShell({ title, main, rail, homeLabel = "home" }) {
  document.documentElement.dataset.theme = "matrix";
  document.getElementById("app").innerHTML = `
      <header>
        <a class="brand" href="#/"><span>nonlinear</span></a>
        <form id="search-form" role="search">
          <input id="global-search" placeholder="search…  ( / )" />
        </form>
        <button type="button" class="hdr-btn">↻ <span class="lbl">refresh</span></button>
        <div class="swatches">
          <button type="button" class="swatch" data-theme="orange"></button>
          <button type="button" class="swatch" data-theme="matrix"></button>
          <button type="button" class="swatch" data-theme="cool"></button>
        </div>
      </header>
      <div class="shell">
        <nav>
          <button data-filter="home" class="active">${homeLabel}</button>
          <div class="nav-folder" id="nav-issues">
            <button type="button" class="nav-folder-h" aria-expanded="true">
              <span class="nav-chev">▾</span>
              issues
            </button>
            <div class="nav-kids">
              <button data-filter="projects">projects <span>3</span></button>
              <button data-filter="maps">maps <span>2</span></button>
              <button data-filter="open">open <span>5</span></button>
              <button data-filter="frontier">frontier <span>1</span></button>
              <button data-filter="closed">closed <span>7</span></button>
              <button data-filter="all">all <span>12</span></button>
            </div>
          </div>
          <button data-go="settings">settings</button>
        </nav>
        <main id="main">${main}</main>
        <aside id="rail">${rail}</aside>
      </div>
      <footer>
        <span>j/k move · enter open · esc back · r refresh · / search</span>
        <span class="foot-counts">3 projects · 1 frontier</span>
        <span class="foot-brand">nonlinear 0.3.16 · prototype</span>
      </footer>`;
  document.title = title + " · nonlinear";
}
