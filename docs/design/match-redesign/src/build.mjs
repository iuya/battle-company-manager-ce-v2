// Generates the static mockup pages (designs A, B, C of issue #24) into ./pages.
// Usage: node build.mjs   (needs ../static/app.css next to ./pages, see README.md)
import fs from 'fs';

const CSS = `
*{box-sizing:border-box}
body{margin:0;background:var(--page);color:var(--fg);font:14px/1.4 system-ui,-apple-system,"Segoe UI",sans-serif;-webkit-font-smoothing:antialiased}
h1,h2,h3,.hd{font-family:"Cormorant Garamond",Georgia,serif;font-weight:700;margin:0}
h1{font-size:34px;line-height:1}
h2{font-size:20px}
a,.lk{color:var(--accent-text);text-decoration:underline}
.pg{padding:14px 16px 28px;max-width:100%}
.mu{color:var(--muted)}
.sm-t{font-size:12px}
.row{display:flex;gap:8px;align-items:center;flex-wrap:wrap}
.sp{flex:1}
.c{border:1px solid var(--line);background:var(--surface);border-radius:12px;padding:12px;margin:10px 0;box-shadow:0 1px 2px rgb(0 0 0/.06)}
.c>h2{margin-bottom:6px}
.b{display:inline-flex;align-items:center;justify-content:center;gap:6px;min-height:44px;padding:0 14px;border:1px solid var(--line);border-radius:8px;background:var(--surface);color:var(--fg);font:600 14px system-ui,sans-serif;white-space:nowrap}
.b.sm{min-height:36px;padding:0 12px;font-size:13px}
.b.xs{min-height:32px;padding:0 10px;font-size:12px}
.b.p{background:var(--accent);color:var(--accent-fg);border-color:var(--accent)}
.b.p[disabled]{opacity:.5}
.b.g{border-color:transparent;background:transparent;color:var(--accent-text)}
.b.w{width:100%}
.ii{display:inline-flex;align-items:center;justify-content:center;width:18px;height:18px;border-radius:50%;border:1px solid var(--muted);color:var(--muted);font:italic 700 11px Georgia,serif;flex:none}
.bd{display:inline-flex;align-items:center;border-radius:99px;background:var(--accent-soft);color:var(--accent-text);padding:1px 9px;font-size:12px;font-weight:600}
.bd.wn{background:color-mix(in oklab,var(--warn) 20%,transparent);color:var(--fg)}
.live-dot{display:inline-block;width:8px;height:8px;border-radius:50%;background:var(--success);margin-right:4px}
.acc{display:flex;align-items:center;gap:8px;min-height:48px;padding:6px 12px;border:1px solid var(--line);border-radius:10px;background:var(--surface);margin:8px 0;font-weight:600}
.acc .ch{color:var(--muted);width:12px}
.acc .su{margin-left:auto;color:var(--muted);font-weight:400;font-size:13px}
.acc.open{border-bottom-left-radius:0;border-bottom-right-radius:0;margin-bottom:0}
.accb{border:1px solid var(--line);border-top:0;border-radius:0 0 10px 10px;background:var(--surface);padding:10px 12px;margin:0 0 8px}
.chip{display:inline-flex;align-items:center;gap:4px;min-height:44px;padding:0 12px;border:1px solid var(--line);border-radius:99px;background:var(--surface);font-size:13px;font-weight:500;color:var(--muted)}
.chip.on{border-color:var(--accent);background:var(--accent-soft);color:var(--fg)}
.chip.sq{border-radius:8px}
.chip.s{min-height:36px;font-size:12px}
.chips{display:flex;flex-wrap:wrap;gap:6px}
.rad{display:inline-flex;align-items:center;gap:6px;min-height:44px;padding-right:12px}
.rad:before{content:"";width:16px;height:16px;border:2px solid var(--muted);border-radius:50%}
.rad.on:before{border-color:var(--accent);background:radial-gradient(var(--accent) 45%,transparent 50%)}
.cb{display:inline-flex;align-items:center;gap:8px;min-height:44px}
.cb:before{content:"";width:18px;height:18px;border:2px solid var(--muted);border-radius:4px;flex:none}
.cb.on:before{border-color:var(--accent);background:var(--accent);box-shadow:inset 0 0 0 2px var(--surface)}
.inp{min-height:44px;width:76px;border:1px solid var(--line);background:var(--surface);border-radius:8px;display:inline-flex;align-items:center;padding:0 10px;color:var(--muted)}
.pt{width:34px;height:34px;border-radius:8px;background:var(--raised);display:inline-flex;align-items:center;justify-content:center;font-size:18px;flex:none}
.role{color:var(--gold);margin:0 2px}
.role.s{color:var(--accent-text)}
.stats{display:inline-flex;border:1px solid var(--line);border-radius:8px;overflow:hidden;font-size:11px}
.stats span{padding:2px 6px;text-align:center;border-left:1px solid var(--line);line-height:1.2}
.stats span:first-child{border-left:0}
.stats b{display:block;font-size:13px}
.trk{border:1px solid var(--line);border-radius:10px;padding:6px 8px;background:var(--surface)}
.trk .lb{font-size:10px;letter-spacing:.06em;text-transform:uppercase;color:var(--muted)}
.sr{display:flex;align-items:center;gap:8px;justify-content:space-between}
.sr b{font-size:20px}.sr span{white-space:nowrap}.trk .sr{gap:4px}.trk .sb{width:40px}
.sr i{font-style:normal;color:var(--muted)}
.sb{width:44px;height:44px;border:1px solid var(--line);border-radius:8px;background:var(--surface);display:inline-flex;align-items:center;justify-content:center;font-size:20px;color:var(--fg);flex:none}
.sb.dim{color:var(--muted);opacity:.55}
.sb.t{width:40px;height:40px}
.cnt{display:grid;grid-template-columns:repeat(4,1fr);gap:6px;margin:8px 0 6px}
.cn{border:1px solid var(--line);border-radius:10px;padding:4px 2px 2px;background:var(--surface);display:flex;flex-direction:column;align-items:center}
.cn .lb{font-size:10px;letter-spacing:.05em;text-transform:uppercase;color:var(--muted);line-height:1.2}
.cn .v{line-height:1.1;white-space:nowrap}.cn .v b{font-size:19px}.cn .v i{font-style:normal;color:var(--muted);font-size:12px}
.ar{position:relative;display:inline-flex;align-items:center;justify-content:center;width:26px;height:16px;border-radius:5px;background:var(--raised);color:var(--fg);flex:none}
.ar svg{width:10px;height:6px}
.ar.dim{color:var(--muted);opacity:.5}
.ar:after{content:"";position:absolute;left:50%;width:44px;height:44px;margin-left:-22px}
.ar.up:after{bottom:0}.ar.dn:after{top:0}
.cn.show .ar:after{outline:1.5px dashed var(--accent);outline-offset:-1px;border-radius:6px}
.ugrid{align-items:start;display:grid;grid-template-columns:1fr;gap:0 16px}
.card-u.live{margin:8px 0}
.hbtn{position:relative;margin-left:auto;flex:none}
.pop{position:absolute;z-index:6;right:0;top:calc(100% + 6px);width:190px;background:var(--surface);border:1px solid var(--line);border-radius:10px;padding:4px;box-shadow:0 8px 24px rgb(0 0 0/.22)}
.pop:before{content:"";position:absolute;right:22px;top:-6px;width:10px;height:10px;background:var(--surface);border-left:1px solid var(--line);border-top:1px solid var(--line);transform:rotate(45deg)}
.pop div{display:flex;align-items:center;min-height:34px;padding:0 10px;border-radius:7px;font-weight:600;font-size:13px}
.pop div:first-child{background:var(--accent-soft)}
.sheet.cmp .tiles .b{min-height:44px;font-size:14px}
.g2{display:grid;grid-template-columns:1fr 1fr;gap:8px}
.gear{font-size:12px;color:var(--fg)}
.gear span{display:inline-flex;align-items:center;gap:3px;border:1px dashed var(--line);border-radius:99px;padding:2px 8px;margin:0 4px 4px 0;background:var(--surface)}
.path{border:1px solid var(--accent);background:var(--accent-soft);border-radius:8px;padding:6px 10px;font-weight:700;font-size:12px;letter-spacing:.05em;text-transform:uppercase;color:var(--accent-text)}
.kw{display:inline-block;background:var(--raised);border-radius:6px;padding:2px 8px;font-size:12px;margin:0 4px 4px 0}
.xp{display:flex;align-items:center;gap:8px;font-size:12px;border-top:1px solid var(--line);padding-top:6px;margin-top:6px}
.bar{height:8px;border-radius:99px;background:var(--raised);overflow:hidden;flex:1;min-width:40px}
.bar i{display:block;height:100%;background:var(--accent)}
.card-u{border:1px solid var(--line);border-radius:12px;background:var(--surface);padding:10px 12px;margin:10px 0;box-shadow:0 1px 2px rgb(0 0 0/.06)}
.card-u.lead{border:2px solid var(--gold)}
.ubanner{border:1px solid var(--accent);background:var(--accent-soft);border-radius:12px;padding:10px 12px}
.tile{border:1px solid var(--line);border-radius:12px;background:var(--surface);padding:8px 12px}
.tile .lb{color:var(--muted);font-size:12px}
.tile b{font-size:22px;display:block}
.dock{position:fixed;left:0;right:0;bottom:0;z-index:5;background:var(--surface);border-top:2px solid var(--accent);padding:8px 12px calc(8px + env(safe-area-inset-bottom));display:flex;gap:8px;box-shadow:0 -4px 12px rgb(0 0 0/.12)}
.dock .b{flex:1}
.dock .b.n{flex:none}
.pad-dock{padding-bottom:84px}
.scrim{position:fixed;inset:0;background:rgb(0 0 0/.5);z-index:10}
.modal{position:fixed;z-index:11;left:12px;right:12px;background:var(--surface);border:1px solid var(--line);border-radius:14px;padding:14px;box-shadow:0 10px 30px rgb(0 0 0/.3);max-height:calc(100vh - 24px);overflow:hidden}
.modal.top{top:12px}
.sheet{position:fixed;z-index:11;left:0;right:0;bottom:0;background:var(--surface);border-radius:18px 18px 0 0;padding:8px 14px 18px;box-shadow:0 -10px 30px rgb(0 0 0/.3);border-top:1px solid var(--line)}
.handle{width:44px;height:5px;border-radius:9px;background:var(--line);margin:0 auto 8px}
.lrow{display:flex;align-items:center;gap:8px;border:1px solid var(--line);border-radius:10px;background:var(--surface);padding:0 14px;margin:8px 0;font-weight:600}
.tiles{display:grid;grid-template-columns:1fr 1fr;gap:8px}
.tiles .b{min-height:56px;font-size:15px;justify-content:center}
.tbl{width:100%;border-collapse:collapse}
.tbl th{text-align:left;font-size:12px;color:var(--muted);font-weight:600;padding:4px 6px}
.tbl td{padding:4px 6px;border-top:1px solid var(--line);font-weight:500}
.log{border-top:1px solid var(--line)}
.log div{display:flex;gap:8px;align-items:center;min-height:44px;border-bottom:1px solid var(--line)}
.log time{color:var(--muted);font-size:12px;width:40px}
.score{display:grid;grid-template-columns:1fr auto 1fr;gap:4px 8px;text-align:center;align-items:center}
.score .n{font-family:"Cormorant Garamond",serif;font-weight:700;font-size:22px;line-height:1.05}
.score .v{font-family:"Cormorant Garamond",serif;font-weight:700;font-size:30px}
.urow{display:flex;flex-direction:column;gap:2px;border-bottom:1px solid var(--line);padding:6px 0}
.urow .l1{display:flex;align-items:center;gap:8px;min-height:44px}
.urow .l2{display:flex;align-items:center;gap:8px}.urow .l2>span:first-child{flex:1}
.urow.sel{background:var(--accent-soft);margin:0 -8px;padding:6px 8px;border-radius:10px;border-bottom-color:transparent}
.nm{font-weight:700;flex:1;min-width:0}
.nm small{font-weight:400;color:var(--muted)}
.dsk{display:none}
.mob{display:block}
.mob.row{display:flex}
@media (min-width:768px){
 .b{min-height:36px}.b.xs{min-height:30px}
 .chip,.rad,.cb{min-height:34px}
 .acc{min-height:44px}
 .dsk{display:block}.dsk.row{display:flex}.mob,.mob.row{display:none}
 .pg{padding:20px 32px 40px}
 .bgrid{display:grid;grid-template-columns:minmax(0,1fr) 380px;gap:32px;align-items:start}
 .stick{position:sticky;top:16px}
 .dock{display:none}.pad-dock{padding-bottom:40px}
 .sb{width:36px;height:36px}
 .tiles{grid-template-columns:repeat(3,1fr)}
 .ugrid{grid-template-columns:1fr 1fr}
 .hbtn .b.on{border-color:var(--accent);background:var(--accent-soft)}
}
@media (min-width:1100px){.ugrid{grid-template-columns:repeat(3,1fr)}}
`;

