// Windows (role="dialog" aria-modal="true", shown and hidden by Alpine's x-show): when one opens, focus moves into it, Tab
// stays inside it and, when it closes, focus goes back to what opened it. Escape and the backdrop are handled by the pages.
(function () {
  const FOCUSABLE = 'a[href],button:not([disabled]),input:not([disabled]):not([type=hidden]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])';
  let opener = null, current = null;

  const shown = d => d.offsetParent !== null || d.getClientRects().length > 0;
  const focusables = d => [...d.querySelectorAll(FOCUSABLE)].filter(e => e.getClientRects().length > 0);

  function sync() {
    const open = [...document.querySelectorAll('[role="dialog"][aria-modal="true"]')].find(shown) || null;
    if (open === current) return;
    if (open) {
      opener = document.activeElement;
      current = open;
      const first = focusables(open)[0];
      (first || open).focus({ preventScroll: true });
    } else {
      const back = opener;
      current = null; opener = null;
      if (back && back.isConnected && back.focus) back.focus({ preventScroll: true });
    }
  }

  document.addEventListener('keydown', e => {
    if (e.key !== 'Tab' || !current) return;
    const items = focusables(current);
    if (!items.length) { e.preventDefault(); return; }
    const first = items[0], last = items[items.length - 1];
    if (!current.contains(document.activeElement)) { e.preventDefault(); first.focus(); }
    else if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
    else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
  });

  let queued = false;
  const queue = () => { if (queued) return; queued = true; requestAnimationFrame(() => { queued = false; sync(); }); };
  document.addEventListener('DOMContentLoaded', () => {
    new MutationObserver(queue).observe(document.body, { attributes: true, attributeFilter: ['style'], subtree: true, childList: true });
  });
})();

// A button with data-open-modal asks the page's modal to open (the company page listens for "open-modal" on the window). This
// replaces an inline onclick, which the Content-Security-Policy would block.
document.addEventListener('click', e => {
  if (e.target.closest('[data-open-modal]')) window.dispatchEvent(new Event('open-modal'));
});
