// The My Company page: expanding one unit card at a time, previews on hover, the level-up button, the windows (recruit,
// equipment) and changing the formation of the units (drag and drop with SortableJS, or the arrows of each handle).
document.addEventListener('alpine:init', () => {
  // A window with a roll in it (recruiting, treating a hero's injuries): the dice (static/js/dice.js), the roll the player may adjust, and the chart row it lands on.
  Alpine.data('rollPanel', () => ({
    ...window.diceMixin(),
    roll: 0, adjust: 0, budget: 0, budgetDown: 0, cost: 1, sides: 6, perLane: 1, lanes: 1, influence: 0, revealed: true, shakeOn: false, // note: `shake` is the method of the dice mixin; a field of that name would hide it
    init() {
      const d = this.$el.dataset;
      this.shakeOn = d.shake === 'true';
      this.roll = Number(d.roll) || 0; this.budget = Number(d.budget) || 0; this.budgetDown = Number(d.budgetDown) || 0; this.cost = Number(d.cost) || 0; this.sides = Number(d.sides) || 6; this.influence = Number(d.influence) || 0;
      // the dice of this roll, from the rules file: faces of each die, dice in one roll (perLane), rolls thrown side by side (lanes)
      this.perLane = Number(d.perLane) || 1; this.lanes = Number(d.lanes) || 1;
      // a roll the server just made is shown rolling first, and its number revealed when the dice stop
      const dice = (d.dice || '').split(',').map(Number).filter(n => n > 0);
      const values = dice.length ? dice : (this.roll > 0 ? [this.roll] : []);
      this.revealed = !(d.animate === 'true' && values.length);
      if (!this.revealed) this.$nextTick(() => this.throwDice({ count: values.length, sides: this.sides, values, then: () => { this.revealed = true; }, groups: (d.groups || '').split(',').map(Number).filter(n => n > 0), labels: (d.labels || '').split(',').filter(Boolean) }));
    },
    // Where Shake the table is on, the Roll button throws the dice in the browser, unless the player typed their own roll.
    typed() { const f = this.$refs.rollForm; return !!f && Array.from(f.elements).some(e => /^roll\d*$/.test(e.name) && Number(e.value) > 0); },
    // The form field of the i-th roll (1-based): roll, roll2, roll3 ... (read back by the server).
    field(i) { return this.$refs.rollForm.elements[i === 1 ? 'roll' : 'roll' + i]; },
    sum(v, from, n) { return v.slice(from, from + n).reduce((a, b) => a + b, 0); },
    // A level-up thrown in the browser: a warrior throws its dice, a hero one set for each result the rules ask for (one lane each);
    // the sum of each set is sent as the roll.
    throwLevel() {
      const hero = this.$root.dataset.hero === 'true', form = this.$refs.rollForm, n = this.perLane, lanes = hero ? this.lanes : 1;
      this.throwDice({ count: n * lanes, sides: this.sides, groups: lanes > 1 ? Array(lanes).fill(n) : null, labels: Array.from({ length: lanes }, (_, k) => ['First roll', 'Second roll'][k] || 'Roll ' + (k + 1)), canShake: true, then: v => {
        for (let k = 0; k < lanes; k++) this.field(k + 1).value = String(this.sum(v, k * n, n));
        form.elements.thrown.value = '1';
        form.requestSubmit();
      } });
    },
    // A casualty's injury roll thrown in the browser, sent as the player's own dice (one field each).
    throwCasualty() {
      const form = this.$refs.rollForm;
      this.throwDice({ count: this.perLane, sides: this.sides, canShake: true, then: v => {
        v.forEach((n, k) => { this.field(k + 1).value = String(n); });
        form.elements.thrown.value = '1';
        form.requestSubmit();
      } });
    },
    // The scenario roll of the picker thrown in the browser: two dice in two lanes, sent as the player's own roll.
    throwScenario() {
      const form = this.$refs.rollForm;
      this.throwDice({ count: 2, sides: this.sides, groups: [1, 1], labels: ['Table', 'Scenario'], canShake: true, then: v => {
        form.elements.roll.value = String(v[0]);
        form.elements.roll2.value = String(v[1]);
        form.elements.thrown.value = '1';
        form.requestSubmit();
      } });
    },
    // What the company can pay for at the rules' price per point (free where the price is 0).
    get affordable() { return this.cost > 0 ? Math.floor(this.influence / this.cost) : Infinity; },
    // Whether the step (points, positive raises the die) is within the attempt's points, the die and the influence.
    allowed(n) { return n === 0 || (n > 0 ? n <= Math.min(this.budget, this.affordable) : -n <= Math.min(this.budgetDown, this.affordable)); },
    get final() { return Math.min(this.sides, Math.max(1, this.roll + this.adjust)); },
    // The browser throws the dice (Shake the table is on) and reports the result as the player's own roll.
    throwHere() {
      this.throwDice({ count: this.perLane, sides: this.sides, canShake: true, then: v => {
        this.$refs.own.value = String(this.sum(v, 0, v.length));
        this.$refs.thrown.value = '1';
        this.$refs.rollForm.requestSubmit();
      } });
    },
  }));

  // The cemetery: its own list of cards with its own drag and drop. A card cannot leave it, and the roster's cards cannot come in
  // (the two lists are separate Sortable instances without a shared group).
  Alpine.data('cemetery', () => ({
    editing: false, order: '', original: [], sortable: null,
    ids() { return [...this.$refs.grid.children].map(c => c.dataset.unitId); },
    sync() { this.order = this.ids().join(','); },
    start() {
      this.original = this.ids(); this.sync();
      if (!this.sortable && window.Sortable) {
        this.sortable = Sortable.create(this.$refs.grid, { animation: 150, draggable: '.unit-slot', handle: '.unit-handle', filter: 'button', preventOnFilter: false,
          ghostClass: 'unit-ghost', dragClass: 'unit-drag', onEnd: () => this.sync(), forceFallback: true, fallbackTolerance: 3, swapThreshold: 0.6 });
      }
      if (this.sortable) this.sortable.option('disabled', false);
      this.editing = true;
    },
    cancel() {
      const grid = this.$refs.grid;
      this.original.forEach(id => grid.appendChild(grid.querySelector(`[data-unit-id="${id}"]`)));
      if (this.sortable) this.sortable.option('disabled', true);
      this.editing = false;
    },
    move(button, delta) {
      const slot = button.closest('.unit-slot');
      const next = delta < 0 ? slot.previousElementSibling : slot.nextElementSibling;
      if (!next) return;
      slot.parentNode.insertBefore(delta < 0 ? slot : next, delta < 0 ? next : slot);
      this.sync();
    },
  }));

  Alpine.data('company', () => ({
    expanded: null, // the id of the one maximised card
    modal: false,   // the recruit / equipment window
    formation: false,     // changing the order of the units
    formationOrder: '',   // their ids in the order they are in now, comma separated (what Save sends)
    sortable: null,
    original: [],
    slotH: 0,       // the height it left behind, so the grid does not reflow while it is open
    notice: '',
    noticeTimer: null,
    // While an aftermath is waiting the company is locked: the controls marked data-lock say why instead of doing anything (the
    // core refuses the action as well, this just saves the round trip).
    lockClick(e) {
      const reason = this.$el.dataset.lock;
      const el = reason && e.target.closest ? e.target.closest('[data-lock]') : null;
      if (!el || el === this.$el) return;
      e.preventDefault(); e.stopPropagation();
      this.notice = reason;
      clearTimeout(this.noticeTimer);
      this.noticeTimer = setTimeout(() => { this.notice = ''; }, 4000);
    },
    tip: { show: false, x: 0, y: 0, title: '', meta: '', text: '', stats: [], path: null },
    tipTimer: null,

    init() {
      this.modal = this.$el.dataset.modal === 'true'; // a recruit is waiting for its options: the window opens by itself
      const lock = () => document.body.classList.toggle('overflow-hidden', this.expanded !== null || this.modal);
      this.$watch('expanded', lock);
      this.$watch('modal', lock);
      lock();
    },
    slotIds() { return [...this.$refs.grid.children].map(c => c.dataset.unitId); },
    syncOrder() { this.formationOrder = this.slotIds().join(','); },
    // Sortable is made when the formation is first changed: the grid is not there yet when the component starts.
    sortableFor(grid) {
      if (!this.sortable && window.Sortable) {
        this.sortable = Sortable.create(grid, {
          animation: 150, draggable: '.unit-slot', handle: '.unit-handle', filter: 'button', preventOnFilter: false,
          ghostClass: 'unit-ghost', dragClass: 'unit-drag', onEnd: () => this.syncOrder(),
          // Sortable's own pointer-based drag instead of the browser's: the same on every browser, on touch screens too, and
          // no drag image or dropped-link surprises.
          forceFallback: true, fallbackTolerance: 3, swapThreshold: 0.6, // the drag clone stays inside the grid, so Alpine keeps it in scope
        });
      }
      return this.sortable;
    },
    startFormation() {
      this.expanded = null;
      this.original = this.slotIds();
      this.syncOrder();
      const sortable = this.sortableFor(this.$refs.grid);
      if (sortable) sortable.option('disabled', false);
      this.formation = true;
    },
    cancelFormation() { // put the cards back where they were
      const grid = this.$refs.grid;
      this.original.forEach(id => grid.appendChild(grid.querySelector(`[data-unit-id="${id}"]`)));
      if (this.sortable) this.sortable.option('disabled', true);
      this.formation = false;
    },
    // The arrows of a handle: the same as dragging a card one place.
    move(button, delta) {
      const slot = button.closest('.unit-slot');
      const next = delta < 0 ? slot.previousElementSibling : slot.nextElementSibling;
      if (!next) return;
      slot.parentNode.insertBefore(delta < 0 ? slot : next, delta < 0 ? next : slot);
      this.syncOrder();
      slot.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    },
    // The page is swapped after a purchase or a recruit: never leave the scroll locked behind.
    destroy() { document.body.classList.remove('overflow-hidden'); },

    toggle(id, button) {
      if (this.expanded === id) { this.expanded = null; return; }
      this.slotH = button.closest('.unit-slot').offsetHeight;
      this.hideTip();
      this.expanded = id;
    },
    // Escape and the backdrop: close what is open; with nothing open, Escape gives up a formation change.
    close() {
      if (this.expanded !== null || this.modal) {
        this.expanded = null;
        this.closeModal();
      } else if (this.formation) {
        this.cancelFormation();
      }
    },
    // Renaming a unit: the pencil after its name, or a double-click on the name. It always opens on the saved name, never on
    // what was typed and given up before; a click elsewhere saves a change and just closes otherwise.
    editing: null,
    async editName(id) {
      this.hideTip();
      const i = document.getElementById('rename-' + id);
      if (i) i.value = i.defaultValue;
      this.editing = id;
      await this.$nextTick();
      if (i) { i.focus(); i.select(); }
    },
    leaveName(form) {
      const i = form.elements.name;
      if (i.value !== i.defaultValue) form.requestSubmit(); else this.editing = null;
    },
    openModal() { this.hideTip(); this.modal = true; },
    closeModal() { this.modal = false; },


    // Hovering (or focusing) a rule, an item with rules, or a glossary link previews it. Not on touch screens.
    over(e) {
      if (!window.matchMedia('(hover: hover)').matches) return;
      const el = e.target.closest && e.target.closest('[data-tip-title]');
      if (!el) return;
      clearTimeout(this.tipTimer);
      this.tipTimer = setTimeout(() => this.showTip(el), 150);
    },
    out(e) { if (e.target.closest && e.target.closest('[data-tip-title]')) this.hideTip(); },
    hideTip() { clearTimeout(this.tipTimer); this.tip.show = false; },
    async showTip(el) {
      let stats = [];
      try { stats = JSON.parse(el.dataset.tipStats || '[]'); } catch (e) { stats = []; }
      let path = null;
      try { path = el.dataset.tipPath ? JSON.parse(el.dataset.tipPath) : null; } catch (e) { path = null; }
      this.tip = { show: true, x: 0, y: 0, title: el.dataset.tipTitle, meta: el.dataset.tipMeta || '', text: el.dataset.tipText || '', stats, path };
      await this.$nextTick();
      const box = this.$refs.tip;
      if (!box || !el.isConnected) { this.tip.show = false; return; } // the page was swapped while the tip waited
      const r = el.getBoundingClientRect();
      const x = Math.max(8, Math.min(r.left, window.innerWidth - box.offsetWidth - 8));
      const below = r.bottom + 8;
      const y = below + box.offsetHeight > window.innerHeight && r.top - box.offsetHeight - 8 > 0 ? r.top - box.offsetHeight - 8 : below;
      this.tip.x = x; this.tip.y = Math.max(8, Math.min(y, window.innerHeight - box.offsetHeight - 8)); // a long text stays inside the window
    },
  }));
});
