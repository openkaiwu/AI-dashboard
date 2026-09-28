# Upstream provenance

- [AI-dashboard](https://github.com/openkaiwu/AI-dashboard), commit e4c7ce94e51f68466468fe56aeb09b6162477006.
  Local Go quota/rule code and React pages derive from this repository. Changes: PostgreSQL SQL port, device auth, sync, client cache, packaging.
  No LICENSE file was present in the retrieved snapshot. This is a local development deliverable; confirm redistribution permission before a public release.
- [codex-quota-band](https://github.com/Vincent-hechuan/codex-quota-band), commit ef4b958e0b6904035ba92b8a0874bb2e06734b7a, MIT.
  Used as a design reference for device-scoped authorization, minimal summaries and freshness semantics. No Xiaomi SDK, Rust/Kotlin application code, or handband assets are bundled into this application.
- Linear Architecture Baseline and the M0 issues define the implementation target. No Linear ticket was changed.

The upstream clones were downloaded only for reference. They are excluded from application and source packages. During disk recovery they were partially relocated to ~/.cache/aihub-m0/upstream; they are not required at runtime.
