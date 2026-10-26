// The To Wound chart window (templates/partials/to_wound.gohtml). The find control keeps the attacker's Strength (s) and the target's
// Defence (d) in the browser; the chart is plain server-rendered HTML whose cells carry data-s, data-d, data-big, data-tier and data-meaning,
// so nothing here needs the network and a live refresh cannot touch it. A button with data-to-wound-open asks the window to open (no inline
// handler: the Content-Security-Policy would block it).
document.addEventListener('click', e => {
  if (e.target.closest('[data-to-wound-open]')) window.dispatchEvent(new Event('open-to-wound'));
});

document.addEventListener('alpine:init', () => {
  Alpine.data('toWound', () => ({
    s: '', d: '', // '' until picked
    lo: { s: 1, d: 1 }, hi: { s: 10, d: 10 }, start: { s: 3, d: 4 },
    init() {
      for (const k of ['s', 'd']) {
        const el = this.$root.querySelector('[data-tw-input="' + k + '"]');
        this.lo[k] = +el.min || 1; this.hi[k] = +el.max || 10;
      }
    },
    set(k, v) {
      v = Math.round(Number(v));
      this[k] = Number.isFinite(v) && v > 0 ? Math.min(this.hi[k], Math.max(this.lo[k], v)) : '';
    },
    clamp(k) { this.set(k, this[k]); },
    step(k, by) { this.set(k, this[k] === '' ? this.start[k] : this[k] + by); },
    pick(s, d) { this.s = s; this.d = d; },
    cell() {
      if (this.s === '' || this.d === '') return null;
      return this.$refs.grid.querySelector('td[data-s="' + this.s + '"][data-d="' + this.d + '"]');
    },
    get found() { return this.cell() !== null; },
    get big() { const c = this.cell(); return c ? c.dataset.big : ''; },
    get tier() { const c = this.cell(); return c ? c.dataset.tier : ''; },
    get meaning() { const c = this.cell(); return c ? c.dataset.meaning : ''; },
  }));
});
