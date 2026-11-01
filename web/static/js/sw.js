/* Service worker: offline shell + instant re-loads. Hand-written, no build
 * step, served from /sw.js (root scope). Rules:
 *   - navigations: network first; offline falls back to the cached shell
 *   - /static/ + /fonts/: cache first (assets are embedded or style files)
 *   - everything else (health data, fragments, the editors): network only —
 *     health status must stay live, never silently stale
 *
 * Bump CACHE whenever a static asset changes (no build step stamps it for us).
 */
var CACHE = "dash-v10";

var SHELL = [
  "/",
  "/static/css/main.css",
  "/static/vendor/htmx.min.js",
  "/static/vendor/alpine.min.js",
  "/static/js/ui.js",
  "/static/icons/icon-192.png",
  "/static/icons/globe.svg"
];

self.addEventListener("install", function (e) {
  e.waitUntil(
    caches.open(CACHE)
      .then(function (c) { return c.addAll(SHELL); })
      .then(function () { return self.skipWaiting(); })
  );
});

self.addEventListener("activate", function (e) {
  e.waitUntil(
    caches.keys()
      .then(function (keys) {
        return Promise.all(keys
          .filter(function (k) { return k !== CACHE; })
          .map(function (k) { return caches.delete(k); }));
      })
      .then(function () { return self.clients.claim(); })
  );
});

self.addEventListener("fetch", function (e) {
  var req = e.request;
  if (req.method !== "GET") return; // run launches + config saves are never touched

  if (req.mode === "navigate") {
    e.respondWith(fetch(req).catch(function () { return caches.match("/"); }));
    return;
  }

  var url = new URL(req.url);
  if (url.origin === location.origin &&
      (url.pathname.indexOf("/static/") === 0 || url.pathname.indexOf("/fonts/") === 0)) {
    e.respondWith(
      caches.match(req).then(function (hit) {
        return hit || fetch(req).then(function (res) {
          if (res.ok) {
            var copy = res.clone();
            caches.open(CACHE).then(function (c) { c.put(req, copy); });
          }
          return res;
        });
      })
    );
  }
  /* everything else falls through untouched: network only */
});