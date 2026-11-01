// Homelab dashboard UI: miller-column group navigation (sidebar + drill-down
// columns of cards and leaf rows), search/tag filtering, clock, style picker,
// editor modals, run items, toasts.
// Plain JS — no framework beyond the vendored Alpine.js for the confirm menus.

(function () {
  "use strict";

  /* ---- group navigation: sidebar (roots) + drill-down level columns ---- */

  var GROUP_KEY = "dashboard.group";
  var sidebar = document.querySelector(".groups");
  var panels = document.querySelectorAll(".group-panel");

  function panelFor(path) {
    for (var i = 0; i < panels.length; i++) {
      if (panels[i].dataset.path === path) return panels[i];
    }
    return null;
  }

  // a selection is valid if the group exists anywhere in the tree
  function groupExists(path) {
    if (panelFor(path)) return true;
    var items = document.querySelectorAll(".group-item");
    for (var i = 0; i < items.length; i++) {
      if (items[i].dataset.path === path) return true;
    }
    return false;
  }

  function selectedGroup() {
    // priority: URL hash (restores refreshes + shareable links) →
    // remembered pick → the server-rendered default
    var fromHash = null;
    try { fromHash = decodeURIComponent((location.hash.match(/group=([^&]+)/) || [])[1] || ""); } catch (e) { /* */ }
    if (fromHash && groupExists(fromHash)) return fromHash;
    var saved = null;
    try { saved = localStorage.getItem(GROUP_KEY); } catch (e) { /* */ }
    if (saved && groupExists(saved)) return saved;
    var first = document.querySelector(".group-panel.selected");
    if (first) return first.dataset.path;
    var firstItem = document.querySelector(".group-item");
    return firstItem ? firstItem.dataset.path : null;
  }

  // a level column is visible when it holds the selection itself or is an
  // ancestor of it (the chain from the sidebar down to the selection)
  function isOnChain(levelPath, sel) {
    return levelPath === sel || sel.indexOf(levelPath + "/") === 0;
  }

  // miller-column sizing: the sidebar plus one track per visible level
  // column; tracks are at most a third of the frame wide — a quarter when a
  // fourth column joins, and so on for deeper nesting
  function layoutFrame() {
    var frame = document.querySelector(".frame");
    if (!frame) return;
    if (window.innerWidth <= 820) { // the media query stacks instead
      frame.style.gridTemplateColumns = "";
      return;
    }
    var n = 1; // the sidebar
    document.querySelectorAll(".group-panel").forEach(function (p) {
      if (!p.classList.contains("hidden")) n++;
    });
    var denom = Math.max(3, n);
    var w = "calc((100% - " + ((denom - 1) * 1.1) + "rem) / " + denom + ")";
    var cols = [];
    for (var i = 0; i < n; i++) cols.push(w);
    frame.style.gridTemplateColumns = cols.join(" ");
  }

  function applySelection(path) {
    // level columns carry the SERVER-rendered `hidden` class — toggle the
    // class, not the hidden property (it can't remove the class)
    document.querySelectorAll(".group-panel").forEach(function (p) {
      p.classList.toggle("hidden", !isOnChain(p.dataset.path, path));
    });
    // exact match lights the picked card …
    document.querySelectorAll(".group-item").forEach(function (b) {
      b.classList.toggle("active", b.dataset.path === path);
    });
    // …and the sidebar additionally highlights the ROOT of the selection
    document.querySelectorAll(".groups .group-item").forEach(function (b) {
      b.classList.toggle("active", b.dataset.path === path.split("/")[0]);
    });
    layoutFrame();
    try { localStorage.setItem(GROUP_KEY, path); } catch (e) { /* */ }
    try { history.replaceState(null, "", "#group=" + encodeURIComponent(path)); } catch (e) { /* */ }
    applyFilters();
  }

  document.addEventListener("click", function (e) {
    var el = e.target.closest(".group-item");
    if (!el || !el.dataset.path) return;
    applySelection(el.dataset.path);
  });

  // keep the selection across sidebar fragment refreshes
  document.body.addEventListener("htmx:afterSwap", function (e) {
    if (e.detail && e.detail.target && e.detail.target.id === "groups-nav") {
      var sel = selectedGroup();
      document.querySelectorAll(".group-item").forEach(function (b) {
        var root = sel ? sel.split("/")[0] : null;
        b.classList.toggle("active", b.dataset.path === root);
      });
    }
  });

  /* ---- search + tag filtering (across every panel) ---- */

  var searchInput = document.getElementById("search");
  var activeTags = new Set();
  var query = "";

  function searchActive() { return query !== "" || activeTags.size > 0; }

  function rowMatches(row) {
    if (query && (row.dataset.text || "").indexOf(query) === -1) return false;
    if (activeTags.size) {
      var tags = (row.dataset.tags || "").split(/\s+/).filter(Boolean);
      for (var t of activeTags) if (tags.indexOf(t) === -1) return false;
    }
    return true;
  }

  /* ---- search: the tinted results box over the column layout ----
     Every matching leaf of the whole tree, server-side filtered
     (GET /results?q=…&tags=…); rows show only there, the columns stay
     untouched while searching. Group names never match — leaf rows do. */

  var resultBox = document.getElementById("search-results");
  var searchTimer = null;

  function resultURL() {
    var p = "";
    if (query) p += "q=" + encodeURIComponent(query);
    activeTags.forEach(function (t) {
      p += (p ? "&" : "") + "tags=" + encodeURIComponent(t);
    });
    return "/results" + (p ? "?" + p : "");
  }

  function refreshResults() {
    if (!resultBox) return;
    if (!searchActive()) {
      resultBox.innerHTML = "";
      resultBox.classList.add("hidden");
      return;
    }
    resultBox.classList.remove("hidden");
    if (window.htmx) htmx.ajax("GET", resultURL(), { target: "#search-results", swap: "innerHTML" });
  }

  // filters drive the results box only
  function applyFilters() { refreshResults(); }

  if (searchInput) {
    searchInput.addEventListener("input", function () {
      query = searchInput.value.trim().toLowerCase();
      clearTimeout(searchTimer); // a small debounce keeps /results cheap
      searchTimer = setTimeout(applyFilters, 150);
    });
  }

  var tagBar = document.getElementById("tag-filter");
  if (tagBar) {
    tagBar.addEventListener("click", function (e) {
      var btn = e.target.closest(".tag");
      if (!btn) return;
      var tag = btn.dataset.tag;
      if (!tag) {
        activeTags.clear();
        tagBar.querySelectorAll(".tag").forEach(function (b) {
          b.classList.toggle("active", !b.dataset.tag);
        });
      } else {
        btn.classList.toggle("active");
        if (btn.classList.contains("active")) activeTags.add(tag);
        else activeTags.delete(tag);
      }
      applyFilters();
    });
  }

  /* ---- header clock: big HH:MM, small seconds, date line ---- */

  var pad2 = function (n) { return (n < 10 ? "0" : "") + n; };

  function tick() {
    var now = new Date();
    var timeEl = document.getElementById("clock-time");
    var secsEl = document.getElementById("clock-seconds");
    var dateEl = document.getElementById("clock-date");
    if (timeEl) timeEl.textContent = pad2(now.getHours()) + ":" + pad2(now.getMinutes());
    if (secsEl) secsEl.textContent = pad2(now.getSeconds());
    if (dateEl) dateEl.textContent = now.toLocaleDateString([], {
      weekday: "short", year: "numeric", month: "short", day: "numeric"
    }).replace(/,/g, " ·");
  }
  tick();
  setInterval(tick, 1000);

  /* ---- style picker (themes from resources/styles, persisted per browser) ---- */

  var STYLE_KEY = "dashboard.style";
  var select = document.getElementById("style-select");
  if (select) {
    var current = resolvedStyle();
    applyStyle(current);
    select.value = current; // the picker always shows what is applied
    select.addEventListener("change", function () {
      applyStyle(select.value);
      try {
        localStorage.setItem(STYLE_KEY, JSON.stringify({ style: select.value, under: configDefault() }));
      } catch (e) { /* ignore */ }
    });
  }

  function configDefault() { return select.dataset["default"] || select.value; }

  function optionExists(id) {
    for (var i = 0; i < select.options.length; i++) {
      if (select.options[i].value === id) return true;
    }
    return false;
  }

  // resolvedStyle: the viewer's saved pick when still valid AND chosen
  // under the same config default; otherwise the config's current default.
  function resolvedStyle() {
    var fallback = configDefault();
    try {
      var rec = JSON.parse(localStorage.getItem(STYLE_KEY) || "null");
      if (rec && rec.style && optionExists(rec.style) && rec.under === fallback) {
        return rec.style;
      }
    } catch (e) { /* ignore */ }
    return fallback;
  }

  function applyStyle(style) {
    if (style) document.documentElement.dataset.style = style;
    // keep the browser UI (installed-app titlebar) in sync with the style
    var bg = getComputedStyle(document.body).getPropertyValue("--bg").trim();
    var meta = document.querySelector('meta[name="theme-color"]');
    if (bg && meta) meta.content = bg;
  }

  /* ---- editors: config (single pane) + style (base | overrides) ---- */

  var overlay = document.getElementById("edit-overlay");
  var editTitle = document.getElementById("edit-title");
  var singleBody = document.getElementById("edit-single");
  var splitBody = document.getElementById("edit-split");
  var textarea = document.getElementById("edit-textarea");
  var baseTextarea = document.getElementById("edit-base");
  var overTextarea = document.getElementById("edit-overwrite");
  var saveBtn = document.getElementById("edit-save");
  var editError = document.getElementById("edit-error");
  var editing = null; // {split, saveUrl}

  function openEditor(title, single, loadBody) {
    editTitle.textContent = title;
    singleBody.classList.toggle("hidden", !single);
    splitBody.classList.toggle("hidden", single);
    editError.textContent = "";
    saveBtn.disabled = true;
    overlay.hidden = false;
    loadBody();
  }

  function loadText(url, fill) {
    return fetch(url)
      .then(function (res) { return res.text(); })
      .then(fill)
      .catch(function (err) { editError.textContent = String(err.message || err); });
  }

  /* ---- data folder: show, open, and switch (host-local viewers only) ---- */

  var dataBtn = document.getElementById("open-data-btn");
  var dataOverlay = document.getElementById("data-overlay");
  if (dataBtn && dataOverlay) {
    var dataPath = document.getElementById("data-path");
    var dataError = document.getElementById("data-error");

    dataBtn.addEventListener("click", function () {
      dataError.textContent = "";
      dataPath.value = "…";
      dataOverlay.hidden = false;
      fetch("/data-dir")
        .then(function (res) { return res.text(); })
        .then(function (text) { dataPath.value = text; })
        .catch(function (err) { dataError.textContent = String(err.message || err); });
    });

    document.getElementById("data-close").addEventListener("click", function () {
      dataOverlay.hidden = true;
    });
    document.addEventListener("keydown", function (e) {
      if (e.key === "Escape" && !dataOverlay.hidden) dataOverlay.hidden = true;
    });

    document.getElementById("data-open").addEventListener("click", function () {
      fetch("/open-data", { method: "POST" }).then(function (res) {
        if (!res.ok) dataError.textContent = "could not open the folder";
      }).catch(function () {
        dataError.textContent = "could not open the folder";
      });
    });

    document.getElementById("data-set").addEventListener("click", function () {
      dataError.textContent = "";
      fetch("/data-dir", {
        method: "POST",
        headers: { "Content-Type": "text/plain" },
        body: dataPath.value
      })
        .then(function (res) {
          return res.text().then(function (text) {
            if (!res.ok) {
              dataError.textContent = text;
              return;
            }
            dataOverlay.hidden = true;
            toast("data folder switched — reloading", "");
            setTimeout(function () { window.location.reload(); }, 600);
          });
        })
        .catch(function (err) {
          dataError.textContent = String(err.message || err);
        });
    });
  }

  if (overlay) {
    document.getElementById("edit-config-btn").addEventListener("click", function () {
      editing = { split: false, saveUrl: "/config-source" };
      openEditor("Edit config.yml", true, function () {
        textarea.value = "loading…";
        loadText("/config-source", function (text) { textarea.value = text; saveBtn.disabled = false; });
      });
    });

    document.getElementById("edit-style-btn").addEventListener("click", function () {
      var styleId = document.getElementById("style-select").value || "retro";
      var selected = document.getElementById("style-select").selectedOptions[0];
      var styleName = selected ? selected.textContent : styleId;
      editing = { split: true, saveUrl: "/style-source/" + encodeURIComponent(styleId) + "/overwrite" };
      openEditor("Edit style: " + styleName + "  (" + styleId + ")", false, function () {
        baseTextarea.value = "loading…";
        overTextarea.value = "loading…";
        loadText("/style-source/" + encodeURIComponent(styleId) + "/base",
          function (text) { baseTextarea.value = text.replace(/^# original.*\n/, ""); });
        loadText("/style-source/" + encodeURIComponent(styleId) + "/overwrite",
          function (text) { overTextarea.value = text.replace(/^# Operator.*\n# here\.\n?/, ""); saveBtn.disabled = false; });
      });
    });

    document.getElementById("edit-close").addEventListener("click", function () {
      overlay.hidden = true;
    });

    saveBtn.addEventListener("click", function () {
      editError.textContent = "";
      var body = editing.split ? overTextarea.value : textarea.value;
      fetch(editing.saveUrl, {
        method: "POST",
        headers: { "Content-Type": "text/yaml" },
        body: body
      })
        .then(function (res) {
          return res.text().then(function (text) {
            if (!res.ok) {
              // validation errors carry line numbers from the YAML parser
              editError.textContent = text.replace(/^.*: /, "");
              return;
            }
            overlay.hidden = true;
            toast(editing.split ? "style saved — reloading" : "config saved — reloading", "");
            setTimeout(function () { window.location.reload(); }, 600);
          });
        })
        .catch(function (err) {
          editError.textContent = String(err.message || err);
        });
    });

    document.addEventListener("keydown", function (e) {
      if (e.key === "Escape" && !overlay.hidden) overlay.hidden = true;
    });
  }

  /* ---- run items ---- */

  // launched from the Alpine confirm menu on the row; the confirm step
  // lives in the row, this only fires the request.
  window.dashboardRun = function (key) {
    fetch("/run/" + encodeURIComponent(key), { method: "POST" })
      .then(function (res) {
        return res.text().then(function (text) {
          toast(text, res.ok ? "" : "error");
        });
      })
      .catch(function (err) {
        toast(String(err.message || err), "error");
      });
  };

  function toast(message, cls) {
    var stack = document.getElementById("toast-stack");
    if (!stack) return;
    var el = document.createElement("div");
    el.className = "toast " + (cls || "");
    el.textContent = message;
    stack.appendChild(el);
    setTimeout(function () {
      el.classList.add("fade");
      setTimeout(function () { el.remove(); }, 500);
    }, 2500);
  }

  /* ---- initial state ---- */

  // offline shell: register the service worker on secure origins only — on
  // plain-HTTP LAN hosts navigator.serviceWorker is undefined and we skip
  // quietly (the manifest/icons/theme-color still work there)
  if ("serviceWorker" in navigator && window.isSecureContext) {
    window.addEventListener("load", function () {
      navigator.serviceWorker.register("/sw.js").catch(function () { /* no SW */ });
    });
  }

  // apply the resolved selection once everything above is in place
  selectGroupInit();
  function selectGroupInit() {
    var sel = selectedGroup();
    if (sel) {
      applySelection(sel);
    } else {
      layoutFrame();
    }
  }
  window.addEventListener("resize", layoutFrame);
})();