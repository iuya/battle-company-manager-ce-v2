# Security policy

## Reporting a vulnerability

Please report vulnerabilities privately, not in a public issue, pull request or discussion.

Use GitHub private vulnerability reporting: open the repository's **Security** tab and choose **Report a vulnerability**
(direct link: <https://github.com/iuya/battle-company-manager-ce-v2/security/advisories/new>). Include what you found, where (page, endpoint or file), the steps to reproduce it,
the version or commit, and what an attacker could do with it. Do not include real user data.

## Scope

In scope: the application in this repository and the published image `ghcr.io/iuya/battle-company-manager-ce-v2`, for example authentication and
sessions, CSRF, authorisation between campaigns and companies, injection, the rules-file upload, the Content-Security-Policy, leaks of
personal data, and the container and compose configuration shipped here.

Out of scope: a deployment's own configuration (weak passwords, an exposed database, a missing reverse proxy), vulnerabilities in third-party
software that have no effect through this app, denial of service by sheer volume, social engineering, and reports that need physical access.
Rules-data mistakes are not security issues: use the "Rules data" issue form.

## What to expect

This is a volunteer project. The maintainers aim to acknowledge a report within a few days, tell you whether they consider it valid,
and fix valid issues as soon as they reasonably can, then publish the fix in a new release and credit you if you wish.
These are intentions, not guarantees. Please keep the details private until a fix is released.

## Supported versions

Only the latest release receives fixes. Upgrade notes are in [docs/operations.md](docs/operations.md).
