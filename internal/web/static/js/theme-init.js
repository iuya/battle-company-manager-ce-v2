// Applies the saved appearance before the first paint (loaded synchronously from the head: no flash of the wrong palette or mode).
// A file and not an inline script so the Content-Security-Policy needs no unsafe-inline for scripts.
(function () {
  var r = document.documentElement;
  var cookie = function (name) {
    var m = document.cookie.match(new RegExp("(?:^|; )" + name + "=([^;]*)"));
    return m ? decodeURIComponent(m[1]) : "";
  };
  var saved = r.dataset.pref || cookie("bcm-theme"); // the account's choice wins over this browser's cookie
  var v = saved ? saved.split(".") : [];
  var mode = ["light", "dark"].indexOf(v[1]) >= 0 ? v[1] : "system";
  var dark = mode === "dark" || (mode === "system" && matchMedia("(prefers-color-scheme: dark)").matches);
  if (["forest", "ember", "mithril"].indexOf(v[0]) >= 0) r.dataset.palette = v[0];
  if (cookie("bcm-auras") === "1") r.dataset.showAuras = "on"; // aura chips on the unit cards: off unless this browser chose them
  if (cookie("bcm-gear") === "tiles") r.dataset.gear = "tiles"; // gear on the unit cards as icons only: this browser's choice
  r.dataset.mode = dark ? "dark" : "light";
  // high contrast: this browser's choice (cookie bcm-contrast = high | normal), else what the operating system asks for
  var c = cookie("bcm-contrast");
  if (c === "high" || (c !== "normal" && matchMedia("(prefers-contrast: more)").matches)) r.dataset.contrast = "high";
})();
