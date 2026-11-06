// node --test tools/dice3d  (make dice3d-test, part of make test): what a live throw does when every die is asleep.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { settle, calmBody, MAX_KICKS, CAP, QUIET } from './src/settle.js';

const flat = { dot: 1, y: 0.5 }, leaning = { dot: 0.85, y: 0.65 }, stacked = { dot: 0.99, y: 1.5 };

test('two flat dice are read at once', () => {
  assert.deepEqual(settle([flat, flat], 0, 2), { kick: [], done: true });
});

test('a die leaning on another die is kicked, not waited for', () => {
  assert.deepEqual(settle([flat, leaning], 0, 2), { kick: [1], done: false });
  assert.deepEqual(settle([leaning, flat], 1, 3), { kick: [0], done: false });
});

test('a leaning die that stays leaning is flattened and read after MAX_KICKS kicks', () => {
  assert.deepEqual(settle([flat, leaning], MAX_KICKS, 3), { kick: [], done: true });
});

test('a die on top of another is always knocked off, with the leaning one while kicks are left', () => {
  assert.deepEqual(settle([stacked, flat], MAX_KICKS + 5, 3), { kick: [0], done: false });
  assert.deepEqual(settle([stacked, leaning], 0, 3), { kick: [0, 1], done: false });
  assert.deepEqual(settle([stacked, leaning], MAX_KICKS, 3), { kick: [0], done: false });
});

test('the cap ends the throw whatever the dice do', () => {
  assert.deepEqual(settle([stacked, leaning], 0, CAP), { kick: [], done: true });
});

test('one die: flat is read, leaning is kicked at the thresholds', () => {
  assert.equal(settle([{ dot: 0.951, y: 0.5 }], 0, 1).done, true);
  assert.deepEqual(settle([{ dot: 0.95, y: 0.5 }], 0, 1), { kick: [0], done: false });
  assert.deepEqual(settle([{ dot: 1, y: 0.76 }], 0, 1), { kick: [0], done: false });
  assert.equal(settle([{ dot: 1, y: 0.75 }], 0, 1).done, true);
});

test('a die is calm when asleep, sleepy (resting on another die) or nearly still, not while it moves', () => {
  assert.equal(calmBody(2, 100), true);
  assert.equal(calmBody(1, 0.0004), true);
  assert.equal(calmBody(1, 100), true, 'sleepy stays calm: it never became asleep in the throws that waited for the cap');
  assert.equal(calmBody(0, QUIET / 2), true);
  assert.equal(calmBody(0, QUIET * 2), false);
});
