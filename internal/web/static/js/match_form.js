// The new-match form (docs/matches.md, "New match"): two sides with the company search only, + Add side (up to the maximum, an added side
// can be removed), All vs all and Practice. State comes from the data-companies / data-sides attributes (JSON) and goes back to the server
// as plain form fields: sides and one side-N field per company; in all vs all mode, mode=ffa and the pool as side-1.
// A player may own several companies (docs/multi-company-spec.md): the search shows owner and faction beside the name, the viewer's own
// companies are highlighted and listed first, and last on the other sides once one of them is placed (playing yourself is rare); a
// non-blocking "Same player" note names a player who has two companies on one side or companies on several sides.
// When the sides are one company each, the Against the Odds block is loaded (GET .../matches/new/ato); Ambush! there turns the scenario
// card into a free choice.
document.addEventListener('alpine:init', () => {
  Alpine.data('matchForm', () => ({
    companies: [],
    sides: [],
    ffa: false, // all vs all: one pool of companies, each fighting alone
    practice: false,
    maxSides: 8,
    atoKey: '',
    campaign: '', // read once: inside a nested handler $el is the element pressed, not the form
    init() {
      this.ffa = this.$el.dataset.ffa === 'true';
      this.practice = this.$el.dataset.practice === 'true';
      this.maxSides = Number(this.$el.dataset.maxSides) || 8;
      this.campaign = this.$el.dataset.campaign;
      this.companies = JSON.parse(this.$el.dataset.companies);
      this.sides = JSON.parse(this.$el.dataset.sides).map(s => ({ ids: s.ids, query: '', open: false }));
      // the block drawn with the page (a failed create) is kept until the sides change
      this.atoKey = this.pairKey();
      if (!document.getElementById('new-ato-card')) this.$nextTick(() => this.loadATO());
    },
    get visible() { return this.ffa ? this.sides.slice(0, 1) : this.sides; },
    setFFA(on) {
      this.ffa = on;
      if (on) { // everybody goes into the pool
        const all = this.sides.flatMap(s => s.ids);
        this.sides.forEach(s => { s.ids = []; });
        this.sides[0].ids = all;
      } else if (this.sides.length < 2) {
        this.sides.push({ ids: [], query: '', open: false });
      }
      this.loadATO();
    },
    addSide() { if (this.sides.length < this.maxSides) { this.sides.push({ ids: [], query: '', open: false }); this.loadATO(); } },
    removeSide(i) { this.sides.splice(i, 1); this.loadATO(); }, // its companies go back to the pool
    company(id) { return this.companies.find(c => c.id === id); },
    taken(id) { return this.sides.some(s => s.ids.includes(id)); },
    sideLabel(side) { return side.ids.map(id => this.company(id).name).join(' + '); },
    // The viewer's own companies come first; once one of them is on another side they come last here.
    results(side, i) {
      const q = side.query.trim().toLowerCase();
      const found = this.companies.filter(c => !this.taken(c.id) &&
        (q === '' || [c.name, c.owner, c.faction].some(v => v.toLowerCase().includes(q))));
      const placed = !this.ffa && this.visible.some((s, j) => j !== i && s.ids.some(id => this.company(id).mine));
      const mine = found.filter(c => c.mine), others = found.filter(c => !c.mine);
      return placed ? [...others, ...mine] : [...mine, ...others];
    },
    // "Same player" notes for one side: a player with two or more companies on it.
    sameOnSide(side) {
      const by = {};
      side.ids.forEach(id => { const c = this.company(id); (by[c.ownerId] = by[c.ownerId] || []).push(c); });
      return Object.values(by).filter(l => l.length > 1).map(l => (l.length === 2 ? 'Both are ' : 'All ' + l.length + ' are ') + l[0].owner + "'s");
    },
    // ...and across the sides: a player with companies on at least two of them.
    sameAcross() {
      const sides = {};
      this.visible.forEach((s, i) => s.ids.forEach(id => { const c = this.company(id); (sides[c.ownerId] = sides[c.ownerId] || { owner: c.owner, set: new Set() }).set.add(i); }));
      return Object.values(sides).filter(o => o.set.size > 1).map(o => 'Same player: ' + o.owner + ' plays ' + (o.set.size === 2 ? 'both sides' : o.set.size + ' sides'));
    },
    add(side, id) { side.ids.push(id); side.query = ''; this.loadATO(); },
    addAll(side) { this.results(side, 0).forEach(c => side.ids.push(c.id)); side.query = ''; },
    remove(side, id) { side.ids = side.ids.filter(i => i !== id); this.loadATO(); },
    // "a-b" for a 1 v 1, "" otherwise: the Against the Odds block exists only for two sides of one company each.
    pairKey() {
      if (this.ffa || this.sides.length !== 2 || this.sides.some(s => s.ids.length !== 1)) return '';
      return this.sides[0].ids[0] + '-' + this.sides[1].ids[0];
    },
    loadATO() {
      const key = this.pairKey(), target = document.getElementById('new-ato');
      if (!target) return;
      if (key === '') { this.atoKey = ''; target.innerHTML = ''; this.ambush(false); return; }
      const want = key + (this.practice ? 'p' : '');
      if (want === this.atoKey && target.firstElementChild) return;
      this.atoKey = want;
      this.ambush(false);
      htmx.ajax('GET', '/campaigns/' + this.campaign + '/matches/new/ato?a=' + this.sides[0].ids[0] + '&b=' + this.sides[1].ids[0] + (this.practice ? '&practice=1' : ''), { target: '#new-ato', swap: 'innerHTML' });
    },
    // Ambush! on: the scenario card becomes the rulebook's list; off: roll again.
    ambush(on) {
      const card = document.getElementById('new-scenario');
      if (!card || (card.dataset.ambush === 'true') === on) return;
      const base = '/campaigns/' + this.campaign + '/matches/new/';
      if (on) htmx.ajax('GET', base + 'scenario?mode=pick&ambush=1', { target: '#new-scenario', swap: 'outerHTML', source: card });
      else htmx.ajax('POST', base + 'scenario-roll', { target: '#new-scenario', swap: 'outerHTML' });
    },
  }));

  // The MVP of an opposing company in the End match and Confirm sheets: a search over its units (the same combobox as the company search of
  // the new match form); the chosen unit goes in the hidden mvp-{company} field.
  Alpine.data('unitPick', () => ({
    units: [],
    q: '',
    id: 0,
    open: false,
    init() {
      this.units = JSON.parse(this.$el.dataset.units || '[]');
      this.id = Number(this.$el.dataset.current) || 0;
      const u = this.units.find(x => x.id === this.id);
      this.q = u ? u.title : '';
    },
    results() {
      const q = this.q.trim().toLowerCase(), cur = this.units.find(x => x.id === this.id);
      if (cur && cur.title.toLowerCase() === q) return this.units;
      return this.units.filter(u => q === '' || u.title.toLowerCase().includes(q) || u.detail.toLowerCase().includes(q));
    },
    pick(u) { this.id = u.id; this.q = u.title; this.open = false; },
  }));

  // The Against the Odds switches of the form: what is left of the gap, and a bonus the rest cannot pay for is disabled.
  Alpine.data('atoPick', () => ({
    gap: 0,
    inclusive: false,
    spent: 0,
    init() {
      this.gap = Number(this.$el.dataset.gap) || 0;
      this.inclusive = this.$el.dataset.inclusive === 'true';
      this.recount();
    },
    recount() {
      this.spent = [...this.$el.querySelectorAll('input[name=ato]:checked')].reduce((n, e) => n + Number(e.dataset.rating || 0), 0);
    },
    left() { return this.gap - this.spent; },
    over(rating, el) {
      if (el && el.checked) return false;
      const left = this.left();
      return rating > left || (!this.inclusive && rating === left);
    },
    changed(e, key) {
      this.recount();
      if (key === 'ambush') this.ambush(e.target.checked);
    },
  }));
});
