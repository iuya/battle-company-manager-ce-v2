import puppeteer from 'puppeteer-core';
import { writeFileSync, mkdirSync, rmSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { findChrome, outDir } from './env.mjs';

export const PASSWORD = 'demo-password';
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
export { sleep };

// A signed-in browser context (own cookies) with one page that records console errors, page errors, failed requests
// and 4xx/5xx answers; `issues()` returns and clears them.
export class Session {
  constructor(h, email, ctx, page) { this.h = h; this.email = email; this.ctx = ctx; this.page = page; this.found = []; this.allow = []; this.dialogs = []; this.reqs = []; this.events = []; this.reqCount = 0; }
  record(kind, text) {
    if (this.allow.some((re) => re.test(text))) return;
    this.found.push(`${this.email}: ${kind}: ${text}`);
  }
  issues() { const f = this.found; this.found = []; return f; }
  url(path) { return this.h.base + path; }
  // 'load', not networkidle: live pages hold an event stream open for ever.
  async goto(path, opts = {}) { await this.page.goto(this.url(path), { waitUntil: 'load', ...opts }); await this.settle(100); }
  async text() { return this.page.evaluate(() => (document.querySelector('main') || document.body).innerText); }
  async has(text) { return this.page.evaluate((t) => document.body.innerText.includes(t), text); }
  async waitText(text, timeout = 8000) {
    await this.page.waitForFunction((t) => document.body.innerText.includes(t), { timeout }, text)
      .catch(() => { throw new Error(`text not seen within ${timeout} ms: "${text}"`); });
  }
  async waitGone(text, timeout = 8000) {
    await this.page.waitForFunction((t) => !document.body.innerText.includes(t), { timeout }, text)
      .catch(() => { throw new Error(`text still present after ${timeout} ms: "${text}"`); });
  }
  // The window of a live action of a unit, opened the way the card's Wounded milestone opens its own (the same sheet route with another
  // kind), through htmx in the page. Only wound and item have one. unitID: a field unit of the page's company; by default the first one.
  async liveSheet(kind, unitID) {
    await this.page.evaluate((kind, id) => {
      const btn = [...document.querySelectorAll('main .unit-slot button[hx-get$="/sheet/wound"]')].find((b) => id == null || b.closest('.unit-slot').dataset.unitId === String(id));
      if (!btn) throw new Error(`no field unit ${id ?? ''} with a Wounded milestone`);
      htmx.ajax('GET', btn.getAttribute('hx-get').replace(/\/wound$/, `/${kind}`), { target: '#modal-body', swap: 'innerHTML' });
      window.dispatchEvent(new CustomEvent('open-modal'));
    }, kind, unitID ?? null);
  }
  // A unit retreats. The card's Failed Courage button exists only while the company is Broken and there is no window for "left the board" any
  // more: send the action as that button does, to the actions route the milestones post to. why: courage (default) or left. unitID: by default
  // the first field unit.
  async liveRetreat(unitID, why = 'courage') {
    await this.page.evaluate((id, why) => {
      const btn = [...document.querySelectorAll('main .unit-slot button[hx-get$="/sheet/wound"]')].find((b) => id == null || b.closest('.unit-slot').dataset.unitId === String(id));
      if (!btn) throw new Error(`no field unit ${id ?? ''} with a Wounded milestone`);
      const url = btn.getAttribute('hx-get').replace(/\/units\/\d+\/sheet\/wound$/, '/actions');
      htmx.ajax('POST', url, { target: '#live-zones', swap: 'outerHTML', values: { kind: 'retreat', unit: btn.closest('.unit-slot').dataset.unitId, why } });
    }, unitID ?? null, why);
  }
  // The mount switch has no button on a live card for now (a GitHub issue brings it back): send its action as that button did, to the
  // actions route the milestones post to. unitID: by default the first field unit.
  async liveMount(mounted, unitID) {
    await this.page.evaluate((mounted, id) => {
      const btn = [...document.querySelectorAll('main .unit-slot button[aria-label^="Got the objective ("]')].find((b) => id == null || b.closest('.unit-slot').dataset.unitId === String(id));
      if (!btn) throw new Error(`no field unit ${id ?? ''} with an Objective milestone`);
      htmx.ajax('POST', btn.getAttribute('hx-post'), { target: '#live-zones', swap: 'outerHTML', values: { kind: 'mount', unit: btn.closest('.unit-slot').dataset.unitId, mounted: mounted ? '1' : '0' } });
    }, mounted, unitID ?? null);
  }
  // Clicks the first visible element matching css whose text contains `text` (exact when text starts with =).
  // A swap can replace the element between finding it and pressing it ("Node is detached"): find it again, as a person would.
  async click(css, text, opts = {}) {
    for (let i = 0; ; i++) {
      try { return await this.clickOnce(css, text, opts); }
      catch (e) { if (i >= 2 || !/detached/i.test(String(e.message))) throw e; await sleep(150); }
    }
  }
  async clickOnce(css, text, { nav = false, timeout = 8000 } = {}) {
    const handle = await this.page.waitForFunction((css, text) => {
      const exact = text && text.startsWith('=');
      const want = exact ? text.slice(1) : text;
      for (const e of document.querySelectorAll(css)) {
        const r = e.getBoundingClientRect();
        if (!r.width || !r.height || getComputedStyle(e).visibility === 'hidden' || e.disabled) continue;
        const t = (e.innerText || e.value || e.getAttribute('aria-label') || '').replace(/\s+/g, ' ').trim();
        if (!want || (exact ? t === want : t.includes(want))) return e;
      }
      return null;
    }, { timeout }, css, text).catch(() => { throw new Error(`nothing to click: ${css} "${text || ''}"`); });
    const el = handle.asElement();
    await el.evaluate((e) => e.scrollIntoView({ block: 'center' }));
    // A sheet slides in and a swapped fragment shifts the layout: on a loaded machine the animation lags and a click at the
    // measured centre lands on whatever is there by then (seen: "Wounded a unit" clicked, no request sent). Click once the box has
    // held still for three frames, and only if the element is still attached.
    await el.evaluate((e) => new Promise((res) => {
      let last = '', same = 0, n = 0;
      const tick = () => {
        const r = e.getBoundingClientRect(), k = [r.x, r.y, r.width, r.height].map(Math.round).join();
        same = k === last ? same + 1 : 0; last = k;
        if (same >= 3 || ++n > 90) res(); else requestAnimationFrame(tick);
      };
      requestAnimationFrame(tick);
    }));
    if (nav) await Promise.all([this.page.waitForNavigation({ waitUntil: 'load' }), el.click()]);
    else {
      // A button that carries an htmx request must send one. On a loaded machine a click was seen to do nothing (the sheet menu stayed,
      // no request left the page; never reproduced on a quiet one): click again once, say so in the output, and fail as before if
      // that does nothing either. Counted in the results line so a rising rate shows.
      const hx = await el.evaluate((e) => !!(e.getAttribute('hx-get') || e.getAttribute('hx-post')) && !e.hasAttribute('hx-confirm')).catch(() => false);
      const before = this.reqCount;
      await el.click();
      if (hx) {
        const sent = async () => { for (let i = 0; i < 15 && this.reqCount === before; i++) await sleep(100); return this.reqCount !== before; };
        if (!(await sent()) && await el.evaluate((e) => e.isConnected).catch(() => false)) {
          this.h.retries = (this.h.retries || 0) + 1;
          console.log(`        (click on "${text || css}" sent no request, clicking again; ${this.email}, url ${this.page.url()})`);
          await el.click().catch(() => {});
        }
      }
    }
    await this.settle();
  }
  // Waits for htmx requests to finish (htmx adds htmx-request while one runs) and for the network to go quiet.
  async settle(ms = 150) {
    await sleep(ms);
    await this.page.waitForFunction(() => !document.querySelector('.htmx-request') && !document.querySelector('.htmx-swapping'), { timeout: 8000 }).catch(() => {});
    await sleep(ms);
  }
  async fill(css, value) {
    await this.page.waitForSelector(css, { visible: true, timeout: 8000 });
    await this.page.$eval(css, (e) => { e.focus(); if ('select' in e && e.select) e.select(); });
    await this.page.keyboard.press('Backspace');
    await this.page.type(css, String(value));
  }
  async select(css, value) {
    await this.page.waitForSelector(css, { visible: true, timeout: 8000 });
    await this.page.select(css, String(value));
    await this.settle();
  }
  async noHScroll() {
    return this.page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1);
  }
  async setTheme(mode) {
    // The theme switch persists in the account or a cookie; the data attribute is what the CSS reads.
    await this.page.evaluate((m) => {
      const d = m === 'dark' || (m === 'system' && matchMedia('(prefers-color-scheme: dark)').matches);
      document.documentElement.setAttribute('data-theme', d ? 'dark' : 'light');
      document.documentElement.classList.toggle('dark', d);
    }, mode);
  }
  async close() { await this.ctx.close().catch(() => {}); }
}

