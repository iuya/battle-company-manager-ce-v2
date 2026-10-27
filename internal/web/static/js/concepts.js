// The helper popup (docs/concept-graph.md): tapping a term (an element with data-concept) opens a sheet with what the rules say about
// it and chips for the things it is linked to; a chip swaps the content, Back returns. Hovering still shows the tooltip that was
// there before; when the lookup fails the popup shows that tooltip's text instead, so a term never does nothing.
//
// The content comes from GET /campaigns/{id}/concepts/resolve?name=... (a displayed name) and .../concepts/{id}: HTML made by the
// server from the rules files (escaped there), inserted as it is. Plain script, no inline handlers (CSP). The sheet is a dialog the
// script keeps in order itself (focus into it, Tab inside it, Escape, focus back), so dialog.js leaves it alone (data-focus-managed).
(function () {
  const wide = window.matchMedia('(min-width: 768px)');
  const FOCUSABLE = 'a[href],button:not([disabled]),summary,input:not([disabled]),[tabindex]:not([tabindex="-1"])';
  const cache = new Map(); // url -> { status, html }: Back and a second visit cost no request
  let layer = null, pop = null, bodyEl = null, backBtn = null;
  let opener = null, anchor = null, stack = [], cur = null, seq = 0;

  const campaignId = () => { const m = /^\/campaigns\/(\d+)(\/|$)/.exec(location.pathname); return m ? m[1] : null; };
  const byId = (cid, id) => `/campaigns/${cid}/concepts/${encodeURIComponent(id)}`;
  const byName = (cid, name) => `/campaigns/${cid}/concepts/resolve?name=${encodeURIComponent(name)}`;
  const el = (tag, cls, text) => { const e = document.createElement(tag); if (cls) e.className = cls; if (text) e.textContent = text; return e; };
  const isPopup = (res) => res && (res.status === 200 || res.status === 404) && res.html.includes('data-concept-root');

  async function load(url) {
    if (cache.has(url)) return cache.get(url);
    let res;
    try {
      const r = await fetch(url, { headers: { 'HX-Request': 'true' }, credentials: 'same-origin' });
      res = { status: r.status, html: await r.text() };
    } catch (e) { return null; }
    if (isPopup(res)) cache.set(url, res); // an answer that is not a popup (a login page, an error) is never kept
    return res;
  }

  function build() {
    layer = el('div', 'concept-layer');
    layer.dataset.focusManaged = '';
    const backdrop = el('div', 'concept-backdrop');
    backdrop.dataset.conceptClose = '';
    pop = el('div', 'concept-pop');
    pop.setAttribute('role', 'dialog');
    pop.setAttribute('aria-modal', 'true');
    pop.setAttribute('aria-label', 'Rules help');
    pop.dataset.focusManaged = '';
    const bar = el('div', 'concept-bar');
    backBtn = el('button', 'btn btn-ghost', '← Back');
    backBtn.type = 'button';
    backBtn.dataset.conceptBack = '';
    backBtn.hidden = true;
    const close = el('button', 'btn btn-ghost ml-auto', 'Close');
    close.type = 'button';
    close.dataset.conceptClose = '';
    close.setAttribute('aria-label', 'Close the rules help');
    bar.append(backBtn, close);
    bodyEl = el('div', 'concept-body');
    pop.append(bar, bodyEl);
    layer.append(backdrop, pop);
    document.body.append(layer);
    document.documentElement.classList.add('concept-open');
  }

  function place() {
    if (!pop) return;
    if (!wide.matches || !anchor || !anchor.isConnected) { pop.style.left = ''; pop.style.top = ''; return; }
    const r = anchor.getBoundingClientRect(), w = pop.offsetWidth, h = pop.offsetHeight, m = 8;
    const x = Math.max(m, Math.min(r.left, window.innerWidth - w - m));
    const below = r.bottom + m;
    let y = below + h > window.innerHeight - m && r.top - h - m > m ? r.top - h - m : below;
    y = Math.max(m, Math.min(y, window.innerHeight - h - m));
    pop.style.left = x + 'px';
    pop.style.top = y + 'px';
  }

  function settle(label) {
    pop.setAttribute('aria-busy', 'false');
    if (label) pop.setAttribute('aria-label', label);
    backBtn.hidden = stack.length === 0;
    bodyEl.scrollTop = 0;
    place();
    const h = bodyEl.querySelector('#concept-title');
    if (h) h.focus({ preventScroll: true });
  }

  function loading() {
    pop.setAttribute('aria-busy', 'true');
    bodyEl.replaceChildren(el('div', 'concept-skel w-2/3'), el('div', 'concept-skel'), el('div', 'concept-skel'), el('div', 'concept-skel w-1/2'));
    backBtn.hidden = stack.length === 0;
    place();
  }

  function showHTML(html, works) {
    bodyEl.innerHTML = html; // made and escaped by the server from the rules files
    if (works) { // "Works against: enemy models with the Orc keyword." of the rule chip that was opened (data-tip-works), after the description
      const line = el('p', 'concept-works', works);
      const text = bodyEl.querySelector('.concept-text');
      const title = bodyEl.querySelector('#concept-title');
      if (text) text.after(line); else if (title) title.after(line);
    }
    const root = bodyEl.querySelector('[data-concept-root]');
    settle(root && root.dataset.title);
  }

  // Today's tooltip as a popup: the text the element already carries.
  function showFallback(f) {
    const box = el('div', 'concept');
    const h = el('h2', 'concept-title', f.title || 'Rules help');
    h.id = 'concept-title';
    h.tabIndex = -1;
    box.append(h);
    if (f.meta) box.append(el('p', 'text-xs text-muted', f.meta));
    box.append(el('p', 'concept-text', f.text || 'No more is known about this.'));
    if (f.href) {
      const p = el('p', 'concept-foot');
      const a = el('a', 'btn btn-secondary !min-h-9', 'Open in glossary');
      a.href = f.href;
      a.setAttribute('hx-boost', 'false');
      p.append(a);
      box.append(p);
    }
    bodyEl.replaceChildren(box);
    settle(f.title);
  }

  function showError() {
    const box = el('div', 'concept');
    const h = el('h2', 'concept-title', 'Could not load this');
    h.id = 'concept-title';
    h.tabIndex = -1;
    const retry = el('button', 'btn btn-secondary', 'Retry');
    retry.type = 'button';
    retry.dataset.conceptRetry = '';
    box.append(h, el('p', 'text-sm', 'The rules help did not load. Check the connection and try again.'), retry);
    bodyEl.replaceChildren(box);
    settle('Could not load this');
  }

  async function go(target, push) {
    if (!layer) return;
    const mine = ++seq;
    if (push && cur) stack.push(cur);
    cur = { target };
    loading();
    const res = await load(target.url);
    if (mine !== seq || !layer) return; // another step or Close came first
    if (res && isPopup(res) && (res.status === 200 || target.byId)) { cur.html = res.html; showHTML(res.html, target.works); return; }
    if (target.fallback) { showFallback(target.fallback); return; } // an unknown name, an old server: the tooltip's own text
    showError();
  }

  function open(opening) {
    const cid = campaignId();
    const name = (opening.dataset.concept || opening.dataset.tipTitle || opening.textContent || '').trim();
    if (!cid || !name) return false;
    for (const t of ['mouseout', 'focusout']) opening.dispatchEvent(new MouseEvent(t, { bubbles: true })); // the hover tooltip steps aside
    if (layer) close(false);
    opener = opening;
    anchor = opening;
    stack = [];
    cur = null;
    build();
    go({
      url: byName(cid, name),
      works: opening.dataset.tipWorks || '',
      fallback: { title: opening.dataset.tipTitle || name, meta: opening.dataset.tipMeta || '', text: opening.dataset.tipText || '', href: opening.matches('a[href]') ? opening.getAttribute('href') : '' },
    }, false);
    return true;
  }

  function close(restore = true) {
    if (!layer) return;
    seq++;
    layer.remove();
    layer = pop = bodyEl = backBtn = null;
    stack = [];
    cur = null;
    document.documentElement.classList.remove('concept-open');
    const back = opener;
    opener = anchor = null;
    if (restore && back && back.isConnected) back.focus({ preventScroll: true });
  }

  function back() {
    const prev = stack.pop();
    if (!prev) return;
    seq++;
    cur = prev;
    if (prev.html) showHTML(prev.html, prev.target && prev.target.works);
    else if (prev.target) go(prev.target, false);
  }

  // A tap or click on a term opens the popup (a link to the glossary keeps working with a modifier key, in a new tab, or without a campaign).
  document.addEventListener('click', e => {
    const t = e.target.closest && e.target.closest('[data-concept]');
    if (!t || (layer && layer.contains(t)) || e.button !== 0 || e.ctrlKey || e.metaKey || e.shiftKey || e.altKey) return;
    if (!open(t)) return;
    e.preventDefault();
    e.stopPropagation();
  }, true);

  // Inside the popup: chips and linked words, Back, Close, Retry.
  document.addEventListener('click', e => {
    if (!layer || !layer.contains(e.target)) return;
    const c = e.target.closest('[data-concept-go]');
    if (c) { e.preventDefault(); const cid = campaignId(); if (cid) go({ url: byId(cid, c.dataset.conceptGo), byId: true }, true); return; }
    if (e.target.closest('[data-concept-back]')) { back(); return; }
    if (e.target.closest('[data-concept-close]')) { close(); return; }
    if (e.target.closest('[data-concept-retry]') && cur) go(cur.target, false);
  });

  document.addEventListener('keydown', e => {
    if (layer) {
      if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); close(); return; }
      if (e.key === 'Tab') {
        const items = [...pop.querySelectorAll(FOCUSABLE)].filter(i => i.getClientRects().length > 0);
        if (!items.length) { e.preventDefault(); return; }
        const first = items[0], last = items[items.length - 1];
        if (!pop.contains(document.activeElement)) { e.preventDefault(); first.focus(); }
        else if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
        else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
      }
      return;
    }
    // Enter or Space on a focusable term that is not a link or a button (a chip is a list item with a tabindex).
    if ((e.key === 'Enter' || e.key === ' ') && !e.ctrlKey && !e.metaKey && !e.altKey) {
      const t = e.target;
      if (t && t.matches && t.matches('[data-concept]:not(a):not(button)') && open(t)) { e.preventDefault(); e.stopPropagation(); }
    }
  }, true);

  // Focus never leaves the sheet while it is open.
  document.addEventListener('focusin', e => {
    if (layer && !pop.contains(e.target)) { const h = pop.querySelector('#concept-title') || pop.querySelector('button'); if (h) h.focus({ preventScroll: true }); }
  });

  window.addEventListener('resize', place);
  // A page change (htmx navigation, back/forward) closes the sheet; the page it came from is gone.
  document.addEventListener('htmx:pushedIntoHistory', () => close(false));
  window.addEventListener('popstate', () => close(false));
})();
