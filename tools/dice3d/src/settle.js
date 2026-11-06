// What a live throw does once every die is asleep (pure, so node tests it without a browser or WebGL; index.js calls it from tick()).
//
// A die rests flat when a face points straight up (the up-dot of that face's normal is above FLAT) and lies on the table (its centre
// is below STACKED). A die that is calm and not flat either lies on another die (STACKED) or leans on one (or on the wall): left to
// itself it would never move again, and the tray used to wait for its 12 s cap (also because such a die stays SLEEPY and never SLEEPING,
// and the tray waited for every die to sleep). Such a die is kicked so that it falls flat again,
// at most MAX_KICKS times for a leaning die; after that it is flattened to the nearest face and the number is read from it.
export const FLAT = 0.95;       // up-dot of the top face above which a die lies flat
export const STACKED = 0.75;    // a die whose centre is higher than this lies on another die (a flat die's centre is at 0.5)
export const CAP = 12;          // seconds after which a throw ends whatever the dice do
export const QUIET = 0.5;       // squared speed (linear plus angular) under which a die that is awake counts as at rest
export const MAX_KICKS = 2;     // how many times a leaning (not stacked) die is kicked before it is flattened where it is

// A die is calm when cannon-es put it to sleep (SLEEPING, 2) or is about to (SLEEPY, 1, which a die resting on another one can stay in
// for ever without becoming SLEEPING, so waiting for "all asleep" ran into the cap), or when it merely moves very little.
export const calmBody = (sleepState, speed2) => sleepState >= 1 || speed2 < QUIET;

// dice: [{ dot, y }] (the up-dot of the top face and the height of the centre of each die), kicks: how many times leaning dice were
// kicked in this throw, elapsed: seconds of the throw. Returns { kick: indexes of the dice to kick, done: read the faces now }.
export function settle(dice, kicks, elapsed) {
  const stacked = [], leaning = [];
  dice.forEach((d, i) => {
    if (d.y > STACKED) stacked.push(i);
    else if (d.dot <= FLAT) leaning.push(i);
  });
  if (elapsed >= CAP) return { kick: [], done: true };
  if (stacked.length) return { kick: stacked.concat(kicks < MAX_KICKS ? leaning : []), done: false }; // a die on another one is always knocked off
  if (leaning.length && kicks < MAX_KICKS) return { kick: leaning, done: false };
  return { kick: [], done: true }; // all flat, or leaning for good: flatten and read
}
