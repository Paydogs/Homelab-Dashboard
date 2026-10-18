// Homelab dashboard UI: group selection (left column + drill-down tiles),
// search/tag filtering, clock, style picker, editor modals, run items, toasts.
// Plain JS — no framework beyond the vendored Alpine.js for the confirm menus.

(function () {
  "use strict";

  /* ---- group selection: sidebar (root groups) + tiles (drill-down) ---- */

  var GROUP_KEY = "dashboard.group";
  var sidebar = document.querySelector(".groups");
  var panels = document.querySelectorAll(".group-panel");

  function panelFor(path) {
    for (var i = 0; i < panels.length; i++) {
      if (panels[i].dataset.path === path) return panels[i];
    }
    return null;
  }

  function selectedGroup() {
    // priority: URL hash (restores refreshes + shareable links) →
    // remembered pick → the server-rendered default
    var fromHash = null;
    try { fromHash = decodeURIComponent((location.hash.match(/group=([^&]+)/) || [])[1] || ""); } catch (e) { /* */ }
    if (fromHash && panelFor(fromHash)) return fromHash;
    var saved = null;
    try { saved = localStorage.getItem(GROUP_KEY); } catch (e) { /* */ }
    if (saved && panelFor(saved)) return saved;
    var first = document.querySelector(".group-panel.selected");
    return first ? first.dataset.path : (panels[0] ? panels[0].dataset.path : null);
  }

  function applySelection(path) {
    document.querySelectorAll(".group-panel").forEach(function (p) {
      p.hidden = p.dataset.path !== path;
    });
    document.querySelectorAll(".group-item, .tile").forEach(function (b) {
      b.classList.toggle("active", b.dataset.path === path);
    });
    // the sidebar highlights the ROOT of the selection
    document.querySelectorAll(".group-item").forEach(function (b) {
      var root = (path.split("/"))[0];
      b.classList.toggle("active", b.dataset.path === root);
    });
    try { localStorage.setItem(GROUP_KEY, path); } catch (e) { /* */ }
    try { history.replaceState(null, "", "#group=" + encodeURIComponent(path)); } catch (e) { /* */ }
    applyFilters();
  }

  document.addEventListener("click", function (e) {
    var el = e.target.closest(".group-item, .tile");
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

  function applyFilters() {
    if (searchActive()) {
      // searching covers the whole tree: reveal every panel that has
      // matches (tiles are meaningless while drilling is off)
      document.querySelectorAll(".group-panel").forEach(function (panel) {
        var matches = 0;
        panel.querySelectorAll(".row").forEach(function (row) {
          var show = rowMatches(row);
          row.classList.toggle("hidden", !show);
          if (show) matches++;
        });
        panel.hidden = matches === 0;
        var tiles = panel.querySelector(".tiles");
        if (tiles) tiles.classList.add("hidden");
      });
    } else {
      document.querySelectorAll(".group-panel").forEach(function (panel) {
        panel.hidden = panel.dataset.path !== selectedGroup();
        panel.querySelectorAll(".row.hidden").forEach(function (r) { r.classList.remove("hidden"); });
        var tiles = panel.querySelector(".tiles");
        if (tiles) tiles.classList.remove("hidden");
      });
    }
  }

  if (searchInput) {
    searchInput.addEventListener("input", function () {
      query = searchInput.value.trim().toLowerCase();
      applyFilters();
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

  // apply the resolved selection once everything above is in place
  selectGroupInit();
  function selectGroupInit() {
    var sel = selectedGroup();
    if (sel) {
      document.querySelectorAll(".group-panel").forEach(function (p) {
        p.hidden = p.dataset.path !== sel;
      });
      applySelection(sel);
    }
  }
})();