const U = [
  ['Rider of Rohan', 'L', 29], ['Warrior of Rohan', 'S', 22], ['Warrior of Rohan', '', 22], ['Warrior of Rohan', '', 22],
  ['Warrior of Rohan', '', 22], ['Warrior of Rohan', '', 22], ['Warrior of Rohan', '', 22], ['Warrior of Rohan', '', 22],
  ['Rohan Outrider', '', 25], ['Rohan Royal Guard', '', 31], ['Rider of Rohan', '', 24], ['Rohan Royal Guard', '', 31],
  ['Rider of Rohan', '', 24], ['Son of Eorl', '', 33], ['Warrior of Rohan', '', 22],
];
const G = ['Ranger of the North', 'Dwarf Warrior', 'Dwarf Warrior', 'Lothlórien Warrior', 'Galadhrim Warrior', 'Galadhrim Warrior',
  'Warrior of Rohan', 'Warrior of Rohan', 'Warrior of Rohan', 'Rider of Rohan', 'Rider of Rohan', "Captain of Helm's Deep"];
const ri = r => r === 'L' ? '<span class="role">♛</span>' : r === 'S' ? '<span class="role s">★</span>' : '';
const gearOf = i => i === 0 ? ['Horse', 'Hand weapon', 'Shield', 'Armour', 'Bow 24" · S2'] : i < 8 ? ['Hand weapon', 'Armour', 'Shield'] : ['Horse', 'Hand weapon', 'Armour'];
const I = '<span class="ii" aria-hidden="true">i</span>';

