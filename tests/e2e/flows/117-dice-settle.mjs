// A live 3D throw (the browser decides) ends when the dice rest: a die asleep and leaning on the other die used to be waited for until the
// 12 s cap of tools/dice3d/src/index.js (about 1 throw in 10 with two dice, 1 in 20 with one, measured on the style guide's tray). The
// tray now kicks it flat (tools/dice3d/src/settle.js), so no throw takes long: the median is about 2 s, the worst of those 20 stays far from the cap.
// Run on the style guide's tray with values decided by the browser; the server-valued playback is not timed (it has its own 9 s record cap).
import { assert } from '../lib/util.mjs';

const THROWS = [[2, 14], [1, 6]]; // [dice, throws]: 20 throws, about 50 s
const LIMIT_S = 9; // the cap is 12 s; a bad throw is slower than this by 3 s or more, a good one is faster by 5

export default async function (h) {
  const s = await h.session('sauron@example.com', { height: 1000 });

  await h.step('dice settle: 20 live throws of one and two dice all stop well before the 12 s cap, on dice showing 1-6', async () => {
    await s.goto('/styleguide');
    const times = [];
    for (const [dice, n] of THROWS) {
      for (let i = 0; i < n; i++) {
        const r = await s.page.evaluate(async (dice) => {
          const d = Alpine.$data(document.querySelector('[x-data="diceDemo"]'));
          Object.assign(d, { count: dice, sides: 6, serverValues: false, lanes: false, canShakeDemo: true });
          const t0 = performance.now();
          d.go();
          await new Promise((res) => { const tick = () => (!d.stopped ? res() : requestAnimationFrame(tick)); tick(); }); // the previous stop is still shown
          await new Promise((res) => { const tick = () => (d.stopped ? res() : requestAnimationFrame(tick)); requestAnimationFrame(tick); });
          return { s: (performance.now() - t0) / 1000, dice: [...d.dice] };
        }, dice);
        assert.equal(r.dice.length, dice, `${dice} dice reported: ${JSON.stringify(r.dice)}`);
        assert.ok(r.dice.every((v) => Number.isInteger(v) && v >= 1 && v <= 6), `faces 1-6: ${JSON.stringify(r.dice)}`);
        times.push(r.s);
      }
    }
    const sorted = [...times].sort((a, b) => a - b);
    assert.ok(sorted.at(-1) < LIMIT_S, `the slowest of ${times.length} throws took ${sorted.at(-1).toFixed(1)} s (limit ${LIMIT_S} s): ${times.map((t) => t.toFixed(1)).join(' ')}`);
  });
}
