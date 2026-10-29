// The new-match form: pick the number of sides, name them, and search the campaign's companies to put on each side.
// State comes from the data-companies / data-sides attributes (JSON) and goes back to the server as plain form fields:
// sides, side-name-N, side-role-N (Attacker/Defender tag, optional) and one side-N field per company; in all vs all mode, mode=ffa and the pool as side-1.
// A player may own several companies (docs/multi-company-spec.md): the search shows owner and faction beside the name, "Mine" narrows it to the
// viewer's own, and a non-blocking "Same player" note names a player who has two companies on one side or companies on several sides.
document.addEventListener('alpine:init', () => {
  Alpine.data('matchForm', () => ({
    companies: [],
    sides: [],
    ffa: false, // all vs all: one pool of companies, each fighting alone
    mineOnly: false, // the "Mine" filter: only the viewer's own companies in the search
    init() {
      this.ffa = this.$el.dataset.ffa === 'true';
      this.companies = JSON.parse(this.$el.dataset.companies);
      this.sides = JSON.parse(this.$el.dataset.sides).map(s => ({ name: s.name, ids: s.ids, role: s.role || '', query: '', open: false }));
    },
    get visible() { return this.ffa ? this.sides.slice(0, 1) : this.sides; },
    setFFA(on) {
      this.ffa = on;
      if (on) { // everybody goes into the pool
        const all = this.sides.flatMap(s => s.ids);
        this.sides.forEach(s => { s.ids = []; });
        this.sides[0].ids = all;
      }
    },
    setCount(n) {
      n = Number(n);
      while (this.sides.length < n) this.sides.push({ name: '', ids: [], role: '', query: '', open: false });
      this.sides.length = n; // companies of a removed side go back to the pool
    },
    company(id) { return this.companies.find(c => c.id === id); },
    taken(id) { return this.sides.some(s => s.ids.includes(id)); },
    get hasMine() { return this.companies.some(c => c.mine); },
    // The viewer's own companies come first, then the rest in the order the server sent.
    results(side) {
      const q = side.query.trim().toLowerCase();
      const found = this.companies.filter(c => !this.taken(c.id) && (!this.mineOnly || c.mine) &&
        (q === '' || [c.name, c.owner, c.faction].some(v => v.toLowerCase().includes(q))));
      return [...found.filter(c => c.mine), ...found.filter(c => !c.mine)];
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
    add(side, id) { side.ids.push(id); side.query = ''; },
    addAll(side) { this.results(side).forEach(c => side.ids.push(c.id)); side.query = ''; },
    remove(side, id) { side.ids = side.ids.filter(i => i !== id); },
  }));
});