const page = (title, body, mode = 'light') => `<!doctype html><html lang="en" data-palette="forest" data-mode="${mode}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>${title}</title><link rel="stylesheet" href="../static/app.css"><style>${CSS}</style></head><body>${body}</body></html>`;

const st = (lb, v, mx) => `<div class="trk"><div class="lb">${lb}</div><div class="sr"><span class="sb">−</span><span><b>${v}</b><i> / ${mx}</i></span><span class="sb dim">+</span></div></div>`;
const wst = `<span class="sr" style="gap:2px"><span class="sb t">−</span><span><b>1</b><i> / 1</i></span><span class="sb t dim">+</span></span>`;
const stats = '<span class="stats"><span>MV<b>6"</b></span><span>FV<b>3</b></span><span>SV<b>4+</b></span><span>S<b>3</b></span><span>D<b>5</b></span><span>A<b>1</b></span><span>C<b>7+</b></span><span>I<b>6+</b></span></span>';
const acc = (t, su = '', open = false, ic = I) => `<div class="acc${open ? ' open' : ''}"><span class="ch">${open ? '▾' : '›'}</span>${t}${ic ? ' ' + ic : ''}<span class="su">${su}</span></div>`;
const rowsLong = U.map(u => u[0]);

/* ---------- unit card (current structure, used by A and by B at desktop) ---------- */
function card(u, i, mode) {
  const [n, r, p] = u;
  const actions = '<button class="b sm" style="margin-left:auto">Actions</button>';
  const head = `<div class="row" style="flex-wrap:nowrap;gap:8px"><span class="pt">♞</span><div class="nm"><div class="hd" style="font-size:19px;white-space:nowrap">${ri(r)}${n}</div>${mode === 'A' ? '<small class="mu">'+p+' pts</small>' : ''}</div>${mode === 'A' ? actions : '<span class="mu">'+p+' pts</span>'}</div>`;
  const body = `<div style="margin:8px 0">${stats}</div>
  <div class="g2">${st('Wounds', 1, 1)}${st('Might', 1, 1)}${st('Will', 1, 1)}${st('Fate', 1, 1)}</div>
  <div class="gear" style="margin-top:8px">${gearOf(i).map(g => `<span>${g}</span>`).join('')}</div>
  ${i === 0 ? '<div class="path" style="margin:6px 0">Path of the Tactician</div><div>' + ['Expert Rider', 'Spear Mastery', 'Arise, Riders of Rohan!'].map(k => `<span class="kw">${k}</span>`).join('') + '</div>' : ''}
  <div class="xp"><b>Lvl 0</b><span class="bar"><i style="width:0"></i></span><span class="mu">0 / 5 XP</span></div>`;
  return `<div class="card-u${r === 'L' ? ' lead' : ''}">${head}${body}</div>`;
}

