// The new-match form: pick the number of sides, name them, and search the campaign's companies to put on each side.
// State comes from the data-companies / data-sides attributes (JSON) and goes back to the server as plain form fields:
// sides, side-name-N, side-role-N (Attacker/Defender tag, optional) and one side-N field per company; in all vs all mode, mode=ffa and the pool as side-1.
document.addEventListener('alpine:init', () => {
  Alpine.data('matchForm', () => ({
    companies: [],
    sides: [],
    ffa: false, // all vs all: one pool of companies, each fighting alone
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
    results(side) {
      const q = side.query.trim().toLowerCase();
      return this.companies.filter(c => !this.taken(c.id) &&
        (q === '' || [c.name, c.owner, c.faction].some(v => v.toLowerCase().includes(q))));
    },
    add(side, id) { side.ids.push(id); side.query = ''; },
    addAll(side) { this.results(side).forEach(c => side.ids.push(c.id)); side.query = ''; },
    remove(side, id) { side.ids = side.ids.filter(i => i !== id); },
  }));
});
