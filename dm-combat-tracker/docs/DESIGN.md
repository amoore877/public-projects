# Design

This doc covers how the requirements in [REQUIREMENTS.md](REQUIREMENTS.md) will be built, and why.

## Decision log

Each entry is a decision made during planning, with what it rules out.

| # | Decision | Rationale / rejected alternatives |
|---|---|---|
| D1 | **Go + Charm stack** ([Bubble Tea](https://github.com/charmbracelet/bubbletea), [Bubbles](https://github.com/charmbracelet/bubbles), [Lip Gloss](https://github.com/charmbracelet/lipgloss)). | The usual choice for Go TUIs: Elm-style update loop, table/textinput components, and a test harness (`teatest`). Rejected: tview (widget-heavy, harder to test), raw tcell (too low-level). |
| D2 | **Not tied to a game system**, with D&D 5e conditions as the default preset. | Keeps the tool usable for other games. No dice, no rules automation (R8). |
| D3 | **Initiative is a hand-entered decimal**; ties are broken by the DM entering decimals (`15.3 > 15.2 > 15.1`). | Simpler and clearer than an app-managed tie-break list. Rejected: app rolling, a tie-break-by-modifier rule. |
| D4 | **Initiative is stored as fixed-point** (`int64` thousandths), not `float64`. | Exact comparisons and round-tripping: `15.1` stays `15.1`, and exact-tie detection is reliable. Up to 3 decimal places are accepted. |
| D5 | **Effects are table rows *and* show up as linked conditions on their targets**, plus an explicit **end/dispel early** control. | The DM sees the effect come up in turn order, and each affected creature shows that it's affected. Linked conditions are *derived* from the effect (never copied), so ending the effect clears everything in one step. |
| D6 | **Autosave the full state file after every change**, written atomically (temp file + `fsync` + `rename`). | A crash or closed laptop never loses the fight. Rejected: manual save/load, in-memory only. |
| D7 | **Append-only Markdown log**, with manual one-line entries plus automatic entries for key events. | Readable with no tools when writing a recap. Rejected: JSON Lines (not as readable; it can be added later as a second sink if needed). |
| D8 | **Group adds create auto-numbered individual combatants**, sharing one initiative with auto-filled decimals by default. | Each goblin needs its own HP and conditions. Rejected: a single group row with pooled HP. |
| D9 | **Hotkeys plus a `:` command bar.** | Hotkeys for common actions on the selected row; the command bar for exact or bulk input. Rejected: modal-forms-only, command-only. |
| D10 | **DM-only view in v1.** | A player view is in [Future](REQUIREMENTS.md#future). Nothing in the domain model rules it out. |
| D11 | **Domain operations return events**; the app layer sends them to both the journal and autosave. | Keeps the domain package pure (no I/O) and fully unit-testable; the log is just one consumer of events. |
| D12 | **Durations count down only at the start of their anchor's turn.** | Gives one rule that doesn't depend on any game system. See [Durations](#durations). Rejected: setting start or end of turn for each item, and counting down at the end of the turn. |
| D13 | **Ending combat archives the state file** as `dmct-state-<timestamp>.json`; the next combat starts with a new file. | Keeps the final state of a finished fight for reference. Rejected: resetting the file in place, asking each time. |
| D14 | **The condition list is built in for v1** (the D&D 5e preset); custom names can always be typed. | No config files until ruleset presets ([Future](REQUIREMENTS.md#future)). Rejected: an optional YAML/JSON condition file in v1. |
| D15 | **Durations don't tick on the opening turn** (`Start`). | Anything applied during Setup counts from its anchor's next turn, so a "1 round" condition set before combat isn't gone the instant the fight begins. |
| D16 | **An effect that expires at the start of its own turn hands the turn straight on** to the next row. | The effect row has nothing left to remind the DM of. One `Next` covers both, and `Previous` restores the effect. |
| D17 | **Removing the row whose turn it is clears turn history.** | `Previous` can't point back at a row that no longer exists. Removing other rows keeps history; a rewind that would land on a removed row is refused. |
| D18 | **Group auto-fill needs a whole-number initiative**; a negative base fills downward (`-1 ×3` → `-1.1, -1.2, -1.3`). | Keeps decimals from spilling into the next whole number (`12.5 + 0.6`). The DM can still give separate initiatives. |
| D19 | **An `internal/session` package is the app layer** (D11): it runs an operation, saves, logs, and archives on end. | The TUI and command bar call one `Apply(op)` and never touch files, so milestones 3–6 don't each re-wire persistence. Save and log failures come back as warnings; the change is never rolled back. |
| D20 | **During Setup only manual notes are logged.** Adds, conditions and effects set up before the fight are summed up by the "Combat started" lineup. | Keeps prep noise out of the recap. Rejected: logging every setup action. |
| D21 | **The log file is bound when it's first needed** (the first note or the start of combat), not when the app opens. | The default name uses the date the combat starts, and an encounter that never logs anything doesn't create a file. |
| D22 | **Next / previous turn moves the selection to the new turn holder**; otherwise selection moves only with `↑↓`/`jk`. | The creature whose turn it is is usually the one the DM acts on next. The DM can still move away to act on another row. If the selected row disappears, selection lands on the row now in its place. |
| D23 | **Bubble Tea v2 / Lip Gloss v2** (`charm.land/...`), with `teatest/v2` for program-level tests. The table is drawn by hand rather than with `lipgloss/table`. | v2 is the current major version. A hand-drawn grid gives exact control over column dropping, truncation with `…`, and styling cells without styling the borders; view tests compare ANSI-stripped golden screens. |
| D24 | **Hotkey input happens in a prompt that takes over the two lines below the table** (status + key hints); the table never moves. Multi-part input (add, condition) is a short chain of prompts; questions (remove? keep or end effects?) are single-key choices. The line editor is hand-written rather than Bubbles' `textinput`. | Keeps the screen layout fixed at 80×24 and every flow testable by sending keys. The editor needs only insert, cursor moves, delete, `ctrl+u/k/w` and paste, so it isn't worth a dependency. Rejected: modal forms over the table (hide the rows the DM is looking at). |
| D25 | **`=N` in the damage or heal prompt sets HP directly** (R4); the add prompt takes `MAX` or `CURRENT/MAX`, and a group's initiative is one whole number to share or one value per member, comma-separated. | Covers "set HP" and "add with current HP" without more hotkeys. Bad input is refused inside the prompt, which stays open with the reason, rather than closing and losing what was typed. |

## Layout

```
dm-combat-tracker/
├── go.mod                     module github.com/amoore877/public-projects/dm-combat-tracker
├── cmd/dmct/main.go           flags, wiring, program start
├── internal/combat/           domain model + operations (pure, no I/O)
├── internal/command/          ":" command-bar parser → combat operations
├── internal/journal/          Markdown log writer (consumes combat events)
├── internal/session/          app layer: apply op → autosave → log → archive on end
├── internal/store/            state file load / atomic save / archive
├── internal/tui/              Bubble Tea model, table view, keymap, overlays
└── docs/
```

The project has its own `go.mod`, as `AGENTS.md` asks; it doesn't depend on sibling projects.

## Domain model (`internal/combat`)

```go
type Initiative int64 // thousandths: 15.3 → 15300

type Encounter struct {
    SchemaVersion int
    Status        Status        // Setup | Active | Ended
    Round         int           // 0 during Setup
    Current       RowID         // whose turn; zero during Setup
    Combatants    []*Combatant
    Effects       []*Effect
    History       []TurnRecord  // for "previous turn" (see below)
    LogPath       string        // log file bound to this encounter
    NextID        int           // ID counter (shared by combatants, effects, conditions)
    NextSeq       int           // AddedSeq counter
}

type Combatant struct {
    ID         RowID
    Name       string
    Kind       Kind          // PC | NPC
    Initiative Initiative
    HP, MaxHP  int
    TempHP     int
    Conditions []*Condition  // own conditions only; effect-linked ones are derived
    Note       string
    AddedSeq   int           // insertion order, used for the tie fallback
}

type Condition struct {
    ID        RowID
    Name      string
    Remaining *int           // nil = indefinite
    Note      string
}

type Effect struct {
    ID         RowID
    Name       string
    SourceID   RowID          // zero for environmental/lair effects
    Initiative Initiative     // defaults to the source's initiative
    Remaining  *int
    Targets    []RowID
    Note       string
    AddedSeq   int
}
```

### Ordering

Rows (combatants and effects together) are sorted by:

1. `Initiative` descending
2. combatants before effects (so an effect at its source's initiative comes up **right after** the source, without a fake `14.999` value)
3. `AddedSeq` ascending

If two **combatants** have exactly the same initiative, the UI shows a warning marker asking the DM to enter a decimal (R2); the order still works using rule 3.

### Group initiative fill (D8)

`Goblin x6 @ 12` → `12.6, 12.5, …, 12.1`. The number of decimal places used is the fewest that fit the count (`n ≤ 9` → 1 place, `n ≤ 99` → 2 places: `12.12 … 12.01`). If an auto-filled value collides with an existing combatant, the DM is warned rather than having values silently shifted.

### Turn advance

`Next()`:

1. Move `Current` to the next row in order; wrapping around increments `Round` and emits `RoundStarted`.
2. When the new current row's turn starts, count down durations anchored to it (see below), emitting `ConditionExpired` / `EffectExpired` and removing expired items.
3. Push a `TurnRecord` with the previous pointer and round, plus every duration change and removal from step 2.

`Previous()` pops the latest `TurnRecord`. It restores the pointer and round, and **reverses the duration changes from that advance** (restoring decremented counters and expired items). It does **not** undo HP or other edits made during the turn, and doesn't remove log lines (the log is append-only); it writes a "Turn rewound to X" entry. `History` is capped at the last ~50 advances.

### Durations

<a id="durations"></a>A duration is a count of rounds tied to an **anchor**:

- a combatant's own condition → that combatant
- an effect (and so all its linked conditions) → the effect's row

At the **start of the anchor's turn**, `Remaining` drops by 1; at 0 the item expires. So a "1 round" condition on a creature lasts until the start of that creature's next turn, and a 10-round effect lasts until the start of its row's 10th following turn. It's simple and predictable for any system. If a DM needs "until end of turn", they can remove the item by hand.

### Removing combatants

- If the removed combatant was `Current`, the turn passes to the next row **without** counting down that row's durations twice; it's treated like a normal advance.
- The combatant is taken off every effect's `Targets`.
- If they were the source of any effects, the operation returns a `NeedsDecision` result, and the TUI asks "keep or end these effects?" (R5). Kept effects keep their initiative position; `SourceID` is cleared.

### HP rules

- `Damage(n)`: take from `TempHP` first, then `HP`; clamp at 0. Crossing to 0 emits `Downed`.
- `Heal(n)`: `HP = min(HP+n, MaxHP)`; going from 0 to above 0 emits `Revived`. Temp HP is unchanged.
- `SetTempHP(n)` replaces the value (temp HP doesn't stack).
- `SetHP(n)` is a direct override, clamped to `[0, MaxHP]`, and emits `Downed`/`Revived` when it crosses 0.
- "Bloodied" (`HP ≤ MaxHP/2`) is only a display state.

### Events (D11)

Every mutating operation returns `[]Event`, for example:

`CombatStarted`, `CombatEnded`, `RoundStarted`, `TurnStarted`, `TurnRewound`, `CombatantAdded`, `CombatantRemoved`, `Damaged`, `Healed`, `Downed`, `Revived`, `ConditionApplied`, `ConditionRemoved`, `ConditionExpired`, `EffectStarted`, `EffectTargetsChanged`, `EffectEnded` (early), `EffectExpired`, `NoteLogged`.

Each event carries the round, the turn-holder's name, and a timestamp (the clock is injected, so tests are deterministic).

## Persistence (`internal/store`)

- JSON, indented, with `schemaVersion` so the format can change later.
- After every operation that changes state: marshal → write `<path>.tmp` → `fsync` → `rename` onto `<path>`.
- On startup, if the state file exists and `Status != Ended`, the app resumes. Otherwise it starts in Setup.
- **End combat** (D13) saves the final state, then renames the file to `dmct-state-<YYYYMMDD-HHMMSS>.json` next to it. The next combat starts in Setup with a new state file.
- The state stores `LogPath`, so a resumed encounter keeps writing to the same log even if the date has changed.
- A state file that can't be parsed, or has a newer `schemaVersion`, is refused and left untouched for the DM to look at.
- A state file found in `Ended` (a crash between ending and archiving) is archived on startup, and a new combat begins.
- Archive names that collide (two fights ended in the same second) get a `-2`, `-3`, … suffix.

### CLI

```
dmct [--state FILE] [--log FILE]
```

| Flag | Default |
|---|---|
| `--state` | `./dmct-state.json` |
| `--log` | `./dmct-log-YYYY-MM-DD.md` (the date the combat starts) |

## Combat log (`internal/journal`)

Append-only. The file is opened with `O_APPEND|O_CREATE|O_WRONLY` and never read back by the app.

```markdown
# Combat — 2026-11-14 19:42

## Round 1

- `19:42` **[R1 · Thorin]** Combat started: Thorin (18), Goblin 1–4 (12.4–12.1), Mira (9)
- `19:43` **[R1 · Thorin]** Goblin 2 dropped to 0 HP
- `19:44` **[R1 · Goblin 1]** 📝 Goblin 1 tried to flee toward the bridge
- `19:45` **[R1 · Mira]** Effect started: Spirit Guardians (Mira, 10 rounds) on Goblin 1, Goblin 3

## Round 2

- `19:47` **[R2 · Mira]** Effect ended early: Spirit Guardians (concentration broken)
- `19:52` **[R2 · Mira]** Combat ended after 2 rounds
```

- Manual notes are marked with 📝 so they stand out when skimming for the recap.
- Logged: combat started/ended, round headings, turn rewound, combatant added/removed, down/revived, conditions applied/removed/expired, effects started/retargeted/ended/expired, notes. Not logged: plain turn changes (the turn holder is on every line), HP arithmetic, initiative and side-note edits.
- During Setup only notes are logged, stamped `[Setup]` (D20).
- A resumed fight doesn't repeat its `# Combat` or `## Round` headings. Rewinding over a round boundary doesn't repeat the heading either; each entry's own `[R…]` stamp is the source of truth.
- Several combats in one session add more `# Combat — …` sections to the same file.
- Writing to the log is best-effort: if it fails, the TUI shows an error banner but combat state is never rolled back.

## TUI (`internal/tui`)

### Screen

```
 Round 3 ─ Turn: Goblin 1                                   [?] help
 ┌───┬────────┬──────────────────┬──────┬────────────┬──────────────────────────┬──────────────┐
 │   │ Init   │ Name             │ Kind │ HP         │ Conditions               │ Note         │
 ├───┼────────┼──────────────────┼──────┼────────────┼──────────────────────────┼──────────────┤
 │   │ 18     │ Thorin           │ PC   │ 31/44      │ Blessed (2)              │              │
 │ ▶ │ 12.4   │ Goblin 1         │ NPC  │ 7/7        │ ◆ Spirit Guardians       │ fleeing      │
 │   │ 12.3   │ Goblin 2         │ NPC  │ DOWN 0/7   │                          │              │
 │   │ 12.2   │ Goblin 3         │ NPC  │ 3/7 ▼      │ Prone, ◆ Spirit Guard…   │              │
 │   │ 9      │ Mira             │ PC   │ 22/27 +5   │ Concentrating            │              │
 │   │ 9      │ ◆ Spirit Guard…  │ FX   │            │ 8 rds · 2 targets        │ 3d8 radiant  │
 └───┴────────┴──────────────────┴──────┴────────────┴──────────────────────────┴──────────────┘
 :dmg gob3 4_
```

- `▶` marks the current turn; a cursor highlight marks the **selected** row (which hotkeys act on), and the two can differ.
- `◆` marks effect rows and effect-linked conditions; `▼` = bloodied; `+5` = temp HP; `DOWN` rows are dimmed.
- At narrow widths the Note column is dropped first, then the Kind column.
- While a prompt is open (D24) it replaces the status line, and its hint line shows completions, input help, or why the input was refused.
- Below the table, a status line shows the latest warning or error (a failed autosave, "no earlier turn"), else a tied-initiative warning, else the scroll position; the bottom line lists the keys that apply, dropping the least-used ones to fit the width (the full list is the `?` overlay).
- When there are more rows than fit, the table scrolls to keep the selection visible.

### Hotkeys (act on the selected row)

| Key | Action |
|---|---|
| `n` / `space` | Next turn |
| `p` | Previous turn |
| `↑↓` / `j k` | Move selection |
| `g` / `G` | Select the first / last row |
| `d` / `h` | Damage / heal (prompts for amount; `=N` sets HP) |
| `t` | Set temp HP |
| `c` / `C` | Add condition (preset picker + custom) / remove condition |
| `e` | New effect (source defaults to the selected combatant; pick targets from a checklist) |
| `x` | On an effect row: end/dispel early. On a combatant: remove (with confirmation) |
| `T` | Edit an effect's targets |
| `a` | Add combatant / group |
| `i` | Edit initiative |
| `N` | Edit the row's side note |
| `l` | Write a log entry |
| `:` | Command bar |
| `?` | Help overlay |
| `S` / `E` | Start combat (from Setup) / end combat (with confirmation) |
| `q` | Quit (state is already saved) |

### Command bar (D9)

Targets are matched case-insensitively against names, with spaces optional (`goblin 2`, `gob2`, `g2`); an ambiguous match gives an error that lists the candidates. A comma-separated list targets several rows.

| Command | Example |
|---|---|
| `:add NAME [xN] INIT HP [pc\|npc]` | `:add Goblin x4 12 7` · `:add Thorin 18 44 pc` |
| `:rm TARGET` | `:rm goblin 2` |
| `:dmg TARGETS N` | `:dmg g1,g3 9` |
| `:heal TARGET N` | `:heal thorin 8` |
| `:temp TARGET N` | `:temp mira 5` |
| `:cond TARGETS NAME [ROUNDS] [-- NOTE]` | `:cond g3 prone` · `:cond thorin blessed 10` |
| `:uncond TARGET NAME` | `:uncond g3 prone` |
| `:fx NAME [ROUNDS] [from SOURCE] on TARGETS` | `:fx Spirit Guardians 10 from mira on g1,g3` |
| `:fxadd` / `:fxrm FX TARGETS` | `:fxrm spirit g1` |
| `:end FX [-- REASON]` | `:end spirit -- concentration broken` |
| `:init TARGET VALUE` | `:init g2 12.35` |
| `:note TARGET TEXT` | `:note g1 fleeing` |
| `:log TEXT` | `:log Thorin shoves the goblin off the bridge` |
| `:next` / `:prev` / `:start` / `:endcombat` | |

The parser in `internal/command` only turns text into combat operations; the hotkeys call those same operations, so both input paths behave the same.

## Testing

- **`internal/combat`:** table-driven tests for ordering, tie fallback, group fill, advance/rewind (including duration reversal), removal edge cases (removing the current turn, removing an effect's source), and HP rules.
- **`internal/command`:** parser tests, including name matching and ambiguity errors.
- **`internal/journal`:** golden-file tests for Markdown output, using a fake clock.
- **`internal/store`:** round-trip and atomic-write tests (no `.tmp` left behind; the previous file survives an interrupted write).
- **`internal/session`:** resume after restart, archive on end, rejected operations save nothing, log failures don't block saves.
- **`internal/tui`:** `teatest` smoke tests for the main flows.

## Milestones

1. ✅ **Domain core:** `internal/combat` model, ordering, turns, HP, conditions, effects, events, with tests.
2. ✅ **Persistence + journal:** autosave, resume, Markdown log.
3. ✅ **Read-only TUI:** table rendering, selection, next/previous turn.
4. ✅ **Editing via hotkeys:** add/remove, damage/heal/temp HP, conditions, initiative, notes, log entries, end combat. Removing an effect's source asks keep-or-end here already, since removal needs it.
5. **Effects:** effect rows, linked conditions, targets, end/dispel early.
6. **Command bar + help overlay.**
7. **Polish:** narrow-terminal layout, error banners, README usage docs.

## Open questions

None right now. The end-combat, duration, and condition-list questions were settled as D13, D12, and D14.
