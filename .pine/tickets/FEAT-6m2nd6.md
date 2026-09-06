---
id: FEAT-6m2nd6
title: Hide done tickets from default pine list
status: done
priority: medium
created: "2026-09-06T06:27:06Z"
updated: "2026-09-06T06:28:45Z"
---

# Description

# Acceptance Criteria
- [ ] Define acceptance criteria

# Implementation Plan

# Notes

# Related Files

# Attachments

## Work Evidence

Closed by `pine close --evidence` on 2026-09-06.

- Base: `49504349` (last commit at or before ticket created 2026-09-06)
- Files changed (base → working tree):

```
 .agents/skills/pine/SKILL.md      |   2 +-
 .claude/skills/pine/SKILL.md      |   2 +-
 README.md                         |   3 +-
 internal/cli/cli_test.go          | 110 ++++++++++++++++++++++++++++++++++++++
 internal/cli/tickets.go           |  26 ++++++---
 internal/setup/templates/skill.md |   2 +-
 6 files changed, 135 insertions(+), 10 deletions(-)
```
