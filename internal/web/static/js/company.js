// The My Company page: expanding one unit card at a time, previews on hover, the level-up button, the windows (recruit,
// equipment) and changing the formation of the units (drag and drop with SortableJS, or the arrows of each handle).
document.addEventListener('alpine:init', () => {
  // The path carousel of the "Choose a path" window: the radios of the mosaic are the state (path = the selected radio's value, always one);
  // step() moves to the neighbour, the cards of the stack slide to the new place (--r, --dur; no slide under prefers-reduced-motion), a swipe on the details
  // moves too (the cards of the stack take their place from rel(): 0 in front, +-1 behind, +-2 away). A blocked path can be looked at but not taken.
  Alpine.data('pathCarousel', () => ({
    path: '', power: '', picking: false, names: [], x0: null, dur: 300, moving: false, timer: null,
    init() {
      const radios = [...this.$root.querySelectorAll('input[name=path]')];
      this.names = radios.map(r => r.value);
      this.path = this.$root.dataset.start || this.names[0] || '';
      // a jump of several paths plays longer (proportional to the distance, at most 0.6 s) with the cards in between going by; none under reduced motion
      this.$watch('path', (now, was) => {
        const far = Math.abs(this.names.indexOf(now) - this.names.indexOf(was));
        const calm = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
        this.dur = calm ? 0 : Math.min(600, 200 + 80 * far);
        clearTimeout(this.timer);
        this.moving = !calm && far > 1;
        if (this.moving) this.timer = setTimeout(() => { this.moving = false; }, this.dur);
      });
    },
    get index() { return this.names.indexOf(this.path); },
    get radio() { return [...this.$root.querySelectorAll('input[name=path]')].find(r => r.value === this.path); },
    get sorcerer() { return !!this.radio && this.radio.dataset.sorcerer === '1'; },
    get blocked() { return !this.radio || this.radio.dataset.blocked === '1'; },
    step(d) {
      const i = Math.min(this.names.length - 1, Math.max(0, this.index + d));
      this.path = this.names[i];
      this.$nextTick(() => this.radio && this.radio.focus({ preventScroll: true }));
    },
    rel(i) { return Math.max(-2, Math.min(2, i - this.index)); },
    get powerLabel() { const r = [...this.$root.querySelectorAll('input[name=power]')].find(x => x.value === this.power); return r ? r.dataset.label : ''; },
    get needsPower() { return this.sorcerer && !this.power; },
    pickPower(e) { if (e.detail > 0) this.picking = false; }, // a click chooses and closes; the arrow keys only move through the list (Enter or the button closes)
    off(i) { return i - this.index; },
    swipeStart(e) { this.x0 = e.changedTouches[0].clientX; },
    swipeEnd(e) {
      if (this.x0 === null) return;
      const dx = e.changedTouches[0].clientX - this.x0; this.x0 = null;
      if (Math.abs(dx) > 50) this.step(dx < 0 ? 1 : -1);
    },
  }));
  // The options of a level-up (level_panel, docs/frontend-level-up.md): a press only SELECTS an option (picked) and the Confirm button sends
  // the choice; nothing is written before. When the result takes the other result too (core.LevelPlan), one option is marked beforehand, a
  // press on another that takes the other moves the mark, and a pick in the other result is sent together with the mark; when both results
  // take the other, each has a mark and Confirm applies both.
  Alpine.data('levelChoices', () => ({
    plan: { marks: ['', ''], both: false, conflicts: {}, alone: [] },
    marks: ['', ''],
    picked: null, // the selected option {i, key}, not sent yet
    init() {
      try { if (this.$root.dataset.plan) this.plan = JSON.parse(this.$root.dataset.plan); } catch (e) { /* no plan: every option applies alone */ }
      this.marks = [...this.plan.marks];
    },
    get planned() { return !!(this.marks[0] || this.marks[1]); },
    // Confirm is on once there is something to send: a pick, or the two marks of "both".
    get ready() { return !!this.picked || (this.planned && this.plan.both); },
    // Selected: the pick, or the mark of a result whose own pick is not made.
    marked(i, key) {
      if (this.picked && (this.picked.i === i || this.picked.i < 0)) return this.picked.i === i && this.picked.key === key;
      return i >= 0 && this.marks[i] === key;
    },
    // The other result's options go with the mark (only when this result has no mark of its own).
    takesOther(i) { return this.planned && !this.plan.both && !this.marks[i]; },
    alsoTakes(key) { return this.planned && !this.plan.alone.includes(key); },
    // Why the option does not fit with the mark of the other result, empty when it does.
    why(i, key) {
      if (i < 0) return '';
      const other = this.marks[1 - i];
      if (!other || (this.marks[i] && !this.plan.both)) return '';
      return this.plan.conflicts[i === 0 ? key + '|' + other : other + '|' + key] || '';
    },
    press(el) {
      const i = Number(el.dataset.offer), key = el.value;
      if (this.why(i, key)) return;
      if (i < 0 || !this.planned) { this.picked = { i, key }; return; } // the company upgrade replaces either result, whatever is marked
      if (this.plan.both) { this.marks[i] = key; if (this.picked && this.picked.i === i) this.picked = null; return; }
      const side = this.marks[0] ? 0 : 1;
      // the other result, or an option that loses the other result or has nothing to give from it: a pick
      if (i !== side || !el.hasAttribute('data-again') || this.plan.alone.includes(key)) { this.picked = { i, key }; return; }
      this.marks[i] = key; // another option that takes the other result: it moves the mark
      if (this.picked && (this.picked.i === i || this.why(this.picked.i, this.picked.key))) this.picked = null;
    },
    // Confirm: the mark and the pick of the other result together, or the pick alone, or both marks.
    confirm() {
      const p = this.picked;
      if (!p) { if (this.planned && this.plan.both) this.applyBoth(); return; }
      if (p.i < 0 || !this.planned || this.plan.both) return this.send(p.key);
      const side = this.marks[0] ? 0 : 1;
      if (p.i !== side) return this.send(this.marks[side], p.key);
      this.send(p.key);
    },
    // The option that takes the other result goes first (ChooseLevelUp).
    applyBoth() {
      const again = [...this.$root.querySelectorAll('[data-opt][data-offer="0"]')].find(b => b.value === this.marks[0])?.hasAttribute('data-again');
      if (again) this.send(this.marks[0], this.marks[1]); else this.send(this.marks[1], this.marks[0]);
    },
    send(first, second, then) {
      const f = this.$root.elements;
      f.choice.value = first;
      f.also.value = second || ''; f.also.disabled = !second;
      f.then.value = then || ''; f.then.disabled = !then;
      this.$root.requestSubmit();
    },
  }));
  // A window with a roll in it (recruiting, treating a hero's injuries): the dice (static/js/dice.js), the roll the player may adjust, and the chart row it lands on.
  Alpine.data('rollPanel', () => ({
    ...window.diceMixin(),
    roll: 0, adjust: 0, budget: 0, budgetDown: 0, cost: 1, sides: 6, perLane: 1, lanes: 1, influence: 0, revealed: true, typing: false, shakeOn: false, // note: `shake` is the method of the dice mixin; a field of that name would hide it
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
      // a result with nothing left to do closes the window by itself, counted from when the dice stop
      const panel = this.$el, after = Number(d.closeAfter) || 0;
      const close = () => setTimeout(() => { if (panel.isConnected) this.finish(panel); }, after);
      if (after && this.revealed) close();
      else if (after) this.$watch('revealed', v => { if (v) close(); });
    },
    // Done: back to the page the window was opened from, refreshed. The aftermath's window reloads its page when it closes; My Company's does not.
    finish(panel) {
      const root = panel || this.$root.closest('[data-back]');
      const reloads = root.closest('[data-reloads-on-close]');
      this.closeModal();
      if (!reloads) htmx.ajax('GET', root.dataset.back, { target: '#main', swap: 'innerHTML show:window:top' });
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
    // The bonus die of a casualty's result (Lesson Learned: a D3) thrown in the browser; its faces and count come from the rules file via the form. The reading is sent as the answer.
    throwBonus() {
      const form = this.$refs.bonusForm, answer = this.$refs.bonusAnswer;
      this.throwDice({ count: Number(form.dataset.count) || 1, sides: Number(form.dataset.sides) || 6, canShake: true, then: v => {
        answer.value = String(this.sum(v, 0, v.length)); answer.disabled = false;
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
    // A scenario card's roll thrown in the browser (Shake the table; the new match form's and the match page's): the two dice are sent to the card's
    // roll URL (data-roll-url), which answers the card into data-roll-target (default the new match card); data-revision, where set, goes with them.
    // `rolling` is set first so the tray's canvas is shown (and has a size) before the dice are thrown.
    throwNewScenario() {
      this.rolling = true;
      this.$nextTick(() => this.throwDice({ count: 2, sides: this.sides, groups: [1, 1], labels: ['Table', 'Scenario'], canShake: true, then: v => {
        const d = this.$root.dataset, values = { roll: v[0], roll2: v[1], thrown: '1' };
        if (d.revision) values.revision = d.revision;
        htmx.ajax('POST', d.rollUrl, { target: d.rollTarget || '#new-scenario', swap: 'outerHTML', values });
      } }));
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

  // The tooltips of the casualties of an aftermath: a rule on hover or keyboard focus (a tap opens its helper popup, concepts.js), and the injuries
  // chart of the dice (data-tip-chart names the hidden element that holds it) on hover, keyboard focus or a tap.
  Alpine.data('aftermathTips', () => ({
    ...window.tipMixin({ focus: 'visible', extra: { html: '' } }),
    tap(e) {
      if (window.matchMedia('(hover: hover)').matches) return;
      const el = e.target.closest && e.target.closest('[data-tip-chart]');
      const again = this.tip.show && this.tipFor === el;
      this.hideTip();
      if (el && !again) this.showTip(el);
    },
    tipContent(el) {
      const chart = el.dataset.tipChart ? document.getElementById(el.dataset.tipChart) : null;
      return { ...tipTexts(el), html: chart ? chart.innerHTML : '' };
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
    // The same controls are exposed to assistive technology as disabled (aria-disabled, which also exempts their dimmed look from the
    // contrast rule), described by the aftermath banner; run again when htmx swaps part of the page, so a refresh keeps the attribute.
    // A control marked data-off is greyed by the server for its own reason (Recruit without the influence): it stays so when unlocked.
    syncLock() {
      const locked = !!this.$el.dataset.lock;
      const banner = document.getElementById('aftermath-banner') ? 'aftermath-banner' : '';
      this.$el.querySelectorAll('[data-lock]').forEach(el => {
        const ctl = el.matches('button, a, input, select, textarea') ? [el] : [...el.querySelectorAll('button, a[href], input:not([type=hidden]), select, textarea')];
        for (const c of ctl) {
          if (locked) {
            if (c.getAttribute('aria-disabled') !== 'true') c.setAttribute('aria-disabled', 'true');
            if (banner && !c.hasAttribute('aria-describedby')) c.setAttribute('aria-describedby', banner);
          } else if (c.hasAttribute('aria-disabled') && !c.hasAttribute('data-off')) {
            c.removeAttribute('aria-disabled');
          }
        }
      });
    },
    ...window.tipMixin({ extra: { stats: [], path: null } }),

    init() {
      this.syncLock();
      new MutationObserver(() => this.syncLock()).observe(this.$el, { childList: true, subtree: true });
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
    tipContent(el) {
      let stats = [];
      try { stats = JSON.parse(el.dataset.tipStats || '[]'); } catch (e) { stats = []; }
      let path = null;
      try { path = el.dataset.tipPath ? JSON.parse(el.dataset.tipPath) : null; } catch (e) { path = null; }
      return { ...tipTexts(el), stats, path };
    },
  }));
});
