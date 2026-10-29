// Applies the saved appearance before the first paint (loaded synchronously from the head: no flash of the wrong palette or mode).
// A file and not an inline script so the Content-Security-Policy needs no unsafe-inline for scripts.
(function () {
  var r = document.documentElement;
  var m = document.cookie.match(/(?:^|; )bcm-theme=([^;]*)/);
  var v = r.dataset.pref ? r.dataset.pref.split(".") : m ? decodeURIComponent(m[1]).split(".") : []; // the account's choice wins over this browser's cookie
  var mode = ["light", "dark"].indexOf(v[1]) >= 0 ? v[1] : "system";
  var dark = mode === "dark" || (mode === "system" && matchMedia("(prefers-color-scheme: dark)").matches);
  if (["forest", "ember", "mithril"].indexOf(v[0]) >= 0) r.dataset.palette = v[0];
  if (/(?:^|; )bcm-auras=1(?:;|$)/.test(document.cookie)) r.dataset.showAuras = "on"; // aura chips on the unit cards: off unless this browser chose them
  if (/(?:^|; )bcm-gear=tiles(?:;|$)/.test(document.cookie)) r.dataset.gear = "tiles"; // gear on the unit cards as icons only: this browser's choice
  r.dataset.mode = dark ? "dark" : "light";
})();