export class Harness {
  constructor(base) { this.base = base; this.sessions = []; this.results = []; }
  async launch() {
    const options = {
      executablePath: findChrome(), headless: true, protocolTimeout: 30000,
      // SwiftShader gives headless Chrome a software WebGL (the 3D dice, flow 86); where the runner has a GPU path it is not used
      args: ['--no-sandbox', '--disable-dev-shm-usage', '--hide-scrollbars', '--use-angle=swiftshader', '--enable-unsafe-swiftshader', '--ignore-gpu-blocklist'],
    };
    // One launch was seen to time out at 30 s on a starved runner; a second one starts a fresh Chrome and almost always succeeds.
    try { this.browser = await puppeteer.launch(options); }
    catch (e) {
      console.log(`[e2e] Chrome did not start (${String(e.message).split('\n')[0]}), trying once more`);
      this.browser = await puppeteer.launch(options);
    }
    await this.warm();
  }

  // The first requests of a fresh server pay for the things it builds on first use (the login page's template set, the compressed copies of
  // the static files, the font and script files read from disk); a timed step or a 10 s navigation must not be the one that pays. Requests /login
  // and every static file the app serves (the stylesheet, the scripts including the lazily loaded dice3d.js, the fonts) once, with the
  // Accept-Encoding of a browser. Nothing here fails the run: a server that cannot answer will fail the first step with a better message.
  async warm() {
    const t0 = Date.now();
    const get = async (path) => {
      const r = await fetch(this.base + path, { headers: { 'Accept-Encoding': 'gzip, br' } }).catch(() => null);
      return r && r.ok ? await r.text().catch(() => '') : '';
    };
    const login = await get('/login');
    const css = await get('/static/app.css');
    const paths = new Set(['/static/js/dice3d.js']);
    for (const m of (login + css).matchAll(/(?:href|src)="(\/static\/[^"?#]+)|url\("?(\/static\/[^")?#]+)/g)) paths.add(m[1] || m[2]);
    await Promise.all([...paths].map(get));
    console.log(`[e2e] warm start: /login and ${paths.size + 1} static files in ${Date.now() - t0} ms`);
  }
  async session(email, { width = 1280, height = 900, password = PASSWORD, login = true } = {}) {
    const ctx = await this.browser.createBrowserContext();
    const page = await ctx.newPage();
    await page.setViewport({ width, height });
    page.setDefaultTimeout(10000);
    page.setDefaultNavigationTimeout(30000); // the first /login of a session on a cold server or a starved runner was seen to take over 10 s
    const s = new Session(this, email, ctx, page);
    // Headless Chrome on Linux has no pointing device, so (hover: hover) is false there (true on macOS) and the app, which shows its
    // hover tooltips only for hover-capable devices, would show none: report a mouse, as a desktop browser does.
    await page.evaluateOnNewDocument(() => {
      const real = window.matchMedia.bind(window);
      window.matchMedia = (q) => {
        const m = real(q);
        if (/^\(\s*(any-)?hover:\s*hover\s*\)$/.test(q) && !m.matches) Object.defineProperty(m, 'matches', { value: true });
        return m;
      };
    });
    // Every Content-Security-Policy violation the browser raises (the report-only policy too) fails the step that caused it.
    await page.evaluateOnNewDocument(() => {
      document.addEventListener('securitypolicyviolation', (e) => {
        console.error(`CSP violation: ${e.disposition} ${e.violatedDirective} blocked=${e.blockedURI} source=${e.sourceFile}:${e.lineNumber} sample=${(e.sample || '').slice(0, 80)}`);
      });
    });
    // Page life, for the failure dump: a renderer crash ('error' on the page: "Page crashed!"), the page closing, the main frame's
    // navigations and a lost WebGL context (the 3D dice stop for ever when it is lost) with the time since the session began. Not failures by
    // themselves; they say why a step that waited for the page timed out.
    const ev0 = Date.now();
    const note = (text) => { s.events.push(`${String(Date.now() - ev0).padStart(7)} ms ${text}`); if (s.events.length > 80) s.events.shift(); };
    page.on('error', (e) => note(`RENDERER CRASH: ${e.message || e}`));
    page.on('close', () => note('page closed'));
    page.on('framenavigated', (f) => { if (f === page.mainFrame()) note(`navigated to ${f.url().replace(this.base, '')}`); });
    page.on('workercreated', (w) => note(`worker ${w.url().replace(this.base, '')}`));
    await page.evaluateOnNewDocument(() => {
      // 'webglcontextlost' does not bubble: listen in the capture phase on the document to see every canvas
      document.addEventListener('webglcontextlost', (e) => console.debug(`[e2e-diag] webglcontextlost canvas=${e.target && e.target.className}`), true);
      document.addEventListener('webglcontextrestored', (e) => console.debug(`[e2e-diag] webglcontextrestored canvas=${e.target && e.target.className}`), true);
    });
    page.on('console', (m) => { if (m.type() === 'debug' && m.text().startsWith('[e2e-diag] ')) note(m.text().slice(11)); });
    page.on('console', (m) => { if (m.type() === 'error') s.record('console', m.text() + ' ' + (m.location()?.url || '')); });
    // hx-confirm and confirm() open native dialogs that block the page until answered: accept them and remember the text.
    page.on('dialog', (d) => { s.dialogs.push(d.message()); d.accept().catch(() => {}); });
    page.on('pageerror', (e) => s.record('pageerror', String(e.message || e)));
    page.on('requestfailed', (r) => {
      const t = r.failure()?.errorText || '';
      if (t.includes('ERR_ABORTED')) return; // navigations and closed event streams
      s.record('requestfailed', `${r.method()} ${r.url()} ${t}`);
    });
    // The last requests of the page with the time they were sent and answered, for the failure dump (what raced with what).
    const t0 = Date.now(), stamp = () => String(Date.now() - t0).padStart(7);
    page.on('request', (r) => { s.reqCount++; if (!/\/static\//.test(r.url())) { s.reqs.push(`${stamp()} ms > ${r.method()} ${r.url().replace(this.base, '')}`); if (s.reqs.length > 60) s.reqs.shift(); } });
    page.on('response', (r) => { if (!/\/static\//.test(r.url())) { s.reqs.push(`${stamp()} ms < ${r.status()} ${r.url().replace(this.base, '')}`); if (s.reqs.length > 60) s.reqs.shift(); } });
    page.on('response', (r) => { if (r.status() >= 400) s.record('http', `${r.status()} ${r.request().method()} ${r.url()}`); });
    this.sessions.push(s);
    if (login) await this.login(s, email, password);
    return s;
  }
  async login(s, email, password = PASSWORD) {
    await s.goto('/login');
    await s.fill('input[name=email]', email);
    await s.fill('input[name=password]', password);
    await Promise.all([s.page.waitForNavigation({ waitUntil: 'load' }), s.page.click('button[type=submit]')]);
    if (new URL(s.page.url()).pathname === '/login') throw new Error(`sign-in as ${email} failed`);
  }

  // Runs one named step. Fails on a thrown error or on any recorded console error / failed request of any session
  // (unless the step allowed it); on failure saves a screenshot and the page text of every session.
  async step(name, fn, { only } = {}) {
    if (only && !name.toLowerCase().includes(only.toLowerCase())) return;
    const t0 = Date.now();
    for (const s of this.sessions) s.issues();
    let err = null;
    try {
      await Promise.race([fn(), new Promise((_, rej) => setTimeout(() => rej(new Error('step took longer than 90 s')), 90000))]);
      const issues = this.sessions.flatMap((s) => s.issues());
      if (issues.length) throw new Error('browser reported problems:\n  ' + issues.join('\n  '));
    } catch (e) { err = e; }
    const ms = Date.now() - t0;
    if (!err) { this.results.push({ name, ok: true, ms }); console.log(`  ok    ${name} (${ms} ms)`); return true; }
    this.results.push({ name, ok: false, ms, error: err.message });
    console.log(`  FAIL  ${name} (${ms} ms)\n        ${String(err.message).replace(/\n/g, '\n        ')}`);
    await this.dump(name);
    return false;
  }
  async dump(name) {
    mkdirSync(outDir, { recursive: true });
    const slug = name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '').slice(0, 60);
    let i = 0;
    for (const s of this.sessions) {
      try {
        const base = join(outDir, `${slug}.${i++}-${s.email.split('@')[0]}`);
        await s.page.screenshot({ path: base + '.png', fullPage: true }).catch(() => {}); // a crashed or closed page has none; the events below still get written
        const live = await s.page.evaluate(() => ({ dot: document.getElementById('live-dot-text')?.textContent, seq: document.getElementById('live-zones')?.dataset.seq, dialogs: [...document.querySelectorAll('[role=dialog]')].filter((d) => d.getBoundingClientRect().height).map((d) => d.innerText.slice(0, 80).replace(/\s+/g, ' ')), busy: !!document.querySelector('.htmx-request') })).catch(() => ({}));
        writeFileSync(base + '.txt', `${s.page.url()}${s.page.isClosed() ? ' (page closed)' : ''}\n\npage events (crash, close, navigations, WebGL context loss):\n${s.events.join('\n') || '(none)'}\n\nlive state: ${JSON.stringify(live)}\nlast requests (> sent, < answered):\n${s.reqs.join('\n')}\n\n${await s.text().catch((e) => `(page text unreadable: ${e.message})`)}\n`);
      } catch {}
    }
    console.log(`        screenshots and page text: tests/e2e/out/${slug}.*`);
  }
  async close() { await this.browser?.close().catch(() => {}); }
}

// Removes the previous run's screenshots and page texts (the built server binary stays).
export function clearOut() {
  mkdirSync(outDir, { recursive: true });
  for (const f of readdirSync(outDir)) if (/\.(png|txt|log)$/.test(f)) rmSync(join(outDir, f), { force: true });
}
