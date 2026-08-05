---
topic: config
updated: 2026-07-14T13:38:17Z
---

# config

- 2026-07-14: Adding a config key needs FOUR touch points or it fails silently: the struct field, Default(), a case in parseOnto(), and a pair in MarshalJSON(). Missing the parseOnto case routes it to Extra (untyped); missing the MarshalJSON pair drops it on every save. Default-true works because parseOnto unmarshals ONTO a Default()-seeded struct, never a zero one. (cites: internal/config/config.go)
