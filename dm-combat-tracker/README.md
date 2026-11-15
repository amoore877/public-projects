# dm-combat-tracker

A terminal UI, written in Go, for running tabletop combat from the DM's side of the screen.

It tracks initiative order, whose turn it is, hit points, conditions, area effects, and per-combatant notes, and it keeps an append-only Markdown log of the fight that you can use when writing a session recap.

It's built with D&D in mind but isn't tied to any one ruleset.

> Status: in progress. The domain core (`internal/combat`), autosave/resume (`internal/store`), the Markdown combat log (`internal/journal`), the app layer that ties them together (`internal/session`), and a read-only TUI (`internal/tui`, `cmd/dmct`: the combat table, selection, start and next/previous turn) are built and tested; editing via hotkeys is next. See:
>
> - [docs/REQUIREMENTS.md](docs/REQUIREMENTS.md): what it must do
> - [docs/DESIGN.md](docs/DESIGN.md): how it's going to be built, and why
