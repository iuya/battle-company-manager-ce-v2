// Double submit: while a form's request is in flight its buttons are disabled and a second submit is ignored, so a quick
// double click or tap cannot send the same action twice (a match, a purchase, a recruit). The server still refuses what is
// no longer allowed; this keeps the second request from being sent at all.
(function () {
  const buttons = form => form.querySelectorAll('button:not([type="button"]):not([type="reset"]), input[type="submit"]');
  const release = form => {
    delete form.dataset.submitting;
    buttons(form).forEach(b => { if (b.dataset.heldBySubmit) { b.disabled = false; delete b.dataset.heldBySubmit; } });
  };
  document.addEventListener('submit', e => {
    const form = e.target;
    if (!(form instanceof HTMLFormElement)) return;
    if (form.dataset.submitting) { e.preventDefault(); e.stopImmediatePropagation(); return; }
    form.dataset.submitting = '1';
    setTimeout(() => buttons(form).forEach(b => { if (!b.disabled) { b.disabled = true; b.dataset.heldBySubmit = '1'; } }), 0);
    // A plain (non htmx) submit leaves the page; if the browser stays (a download, a cancelled navigation) let go later.
    if (!form.hasAttribute('hx-post') && !form.hasAttribute('hx-put') && !form.hasAttribute('hx-boost') && form.getAttribute('hx-boost') !== 'true') {
      setTimeout(() => { if (form.isConnected) release(form); }, 8000);
    }
  }, true);
  document.addEventListener('htmx:afterRequest', e => {
    const form = e.detail && e.detail.elt && (e.detail.elt.closest ? e.detail.elt.closest('form') : null);
    if (form) release(form);
  });
})();
