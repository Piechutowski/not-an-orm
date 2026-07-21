# SQLBoiler documentation (reference copy)

The SQLBoiler documentation used to build
[`../../orm-capability-matrix.md`](../../orm-capability-matrix.md).
SQLBoiler (https://github.com/aarondl/sqlboiler) is the original
database-first Go ORM generator — the project whose "ActiveRecord
productivity, Go-like feel, no reflection" goals this project shares,
and whose design Bob (see [`../bob/`](../bob/)) inherits and revises.
It is in maintenance mode, so its README documents the *settled* form
of the design: query mods, finishers, `R`/`L` relationship structs,
eager loading via `Load`, relationship set-ops (`SetX`/`AddX`/`RemoveX`),
hooks, auto timestamps, soft delete. The differences between SQLBoiler
and Bob are a map of which parts of that design its own author
considered worth changing. Refresh with `./fetch.sh`.

- `sqlboiler_readme.md` — verbatim copy of the repository README, the
  project's entire manual (features & examples, configuration/aliasing,
  query mods, relationships, hooks, FAQ, benchmarks).

SQLBoiler is © Aaron L and contributors, BSD-3-Clause.
