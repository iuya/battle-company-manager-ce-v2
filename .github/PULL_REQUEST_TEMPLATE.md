<!-- Keep every heading and checklist line: the pr-check workflow reads them. The title must be a Conventional Commit, e.g. "fix(web): keep the roster on one line". -->

## Summary

<!-- What changes and why, in a few lines. -->

## Type

<!-- Tick at least one. It should match the type in the title. -->

- [ ] feat: new behaviour
- [ ] fix: bug fix
- [ ] perf: performance
- [ ] refactor: no behaviour change
- [ ] docs: documentation only
- [ ] test: tests only
- [ ] build / ci: build, dependencies or workflows
- [ ] chore: anything else
- [ ] Breaking change (title has `!` or the description a `BREAKING CHANGE:` note)

## Linked issue

<!-- "Closes #123", or "None" for a trivial change. Non-trivial changes start as an issue. -->

## How it was tested

<!-- Commands you ran (make test, make test-integration, make e2e...) and what you checked by hand. -->

## Checklist

- [ ] `make test` passes
- [ ] Documentation under `docs/` is updated, or the change needs none
- [ ] Rules-data changes are validated with `make validate`, or the change touches no rules data
- [ ] UI changes are checked at 360, 768 and 1280 px in light and dark (docs/frontend.md), or the change has no UI
- [ ] No secrets, tokens or personal data are included
- [ ] Licence and copyright: I wrote this or have the right to contribute it under CC BY-NC-SA 4.0, and it contains no copyrighted rulebook text
