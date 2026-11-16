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
| D12 | **Durations count down only at the start of their anchor's turn**; an effect's anchor is its source (found through `SourceID`), or its own row when it has none. | Gives one rule that doesn't depend on any game system, and "N rounds" cast on the caster's turn lasts N full rounds. See [Durations](#durations). Rejected: setting start or end of turn for each item; counting down at the end of the turn; counting a sourced effect down on its own row's turn (the row comes right after the caster, so the first count fell in the round it was cast and a 1-round effect ended at once); nudging the effect's initiative to just above the caster's (a fake value, which the ordering rules exist to avoid). |
| D13 | **Ending combat archives the state file** as `dmct-state-<timestamp>.json`; the next combat starts with a new file. | Keeps the final state of a finished fight for reference. Rejected: resetting the file in place, asking each time. |
| D14 | **The condition list is built in for v1** (the D&D 5e preset); custom names can always be typed. | No config files until ruleset presets ([Future](REQUIREMENTS.md#future)). Rejected: an optional YAML/JSON condition file in v1. |
| D15 | **Durations don't tick on the opening turn** (`Start`). | Anything applied during Setup counts from its anchor's next turn, so a "1 round" condition set before combat isn't gone the instant the fight begins. |
| D16 | **An unsourced effect that expires at the start of its own turn hands the turn straight on** to the next row. | The effect row has nothing left to remind the DM of. One `Next` covers both, and `Previous` restores the effect. (A sourced effect expires as its caster's turn starts, so its row is simply gone.) |
| D17 | **Removing the row whose turn it is clears turn history.** | `Previous` can't point back at a row that no longer exists. Removing other rows keeps history; a rewind that would land on a removed row is refused. |
| D18 | **Group auto-fill needs a whole-number initiative**; a negative base fills downward (`-1 ×3` → `-1.1, -1.2, -1.3`). | Keeps decimals from spilling into the next whole number (`12.5 + 0.6`). The DM can still give separate initiatives. |
| D19 | **An `internal/session` package is the app layer** (D11): it runs an operation, saves, logs, and archives on end. | The TUI and command bar call one `Apply(op)` and never touch files, so milestones 3–6 don't each re-wire persistence. Save and log failures come back as warnings; the change is never rolled back. |
| D20 | **During Setup only manual notes are logged.** Adds, conditions and effects set up before the fight are summed up by the "Combat started" lineup. | Keeps prep noise out of the recap. Rejected: logging every setup action. |
| D21 | **The log file is bound when it's first needed** (the first note or the start of combat), not when the app opens. | The default name uses the date the combat starts, and an encounter that never logs anything doesn't create a file. |
| D22 | **Next / previous turn moves the selection to the new turn holder**; otherwise selection moves only with `↑↓`/`jk`. | The creature whose turn it is is usually the one the DM acts on next. The DM can still move away to act on another row. If the selected row disappears, selection lands on the row now in its place. |
| D23 | **Bubble Tea v2 / Lip Gloss v2** (`charm.land/...`), with `teatest/v2` for program-level tests. The table is drawn by hand rather than with `lipgloss/table`. | v2 is the current major version. A hand-drawn grid gives exact control over column dropping, truncation with `…`, and styling cells without styling the borders; view tests compare ANSI-stripped golden screens. |
| D24 | **Hotkey input happens in a prompt that takes over the two lines below the table** (status + key hints); the table never moves. Multi-part input (add, condition) is a short chain of prompts; questions (remove? keep or end effects?) are single-key choices. The line editor is hand-written rather than Bubbles' `textinput`. | Keeps the screen layout fixed at 80×24 and every flow testable by sending keys. The editor needs only insert, cursor moves, delete, `ctrl+u/k/w` and paste, so it isn't worth a dependency. Rejected: modal forms over the table (hide the rows the DM is looking at). |
| D25 | **`=N` in the damage or heal prompt sets HP directly** (R4); the add prompt takes `MAX` or `CURRENT/MAX`, each with an optional `+TEMP` (R3; `:add` also takes `-- NOTE`, the hotkey flow leaves the note to `N`), and a group's initiative is one whole number to share or one value per member, comma-separated. | Covers "set HP" and "add with current HP" without more hotkeys. Bad input is refused inside the prompt, which stays open with the reason, rather than closing and losing what was typed. |
| D26 | **Effect targets are picked on the table itself**: while the checklist is open the marker column shows `✓` / `·` on combatant rows, `↑↓`/`jk` move as usual, and `space` ticks the selected row and moves on. A new effect asks for its name, then its source (the selected combatant by default, or none, which needs an initiative), then its duration, then its targets. | The DM sees HP and conditions while choosing who is in the area, and the screen layout stays fixed (D24). The turn marker is hidden while picking; the title still names the turn holder. Rejected: a separate list overlay (hides the table). |
| D27 | **Command-bar names match in tiers**: an exact name (case and spaces ignored) wins; otherwise a name prefix (`band` → Bandit Captain) or a shortened base plus the number (`gob2`, `g2` → Goblin 2, never Goblin 20). More than one match at the winning tier is an error listing them. Where a target and a free-text part both follow the verb (`:cond goblin 2 hold person 3`), the longest leading run of words that resolves is the target. | Typing at the table needs to be short and forgiving, but never guess between two creatures. |
| D28 | **A refused command keeps the bar open** with the reason and the text, whether the parser or the domain refused it; only commands that ran go into the `↑↓` history. Multi-target commands check every target before changing anything. `:rm` of an effect's source asks keep-or-end like `x`; `:endcombat` doesn't ask for confirmation (typing it is the confirmation). | A typo costs one edit, not a retype, and a list command can't half-apply. |
| D29 | **Help is a scrollable screen drawn in place of the table**, built from the same key and command lists the code uses (a test checks every verb and hotkey is listed). Any key that isn't a scroll key closes it without acting. | One source of truth, so the help can't go stale. |
| D30 | **Narrow terminals degrade in tiers**: drop Note, then Kind, then narrow Init, HP, Name and Conditions; below 43×9 a "terminal too small" screen replaces the table and only `q`/`ctrl+c` work. Truncated names keep a trailing group number (`Gob… 2`), and when the selected row's name, conditions or note are cut off, the status line spells them out. | The DM can still tell goblins apart and read what's hidden by selecting the row. Keys are ignored on the too-small screen so nothing changes unseen. Rejected: wrapping rows onto two lines (breaks the fixed layout, D24). |
| D31 | **Save and log failures show a sticky red banner** under the title, which takes one table row. It stays up through prompts and other messages until a later change writes successfully; a recovered log reports how many changes it missed. While autosave is failing, `q` asks before quitting. | A transient status message was easy to miss and was cleared by the next prompt. The summary comes before the raw error, since narrow terminals cut the end off. |

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
- an effect (and so all its linked conditions) → its source; with no source (lair and environment effects, or a source that left and whose effects were kept) → the effect's own row

At the **start of the anchor's turn**, `Remaining` drops by 1; at 0 the item expires. So a "1 round" condition on a creature lasts until the start of that creature's next turn, and a 10-round effect cast on Mira's turn lasts until the start of her 10th following turn. The effect's own row still comes up right after its source as a reminder, without counting down. It's simple and predictable for any system. If a DM needs "until end of turn", they can remove the item by hand.

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
- At narrow widths the Note column is dropped first, then the Kind column, then the rest narrow; below 43×9 the screen asks for a bigger window (D30).
- While a prompt is open (D24) it replaces the status line, and its hint line shows completions, input help, or why the input was refused.
- Below the table, a status line shows the latest warning or error (a failed autosave, "no earlier turn"), else a tied-initiative warning, else the scroll position; the bottom line lists the keys that apply, dropping the least-used ones to fit the width (the full list is the `?` overlay).
- When there are more rows than fit, the table scrolls to keep the selection visible.
- Below the title, a red banner shows while autosave or the log is failing (D31).
- At narrow widths names keep their group number (`Gob… 2`), and the status line spells out the selected row's cut-off name, conditions and note (D30).

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
| `:add NAME [xN] INIT HP [pc\|npc] [-- NOTE]` | `:add Goblin x4 12 7` · `:add Thorin 18 31/44+5 pc -- cursed` |
| `:rm TARGET` | `:rm goblin 2` |
| `:dmg TARGETS N` | `:dmg g1,g3 9` |
| `:heal TARGETS N` | `:heal thorin 8` |
| `:hp TARGET N` | `:hp g2 3` (set HP directly) |
| `:temp TARGETS N` | `:temp mira 5` |
| `:cond TARGETS NAME [ROUNDS] [-- NOTE]` | `:cond g3 prone` · `:cond thorin blessed 10` |
| `:uncond TARGET NAME` | `:uncond g3 prone` |
| `:fx NAME [ROUNDS] [from SOURCE] [at INIT] [on TARGETS]` | `:fx Spirit Guardians 10 from mira on g1,g3` · `:fx Lair rumble at 20` |
| `:fxadd FX TARGETS` / `:fxrm FX TARGETS` | `:fxrm spirit g1` |
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
- **`cmd/dmct` (end to end):** the built binary runs in a pseudo-terminal (`creack/pty`) and a terminal emulator (`charmbracelet/x/vt`) turns its output back into the screen, so each test types a story from the requirements and checks what the DM would see plus the state, archive and log files left behind: a full fight, quit and resume, step-by-step rewind (and where it is refused), removing an effect's source, ties and expiry, help and terminal sizes, a failing autosave, and refusals at startup. Unix only; both libraries are test-only dependencies.

```bash
go test ./...                 # unit + end-to-end
go test -cover ./...          # target: 90% or more in every package
```

## Milestones

1. ✅ **Domain core:** `internal/combat` model, ordering, turns, HP, conditions, effects, events, with tests.
2. ✅ **Persistence + journal:** autosave, resume, Markdown log.
3. ✅ **Read-only TUI:** table rendering, selection, next/previous turn.
4. ✅ **Editing via hotkeys:** add/remove, damage/heal/temp HP, conditions, initiative, notes, log entries, end combat. Removing an effect's source asks keep-or-end here already, since removal needs it.
5. ✅ **Effects:** new effect (`e`), edit targets (`T`), end/dispel early (`x` on an effect row), on top of the effect rows and linked conditions drawn since milestone 3.
6. ✅ **Command bar + help overlay.**
7. ✅ **Polish:** narrow-terminal layout, error banners, README usage docs.

## Planned: rulesets

Not started. The requirements are [Rulesets](REQUIREMENTS.md#rulesets) (gaps G1–G9, sub-features RS1–RS9). v1 hard-codes the 5e answers in three places: `health.go` (clamp at 0), `Combatant.Down()` (one state at HP ≤ 0) and `DefaultConditions`.

### Model sketch

```go
type Ruleset struct {
    Name        string        // "5e", "3.5e"
    HPFloor     *int          // nil = no floor; 5e = 0
    States      []HealthState // ordered; first whose range holds HP wins
    Nonlethal   bool
    FixedAnchor bool          // effects count down on their starting count (RS7)
    Conditions  []string
}

type HealthState struct {
    Name     string // "down", "disabled", "dying", "dead"
    Min, Max *int   // inclusive HP range; nil = open
    Acts     bool   // still takes turns normally (3.5 disabled)
}
```

- `Encounter` gains `Ruleset` (saved in the state file; absent means `5e`).
- `Combatant` gains `Nonlethal int`. `Down()` becomes `State() *HealthState`; the `Downed`/`Revived` events become one `HealthStateChanged{From, To}`.
- `Condition` gains an optional `SourceID`. `Effect` gains `AnchorInit`, the count it ticks on when the ruleset has `FixedAnchor`.
- `tick` changes from "everything anchored to this row" to "everything anchored to this row *or this initiative count*". A fixed-count anchor with no row at that count needs a tick as the turn order passes it; see open question 1.

### Build order

Each step keeps `5e` behavior identical to v1, checked by the existing unit, golden-screen and E2E suites.

| Milestone | Sub-features | Notes |
|---|---|---|
| 8. Ruleset core | RS1, RS8 | Adds the `Ruleset` type with the two built-ins, the Setup flag and command, the header label, and per-ruleset condition lists. No behavior change yet beyond the picker. |
| 9. HP model | RS2, RS3 | Floor and health states, the event change, table styling, log wording. This is the part that makes 3.5 usable (G1–G3). |
| 10. HP extras | RS4, RS5, RS6 | Nonlethal, max HP edit, `+N` temp HP. Independent of each other; can be split. |
| 11. Duration anchors | RS7 | Condition sources and fixed-count effects, including `Previous` rewinding both. The riskiest step: it touches `tick`, `TurnRecord` and removal. |
| 12. Ruleset files | RS9 | File format, validation, built-ins moved to the same form. |

Milestones 8 and 9 alone are enough to run a 3.5 fight; 10–12 remove workarounds.

## Open questions

The end-combat, duration, and condition-list questions were settled as D13, D12, and D14. Open for the ruleset work:

1. **Fixed-count ticking (RS7).** When a 3.5 caster delays, their effect should still tick at the old count, where there may be no row. Options: tick as `advance` passes that count, or pin the effect's own row to the old count and tick on the row (the row exists already and keeps its own `Initiative`). The second looks simpler and reuses the unsourced-effect path.
2. **Correcting an initiative versus Delay/Ready.** With a fixed anchor, fixing a mistyped initiative shouldn't leave the caster's effects behind on the wrong count. Either the edit asks "move effects too?", or there are two commands.
3. **State-file format.** Whether to store the whole ruleset in the state file or only its name. Storing all of it keeps an archived fight readable after a ruleset file changes.
4. **HP floor in 3.5.** No floor at all, or stop at -10 since anything lower is equally dead.