/* ---------- MATCH PAGE ---------- */
const matchTop = (h1 = 34) => `<a class="lk">← Matches</a><div class="row" style="margin:6px 0 2px;gap:10px"><h1 style="font-size:${h1}px">Match 1</h1><span class="bd">Open</span><span class="mu">30 Oct 2026</span></div>`;

function matchA() {
  return page('Match page, design A', `<div class="pg">
${matchTop()}
<div class="row" style="margin:10px 0"><button class="b sm">To wound</button><button class="b sm p" disabled>Propose to end</button>${I}</div>
<div class="c" style="padding:10px 12px"><div><b>Riders on every horse</b> <span class="mu">· Éowyn</span></div><div class="mu sm-t" style="margin:2px 0">versus</div><div><b>Garrison of Helm's Deep</b> <span class="mu">· Sauron</span></div></div>
<div class="c"><h2>Scenario</h2><div style="font-weight:600;margin:4px 0">Chance Encounter</div>
<div class="row"><button class="b sm">Change</button><button class="b sm">One-off</button><button class="b sm g">Remove</button></div>
<div class="row" style="margin-top:8px"><span class="chip s">› Prizes</span><span class="chip s">› About</span><span class="chip s">› How scored</span></div></div>
${acc('End checks', '0 of 27 set')}
<div class="c"><h2>Result</h2><div class="mu sm-t">Who won?</div><div class="row" style="gap:0"><span class="rad">Riders</span><span class="rad">Garrison</span><span class="rad">Draw</span></div>
<div class="row" style="margin-top:4px"><span class="mu">VP</span><span class="inp">0</span><span class="inp">0</span><span class="sp"></span><button class="b sm">Save result</button></div></div>
${acc('Roles', '', false, '')}${acc('Match log', '0', false, '')}
</div>`);
}

function matchB() {
  const aside = `<aside class="dsk stick"><div class="c" style="padding:16px"><div class="score"><div class="n">Riders</div><div class="mu">vs</div><div class="n">Garrison</div><div class="mu sm-t">Éowyn · 143</div><div></div><div class="mu sm-t">Sauron · 54</div></div>
<button class="b p w" style="margin:12px 0 8px" disabled>Propose to end</button>
<div class="row"><button class="b sm">Result</button><button class="b sm">End checks</button><button class="b sm">To wound</button><button class="b sm">Roles</button><button class="b sm">Match log</button></div>
<div class="mu sm-t" style="margin-top:8px">Sticky panel: always in view at this width, so there is no bottom bar.</div></div></aside>`;
  return page('Match page, design B', `<div class="pg pad-dock"><a class="lk">← Matches</a><div class="bgrid"><div>
<div class="row" style="margin:6px 0 8px;gap:10px"><h1 style="font-size:30px">Match 1</h1><span class="bd">Open</span><span class="mu">Éowyn v Sauron</span></div>
${acc('Scenario: Chance Encounter', '<button class="b xs">Change</button>', true, '')}
<div class="accb"><div class="row"><span class="chip s">Prizes ${I}</span><span class="chip s">About ${I}</span><span class="chip s">Scored ${I}</span></div></div>
${acc('End checks', '0 of 27 set', false, '')}${acc('Result', 'not decided', false, '')}${acc('Roles / Log', '', false, '')}
</div>${aside}</div>
<div class="dock mob"><button class="b n">… More</button><button class="b">End checks</button><button class="b p" disabled>Propose end</button></div></div>`);
}

