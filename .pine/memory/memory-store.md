---
topic: memory-store
updated: 2026-07-14T14:24:58Z
---

# memory-store

- 2026-07-14: pine learn -g appends straight to the global MEMORY.md instead of routing through Suggest: Suggest seeds MEMORY.md at 0.15 but adds NEW:<slug> at 0.2 when no topic scores >= 0.4, and Confident() rejects any NEW: prefix — so any store without topics is always 'ambiguous'. That is also why a fresh project store errors on a novel insight. (cites: internal/memory/suggest.go)
- 2026-07-14: findPineDir accepts any dir named .pine that IsDir() with no config.json check, so anything that creates ~/.pine (e.g. pine learn -g) makes every command in a non-repo dir under $HOME resolve it as the project store and fail on its absent config.json. isGlobalOnlyStore skips it, but only when config.json is absent so a real project at that path still resolves. (cites: internal/cli/root.go)
