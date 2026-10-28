// Searchable lists: the campaigns on the main page (listSearch) and the glossary (glossary, which adds jumping between
// terms, a way back and previews on hover). Everything runs in the browser over what is already on the page.
document.addEventListener('alpine:init', () => {
  // Shared by both: items carry data-name and optionally data-aliases (separated by |).
  const search = () => ({
    q: '',
    cat: '', // only the terms of this category (the glossary)
    fold(s) { return s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase(); },
    shows(el) {
      if (this.cat && el.dataset.category !== this.cat) return false;
      const q = this.fold(this.q.trim());
      return q === '' || this.fold(el.dataset.name).includes(q) ||
        (el.dataset.aliases || '').split('|').some(a => a !== '' && this.fold(a).includes(q));
    },
    // Whether any term of a category shows (its heading is hidden when none does).
    catShows(cat) {
      return [...this.$root.querySelectorAll('[data-category]')].some(el => el.dataset.category === cat && this.shows(el));
    },
    noMatch() {
      const items = [...this.$root.querySelectorAll('[data-name]')];
      return items.length > 0 && !items.some(el => this.shows(el));
    },
  });

  Alpine.data('listSearch', search);

  // The Armory: search, a switch that hides what no unit of the company can take (on by default, remembered in the browser), the main
  // categories and, under them, the item categories that apply to the one picked. Items carry data-main, data-sub and data-avail
  // ("0": a rule rules every unit out). Names on the creature and wanderer cards carry data-tip-* for the tooltip.
  Alpine.data('armory', () => ({
    ...search(),
    main: '', sub: '', mine: true, applies: {},
    tip: { show: false, x: 0, y: 0, title: '', meta: '', text: '' },
    tipTimer: null,
    init() {
      try { this.applies = JSON.parse(this.$root.dataset.applies || '{}'); } catch (e) { this.applies = {}; }
      try { if (localStorage.getItem('bcm-armory-mine') === '0') this.mine = false; } catch (e) { /* private window */ }
    },
    toggleMine() {
      this.mine = !this.mine;
      try { localStorage.setItem('bcm-armory-mine', this.mine ? '1' : '0'); } catch (e) { /* private window */ }
    },
    // the item categories of the main category picked (all of them with none); creatures and wanderers have none
    subApplies(key) { return this.main === '' || (this.applies[this.main] || []).includes(key); },
    pickMain(m) { this.main = m; if (this.sub && !this.subApplies(this.sub)) this.sub = ''; },
    clear() { this.q = ''; this.main = ''; this.sub = ''; this.mine = false; },
    ignoring(el) {
      if (this.main && el.dataset.main !== this.main) return false;
      if (this.sub && el.dataset.sub !== this.sub) return false;
      const q = this.fold(this.q.trim());
      return q === '' || this.fold(el.dataset.name).includes(q) || (el.dataset.aliases || '').split('|').some(a => a !== '' && this.fold(a).includes(q));
    },
    shows(el) { return this.ignoring(el) && !(this.mine && el.dataset.avail === '0'); },
    // items the switch hides that would show otherwise
    hiddenCount() { return [...this.$root.querySelectorAll('[data-avail="0"]')].filter(el => this.ignoring(el)).length; },
    mainShows(m) { return [...this.$root.querySelectorAll('[data-main="' + m + '"]')].some(el => this.shows(el)); },
    over(e) {
      if (!window.matchMedia('(hover: hover)').matches && e.type !== 'focusin') return;
      const el = e.target.closest && e.target.closest('[data-tip-title]');
      if (!el) return;
      clearTimeout(this.tipTimer);
      this.tipTimer = setTimeout(() => this.showTip(el), 150);
    },
    out(e) { if (e.target.closest && e.target.closest('[data-tip-title]')) this.hideTip(); },
    hideTip() { clearTimeout(this.tipTimer); this.tip.show = false; },
    async showTip(el) {
      this.tip = { show: true, x: 0, y: 0, title: el.dataset.tipTitle, meta: el.dataset.tipMeta || '', text: el.dataset.tipText || '' };
      await this.$nextTick();
      const box = this.$refs.tip;
      if (!box || !el.isConnected) { this.tip.show = false; return; }
      const r = el.getBoundingClientRect();
      const x = Math.max(8, Math.min(r.left, window.innerWidth - box.offsetWidth - 8));
      const below = r.bottom + 8;
      const y = below + box.offsetHeight > window.innerHeight && r.top - box.offsetHeight - 8 > 0 ? r.top - box.offsetHeight - 8 : below;
      this.tip.x = x; this.tip.y = Math.max(8, Math.min(y, window.innerHeight - box.offsetHeight - 8));
    },
  }));

  // The company browser: a searchable list, an alignment filter, and tooltips that show the card of a unit (a hidden element named
  // by data-preview) or the text of an item (data-tip-title / data-tip-text).
  Alpine.data('companyBrowser', () => ({
    ...search(),
    align: '', picked: '',
    tip: { show: false, x: 0, y: 0, html: '', title: '', meta: '', text: '' },
    tipTimer: null,
    init() { this.picked = this.$el.dataset.picked || ''; },
    shows(el) {
      if (this.align && el.dataset.align !== this.align) return false;
      const q = this.fold(this.q.trim());
      return q === '' || this.fold(el.dataset.name).includes(q);
    },
    over(e) {
      if (!window.matchMedia('(hover: hover)').matches && e.type === 'mouseover') return;
      const el = e.target.closest && e.target.closest('[data-preview], [data-tip-title]');
      if (!el) return;
      clearTimeout(this.tipTimer);
      this.tipTimer = setTimeout(() => this.showTip(el), 120);
    },
    out(e) { if (e.target.closest && e.target.closest('[data-preview], [data-tip-title]')) this.hide(); },
    hide() { clearTimeout(this.tipTimer); this.tip.show = false; },
    async showTip(el) {
      const src = el.dataset.preview ? document.getElementById(el.dataset.preview) : null;
      this.tip = { show: true, x: 0, y: 0, html: src ? src.innerHTML : '', title: el.dataset.tipTitle || '', meta: el.dataset.tipMeta || '', text: el.dataset.tipText || '' };
      await this.$nextTick();
      const box = this.$refs.tip;
      if (!box || !el.isConnected) { this.tip.show = false; return; } // the page was swapped while the tip waited
      const r = el.getBoundingClientRect();
      const x = Math.max(8, Math.min(r.left, window.innerWidth - box.offsetWidth - 8));
      const below = r.bottom + 8;
      const y = below + box.offsetHeight > window.innerHeight && r.top - box.offsetHeight - 8 > 0 ? r.top - box.offsetHeight - 8 : below;
      this.tip.x = x; this.tip.y = Math.max(8, Math.min(y, window.innerHeight - box.offsetHeight - 8));
    },
  }));

  Alpine.data('glossary', () => ({
    ...search(),
    trail: [],        // the terms you followed links from, newest last: what "Back" returns to
    hl: '',           // the term you jumped to (highlighted)
    scrolled: false,  // far enough down to offer "Top"
    tip: { show: false, x: 0, y: 0, name: '', category: '', text: '' },
    tipTimer: null,

    init() {
      // the category filter lives in the address (?c=), so a filtered view can be shared; an unknown category is ignored
      const c = new URLSearchParams(location.search).get('c') || '';
      if ([...this.$root.querySelectorAll('[data-chip]')].some(b => b.dataset.chip === c)) this.cat = c;
      this.$watch('cat', v => {
        const u = new URL(location.href);
        if (v) u.searchParams.set('c', v); else u.searchParams.delete('c');
        history.replaceState(history.state, '', u);
      });
      if (location.hash.startsWith('#t-')) this.reveal(location.hash.slice(1));
    },

    termOf(el) {
      const a = el && el.closest ? el.closest('a') : null;
      const href = a && a.getAttribute('href');
      return href && href.startsWith('#t-') ? { a, id: href.slice(1) } : null;
    },

    // Shows the term even when the search hid it, then scrolls to it. (Clearing the search has to be rendered before the
    // jump, which is why a plain #fragment link did nothing in a filtered list.)
    async reveal(id) {
      this.q = '';
      this.cat = '';
      this.hl = id;
      await this.$nextTick();
      const el = document.getElementById(id);
      if (el) el.scrollIntoView({ block: 'start' });
      history.replaceState(history.state, '', '#' + id);
    },

    onClick(e) {
      const t = this.termOf(e.target);
      if (!t) return;
      e.preventDefault();
      this.hideTip();
      const from = t.a.closest('article');
      if (from) this.trail.push({ id: from.id, name: from.dataset.name });
      this.reveal(t.id);
    },
    back() { const prev = this.trail.pop(); if (prev) this.reveal(prev.id); },
    top() {
      this.hl = '';
      history.replaceState(history.state, '', location.pathname + location.search);
      window.scrollTo({ top: 0 });
    },
    onHash() { if (location.hash.startsWith('#t-')) this.reveal(location.hash.slice(1)); },

    // Hover (or keyboard focus) on a link previews its term: plain text only, so previews never nest. Not on touch
    // screens, where a tap simply follows the link.
    over(e) {
      if (!window.matchMedia('(hover: hover)').matches) return;
      const t = this.termOf(e.target);
      if (!t) return;
      clearTimeout(this.tipTimer);
      this.tipTimer = setTimeout(() => this.showTip(t.a, t.id), 200);
    },
    out(e) {
      if (!this.termOf(e.target)) return;
      this.hideTip();
    },
    hideTip() { clearTimeout(this.tipTimer); this.tip.show = false; },
    async showTip(a, id) {
      const art = document.getElementById(id);
      const def = art && art.querySelector('[data-def]');
      if (!def) return;
      let text = def.innerText.trim().replace(/\s+/g, ' ');
      if (text.length > 480) text = text.slice(0, 480).replace(/\s+\S*$/, '') + '…';
      this.tip = { show: true, x: 0, y: 0, name: art.dataset.name, category: art.dataset.category, text };
      await this.$nextTick();
      const box = this.$refs.tip;
      if (!box || !a.isConnected) { this.tip.show = false; return; } // the page was swapped while the tip waited
      const r = a.getBoundingClientRect();
      const x = Math.max(8, Math.min(r.left, window.innerWidth - box.offsetWidth - 8));
      const below = r.bottom + 8;
      const y = below + box.offsetHeight > window.innerHeight && r.top - box.offsetHeight - 8 > 0 ? r.top - box.offsetHeight - 8 : below;
      this.tip.x = x; this.tip.y = y;
    },
  }));

  // A count and its noun in words ("a leader", "two sergeants", "no sergeants"): the numbers come from the rules.
  const count = (n, noun) => n === 1 ? 'a ' + noun : (['no', 'one', 'two', 'three', 'four', 'five', 'six'][n] || n) + ' ' + noun + 's';

  // Taking command of a company: the places for the leaders and the sergeants, the other starting models in a pool, dragged
  // between them (SortableJS) or moved with the buttons of a card. Nothing but the places decides who is an officer.
  Alpine.data('commission', () => ({
    name: '', leaders: [], sergeants: [], need: { leaders: 0, sergeants: 0 }, suggested: false, touched: false, // touched: the player has changed something, so what is missing is a mistake, not just the next step
    init() {
      this.suggested = this.$root.dataset.suggested === 'true';
      this.need = { leaders: parseInt(this.$root.dataset.leaders, 10) || 0, sergeants: parseInt(this.$root.dataset.sergeantsNeeded, 10) || 0 }; // the places the rules give a company
      this.name = (this.$root.querySelector('input[name=name]') || {}).value || '';
      this.$nextTick(() => { this.setup(); this.read(); });
    },
    slots() { return [...this.$root.querySelectorAll('.officer-slot')]; },
    slotsOf(kind) { return this.slots().filter(s => s.dataset.slot === kind); },
    pool() { return this.$root.querySelector('#start-pool'); },
    setup() {
      if (!window.Sortable || !this.pool()) return;
      const group = { name: 'start-' + this.$root.dataset.company, pull: true,
        // only a model the company lets lead may go to an officer's place; one that was there goes back to the pool (or swaps
        // with the dragged one). The locks (data-lock, from core.OfficerLocks): "leader" never leaves the Leader place nor gives
        // it to another; "officer" never goes to the pool and nobody replaces it from the pool; "barred" is never eligible.
        put: (to, from, dragged) => to.el === this.pool() ? !this.locked(dragged)
          : dragged.dataset.eligible === 'true' && !to.el.querySelector('[data-lock=leader]') && !(from.el === this.pool() && to.el.querySelector('[data-lock=officer]')) };
      [this.pool(), ...this.slots()].forEach(list => Sortable.create(list, { group, animation: 150, draggable: '.start-card:not([data-keep])', forceFallback: true, fallbackTolerance: 3,
        ghostClass: 'unit-ghost', dragClass: 'unit-drag', filter: 'button, a', preventOnFilter: false, onAdd: e => this.dropped(e), onSort: () => this.read() }));
    },
    // a card dropped on an officer's place that was taken sends the one that was there back where the dragged one came from
    dropped(e) {
      if (e.to !== this.pool()) [...e.to.children].filter(c => c !== e.item).forEach(old => e.from.appendChild(old));
      this.changed();
    },
    // a change of the player's own: the company's suggestion no longer applies
    changed() { this.suggested = false; this.touched = true; this.read(); },
    read() {
      const index = lists => lists.flatMap(list => [...list.children].map(c => c.dataset.index));
      this.leaders = index(this.slotsOf('leader'));
      this.sergeants = index(this.slotsOf('sergeant'));
    },
    card(i) { return this.$root.querySelector(`.start-card[data-index="${i}"]`); },
    officerName(i) { const c = this.card(i); return c ? c.dataset.unit : ''; },
    locked(card) { return card.dataset.lock === 'leader' || card.dataset.lock === 'officer'; },
    // the place a card goes to with the button of a role, or null: the one that was there goes where the card came from, so a locked
    // card is never displaced into the pool
    target(card, role) {
      void this.leaders; void this.sergeants; // the buttons are re-evaluated when the places change
      if (card.dataset.eligible !== 'true' || card.dataset.lock === 'leader') return null;
      const from = card.parentElement;
      const free = s => !s.children.length;
      const takeable = s => s !== from && !s.querySelector('[data-lock=leader]') && !(from === this.pool() && s.querySelector('[data-lock=officer]'));
      return this.slotsOf(role).filter(s => s !== from && takeable(s)).sort((a, b) => free(b) - free(a))[0] || null;
    },
    blocked(card, role) { return !this.target(card, role); },
    // the id of the lock text that explains why a button does nothing (for aria-describedby)
    blocker(card, role) {
      if (this.target(card, role)) return null;
      const places = this.slotsOf(role);
      const c = places.map(s => s.querySelector('[data-lock]')).find(Boolean);
      return c ? 'lock-' + c.dataset.index : null;
    },
    roleOf(card) {
      const i = card.dataset.index;
      return this.leaders.includes(i) ? 'leader' : this.sergeants.includes(i) ? 'sergeant' : '';
    },
    // the buttons of a card: Leader and Sergeant send it to a place, Remove sends it back to the pool
    assign(card, role) {
      const to = this.target(card, role);
      if (!to) return;
      const from = card.parentElement;
      [...to.children].forEach(old => from.appendChild(old));
      to.appendChild(card);
      this.changed();
    },
    unassign(card) { if (this.locked(card)) return; this.pool().appendChild(card); this.changed(); },
    incomplete() {
      if (this.leaders.length !== this.need.leaders || this.sergeants.length !== this.need.sergeants) return 'Choose ' + count(this.need.leaders, 'leader') + ' and ' + count(this.need.sergeants, 'sergeant') + '.';
      if (!this.name.trim()) return 'Give your company a name in the oath.';
      return '';
    },
    swear() { this.read(); if (!this.incomplete()) this.$refs.confirm.showModal(); },
  }));
  // The officer panel of the recruit window (a Ranger of the North takes command): the same places and cards as "commission", but the
  // Ranger's card (data-keep) may only move between officer places and a card dropped on an occupied place swaps with the one there.
  Alpine.data('officers', () => ({
    leader: '', sergeants: [], drop: 0, ranger: '',
    init() {
      this.drop = parseInt(this.$root.dataset.drop, 10) || 0;
      this.ranger = this.$root.dataset.ranger || '';
      this.$nextTick(() => { this.setup(); this.read(); });
    },
    slots() { return [...this.$root.querySelectorAll('.officer-slot')]; },
    pool() { return this.$root.querySelector('#start-pool'); },
    setup() {
      if (!window.Sortable || !this.pool()) return;
      const group = { name: 'officers-' + this.$root.dataset.company, pull: true,
        // the Ranger never goes to the pool, and nobody may take the place of the Ranger from the pool (he would end up in it)
        put: (to, from, dragged) => to.el === this.pool() ? dragged.dataset.keep !== 'true'
          : !(from.el === this.pool() && [...to.el.children].some(c => c.dataset.keep === 'true')) };
      [this.pool(), ...this.slots()].forEach(list => Sortable.create(list, { group, animation: 150, draggable: '.start-card', forceFallback: true, fallbackTolerance: 3,
        ghostClass: 'unit-ghost', dragClass: 'unit-drag', filter: 'button, a', preventOnFilter: false, onAdd: e => this.dropped(e), onSort: () => this.read() }));
    },
    // a card dropped on a place that was taken swaps: the one that was there goes where the dragged card came from
    dropped(e) {
      if (e.to !== this.pool()) [...e.to.children].filter(c => c !== e.item).forEach(old => e.from.appendChild(old));
      this.read();
    },
    read() {
      const ids = list => [...list.children].map(c => c.dataset.id);
      const [leader, ...sergeants] = this.slots();
      this.leader = ids(leader)[0] ?? '';
      this.sergeants = sergeants.flatMap(ids);
    },
    cards() { return [...this.$root.querySelectorAll('.start-card')]; },
    title(id) { const c = this.cards().find(c => c.dataset.id === String(id)); return c ? c.dataset.title : ''; },
    // the buttons of a card: Leader and Sergeant send it to a place (the one there swaps with it), Not an officer sends it to the pool
    assign(card, role) {
      const from = card.parentElement;
      const [leader, ...sergeants] = this.slots();
      const to = role === 'leader' ? leader : (sergeants.find(s => !s.children.length) || sergeants.find(s => s !== from));
      if (!to || to === from) return;
      [...to.children].forEach(old => from.appendChild(old));
      to.appendChild(card);
      this.read();
    },
    unassign(card) { this.pool().appendChild(card); this.read(); },
    incomplete() {
      if (this.leader === '') return 'Choose a Leader.';
      const out = [...this.pool().children].filter(c => c.dataset.officer === 'true').length;
      if (out !== this.drop) return this.drop === 1 ? 'Exactly one of the old officers stops being an officer: put him in the pool.' : 'Exactly ' + this.drop + ' of the old officers stop being officers: put them in the pool.';
      return '';
    },
    summary() {
      const names = this.sergeants.map(i => this.title(i));
      return (this.leader === '' ? 'No Leader yet' : this.title(this.leader) + ' leads') + (names.length ? '; ' + names.join(' and ') + (names.length > 1 ? ' are Sergeants.' : ' is a Sergeant.') : '.');
    },
  }));
});