function matchC() {
  const logs = [['00:26', 'Éowyn proposed to end', 1], ['00:21', 'Ranger of the North died', 1], ['00:14', 'Warrior of Rohan retreated', 1], ['00:10', 'Turn 1 started', 0]];
  return page('Match page, design C', `<div class="pg pad-dock">
<div class="row" style="margin:2px 0 10px"><h2>Match 1</h2><span class="bd">Open</span><span class="sp"></span><button class="b sm">Tools</button></div>
<div class="c"><div class="score"><div class="n">Riders</div><div class="mu sm-t">143 · 0 VP</div><div class="n">Garrison</div>
<div class="v">0</div><div class="mu">:</div><div class="v">0</div><div class="mu sm-t">Éowyn</div><span class="bd wn">Not decided</span><div class="mu sm-t">Sauron</div></div>
<button class="b p w" style="margin-top:10px" disabled>Propose to end</button></div>
<div class="row" style="margin-top:14px"><h2>Match log</h2><span class="mu sm-t" style="margin-left:auto">newest first</span></div>
<div class="log">${logs.map(l => `<div><time>${l[0]}</time><span style="flex:1">${l[1]}</span>${l[2] ? '<button class="b xs">Undo</button>' : ''}</div>`).join('')}</div>
<button class="b w g" style="margin-top:6px">Show all entries</button>
<div class="dock"><button class="b p">Match tools ▴</button></div></div>`);
}

/* ---------- LIVE COMPANY PAGE ---------- */
const bar75 = `<div class="row" style="flex-wrap:nowrap;font-size:13px;gap:8px"><span style="white-space:nowrap">Break 0 / 7.5</span><span class="bar"><i style="width:0"></i></span><a class="lk">Details</a></div>`;

function liveA() {
  return page('Live company page, design A', `<div class="pg">
<div class="ubanner"><div class="row"><b>Match 1</b><span>running</span><span class="mu">·</span><span class="mu"><span class="live-dot"></span>Live</span><span class="sp"></span><button class="b sm">Match ▾</button></div>
<div class="row" style="margin-top:6px"><span class="mu">Turn not started</span><span class="sp"></span><button class="b sm">Start turn 1</button></div>
<div style="margin-top:6px">${bar75}</div></div>
<div class="g2" style="margin:10px 0"><div class="tile"><span class="lb">Points</span><b>197</b></div><div class="tile"><span class="lb">Units</span><b>15 <small class="mu">/ 15</small></b></div></div>
<div class="row" style="margin:12px 0 2px"><h2>Battle company</h2><span class="sp"></span><span class="chip s">› Limits</span></div>
${U.map((u, i) => card(u, i, 'A')).join('')}
</div>`);
}

const chevU = '<svg viewBox="0 0 10 6" aria-hidden="true"><path d="M1 5l4-4 4 4" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>';
const chevD = '<svg viewBox="0 0 10 6" aria-hidden="true"><path d="M1 1l4 4 4-4" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>';
/* a counter: up arrow above the number (raises), down arrow below (lowers); full = value at maximum, so the up arrow is dimmed */
const cnt = (lb, v, mx, show) => `<div class="cn${show ? ' show' : ''}"><span class="lb">${lb}</span><span class="ar up dim">${chevU}</span><span class="v"><b>${v}</b><i> / ${mx}</i></span><span class="ar dn">${chevD}</span></div>`;
const KW = { L: ['Cavalry', 'Man', 'Hero'], S: ['Infantry', 'Man'], '': ['Infantry', 'Man'] };
const kwOf = (n, r) => /Rider|Outrider/.test(n) ? ['Cavalry', 'Man'] : /Royal Guard/.test(n) ? ['Infantry', 'Man', 'Bodyguard'] : /Son of Eorl/.test(n) ? ['Cavalry', 'Man', 'Hero'] : KW[r];
const rulesOf = (n, r, i) => i === 0 ? ['Expert Rider', 'Spear Mastery', 'Arise, Riders of Rohan!'] : /Rider|Outrider|Son of Eorl/.test(n) ? ['Expert Rider'] : /Royal Guard/.test(n) ? ['Shieldwall', 'Bodyguard'] : r === 'S' ? ['Shieldwall', 'Sergeant'] : ['Shieldwall'];

