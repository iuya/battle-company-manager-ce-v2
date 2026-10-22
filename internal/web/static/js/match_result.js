// The result form of a match: warns, as the players tick, when the winner is a side with a company that fled the battlefield
// (All is Lost: the opponent claims the win). The server checks it again when the match is ended.
// State comes from data-result (JSON: companies with id, name, side, fled; side names) and data-winner on the form.
document.addEventListener('alpine:init', () => {
  Alpine.data('matchResult', () => ({
    winner: '',
    fled: {},
    companies: [],
    sides: [],
    vps: [], // reported Victory Points per side (null: not reported)
    held: {}, // label of a one-holder objective -> names of the companies that ticked it
    init() {
      const data = JSON.parse(this.$el.dataset.result);
      this.companies = data.companies || [];
      this.sides = data.sides || [];
      this.winner = this.$el.dataset.winner || '';
      this.companies.forEach(c => { this.fled[String(c.id)] = c.fled; });
      this.refresh();
    },
    // Reads the Victory Points and the one-holder ticks from the fields (they are plain inputs, posted as they are).
    refresh() {
      const f = this.$el;
      this.vps = [...f.querySelectorAll('input[data-vp]')].map(i => (i.value.trim() === '' ? null : Number(i.value)));
      const held = {};
      f.querySelectorAll('input[data-holds]:checked').forEach(i => { (held[i.dataset.holds] = held[i.dataset.holds] || []).push(i.dataset.company); });
      this.held = held;
    },
    // Hints that do not stop anything: the players know what happened at the table.
    hints() {
      const out = [];
      if (this.winner === 'draw' && this.vps.filter(v => v !== null).length > 1 && new Set(this.vps.filter(v => v !== null)).size > 1) {
        out.push('You reported a draw, but the Victory Points differ.');
      }
      const side = Number(this.winner);
      if (side && this.vps[side - 1] !== null && this.vps[side - 1] !== undefined) {
        const best = this.vps.reduce((m, v, i) => (v !== null && v > m.v ? { v, i } : m), { v: -1, i: -1 });
        if (best.i >= 0 && best.i !== side - 1 && best.v > this.vps[side - 1]) {
          out.push(`${this.sides[side - 1]} is reported as the winner, but ${this.sides[best.i]} has more Victory Points (${best.v} to ${this.vps[side - 1]}).`);
        }
      }
      Object.entries(this.held).forEach(([label, names]) => {
        if (names.length > 1) out.push(`${names.join(' and ')} both ticked "${label}", which only one company can have.`);
      });
      return out;
    },
    // The message to show, or '' when the winner and the fled companies agree.
    conflict() {
      const side = Number(this.winner);
      if (!side) return '';
      const c = this.companies.find(c => c.side === side && this.fled[String(c.id)]);
      return c ? `${c.name} fled the battlefield, so ${this.sides[side - 1]} cannot be the winner.` : '';
    },
  }));
});
