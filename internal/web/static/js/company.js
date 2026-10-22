// The My Company page: expanding one unit card at a time, previews on hover, the level-up button, the windows (recruit,
// equipment) and changing the formation of the units (drag and drop with SortableJS, or the arrows of each handle).
document.addEventListener('alpine:init', () => {
  // A window with a roll in it (recruiting, treating a hero's injuries): the dice (static/js/dice.js), the roll the player may adjust, and the chart row it lands on.
  Alpine.data('rollPanel', () => ({
    ...window.diceMixin(),
    roll: 0, adjust: 0, budget: 0, influence: 0, revealed: true, shakeOn: false, // note: `shake` is the method of the dice mixin; a field of that name would hide it
    init() {
      const d = this.$el.dataset;
      this.shakeOn = d.shake === 'true';
      this.roll = Number(d.roll) || 0; this.budget = Number(d.budget) || 0; this.influence = Number(d.influence) || 0;
      // a roll the server just made is shown rolling first, and its number revealed when the dice stop
      const dice = (d.dice || '').split(',').map(Number).filter(n => n > 0);
      const values = dice.length ? dice : (this.roll > 0 ? [this.roll] : []);
      this.revealed = !(d.animate === 'true' && values.length);
      if (!this.revealed) this.$nextTick(() => this.throwDice({ count: values.length, values, then: () => { this.revealed = true; }, groups: (d.groups || '').split(',').map(Number).filter(n => n > 0), labels: (d.labels || '').split(',').filter(Boolean) }));
    },
    // Where Shake the table is on, the Roll button throws the dice in the browser, unless the player typed their own roll.
    typed() { const f = this.$refs.rollForm; return !!f && ['roll', 'roll2'].some(n => f.elements[n] && Number(f.elements[n].value) > 0); },
    // A level-up thrown in the browser: a warrior throws one die, a hero two pairs (in two lanes); the result is sent as the roll.
    throwLevel() {
      const hero = this.$root.dataset.hero === 'true', form = this.$refs.rollForm;
      this.throwDice({ count: hero ? 4 : 1, groups: hero ? [2, 2] : null, labels: ['First roll', 'Second roll'], canShake: true, then: v => {
        form.elements.roll.value = String(hero ? v[0] + v[1] : v[0]);
        if (hero) form.elements.roll2.value = String(v[2] + v[3]);
        form.elements.thrown.value = '1';
        form.requestSubmit();
      } });
    },
    // A casualty's injury roll (2D6) thrown in the browser, sent as the player's own two dice.
    throwCasualty() {
      const form = this.$refs.rollForm;
      this.throwDice({ count: 2, canShake: true, then: v => {
        form.elements.roll.value = String(v[0]);
        form.elements.roll2.value = String(v[1]);
        form.elements.thrown.value = '1';
        form.requestSubmit();
      } });
    },
    // The scenario roll of the picker thrown in the browser: two dice in two lanes, sent as the player's own roll.
    throwScenario() {
      const form = this.$refs.rollForm;
      this.throwDice({ count: 2, groups: [1, 1], labels: ['Table', 'Scenario'], canShake: true, then: v => {
        form.elements.roll.value = String(v[0]);
        form.elements.roll2.value = String(v[1]);
        form.elements.thrown.value = '1';
        form.requestSubmit();
      } });
    },
    get max() { return Math.min(this.budget, this.influence); },
    get final() { return Math.min(6, Math.max(1, this.roll + this.adjust)); },
    // The browser throws the dice (Shake the table is on) and reports the result as the player's own roll.
    throwHere() {
      this.throwDice({ count: 1, canShake: true, then: v => {
        this.$refs.own.value = String(v[0]);
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
    // Renaming a unit: double-click its name, or the pencil on the expanded card.
    editing: null,
    async editName(id) { this.hideTip(); this.editing = id; await this.$nextTick(); const i = document.getElementById('rename-' + id); if (i) { i.focus(); i.select(); } },
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