/* the same full unit card as the non-live company page (name, race, characteristics, gear, keywords, rules, XP), plus the live counters and an Actions button in the header */
function cardLive(u, i, o = {}) {
  const [n, r, p] = u;
  const pop = o.pop ? `<div class="pop dsk">${['Wounded', 'Fate roll', 'Broken: passed', 'Retreated', 'Died', 'Mount removed'].map(x => `<div>${x}</div>`).join('')}</div>` : '';
  const head = `<div class="row" style="flex-wrap:nowrap;gap:8px"><span class="pt">♞</span><div class="nm"><div class="hd" style="font-size:19px;white-space:nowrap">${ri(r)}${n}</div><small class="mu">Man of Rohan · ${p} pts</small></div>
<span class="hbtn"><button class="b sm${o.pop ? ' on' : ''}">Actions ▾</button>${pop}</span></div>`;
  const body = `<div style="margin:8px 0 0">${stats}</div>
  <div class="cnt">${cnt('Wounds', 1, 1, o.show)}${cnt('Might', 1, 1)}${cnt('Will', 1, 1)}${cnt('Fate', 1, 1)}</div>
  ${o.show ? '<div class="mu" style="font-size:11px;margin:-2px 0 6px">Dashed: the 44 px touch target of the Wounds arrows; the arrows themselves stay 26x16 px.</div>' : ''}
  <div class="gear">${gearOf(i).map(g => `<span>${g}</span>`).join('')}</div>
  ${i === 0 ? '<div class="path" style="margin:2px 0 6px">Path of the Tactician</div>' : ''}
  <div>${kwOf(n, r).map(k => `<span class="kw mu">${k}</span>`).join('')}${rulesOf(n, r, i).map(k => `<span class="kw">${k}</span>`).join('')}</div>
  <div class="xp"><b>Lvl 0</b><span class="bar"><i style="width:0"></i></span><span class="mu">0 / 5 XP</span></div>`;
  return `<div class="card-u live${r === 'L' ? ' lead' : ''}">${head}${body}</div>`;
}

function liveB(open = false) {
  const list = `<div>
<div class="row" style="margin:8px 0 0"><h2>Field (15)</h2></div>
<div class="ugrid">${U.map((u, i) => cardLive(u, i, { pop: open && i === 0, show: i === 0 })).join('')}</div>
<div class="row" style="margin-top:8px"><span class="chip s">› Retreated (0)</span><span class="chip s">› Casualties (0)</span></div></div>`;
  return page('Live company page, design B', `<div class="pg pad-dock">
<div class="row" style="gap:10px"><h2 style="font-size:22px">Riders on every horse</h2><span class="mu">Match 1</span><span class="mu"><span class="live-dot"></span>Live</span>
<span class="dsk row" style="margin-left:auto;gap:10px"><span class="mu">Turn 0</span><button class="b sm">Match ▾</button><button class="b sm p">Start turn 1</button><span style="min-width:220px">${bar75}</span></span></div>
<div class="mob" style="margin:4px 0 2px"><div class="row" style="flex-wrap:nowrap"><span class="mu" style="white-space:nowrap">Turn not started</span><span class="sp"></span><span style="width:170px">${bar75}</span></div></div>
${list}
<div class="dock mob"><button class="b p">Start turn 1</button><button class="b">Match ▾</button></div></div>`);
}

function liveC() {
  const rows = U.map(([n, r]) => `<tr><td style="padding:2px 0"><div class="row" style="flex-wrap:nowrap;min-height:48px"><span class="nm">${ri(r)}${n}</span>${wst}<span class="mu" style="width:16px">›</span></div></td></tr>`).join('');
  return page('Live company page, design C', `<div class="pg pad-dock">
<div class="row"><h2>Riders</h2><span class="mu">Match 1</span><span class="mu"><span class="live-dot"></span>Live</span><span class="mu">Turn 0</span></div>
<div class="row" style="margin:6px 0;flex-wrap:nowrap"><span style="flex:1">${bar75.replace('Details', '')}</span><button class="b sm p">Start turn 1</button></div>
<h2 style="margin:10px 0 0">Units (15)</h2>
<table class="tbl"><tbody>${rows}</tbody></table>
<div class="dock"><button class="b">Unit actions</button><button class="b p">Match tools</button></div></div>`);
}

/* ---------- CONTROLS: backdrops reuse the live page of each design ---------- */
const overlay = (html, ov) => html.replace('</body>', ov + '</body>');
const actNames = ['Wounded a unit', 'Passed a fate roll', 'Passed a Broken courage roll', 'Retreated', 'Died', 'Mount removed'];
const xbtn = '<button class="b xs g" style="min-height:44px">✕</button>';

