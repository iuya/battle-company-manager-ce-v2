# The demo world (`make demo`)

A rich development database for exploring and testing the app: many campaigns in every state, with accounts to sign in as. This page lists what each campaign shows and where to click.

`make demo` **destroys the development database's contents** (`make db-reset`; back it up first if you need it), loads the demo world with `server seed --world`, and starts the
server on http://localhost:8080 (no live reload; for that load the world with `go run ./cmd/server seed --world` and use `make dev`). The
world is a script, `cmd/server/world*.go`, that drives the **real core services** through `newBackend`: nothing is inserted
behind their back except the few states listed under [Gaps](#gaps). Every die that decides an outcome is typed in (campaigns
with `ManualRolls`) or fixed (`core.FixedDice` through `demoDice`, campaigns without), so the world is the same every run.

```
make demo                      # empty database, the world, the server on :8080
make demo PASSWORD=...         # another password for every account (at least 10 characters)
make seed                      # the minimal demo ([frontend.md](frontend.md#demo-data-and-signing-in))
go run ./cmd/server seed --world   # load the world into the current database (no reset; does nothing if it is there)
```

What the script printed (accounts, the three invite links with their secrets, gaps, and a line per company it built) is kept in
`tmp/demo-world.txt`. The seed is idempotent in the sense that it does nothing when the first campaign exists; `make demo` is
repeatable because it starts from an empty database. Test: `TestSeedWorld` in `cmd/server/world_test.go` (needs
`TEST_DATABASE_URL` (a scratch database, never the development one); it fails when the script meets a state it cannot reach, and checks the gallery, the finished campaign and the
aftermath stages).

## Accounts

All share the password `demo-password` (or `PASSWORD`). `admin@example.com`, `you@example.com`, `eowyn@example.com` and
`sauron@example.com` are the accounts of `make seed` too.

| Email | Role | What it shows |
|---|---|---|
| `admin@example.com` | superuser | `/admin`, reopening matches, waiving aftermaths; a member of every campaign |
| `steward@example.com` | admin | the same powers on campaigns, none over users and rules files; a member of every campaign |
| `you@example.com` | user, may create campaigns | organises nearly every campaign: grants, matches, invites |
| `faramir@example.com` | user, may create campaigns | organises one campaign, receives another by handover |
| `eowyn@example.com`, `sauron@example.com`, `aragorn@example.com`, `gimli@example.com`, `bilbo@example.com`, `galadriel@example.com`, `radagast@example.com` | user | the players; each has a company in many campaigns (see below) |
| `pippin@example.com` | user, **cannot** create campaigns | no "New campaign" button; plays in a few campaigns and used the one-use invite link |
| `frodo@example.com` | user, may create campaigns | in no campaign: the empty home page. Open the valid invite link as him |
| `boromir@example.com` | **closed** account | closed at the end of the script (`Auth.CloseAccount`): the campaign "Cemetery" shows him as "Former player" and the address no longer signs in |
| `<company>@example.com` | user | the gallery: one account per Battle Company (`minas-tirith@`, `osgiliath-ithilien@`, `goblin-town@`, `wanderers-in-the-wild@` ...) |

## Campaigns

Open any campaign as `you@example.com` (organiser, sees every button) or as the player named in the table (owner of the
company, sees "My company"). Page names are those of the navigation: Overview, My company, Companies, Rankings, Matches, Armoury.

