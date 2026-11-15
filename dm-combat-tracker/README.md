# dm-combat-tracker

A terminal UI, written in Go, for running tabletop combat from the DM's side of the screen.

It tracks initiative order, whose turn it is, hit points, conditions, area effects, and per-combatant notes, and it keeps an append-only Markdown log of the fight that you can use when writing a session recap.

It's built with D&D in mind but isn't tied to any one ruleset.

> Status: in progress. The domain core (`internal/combat`), autosave/resume (`internal/store`), the Markdown combat log (`internal/journal`), the app layer that ties them together (`internal/session`), and the TUI (`internal/tui`, `cmd/dmct`: the combat table, turn controls, and hotkeys to add/remove combatants, damage/heal, set temp HP, add/remove conditions, edit initiative and notes, write log entries, and end combat) are built and tested; creating and editing effects is next. See:
>
> - [docs/REQUIREMENTS.md](docs/REQUIREMENTS.md): what it must do
> - [docs/DESIGN.md](docs/DESIGN.md): how it's going to be built, and why
