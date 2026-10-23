// Content-Security-Policy: the header of the mode the server runs in, and a control that proves the harness sees a violation
// (otherwise "no violation" in the other flows would prove nothing). Run the whole suite with E2E_CSP_MODE=enforce
// (`make e2e-enforce`) to see every page, swap, stream and the 3D dice under the enforcing policy (docs/security-and-risks.md).
import { assert } from '../lib/util.mjs';

export default async function (h) {
  const s = await h.session('you@example.com');
  const mode = process.env.E2E_CSP_MODE || 'report';

  await h.step(`csp: pages carry the ${mode} policy header`, async () => {
    await s.goto('/');
    const hdr = await s.page.evaluate(async () => {
      const r = await fetch('/', { credentials: 'same-origin' });
      return { enforced: r.headers.get('content-security-policy'), report: r.headers.get('content-security-policy-report-only') };
    });
    if (mode === 'enforce') assert.ok(hdr.enforced && !hdr.report, 'enforce: Content-Security-Policy only: ' + JSON.stringify(hdr));
    else assert.ok(hdr.report && !hdr.enforced, 'report: Content-Security-Policy-Report-Only only: ' + JSON.stringify(hdr));
    const p = hdr.enforced || hdr.report;
    for (const d of ["default-src 'self'", "frame-ancestors 'none'", "object-src 'none'", 'report-uri /csp-report']) assert.ok(p.includes(d), 'policy has ' + d);
  });

  await h.step('csp: control, a foreign script and a foreign image are reported by the browser', async () => {
    await s.goto('/');
    s.allow.push(/CSP violation|Refused to/);
    try {
      const out = await s.page.evaluate(() => new Promise((res) => {
        const seen = [];
        document.addEventListener('securitypolicyviolation', (e) => seen.push(e.violatedDirective), { once: false });
        // Both URLs carry csp-control: run.mjs leaves these two reports out of its "no violation in the server log" check.
        const sc = document.createElement('script'); sc.src = 'https://example.invalid/csp-control.js'; document.body.appendChild(sc);
        const img = new Image(); img.src = 'https://example.invalid/csp-control.png'; document.body.appendChild(img);
        setTimeout(() => res({ seen }), 500);
      }));
      assert.ok(out.seen.some((d) => d.startsWith('script-src')), 'foreign script reported: ' + JSON.stringify(out));
      assert.ok(out.seen.some((d) => d.startsWith('img-src') || d.startsWith('default-src')), 'foreign image reported: ' + JSON.stringify(out));
    } finally { s.allow.pop(); s.issues(); }
  });
}