function actA() {
  return overlay(liveA(), `<div class="scrim"></div><div class="modal top"><div class="row" style="flex-wrap:nowrap"><h2 style="flex:1">Rider of Rohan</h2>${xbtn}</div>
<div class="row" style="margin:2px 0 6px"><span class="mu">What happened?</span>${I}</div>
${actNames.map(n => `<div class="lrow" style="min-height:48px;margin:6px 0">${n}<span class="mu" style="margin-left:auto">›</span></div>`).join('')}
<div class="mu sm-t" style="margin-top:8px">No Close button: the cross and Esc close it. One line per row, hints are tooltips.</div></div>`);
}
function actB() {
  return overlay(liveB(true), `<div class="scrim"></div><div class="sheet cmp"><div class="handle"></div><div class="row" style="flex-wrap:nowrap"><b style="flex:1;font-size:15px">Actions · Rider of Rohan</b>${xbtn}</div>
<div class="tiles" style="margin-top:2px">${['Wounded', 'Fate roll', 'Broken: passed', 'Retreated', 'Died', 'Mount removed'].map(n => `<button class="b">${n}</button>`).join('')}</div>
<div class="mu sm-t" style="margin:8px 0 0">Small sheet (about 230 px) opened by the Actions button of the card header. At 768 px and up the same six items are a popover under that button (see the 1280 picture).</div></div>`);
}
function actC() {
  return overlay(liveC(), `<div class="scrim"></div><div class="sheet"><div class="handle"></div><div class="row" style="flex-wrap:nowrap"><h2 style="flex:1">Rider of Rohan</h2>${xbtn}</div>
<div class="row" style="margin:6px 0"><span class="inp" style="width:100%">Quick action: type "wound"…</span></div>
${actNames.map(n => `<div class="lrow" style="min-height:44px;margin:5px 0">${n}</div>`).join('')}
<div class="mu sm-t" style="margin-top:6px">Flat list of 44 px rows, no hints. "/" focuses the quick action field on a keyboard.</div></div>`);
}

const endHead = t => `<div class="row" style="flex-wrap:nowrap"><h2 style="flex:1">${t}</h2>${xbtn}</div>`;
function endA() {
  const half = names => `<div class="g2">${names.map((n, i) => `<span class="cb${i === 0 ? ' on' : ''}" style="border:1px solid var(--line);border-radius:8px;padding:0 8px;font-weight:600;font-size:13px">${n}</span>`).join('')}</div>`;
  return overlay(liveA(), `<div class="scrim"></div><div class="modal top">${endHead('End checks ' + I)}
<div style="font-weight:700;margin:6px 0 4px">Objective · Riders on every horse</div>${half(rowsLong.slice(0, 12))}
<div class="mu sm-t" style="margin:6px 0">... 3 more rows. A checkmark replaces the word "No"; the sentence is a tooltip. Two columns of toggles.</div>
<div style="font-weight:700;margin:10px 0 4px">MVP · Garrison of Helm's Deep</div>${half(G.slice(0, 4))}</div>`);
}
function endB() {
  const short = n => n.replace('Warrior of Rohan', 'Warrior').replace('Rohan ', '').replace('Rider of Rohan', 'Rider');
  const comp = (t, names, on, radio) => `<div style="font-weight:700;margin:8px 0 6px">${t} ${I}</div><div class="chips">${names.map((n, i) => `<span class="chip sq${on.includes(i) ? ' on' : ''}">${radio ? (on.includes(i) ? '● ' : '○ ') : (on.includes(i) ? '✓ ' : '')}${short(n)}</span>`).join('')}</div>`;
  return overlay(liveB(), `<div class="scrim"></div><div class="sheet" style="max-height:92vh;overflow:hidden"><div class="handle"></div>${endHead('End checks')}
${comp('Objective · Riders on every horse', rowsLong, [0, 9], false)}${comp("MVP · Garrison of Helm's Deep", G, [], true)}</div>`);
}
function endC() {
  const t = (names, lbl) => `<table class="tbl"><thead><tr><th>${lbl}</th><th>Got it</th><th>MVP</th></tr></thead><tbody>${names.map((n, i) => `<tr><td>${n}</td><td><span class="cb${i === 0 ? ' on' : ''}" style="min-height:36px"></span></td><td><span class="rad" style="min-height:36px"></span></td></tr>`).join('')}</tbody></table>`;
  return overlay(liveC(), `<div class="scrim"></div><div class="sheet" style="max-height:92vh;overflow:hidden"><div class="handle"></div>${endHead('Match tools · End checks')}${t(rowsLong.slice(0, 11), 'Unit')}
<div class="mu sm-t" style="margin-top:4px">Table of 27 rows, shortened here.</div></div>`);
}

/* ---------- AFTERMATH ---------- */
const xpShort = ['Wounded', 'Slew', 'Fate', 'Courage', 'Objective', 'MVP'];
const steps = (cur = 0) => `<div class="g2" style="grid-template-columns:repeat(4,1fr);margin:10px 0">${['Sheet', 'Injuries', 'Level-ups', 'Finish'].map((s, i) => `<div class="chip sq${i === cur ? ' on' : ''}" style="justify-content:center;flex-direction:column;gap:0;padding:2px 0;font-size:12px"><b style="color:var(--fg)">${i + 1}</b>${s}</div>`).join('')}</div>`;
const aHead = `<a class="lk">← Match 1</a><h1 style="margin:6px 0 4px;font-size:30px">Riders on every horse</h1><div class="row"><span class="bd">Draw</span><span class="bd wn">Pending</span></div>`;

