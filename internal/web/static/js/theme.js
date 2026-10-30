// Theme state (palette, light/dark/system mode, heading font) shared by every Alpine component through $store.theme.
// The <html> attributes are first set by the inline script in the layout's <head>, before the first paint (no flash);
// this store takes over once Alpine starts. The choice lives in localStorage ("bcm-theme"), per browser.
// Appearance state (palette + light/dark/system mode) shared by every Alpine component through $store.theme.
// It is kept in a cookie ("bcm-theme", value "<palette>.<mode>") so the server can read it too: once there are
// accounts it will be stored in the database for logged-in users and this cookie only covers anonymous visitors.
// The <html> attributes are first set by the inline script in the layout's <head>, before the first paint (no flash);
// this store takes over once Alpine starts.
document.addEventListener("alpine:init", () => {
  const COOKIE = "bcm-theme";
  const dark = matchMedia("(prefers-color-scheme: dark)");
  const readCookie = () => {
    const m = document.cookie.match(new RegExp("(?:^|; )" + COOKIE + "=([^;]*)"));
    const [palette, mode] = m ? decodeURIComponent(m[1]).split(".") : [];
    return { palette, mode };
  };

  Alpine.store("theme", {
    palettes: [
      { key: "forest", name: "Forest & Gold" },
      { key: "ember", name: "Parchment & Ember" },
      { key: "mithril", name: "Mithril & Night" },
    ],
    modes: ["system", "light", "dark"],
    palette: "forest",
    mode: "system", // system | light | dark
    auras: false, // show the aura chips on the unit cards; per browser (cookie bcm-auras), off by default
    gears: [
      { key: "chips", name: "Icon and name" },
      { key: "tiles", name: "Icons only" },
    ],
    gear: "chips", // how the unit cards draw the gear; per browser (cookie bcm-gear)
    highContrast: false, // the high-contrast variant of the palette; per browser (cookie bcm-contrast), else the OS's prefers-contrast

    init() {
      this.highContrast = document.documentElement.dataset.contrast === "high";
      this.auras = document.documentElement.dataset.showAuras === "on";
      this.gear = document.documentElement.dataset.gear === "tiles" ? "tiles" : "chips";
      // signed in: the account's choice (data-pref, "<palette>.<mode>") wins over this browser's cookie
      const pref = (document.documentElement.dataset.pref || "").split(".");
      const saved = pref[0] ? { palette: pref[0], mode: pref[1] } : readCookie();
      if (this.palettes.some((p) => p.key === saved.palette)) this.palette = saved.palette;
      if (this.modes.includes(saved.mode)) this.mode = saved.mode;
      dark.addEventListener("change", () => this.mode === "system" && this.apply());
      this.apply();
    },
    set(key, value) {
      this[key] = value;
      document.cookie = `${COOKIE}=${this.palette}.${this.mode}; Path=/; Max-Age=31536000; SameSite=Lax${location.protocol === "https:" ? "; Secure" : ""}`;
      this.apply();
      this.save();
    },
    setAuras(on) {
      this.auras = !!on;
      document.cookie = `bcm-auras=${this.auras ? 1 : 0}; Path=/; Max-Age=31536000; SameSite=Lax${location.protocol === "https:" ? "; Secure" : ""}`;
      if (this.auras) document.documentElement.dataset.showAuras = "on";
      else delete document.documentElement.dataset.showAuras;
    },
    setContrast(on) {
      this.highContrast = !!on;
      document.cookie = `bcm-contrast=${this.highContrast ? "high" : "normal"}; Path=/; Max-Age=31536000; SameSite=Lax${location.protocol === "https:" ? "; Secure" : ""}`;
      if (this.highContrast) document.documentElement.dataset.contrast = "high";
      else delete document.documentElement.dataset.contrast;
      this.apply();
    },
    setGear(g) {
      this.gear = g === "tiles" ? "tiles" : "chips";
      document.cookie = `bcm-gear=${this.gear}; Path=/; Max-Age=31536000; SameSite=Lax${location.protocol === "https:" ? "; Secure" : ""}`;
      if (this.gear === "tiles") document.documentElement.dataset.gear = "tiles";
      else delete document.documentElement.dataset.gear;
    },
    // Signed in: keep the choice in the account too. A failure is silent; the cookie keeps working.
    save() {
      const root = document.documentElement;
      if (!root.dataset.auth) return;
      root.dataset.pref = `${this.palette}.${this.mode}`;
      fetch("/account/appearance", { method: "PUT", credentials: "same-origin", headers: { "Content-Type": "application/x-www-form-urlencoded" }, body: new URLSearchParams({ palette: this.palette, mode: this.mode }) }).catch(() => {});
    },
    get resolvedMode() {
      return this.mode === "system" ? (dark.matches ? "dark" : "light") : this.mode;
    },
    apply() {
      const root = document.documentElement;
      root.dataset.palette = this.palette;
      root.dataset.mode = this.resolvedMode;
      const meta = document.querySelector('meta[name="theme-color"]');
      // the computed page color needs the new attributes to be applied first
      requestAnimationFrame(() => {
        if (meta) meta.content = getComputedStyle(root).getPropertyValue("--page").trim();
        window.dispatchEvent(new CustomEvent("themechange"));
      });
    },
  });

  // Contrast table of the style guide: reads the live tokens, so it follows whatever theme is active.
  Alpine.data("contrast", () => ({
    rows: [],
    tokens: [],
    names: ["page", "surface", "raised", "line", "fg", "muted", "accent", "accent-fg", "accent-text", "accent-soft", "danger", "danger-soft", "success", "success-soft", "gold", "leader-soft", "sergeant", "sergeant-soft", "hero", "hero-soft"],
    pairs: [
      ["fg", "page"], ["fg", "surface"], ["muted", "page"], ["muted", "surface"], ["accent-fg", "accent"],
      ["accent-text", "page"], ["accent-text", "surface"], ["danger", "surface"], ["success", "surface"], ["fg", "accent-soft"], ["muted", "leader-soft"], ["muted", "sergeant-soft"], ["muted", "hero-soft"],
    ],
    init() {
      window.addEventListener("themechange", () => this.compute());
      this.compute();
    },
    lum(hex) {
      const [r, g, b] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255).map((c) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4));
      return 0.2126 * r + 0.7152 * g + 0.0722 * b;
    },
    compute() {
      const css = getComputedStyle(document.documentElement);
      const v = (n) => css.getPropertyValue("--" + n).trim();
      this.tokens = this.names.map((n) => ({ n, hex: v(n) }));
      this.rows = this.pairs.map(([a, b]) => {
        const [hi, lo] = [this.lum(v(a)), this.lum(v(b))].sort((x, y) => y - x);
        const ratio = (hi + 0.05) / (lo + 0.05);
        return { a, b, fa: v(a), fb: v(b), ratio: ratio.toFixed(2), level: ratio >= 7 ? "AAA" : ratio >= 4.5 ? "AA" : "fail" };
      });
    },
  }));
});
