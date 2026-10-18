// Homelab dashboard UI: clock, search, tag filtering, style picker,
// launch confirmations, toasts.
// Plain JS — no framework beyond the vendored Alpine.js for the confirm menus.

(function () {
  "use strict";

  var searchInput = document.getElementById("search");
  var activeTags = new Set();
  var query = "";

  /* ---- search + tag filtering (over all panels) ---- */

  function cardMatches(card) {
    if (query && (card.dataset.text || "").indexOf(query) === -1) return false;
    if (activeTags.size) {
      var tags = (card.dataset.tags || "").split(/\s+/).filter(Boolean);
      for (var t of activeTags) if (tags.indexOf(t) === -1) return false;
    }
    return true;
  }

  function applyFilters() {
    document.querySelectorAll(".group-panel").forEach(function (panel) {
      var matches = 0;
      panel.querySelectorAll(".row").forEach(function (row) {
        var show = cardMatches(row);
        row.classList.toggle("hidden", !show);
        if (show) matches++;
      });
      // an unfiltered page shows every panel; a search hides the empty ones
      panel.classList.toggle("hidden", searchActive() && matches === 0);
    });
  }

  function searchActive() { return query !== "" || activeTags.size > 0; }

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
        // "all" resets the selection
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

  /* ---- header clock: big HH:MM, small seconds, full date ---- */

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

  /* ---- style picker (accent themes, persisted per browser) ---- */

  var STYLE_KEY = "dashboard.style";
  var select = document.getElementById("style-select");
  if (select) {
    try {
      var saved = localStorage.getItem(STYLE_KEY);
      if (saved) select.value = saved;
    } catch (e) { /* ignore */ }
    applyStyle(select.value);
    select.addEventListener("change", function () {
      applyStyle(select.value);
      try { localStorage.setItem(STYLE_KEY, select.value); } catch (e) { /* ignore */ }
    });
  }

  function applyStyle(style) {
    document.documentElement.dataset.style = style || "retro";
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
})();