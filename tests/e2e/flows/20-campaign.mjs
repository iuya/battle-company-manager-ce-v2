// An organiser creates a campaign, invites a player, both take command of a company; Manage campaign pause / end.
import { assert, campaignId, confirmDialog, MODAL } from '../lib/util.mjs';

export default async function (h, { only, shared }) {
  const you = h.sessions.find((s) => s.email === 'you@example.com') || await h.session('you@example.com');
  const name = `E2E campaign ${Date.now() % 100000}`;
  let id, frodo, link;

  await h.step('campaign: the organiser creates a campaign', async () => {
    await you.goto('/campaigns/new');
    await you.fill('input[name=name]', name);
    await you.fill('textarea[name=description]', 'Made by the browser smoke tests.');
    await you.click('button', 'Create campaign', { nav: true });
    id = await campaignId(you, name);
    await you.goto(`/campaigns/${id}`);
    await you.waitText(name);
    await you.waitText('Choose your company');
  }, { only });
  if (!id) return;
  shared.campaign = { id, name };

  await h.step('campaign: invite link is shown', async () => {
    await you.click('button', 'Invite players');
    await you.waitText('Invitation link');
    link = await you.page.evaluate(() => {
      const m = document.body.innerHTML.match(/\/join\/[A-Za-z0-9_-]{20,}/);
      return m && m[0];
    });
    assert.ok(link, 'the invitation link is on the page');
  }, { only });
  if (!link) return;

  await h.step('campaign: a signed-in player joins by the link', async () => {
    frodo = await h.session('frodo@example.com');
    await frodo.goto(link);
    await frodo.waitText(name);
    await frodo.click('button', 'Join', { nav: true });
    await frodo.goto(`/campaigns/${id}`);
    await frodo.waitText('Choose your company');
    await you.goto(`/campaigns/${id}`);
    await you.waitText('Players');
  }, { only });
  if (!frodo) return;

  const takeCommand = async (s, faction, companyName) => {
    await s.goto(`/campaigns/${id}/company/new`);
    await s.click('main button', `${faction} `);
    await s.click('main a, main button', 'Take command');
    await s.fill('input[name=name]', companyName);
    await s.click('main form button.btn-primary', '');
    await s.waitText('These decisions are final');
    await confirmDialog(s, '=Yes', { nav: true });
  };
  await h.step('company: take command of a company (confirmation dialog, then My company)', async () => {
    await takeCommand(frodo, 'The Shire', 'Frodo and friends');
    assert.match(new URL(frodo.page.url()).pathname, /my-company$/);
    await frodo.waitText('Frodo and friends');
    await frodo.waitText('Your company');
    await takeCommand(you, 'Rohan', 'Riders of E2E');
    await you.waitText('Riders of E2E');
  }, { only });

  await h.step('company: an officer chooses a path', async () => {
    await you.goto(`/campaigns/${id}/my-company`);
    await you.click('main a, main button', 'Choose a path');
    await you.click(MODAL + ' label', 'Path of the Tactician');
    await you.click(MODAL + ' button', 'Take this path');
    await you.waitText('The path is chosen');
  }, { only });

  await h.step('manage campaign: pause and resume, then end with the typed name', async () => {
    await you.goto(`/campaigns/${id}`);
    await you.click('main button', 'Pause campaign');
    await you.click('main button', '=Pause');
    await you.waitText('Paused');
    await you.click('main button', '=Resume campaign');
    await you.click('main button', '=Resume');
    await you.waitGone('The campaign is paused');
    await you.click('main button', 'End campaign');
    await you.click('main button', '=Continue');
    // step two asks for the campaign's name; Continue stays disabled until it is typed exactly
    const field = 'main input:not([type=hidden]):not([type=checkbox]):not([type=radio])';
    await you.fill(field, 'not the name');
    const off = () => you.page.evaluate(() => [...document.querySelectorAll('main button')].filter((b) => b.getBoundingClientRect().width && b.innerText.trim() === 'Continue').map((b) => b.disabled));
    assert.deepEqual(await off(), [true], 'Continue is disabled with a wrong name');
    await you.fill(field, name);
    assert.deepEqual(await off(), [false], 'Continue is enabled with the right name');
    await you.click('main button', '=Continue');
    await you.click('main button', 'End the campaign for good');
    await you.goto(`/campaigns/${id}`);
    await you.waitText('Finished');
    assert.ok(!(await you.has('Invite players')), 'a finished campaign has no invite button');
  }, { only });
}
