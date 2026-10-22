// "Which rules file?" choice of the new-campaign form: the app's default, or a file of your own.
//
// The browser hashes the file before anything is uploaded (SHA-256 of the JSON without its insignificant whitespace,
// the same value as core.RulesetHash on the server) and asks GET /rulesets/{hash}?kind=&name=&version= (vanilla files
// are recognised by their name and version, the others by their hash). If the server already has that file
// the input is cleared, so only the hash travels with the form; otherwise the file is uploaded and the server checks it.
document.addEventListener('alpine:init', () => {
  const WS = new Set([0x20, 0x09, 0x0a, 0x0d]);
  const MAX = 16 << 20;

  // Removes whitespace outside strings, byte for byte like Go's json.Compact (quotes and backslashes never occur
  // inside multi-byte UTF-8 sequences, so working on bytes is safe).
  function compact(bytes) {
    const out = new Uint8Array(bytes.length);
    let n = 0, inString = false, escaped = false;
    for (const b of bytes) {
      if (inString) {
        out[n++] = b;
        if (escaped) escaped = false;
        else if (b === 0x5c) escaped = true;
        else if (b === 0x22) inString = false;
      } else if (b === 0x22) { inString = true; out[n++] = b; }
      else if (!WS.has(b)) out[n++] = b;
    }
    return out.subarray(0, n);
  }

  async function sha256(bytes) {
    const digest = await crypto.subtle.digest('SHA-256', bytes);
    return [...new Uint8Array(digest)].map(b => b.toString(16).padStart(2, '0')).join('');
  }

  // Same rules as core.RulesetMeta (the server's answer is the one that counts).
  function meta(kind, doc) {
    if (kind === 'vanilla') {
      return doc.appVersion == null || !doc.game ? null : { name: doc.game, version: String(doc.appVersion) };
    }
    const marker = kind === 'glossary' ? doc.terms : doc.extends;
    return marker == null || !doc.name || doc.version == null ? null : { name: doc.name, version: String(doc.version) };
  }

  Alpine.data('rulesetPicker', () => ({
    kind: '',
    label: '',
    mode: 'default', // default | file
    state: 'idle',   // idle | busy | ready | error (the user never sees whether the server already had the file)
    hash: '',
    info: null,
    message: '',
    get short() { return this.hash.slice(0, 8); },
    init() { this.kind = this.$el.dataset.kind; this.label = this.$el.dataset.label; },
    reset() { this.state = 'idle'; this.hash = ''; this.info = null; this.message = ''; },
    fail(message) { this.reset(); this.state = 'error'; this.message = message; this.$refs.file.value = ''; },
    async pick() {
      const file = this.$refs.file.files[0];
      this.reset();
      if (!file) return;
      if (!(window.crypto && crypto.subtle)) return this.fail('This browser cannot read the file here (it needs a secure connection).');
      if (file.size > MAX) return this.fail('That file is too large.');
      this.state = 'busy';
      try {
        const bytes = new Uint8Array(await file.arrayBuffer());
        let doc;
        try { doc = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes)); }
        catch { return this.fail('That file is not valid JSON.'); }
        const local = doc && typeof doc === 'object' ? meta(this.kind, doc) : null;
        if (!local) return this.fail('That does not look like a ' + this.label + ' file.');
        let hash = await sha256(compact(bytes));
        const res = await fetch('/rulesets/' + hash + '?kind=' + encodeURIComponent(this.kind), { headers: { Accept: 'application/json' } });
        if (res.ok) {
          const known = await res.json();
          if (known.kind !== this.kind) return this.fail('That file is already stored as another kind of rules file.');
          this.$refs.file.value = ''; // the server has it: nothing is uploaded
          this.info = known;
          console.debug('ruleset ' + known.hash + ' is already stored: not uploading it');
        } else {
          this.info = local;
          console.debug('ruleset ' + hash + ' is new: uploading it');
        }
        this.state = 'ready';
        this.hash = hash;
      } catch (e) {
        this.fail('Could not read that file.');
      }
    },
  }));
});
