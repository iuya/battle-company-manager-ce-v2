# Plan: what is left (2026-10-22)

State: the campaign loop, live match mode, end-by-all-participants, the unbuilt rules (Herbs, Cram, Against the Odds extras,
break point), the Ranger of the North backend, `make e2e`, production compose and hardening are built. Gates pass (`make test`,
`make test-integration`, validator). About 276 local commits are unpushed; `.beads/issues.jsonl` is uncommitted.
Working rules: one agent per role at a time, beads are the shared memory (`bd list`, `bd memories user-decisions-*`), agents commit
their own paths only, no `pkill`, reports short. Decisions already taken are in `docs/review-register.md` (Confirmed table).

## Order of work

1. **Ranger reshuffle and officer succession screens (frontend implementer, one agent).**
   `d1z` (recruit window swap option, officer panel step with the Ranger draggable between officer places and no remove button,
   My Company banner; add `PendingOfficers` and `ArrangeOfficers` to the web `campaigns` interface and regenerate the mock;
   the `officer_panel` partial exists from `1dx`; backend done in kmn/eye) then `6n1.2` (Sergeant replacement question and the
   mandatory Warrior-to-Hero fallback in the aftermath succession UI; backend done in `6n1.1`). Browser check, tests, docs.
2. **Curator: `vhn`** (separate 'situational' from 'cancelled by own gear' on gear stat bonuses), then close `36y.1`, `36y.2`,
   `36y.3` if the data is complete (it is, per the notes).
3. **Housekeeping (domain expert or one agent):** audit the open epics whose children are done and close them with notes (`amy`,
   `mtu`, `pib`, `36y`, `6uh`, `mqn`, `6lu` stays open for `6lu.9`); refresh `docs/review-register.md` (status after the 2026-10-22
   answers), `docs/security-and-risks.md` scores, bead `pdd` (feature map) and the feature map artifact
   (https://claude.ai/artifact/P4D9MZReMhPkZtcnFshEzY, source generator in the job tmp dir may be gone: rebuild from the doc).
4. **QA tester:** `mqn.159.3` (stream reconnect and poll fallback, formation drag and drop by hand-like events, axe-core pass,
   CI job, keyboard walk); add a permission matrix row for `ArrangeOfficers` (needs a Shire fixture with a fallen Ranger).
5. **CSP:** `mtu.14` / `mqn.94`: after about a week of real use of report mode, read the `/csp-report` log, fix violations, switch
   `CSP_MODE=enforce` for production (dev stays report). Very low priority.
6. **Concept graph for helper popups (`6lu.9`):** expert spec first (defaults saved on the bead), then build. Low priority.
7. **Full gates, then the owner reviews:** `make test`, `make test-integration`, `python3 tools/validate_definitions.py`,
   `go vet ./...`, `gofmt -l`; the owner runs `make demo`, plays the new flows, commits `.beads/issues.jsonl`, pushes, runs
   `bd dolt push` if wanted.

## Not planned (decided)
Calendar, standings, campaign end rules beyond the Manage campaign card, alliances, custom company creation (done in the json),
60-point check, join requests (invite only).

## Open questions for the owner
None blocking. Items to verify by hand: drag and drop in company creation and in the officer panel (headless Chrome cannot
simulate it), the Small Shield note, the secret objective labels, the tie wording on the MVP hint, mount-granted rules and
Cavalry in the live match, the promotion picker with several options.
