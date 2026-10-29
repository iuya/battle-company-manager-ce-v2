# Examples for customizing rulesets

Small, complete Battle Companies files (lean format, `schemaVersion` 2) that the guide [docs/customizing-rulesets.md](../../docs/customizing-rulesets.md) quotes.
Each one is a **whole file** that passes `make check-ruleset` and loads in the app, so it can be uploaded as it is (the Battle Companies file of a
ruleset, next to the shipped vanilla file and glossary). A real ruleset starts from a copy of
`definitions/battle_companies/battle_companies.json`; these files show one concept at a time, and the entries of each can be pasted into that copy.

```
make check-ruleset FILE=examples/customizing-rulesets/custom-unit.json
```

The texts in the examples are invented placeholders for the walkthrough ("Example text, not a Battle Companies rule: ..."). A real ruleset copies
the rule texts from the rulebook.

| File | Shows |
|---|---|
| [minimal.json](minimal.json) | The smallest valid file: the header, the `campaign` engine parameters and four scenarios. Every other example is this file plus one concept. |
| [custom-rule.json](custom-rule.json) | `rules[]`: a new rule, a rule with a `target` and an aura ring, a vanilla rule redefined with `overrides`, a vanilla rule annotated with a note. |
| [custom-unit.json](custom-unit.json) | The three forms of a unit: built on a vanilla unit (`extends`, `with`, `addRules`, `addKeywords`, in the company's own `units`), an annotation of a vanilla unit (`tts`), and a brand-new unit with no base (accepted by the checker; the app cannot field it yet); a company that fields the first. |
| [custom-item.json](custom-item.json) | `gear[]`: a new piece of equipment, a wargear item built on another (`extends`) and restricted with `who` and `limit`, an item that adds equipment slots, a vanilla item annotated so the armoury sells it. |
| [custom-path.json](custom-path.json) | A heroic path: dice, maximums, mastery, a Heroic Action, and a 2D6 table whose results are stats, a rule by name, an inline rule, a choice (`oneOf`) and an open choice (`anyMinorRule`). |
| [custom-company.json](custom-company.json) | A company: oath, starting force, reinforcement and special charts (a `choose` row, an `options` row), advancements, hero upgrades, local unit, rule and unique wargear, a hero promotion, `heroes.who`, a limit override, a granted rule, `elvenWeaponry`, and a second company that `extends` the first. |
| [custom-scenario.json](custom-scenario.json) | A scenario with text sections, a special rule and three scoring objectives (`tiers`, `each`, and a secret one with a `nomination`), placed on the scenario selection table. |
| [parameter-override.json](parameter-override.json) | Engine parameters changed in `campaign`: the Influence adjustment cap 5 instead of 3, and a few others. |

[broken/typos.json](broken/typos.json) is not an example to copy but a file with four typical mistakes, and [broken/typos.report.txt](broken/typos.report.txt) is the
report `make check-ruleset` prints for it, which the guide walks through.

`cmd/rulecheck/examples_test.go` runs all of them in `make test`: through the checker and the loader, each added to a copy of the shipped file, and the
parameters on the shipped file with the engine reading the new values. It also fails when the guide quotes a block that is no longer in its file.
