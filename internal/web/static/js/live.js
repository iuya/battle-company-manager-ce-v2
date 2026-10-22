// Live match mode: the "Wounded a unit" window (below) and the sync of the page with the match (further down; docs/live-match.md).
document.addEventListener('alpine:init', () => {
  // The window of a wound declaration: the target picked from the list, the part, wounds caused and saved by Fate, and a sentence
  // that says what will happen, in the words of the log.
  Alpine.data('woundSheet', () => ({
    q: '', t: null, part: 'rider', caused: 1, saved: 0,
    match(el) {
      const q = this.q.trim().toLowerCase();
      return !q || (el.dataset.search || '').toLowerCase().includes(q) || (el.closest('div')?.querySelector('p')?.textContent || '').toLowerCase().includes(q);
    },
    pick(el) {
      const d = el.dataset;
      this.t = { name: d.name, wounds: +d.wounds, start: +d.start, fate: +d.fate, mounted: d.mounted === '1', mountWounds: +d.mountWounds, kill: d.kill === '1' };
      if (!this.t.mounted) this.part = 'rider';
    },
    get maxSaved() { return this.t && this.part === 'rider' ? Math.max(0, Math.min(this.caused, this.t.fate)) : 0; },
    get dealt() { return this.caused - Math.min(this.saved, this.maxSaved); },
    get preview() {
      const t = this.t; if (!t) return '';
      const who = this.$root.dataset.attacker, first = this.$root.dataset.xp === '0';
      const saved = Math.min(this.saved, this.maxSaved);
      let s;
      if (this.part === 'mount') {
        const left = Math.max(0, t.mountWounds - this.caused);
        s = left === 0 ? `${t.name}'s mount is lost: ${t.name} is on foot.` : `${t.name}'s mount: Wounds ${t.mountWounds} → ${left}.`;
      } else {
        const left = Math.max(0, t.wounds - this.dealt);
        s = left === 0 ? `${t.name} falls.` : `${t.name}: Wounds ${t.wounds} → ${left}.`;
        if (saved > 0) s += ` ${saved} saved by Fate (Fate ${t.fate} → ${t.fate - saved}).`;
        if (left === 0 && t.kill) s += ` ${who} earns a kill XP.`;
      }
      return s + (first ? ` ${who} earns the wounding XP.` : '');
    },
    init() { this.$watch('caused', () => { if (this.caused < 1) this.caused = 1; if (this.saved > this.maxSaved) this.saved = this.maxSaved; }); },
  }));
});

// ---- Sync: keeps the page of a company in a running match (and the log card of the match page) in step with the match. -----------
// The server streams pointers ("company 12 changed, seq 41"), never state: the page asks for its own fragment again. Nothing is
// opened on a page that has no live part. The stream is closed while the tab is hidden (a browser allows few connections per site) and
// opened again, with a refetch, when it is shown. When the stream cannot be had (the caps, no EventSource) the page polls.
(function () {
  const POLL_MS = 10000;
  let source = null, key = '', poll = null, busy = false, again = false;

  const zones = () => document.getElementById('live-zones');
  const slot = () => document.querySelector('[data-live-match]');
  // What this page follows: the match, the campaign and (company page) the company.
  function target() {
    const z = zones(), s = slot();
    if (z) return { match: z.dataset.match, campaign: z.dataset.campaign, company: z.dataset.company };
    if (s) return { match: s.dataset.liveMatch, campaign: s.dataset.campaign, company: '' };
    return null;
  }
  function dot(state, text) {
    const el = document.getElementById('live-dot');
    if (!el) return;
    el.dataset.state = state;
    const t = document.getElementById('live-dot-text');
    if (t) t.textContent = text;
  }
  function stop() {
    if (source) { source.close(); source = null; }
    if (poll) { clearInterval(poll); poll = null; }
    key = '';
  }

  // The zones and the log are fetched again; one request at a time, one more if something came meanwhile.
  function refresh() {
    const t = target();
    if (!t || !window.htmx) return;
    if (busy) { again = true; return; }
    busy = true;
    const jobs = [];
    if (t.company && zones()) {
      jobs.push(htmx.ajax('GET', `/campaigns/${t.campaign}/companies/${t.company}/live`, { target: '#live-zones', swap: 'outerHTML' }));
    }
    document.body.dispatchEvent(new CustomEvent('live:match'));
    Promise.allSettled(jobs).finally(() => { busy = false; if (again) { again = false; refresh(); } });
  }

  function startPolling() {
    if (poll) return;
    dot('retry', 'Updating every few seconds');
    poll = setInterval(() => { if (!document.hidden) refresh(); }, POLL_MS);
  }

  function open() {
    const t = target();
    if (!t) { stop(); return; }
    const k = `${t.campaign}/${t.match}/${t.company}`;
    if (key === k && (source || poll)) return;
    stop();
    key = k;
    if (document.hidden) return; // opened when the tab is shown
    if (!window.EventSource) { startPolling(); return; }
    const es = new EventSource(`/campaigns/${t.campaign}/matches/${t.match}/events`);
    source = es;
    let lost = false;
    es.onopen = () => { dot('on', 'Live'); if (lost) { lost = false; refresh(); } }; // what happened while the stream was down
    es.onerror = () => {
      if (es.readyState === EventSource.CLOSED) { // refused (the caps) or ended: the browser will not try again
        source = null;
        startPolling();
      } else {
        lost = true;
        dot('retry', 'Reconnecting…'); // the browser retries by itself; once it is back the page refetches
      }
    };
    const changed = ev => {
      let seq = 0;
      try { seq = JSON.parse(ev.data).seq || 0; } catch (e) { seq = 0; }
      const z = zones(), have = z ? Number(z.dataset.seq || 0) : 0;
      if (z && seq && seq <= have) return; // our own change: the answer to it is already on the page
      refresh();
    };
    if (t.company) es.addEventListener(`company-${t.company}`, changed);
    // A match-level change (the end proposal, the turn): the match page refetches its cards; a company page its zones and banner, unless the
    // page already shows that sequence (its own change).
    es.addEventListener('match', ev => {
      const z = zones();
      if (!z) { document.body.dispatchEvent(new CustomEvent('live:match')); return; }
      let seq = 0;
      try { seq = JSON.parse(ev.data).seq || 0; } catch (e) { seq = 0; }
      if (!seq || seq > Number(z.dataset.seq || 0)) refresh();
    });
    es.addEventListener('ended', () => { stop(); location.reload(); }); // the match is over: the page is the one it has now
  }

  document.addEventListener('visibilitychange', () => {
    if (document.hidden) { stop(); return; }
    open();
    if (key) refresh(); // what happened meanwhile
  });
  document.addEventListener('DOMContentLoaded', open);
  document.addEventListener('htmx:afterSettle', open); // a page swapped in by htmx: follow it, or let go
  window.addEventListener('pagehide', stop);
})();
