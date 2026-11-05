// The new-match form (docs/matches.md, "New match"): two sides with the company search only, + Add side (up to the maximum, an added side
// can be removed), All vs all and Practice. State comes from the data-companies / data-sides attributes (JSON) and goes back to the server
// as plain form fields: sides and one side-N field per company; in all vs all mode, mode=ffa and the pool as side-1.
// A player may own several companies (docs/matches.md, "Several companies of one player"): the search shows owner and faction beside the name, the viewer's own
// companies are highlighted and listed first, and last on the other sides once one of them is placed (playing yourself is rare); a
// non-blocking "Same player" note names a player who has two companies on one side or companies on several sides.
// A company that cannot play right now (data-companies "unavailable": the reason) is listed struck through and disabled, after the free ones of its group.
document.addEventListener('alpine:init', () => {
  const newSide = (ids = []) => ({ ids, query: '', open: false });

  Alpine.data('matchForm', () => ({
    companies: [],
    sides: [],
    ffa: false, // all vs all: one pool of companies, each fighting alone
    practice: false,
    maxSides: 0,
    init() {
      this.ffa = this.$el.dataset.ffa === 'true';
      this.practice = this.$el.dataset.practice === 'true';
      this.maxSides = Number(this.$el.dataset.maxSides);
      this.companies = JSON.parse(this.$el.dataset.companies);
      this.sides = JSON.parse(this.$el.dataset.sides).map(s => newSide(s.ids));
    },
    get visible() { return this.ffa ? this.sides.slice(0, 1) : this.sides; },
    setFFA(on) {
      this.ffa = on;
      if (on) { // everybody goes into the pool
        const all = this.sides.flatMap(s => s.ids);
        this.sides.forEach(s => { s.ids = []; });
        this.sides[0].ids = all;
      } else if (this.sides.length < 2) {
        this.sides.push(newSide());
      }
    },
    addSide() { if (this.sides.length < this.maxSides) { this.sides.push(newSide()); } },
    removeSide(i) { this.sides.splice(i, 1); }, // its companies go back to the pool
    company(id) { return this.companies.find(c => c.id === id); },
    taken(id) { return this.sides.some(s => s.ids.includes(id)); },
    sideLabel(side) { return side.ids.map(id => this.company(id).name).join(' + '); },
    // The viewer's own companies come first; once one of them is on another side they come last here.
    results(side, i) {
      const q = side.query.trim().toLowerCase();
      const found = this.companies.filter(c => !this.taken(c.id) &&
        (q === '' || [c.name, c.owner, c.faction].some(v => v.toLowerCase().includes(q))));
      const placed = !this.ffa && this.visible.some((s, j) => j !== i && s.ids.some(id => this.company(id).mine));
      const group = own => [...found.filter(c => c.mine === own && !c.unavailable), ...found.filter(c => c.mine === own && c.unavailable)];
      return placed ? [...group(false), ...group(true)] : [...group(true), ...group(false)];
    },
    free(side, i) { return this.results(side, i).filter(c => !c.unavailable); },
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
    add(side, id) { side.ids.push(id); side.query = ''; },
    addAll(side) { this.free(side, 0).forEach(c => side.ids.push(c.id)); side.query = ''; },
    remove(side, id) { side.ids = side.ids.filter(i => i !== id); },
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
});