| Campaign (in order of creation) | Players | What it demonstrates | Where to click |
|---|---|---|---|
| **Brand new campaign: no companies yet** | Aragorn joined, no company | the empty campaign: no ranking, no matches; a member without a company ("pending join") | Overview as `aragorn`: "Create your company" |
| **Companies being built: paths still to choose** | Éowyn (Rohan), Sauron (Mordor), Aragorn (Arnor), Gimli (Durin's Folk), Bilbo (no company) | officers with **no path** (Éowyn), **one path chosen** (Sauron: the Leader only), a company ready to play, Gimli's own choice of Leader and Sergeants, a member who has not built a company; a match cannot be opened for a company with a path pending | My company as `eowyn`: the pending officers ask for a path; Matches > New match with Éowyn's company is refused |
| **Gallery: all 37 Battle Companies, a few games in** | one gallery account per company | all 37 factions with the same treatment: officers on different paths, a recruit on **every row** of the reinforcement chart and of the special chart, the company's special units, a piece of gear for the Leader. See [Company types](#company-types) | Companies; open a company, My company as its gallery account |
| **Injuries and treatment: every wound on the chart** | Éowyn (Rohan), Sauron (Mordor), Aragorn (Arnor) | see [Injuries](#injuries-and-treatment) | My company of each |
| **Level-ups waiting: warriors, heroes, paths and a Sorcerer** | Galadriel (Lothlorien), Bilbo (The Shire) | see [Level-ups](#level-ups) | My company: the unit cards with "Level up" |
| **Limits, creatures and wanderers: the armoury at its edge** | Radagast (Mirkwood), Pippin (The Shire), Gimli (Erebor & Dale), Éowyn (Rohan), Sauron (Helm's Deep), Galadriel (Rivendell) | see [Units, creatures and limits](#units-creatures-and-limits) | Armoury, My company |
| **Keyword targets: who the rules work against** | Gimli (Erebor & Dale, Ancient Enemies (Orc)), Radagast (Mirkwood, Orcs, Goblins and Spiders) | two recruited companies and no match: open one between them to see the "Ancient Enemies (Orc) applies" markers on the enemy's units in the live match (e2e flow `95-keyword-targets`) | Matches > new match; the live company pages |
| **Matches: every scenario and every state** | six pool companies (Éowyn, Sauron, Aragorn, Galadriel, Gimli, Bilbo) and 22 more played by gallery accounts | see [Matches](#matches) | Matches |
| **Aftermath ward 1: every step of the War's Aftermath** | eight companies | see [Aftermaths](#aftermaths) | Matches > a match > the company's aftermath; Round 3 of 12 on the Overview |
| **Aftermath ward 2: draws, odds, perks and a succession** | six companies | see [Aftermaths](#aftermaths) | the same |
| **Aftermath ward 3: no hero left to step up** | Sauron (Mordor), Gimli (Durin's Folk), Aragorn (Arnor), Galadriel (Lothlorien) | a Sergeant fell and the company has no hero: the Warrior with the most Experience (one clear choice), two Warriors tied (the player chooses), and a Leader who fell with both Sergeants (a Leader, then Sergeants, all from the Warriors); see [Aftermaths](#aftermaths) | the aftermath of each company in Match 1 |
| **Settings: the server rolls every die** | Éowyn, Sauron | `ShakeTheTable` and `ManualRolls` **off**: roll panels have only a Roll button; the script fixed the server's dice, so the results never change | My company > recruit / level up; the aftermath of Match 1 |
| **Settings: heroes drop profile gear** | Aragorn | `HeroesDropProfileGear` **on** (a hero dropped an item its unit came with); the campaign is **paused**, Round 5 of 8 | Aragorn's Leader card; the Overview status |
| **Settings: everything on, own paths level up** | Galadriel | all four settings on; `CustomOnTheirOwnPath`: the Wood Elf Sentinels are warriors that earn experience: one has two level-ups waiting, another rolled a 6 and became a hero on the "Path of Their Own Making" | My company |
| **Pinned to another edition: Battle Companies 2.1-demo** | Gimli, Bilbo | a campaign pinned to a **forked Battle Companies file** (the app's file with another name and version) while the other campaigns use the default | Overview > Rules card (name, version, short hash); `/admin` > Rules files lists both |
| **Finished campaign: the Siege of Osgiliath** | Faramir, Sauron | a campaign played to its end (status finished, Round 12 of 12): no invite, no grant, no new match; two matches with finished aftermaths | Overview; Rankings; the home page's "finished" group |
| **Aura parade: every aura ring a company can carry** | You (Fiefdoms), Éowyn (Rohan), Galadriel (Lothlorien), Sauron (Angmar & Carn Dum) and six gallery accounts (Lake-town, Moria, City of Umbar, Osgiliath & Ithilien, Goblin-Town, Wanderers in the Wild) | ten companies that between them carry **every aura of the data that a company can reach**, for the smoke test of the rings in Tabletop Simulator; `CustomOnTheirOwnPath` is on (Lead by Example). See [Aura parade](#aura-parade) | My company (as `you`: Fiefdoms; the others under Companies) > the Tabletop Simulator button |
| **Cemetery: the fallen of the Misty Mountains** | Boromir (closed account), Bilbo (The Shire, "Hobbits who lost their Ranger": the Ranger of the North fell in a lost Rescue match against Boromir's company, played through the aftermath: a Hobbit who had rolled Hero in the Making took the Sergeant place the promotion of a Sergeant to Leader left, so the company has the three officers it needs to recruit; the Ranger is in the cemetery, so Recruit may swap a successful result for a new one) | a cemetery with a dead and a retired Sergeant and four dead warriors, in the order the player chose; the owner shown as "Former player" | My company > Cemetery (as `you`: Companies > the company) |
| **Handed over: Faramir organises now** | Éowyn, Faramir | `Campaigns.HandOver`: created by You, organised by Faramir; You is a player without a company | Overview as `faramir` (organiser buttons) and as `you` (none) |
| **Faramir's campaign: a small start** | Aragorn, Pippin | a campaign organised by Faramir from the start | Overview as `faramir` |

### Aura parade

Step `world_aura.go`, campaign **Aura parade: every aura ring a company can carry** (organiser `you`). Spec: [aura-ranges.md](aura-ranges.md); data: [`definitions.md`](definitions.md).
Every officer's level-ups are typed rolls (`w.gain`: 5 experience, the roll of the path chart that offers the rule, the choice; a warrior becomes a hero with a 6, a Sorcerer names
its first power), items are bought with gold, so everything is reached through the real services. A company in a campaign belongs to one player and a company has its own upgrades, so the
auras that depend on the company are spread over ten companies (the three "main" ones carry the items, the paths, the powers and the creature; the six gallery accounts carry one hereditary upgrade
each). The script prints the auras of every unit and creature (`aura ...` lines) and the Tabletop Simulator export (`Tabletop Simulator: N rings on M of K figures`), and records a gap
(failing `TestSeedWorld`) when an aura entry of the data is carried by no company and `auraUnreachable` (`world_aura.go`) does not explain it. The default selection of the dialog applies
(the `cast`, `target`, `large` and `optional` groups are off), so the spells, the targeting ranges, the war horn's 24" and the Torch ring show once their switches are on.

| Company (account) | What it carries | Auras |
|---|---|---|
| **Fiefdoms of Gondor** (`you`) | Warlord Leader (Furious Tirade, Commanding Bellow, Master of Battle (3+), Voice of Command, **Lord of Dol Amroth**) with an Armoured horse and a Company Horn; Tactician Sergeant (Heroic Move Mastery, Reading the Portents, Spy, Gatekeeper, **Chief of Lamedon**); Marksman Sergeant (Hold positions, Take Aim) with a Company Instrument; a Soldier hero (A Good Sort, Relentless Commander, **Lord of Lossarnach**) with a Torching Brand; a Sharpshooter hero (Knife Mastery, **Lord of the Vale**); a Warrior with a Company Standard; a Warrior with a Company Drum | Company Standard 3" banner; Company Instrument 3" support; Company Drum 12" command; Company Horn 24" command and 9" Stand Fast extension (Hero); Torching Brand 6" utility; Stand Fast 6" (every hero); Take Aim; Voice of Command 12" (replaces the 6" command rings of that Leader); Furious Tirade; Commanding Bellow; Master of Battle; Lord of Dol Amroth; Chief of Lamedon; Lord of Lossarnach; Lord of the Vale; Heroic Move Mastery; Reading the Portents; Spy; Gatekeeper; Knife Mastery; "Hold positions, loose the arrows!"; A Good Sort; Relentless Commander |
| **Riders of the Mark** (`eowyn`) | Claimant Leader (Claim the Throne, Heroic Challenge), Ranger Sergeant (Chief Advisor, Heroic March), Berserker Sergeant (To Arms!), Duelist hero (For Glory), Shieldbearer hero (The King's Protector), a Rider of Rohan with a Company Standard | Claim the Throne; Heroic Challenge; At the Double; Chief Advisor; To Arms!; For Glory; The King's Protector; Company Standard on a rider |
| **Golden Wood** (`galadriel`) | Sorcerer Leader who knows Aura of Command, Aura of Dismay, Blinding Light, Fog of Disarray, Instill Fear, Writhing Vines and Fury (Beast); a Falcon (shown as Crebain); Wood Elf Sentinels, one of them a hero on the Path of Their Own Making with Lead by Example | Aura of Command, Fury, Aura of Dismay, Fog of Disarray, Instill Fear, Blinding Light, Writhing Vines (all `cast`); Keen Sight and Handler's Glove (the Falcon's own figure); Enchanting Song (Lay of Gondolin, Hymn of Elbereth); Lead by Example |
| **Angmar** (`sauron`) | Warlord Leader with no rule yet (Heroic Resolve alone); Warg Riders, one with a Company Standard (the rider, its on-foot version and the Wild Warg mount are figures: the mount gets no ring); a Spectre | Heroic Resolve; A fell light is in them (`target`); Company Standard on a rider and on its on-foot version |
| **Lake-town** (`lake-town@`) | an officer with **Moneybags** | Moneybags |
| **Moria** (`moria@`) | an officer with **Callous Leadership** | Callous Leadership |
| **City of Umbar** (`city-of-umbar@`) | an officer with **Seize Command** | Seize Command |
| **Osgiliath & Ithilien** (`osgiliath-ithilien@`) | an officer with **Remember Today** | Remember Today |
| **Goblin-Town** (`goblin-town@`) | an officer with **Swap with Me!** | Swap with Me! (`target`) |
| **Wanderers in the Wild** (`wanderers-in-the-wild@`) | an officer with **You Have My Sword** | You Have My Sword |

Every company also shows Stand Fast, With Me! and Loose on its heroes; the Marksman officers of the satellite companies show Take Aim.

**Auras no company can reach** (`auraUnreachable`):

- **Banner (3")**, the plain item: a Battle Company cannot own it. The Battle Companies equipment table sells the Company Standard instead and the vanilla options "Banner", "War horn" and "War drum"
  are `notSold` (`data.optionSales`). The Company Standard (same 3" ring), the Company Horn and the Company Drum (the same names as the entries `horn-*` and `drum-12`) are carried above. The entry would show
  up in a custom ruleset that sells a banner.
- The rules the data lists as `receiver-side`, `centred-elsewhere`, `weapon-range`... (`excluded` in `tools/data/aura_audit.json`) and the vanilla rules of characters no company can field (`unreachable`) are not entries.
- Everything else of the entries is carried by at least one company above; the ring of `Lead by Example` is Buhrdur's alone (no company has him), so it is listed among the unreachable entries; the homebrew path's `Lead by Example (Their Own Making)` has no ring and needs the campaign setting `CustomOnTheirOwnPath`, which this campaign switches on.

### Company types

The gallery shows, among the 37 companies (names as in the rules file, recruits as the chart rolls dictate):

- **Fornost**: the special chart with its rare Rivendell Warrior (choice of options, rare 5) and the Arnor oath text; **Arnor**
  and the other companies show their oath on the Overview of the company page.
- **Wanderers in the Wild**: a roll of 2 that offers four Hobbits to choose from (rare 1 each), the rare Warriors of Númenor,
  Dwarf Rangers and Lothlórien Warriors, the "Choice of the Above" 6.
- **Durin's Folk**: the **Vault Warden Team** (rare 1) joins as two units, the Iron Shield Bearer and the Foe Spear Wielder;
  rolling it again offers the missing member.
- **Mirkwood**: spiders and the Warg Marauder (rare); "Marauders of the Greenwood" in the limits campaign has the **Warg
  Marauder** alone, as one unit of three models.
- **Moria** and **Goblin-Town**: results with two models (a roll of 5, the special chart's pairs), the Cave Troll / Gundabad Ogre
  bought as special units, units **On Their Own Path** (they earn no experience).
- **The Shire** (25 models, "Choose any 2 Hobbits"), **Rohan** ("Choice of above", "Choice of Any", the Son of Eorl), **Lake-town**
  (pairs, 20 models), **Muster of Isengard**, **Helm's Deep** (units of several races with the races' limits: Elf 4 of 4 in the
  limits campaign).
- A **lower result** (a rare unit exhausted, the company at its maximum of models) happens in many companies; **companies at their
  maximum of models** (12, 15, 20, 25) show "The company is full".

### Injuries and treatment

"Injuries and treatment" plays one match between Rohan and Mordor and rolls the casualties with typed dice:

- **Rohan** (all finished, aftermath done): the Leader got an **Arm Wound**, a Sergeant a **Leg Wound** (the treatment attempt on
  a 4, which needs a 5, is **waiting for the player to spend 1 more Influence**), the other Sergeant **Broken Honour** (sent to
  the healers: removed, but it misses the next game), one warrior **Injured**, one **dead**, one **Lesson Learned** (+3 Experience),
  one **Full Recovery**, one more Injured and then treated (a warrior's treatment costs 1 Influence).
- **Mordor**: the Leader's **Just a Scratch**, answered "miss the next game"; a failed treatment attempt that lost its Influence;
  a Sergeant's **Wounds of a Hero**; an Injured warrior with no Influence left to treat it.
- **Arnor** (no match): a Sergeant with an Arm Wound, another **retired** by the second Leg Wound, a dead warrior in the
  cemetery (`Cemetery.Injure` and `Cemetery.Bury`).

### Level-ups

- Galadriel's Galadhrim: a warrior with a **roll waiting** (5 Experience, not rolled); one that rolled a **6, Hero in the Making**
  (a path to choose); one that rolled a 6 and **chose the Sorcerer path: its first magical power is still to choose**; one with
  **15 Experience (three rolls waiting)**; one that rolled a 2 (no change, Level 1); the Leader with **two rolls waiting** and a hero
  roll of 7 and 9 to choose between; a Sergeant with a roll waiting.
- Bilbo's Hobbits: a roll of 4 and a roll of 5 that leave a **promotion to choose**, and a warrior with 10 Experience.

### Units, creatures and limits

- **Radagast**: a Warg Marauder (three models on one card, On Their Own Path) and a **Hunting Dog** for an officer.
- **Pippin**: a **Baggage Pony** with two items in its **pack** (the Pack card), a **Falcon**, and a **Wandering Sage** (see
  [Gaps](#gaps)).
- **Gimli**: the **bow** and **throwing** limits both at 2 of 2 (the armoury refuses more, with the reason).
- **Éowyn**: a company at its 15 models with cavalry (the **cavalry** limit counts cavalry units).
- **Sauron** (Helm's Deep): the **race** limit (Elf 4 of 4) and a bow count above its limit, because recruits come with their gear.
- **Galadriel** (Rivendell): 12 of 12 models and the cavalry limit near full.

### Matches

A company is in **one open match at a time**, and a company with an aftermath still to play cannot start a new match. The
script therefore plays the matches that end with six pool companies (Éowyn, Sauron, Aragorn, Galadriel, Gimli, Bilbo) and
**waives their aftermaths** through the organiser's waive path (`WaiveAftermath`, note "Not played in the demo"), which frees them
for the next match. Every match that must stay open or unfinished has companies of its own, played by gallery accounts
(`<company>@example.com`, joined to this campaign). The aftermath wards play the aftermaths for real.

"Matches: every scenario and every state" has 24 matches; open the Matches page and filter by state:

- Match 1: **open, no scenario**.
- Matches 2 to 19: **one per rulebook scenario**, all 18 (the three tables of six). Every fourth (scenarios 1, 5, 9, 13, 17) is
  **open with the scenario chosen and nothing reported**; the others have a **winner and Victory Points**, the other side
  winning, or a **draw** (by turn), and are **ended with the aftermaths waived**. The scenarios with tickable objectives
  (**Scavenge the Battlefield**, **Relic of Ages Past**, **Baggage Train**, and the Rescue / Chance Encounter rewards) have
  their objective ticked for the first company when they have a result: open the match and look at the result form.
- A **one-off scenario** ("The Bridge of Sighs") with description and victory conditions, result entered, ended.
- A **three-sided match** (The Wolves Attack, third side wins), ended.
- A **two-sided match with two companies per side**, open.
- **Result entered, not ended** (Show of Strength): the next click is "End match".
- **Ended, aftermath not started** (Chance Encounter): both companies are locked until it is played.
- **Ended, reopened by an administrator, result corrected** (Hold the Line): the match history shows end, reopen and the new
  result; the match is open again.

### Aftermaths

Ward 1 (matches 1 to 4 of the campaign):

| Match | Company (owner) | State |
|---|---|---|
| 1, Seize Ground | Rohirrim waiting at the gate (Éowyn) | **not started** |
| | Orcs with a half-filled sheet (Sauron) | **draft sheet** saved (a casualty, a unit that fled, Against the Odds: Influence) |
| 2, Chance Encounter | Rangers with a Scratch to decide (Aragorn) | **injuries**: a Sergeant's *Just a Scratch* waits for "roll again / miss the next game" |
| | Galadhrim whose Leader fell (Galadriel) | **injuries**: the Leader is dead; **succession** waits for a new Leader |
| 3, Scavenge the Battlefield | Dwarves with level-ups to roll (Gimli) | **level-ups**: experience applied, level-up rolls waiting, and the **free equipment** reward (all markers scored) to choose before finishing |
| | Hobbits who finished (Bilbo) | **finished** |
| 4, Relic of Ages Past | Wild Wargs that fled (Radagast) | **All is Lost**: the company fled; its aftermath **waived** by the organiser with a note |
| | Pippin's patient company (Pippin) | **finished**, with the Relic bearer's perk *A Final Blessing* in the company's perks |

Ward 2 (a company in the next match has finished its aftermath, so the Hobbits' last game is against the Dwarves): a **draw** (Rohan finished; the Underdog Mordor took the three **Against the Odds** bonuses, has two **re-rolls**, one used
and one left, and its injuries are not yet applied); a **Rescue** where Arnor's Leader fell and the **succession is settled**
(a new Leader and a new Sergeant chosen) and finished; scenario rewards finished for the Elves (perk *Vantage Point*), the Dwarves
(*cargo*, +1 Influence) and the Hobbits (perk *Rendezvous*); the organiser's grants in the Dwarves' history.

Ward 3 (one match, three companies on one side): the **Warrior-to-Hero fallback**, built with the real services (experience granted by the organiser before
the match, casualties rolled with typed dice): the Orcs' Sergeant is dead and one Warrior is ahead in Experience (offered alone, already chosen), the
Dwarves' Sergeant is dead and two Warriors are tied for the most Experience (both listed, none chosen, an answer is required), and the Rangers lost the Leader
and both Sergeants (the Leader comes from the Warriors, then the Sergeants). The answer is mandatory in every case (no "nobody"); the promoted Warrior takes
its path on My Company once the aftermath is finished (before that the path window says "Finish the aftermath ... first").
The Elven Cloak: the Galadhrim of ward 1 (Galadriel) carry it as a unit upgrade badge with a tooltip.

## Gaps

States of the wish list that the services cannot produce, or that need raw SQL (the only raw SQL of the world, in
`world_settings.go` and `world_final.go`, run through one `pgx` connection):

- **Rounds** (`Round 3 of 12`): no service sets a campaign's round; the script updates `campaigns` directly. (Pausing and ending a campaign go through `SetCampaignStatus`.)
- **Expired and used-up invite links**: `Invites` has no clock to inject and no max-uses option; the script updates
  `campaign_invites` (`expires_at` in the past; `max_uses = uses = 1`). The valid link is real. The secrets are random per
  run: see the end of `tmp/demo-world.txt`.
- **A second rules edition**: only one vanilla and one Battle Companies file ship with the app. The "other edition" is a copy of
  the Battle Companies file with another name and version stored through `Rulesets.StoreAs`; it plays like the original.
- **A join request awaiting approval**: joining is by invite link only; the "pending join" state is a member with no company.

## Extending the world

`seedWorld` (`world.go`) runs the steps in order; each campaign is a function in `world_*.go` that creates its campaign, companies
and matches with helpers (`w.campaign`, `w.company`, `w.recruit`, `w.played`, `w.settle`, `w.rollLevel` ...). A step that the rules
may refuse uses `w.try` (it prints and goes on); a step that must work uses `w.must` (it aborts the script). A state the services
cannot reach is recorded with `w.gap`, which `TestSeedWorld` turns into a failure, so a new gap has to be documented here and
accepted deliberately. Dice: type the result when the campaign allows it, otherwise `w.rollWith(...)` fixes the server's dice.
