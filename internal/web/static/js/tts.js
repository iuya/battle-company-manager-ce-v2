// The Tabletop Simulator army list dialog (templates/partials/tts.gohtml): remembers the two switches in localStorage and adds them to
// the requests that fetch the list, and copies the list to the clipboard. No inline handlers: the page's CSP forbids them.
(function () {
  const KEY = 'bcm-tts';
  const DEFAULTS = { foot: '1', mounts: '1', rules: 'none' };
  const read = () => {
    try { return Object.assign({}, DEFAULTS, JSON.parse(localStorage.getItem(KEY) || '{}')); } catch (e) { return Object.assign({}, DEFAULTS); }
  };
  const write = (v) => { try { localStorage.setItem(KEY, JSON.stringify(v)); } catch (e) { /* private window: the choice is not kept */ } };

  // The dialog opens with the remembered switches; a change of a switch is remembered and asked for again. Two forms feed the same
  // request: the list's switches (foot, mounts) and the rule texts of the names (rules); what a form lacks comes from the memory.
  document.addEventListener('htmx:configRequest', (e) => {
    const el = e.detail.elt;
    if (!el || !el.hasAttribute) return;
    const v = read();
    if (el.hasAttribute('data-tts-form')) {
      for (const name of ['foot', 'mounts']) { const c = el.elements[name]; if (c) v[name] = c.checked ? '1' : '0'; }
      const r = el.elements['rules'];
      if (r && r.value) v.rules = r.value;
      write(v);
    } else if (!el.hasAttribute('data-tts-open')) {
      return;
    }
    e.detail.parameters.foot = v.foot;
    e.detail.parameters.mounts = v.mounts;
    e.detail.parameters.rules = v.rules;
  });

  // The switches are drawn from the request, which was made with the remembered values; the forms are not swapped, only the results.
  document.addEventListener('click', async (e) => {
    const btn = e.target.closest && e.target.closest('[data-tts-copy]');
    if (!btn) return;
    const area = document.getElementById(btn.dataset.ttsCopy);
    const status = document.getElementById(btn.dataset.ttsStatus);
    if (!area || !status) return;
    const say = (t) => { status.textContent = t; };
    let ok = false;
    try {
      if (navigator.clipboard && window.isSecureContext) { await navigator.clipboard.writeText(area.value); ok = true; }
    } catch (err) { ok = false; }
    if (!ok) { // no clipboard permission or an insecure page: select the text so Ctrl+C works
      area.focus();
      area.select();
      try { ok = document.execCommand('copy'); } catch (err) { ok = false; }
    }
    clearTimeout(btn._ttsTimer);
    if (ok) {
      const n = btn.dataset.ttsModels;
      const what = btn.dataset.ttsWhat;
      say(n ? 'Copied: ' + n + ' models for the mod.' : what ? 'Copied ' + what + '.' : 'Copied.');
      btn._ttsTimer = setTimeout(() => { if (status.isConnected) say(''); }, 4000);
    } else {
      say('Could not copy by itself: the text is selected, press Ctrl+C (Cmd+C on a Mac).');
    }
  });
})();