function afterA() {
  const unit = (n, r, i) => `<div style="border-top:1px solid var(--line);padding:8px 0"><div style="font-weight:700">${ri(r)}${n}${i === 3 ? ' 4' : ''}</div>
<div class="row" style="gap:6px;margin-top:2px"><span class="chip sq${i === 3 ? '' : ' on'}">Survived</span><span class="chip sq${i === 3 ? ' on' : ''}">Casualty</span><span class="chip sq">Fled</span><span class="cb on" style="font-size:12px">Took part</span></div>
<div class="chips" style="margin-top:6px">${xpShort.map(x => `<span class="chip sq">${x === 'Slew' ? 'Slew 0 −+' : x} ${I}</span>`).join('')}</div></div>`;
  return page('Aftermath, design A', `<div class="pg">${aHead}${steps()}
<div class="c"><div class="row"><h2>1. Sheet</h2><span class="bd">From the live match</span>${I}</div>
${acc('Against the Odds', '6 bonuses', false, '')}
<div style="margin-top:6px">${U.map((u, i) => unit(u[0], u[1], i)).join('')}</div>
<div class="row" style="margin-top:10px"><button class="b sm">Save sheet</button><button class="b p sm">Start aftermath</button></div></div></div>`);
}
function afterB() {
  const open = `<div class="accb" style="margin-bottom:8px"><div class="row" style="gap:6px"><span class="chip sq s">Survived</span><span class="chip sq s on">Casualty</span><span class="chip sq s">Fled</span><span class="cb on" style="font-size:12px">Took part</span></div>
<div class="chips" style="margin-top:6px">${xpShort.map(x => `<span class="chip sq s">${x} ${I}</span>`).join('')}</div></div>`;
  const rows = U.map((u, i) => {
    const cas = i === 3;
    return acc(`${ri(u[1])}${u[0]}${cas ? ' 4' : ''}`, `${cas ? 'Casualty' : 'Survived'} · +${cas ? 1 : 0} XP`, cas, '') + (cas ? open : '');
  }).join('');
  const side = `<aside class="dsk stick"><div class="c"><b>Summary</b><div class="mu" style="margin:4px 0 10px">15 units, 1 casualty, +14 XP</div>${acc('Against the Odds', '6 bonuses', false, '')}<div class="row" style="margin-top:8px"><button class="b">Save sheet</button><button class="b p">Start aftermath</button></div></div></aside>`;
  return page('Aftermath, design B', `<div class="pg pad-dock">${aHead}${steps()}<div class="bgrid"><div>
<div class="row" style="margin:6px 0"><span class="mu">Defaults: Survived, took part</span>${I}</div>
<div class="mob">${acc('Against the Odds', '6 bonuses', false, '')}</div>${rows}</div>${side}</div>
<div class="dock mob"><button class="b">Save sheet</button><button class="b p">Start aftermath</button></div></div>`);
}
function afterC() {
  const unit = (u, i) => `<div style="border-top:1px solid var(--line);padding:6px 0"><div class="row" style="flex-wrap:nowrap"><b class="nm">${ri(u[1])}${u[0]}${i === 3 ? ' 4' : ''}</b><span class="b sm" style="min-width:110px;justify-content:space-between">${i === 3 ? 'Casualty' : 'Survived'} <span>▾</span></span></div>
<div class="row" style="gap:4px;flex-wrap:nowrap"><span class="cb on" style="min-height:44px;width:30px;flex:none"></span>${['Wnd', 'Slew 0', 'Fate', 'Cour', 'Obj', 'MVP'].map(x => `<span class="chip sq" style="min-height:44px;padding:0 2px;flex:1;justify-content:center;font-size:11px">${x}</span>`).join('')}</div></div>`;
  return page('Aftermath, design C', `<div class="pg pad-dock">${aHead}
<div class="row" style="margin:8px 0"><span>XP so far <b>+0</b></span><span class="mu">·</span><span>Influence <b>+3</b></span></div>${steps()}
${U.map(unit).join('')}
<div class="dock"><button class="b">Save sheet</button><button class="b p">Start aftermath</button></div></div>`);
}

const out = {
  'match-A': matchA, 'match-B': matchB, 'match-C': matchC,
  'live-A': liveA, 'live-B': () => liveB(true), 'live-C': liveC,
  'actions-A': actA, 'actions-B': actB, 'actions-C': actC,
  'endchecks-A': endA, 'endchecks-B': endB, 'endchecks-C': endC,
  'aftermath-A': afterA, 'aftermath-B': afterB, 'aftermath-C': afterC,
};
fs.mkdirSync('pages', { recursive: true });
for (const [k, f] of Object.entries(out)) fs.writeFileSync(`pages/${k}.html`, f());
for (const k of ['match-B', 'live-B']) fs.writeFileSync(`pages/${k}-dark.html`, out[k]().replace('data-mode="light"', 'data-mode="dark"'));
console.log('pages written:', Object.keys(out).length + 2);
