// Alpine rejects the promise of a transition that is cancelled by the next toggle ({isFromCancelledTransition: true}); theme-init.js marks that
// one rejection handled so the browser raises no unhandled rejection (the harness fails a step on "pageerror: Object"), and only that one.
// The My Company window (x-show + x-transition) is closed and reopened in the middle of its transition many times, with no pageerror.
import { assert, campaignId, sleep } from '../lib/util.mjs';

export default async function (h) {
  const s = await h.session('galadriel@example.com');

  await h.step('cancelled transition: a window closed and reopened mid-transition raises no unhandled rejection', async () => {
    await s.goto(`/campaigns/${await campaignId(s, 'Level-up options')}/my-company`);
    await s.page.evaluate(() => {
      window.__seen = [];
      window.addEventListener('unhandledrejection', (e) => window.__seen.push({ cancelled: !!(e.reason && e.reason.isFromCancelledTransition), prevented: e.defaultPrevented, text: String((e.reason && e.reason.message) || '') }));
    });
    const toggles = await s.page.evaluate(async () => {
      const root = [...document.querySelectorAll('[x-data]')].find((e) => 'modal' in window.Alpine.$data(e));
      const d = window.Alpine.$data(root);
      let n = 0;
      for (let i = 0; i < 60; i++) { // a toggle every 20-40 ms against a transition of 150 ms or more: each one cancels the last
        d.modal = !d.modal; n++;
        await new Promise((r) => setTimeout(r, 20 + (i % 3) * 10));
      }
      d.modal = false;
      return n;
    });
    await sleep(600);
    assert.equal(toggles, 60);
    const seen = await s.page.evaluate(() => window.__seen);
    assert.ok(seen.length > 0, 'the toggling cancelled at least one transition (else the test proves nothing)');
    assert.ok(seen.every((x) => x.cancelled && x.prevented), `every cancelled-transition rejection is marked handled: ${JSON.stringify(seen)}`);
  });

  await h.step('cancelled transition: any other unhandled rejection is not swallowed', async () => {
    // A rejection raised by code run through the debugger is not reported to the page, so the handler is probed with the event itself.
    const handled = (reason) => s.page.evaluate((r) => !window.dispatchEvent(new PromiseRejectionEvent('unhandledrejection', { promise: Promise.resolve(), reason: r, cancelable: true })), reason);
    assert.equal(await handled({ isFromCancelledTransition: true }), true, 'the cancelled transition is marked handled');
    assert.equal(await handled(new Error('boom')), false, 'an error stays unhandled');
    assert.equal(await handled({ isFromCancelledTransition: false }), false, 'only a cancelled transition is swallowed');
    assert.equal(await handled(null), false, 'a rejection with no reason stays unhandled');
    assert.equal(await handled('text'), false, 'a plain reason stays unhandled');
  });
}
