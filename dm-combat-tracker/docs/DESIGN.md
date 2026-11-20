# Design

This doc covers how the requirements in [REQUIREMENTS.md](REQUIREMENTS.md) will be built, and why.

## Decision log

Each entry is a decision made during planning, with what it rules out.

| # | Decision | Rationale / rejected alternatives |
|---|---|---|
| D1 | **Go + Charm stack** ([Bubble Tea](https://github.com/charmbracelet/bubbletea), [Bubbles](https://github.com/charmbracelet/bubbles), [Lip Gloss](https://github.com/charmbracelet/lipgloss)). | The usual choice for Go TUIs: Elm-style update loop, table/textinput components, and a test harness (`teatest`). Rejected: tview (widget-heavy, harder to test), raw tcell (too low-level). |
| D2 | **Not tied to a game system**, with D&D 5e conditions as the default preset. | Keeps the tool usable for other games. No dice, no rules automation (R8). Dice revised by D45: a roller that only shows results. |
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
| D14 | **The condition list is built in** (the D&D 5e preset in v1, now one list per built-in [ruleset](#rulesets)); custom names can always be typed. | No config files until ruleset presets ([Future](REQUIREMENTS.md#future)). Rejected: an optional YAML/JSON condition file in v1. |
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
| D32 | **In `3.5e` a sourced effect's row stays on the count it started on, and the effect counts down on that row** (RS7, G7), sorted just before its source on a tie so it ends just before that count comes up. In `5e` it still follows its source (D12). | Reuses the unsourced-effect path; the row already has its own `Initiative`. Rejected: an `AnchorInit` field ticked as `advance` passes the count, which needs crossing checks with wrap-around in both `Next` and `Previous`. |
| D33 | **In `3.5e`, changing the initiative of a creature that is the source of effects asks "move its effects too?"** Yes for a mistyped initiative, no for Delay or Ready. | One command and no new concept, and it only asks when the answer matters. Rejected: a separate `:delay` command alongside an initiative edit that always moves the effects. |
| D34 | **A condition can name a source; it then counts down at the start of the source's turn** (RS7, G8). The `c` flow asks only for a timed condition, only during combat, and only offers the turn holder (enter keeps the bearer); `:cond … by SOURCE` names anyone. When the source leaves, its conditions count down on their bearer's turn again. | The usual case is "until the start of your next turn", cast on the caster's turn. A condition has no row to hold the source's old count, so following the bearer is the only rule that needs no hidden anchor. Rejected: a source picker over every combatant in the `c` flow (more keys for the rare case). |
| D35 | **The state file stores only the ruleset's name** (RS9), as it has since milestone 8. | Simple, and the file stays small and readable. Accepted cost: if a ruleset file changes, an archived or resumed fight is read under the new definition. Rejected: copying the whole ruleset into each state file. |
| D36 | **Moving the turn holder past the row after it passes the turn on** (RS10, G10), as `n` would, recorded in turn history so `p` steps back over it. The mover goes into `Encounter.Delayed` until its turn comes again at its new count, which then skips the countdown; the list clears each round. The status line says "Mira steps aside · turn passes to Orc" and the selection follows the turn (D22). | Matches the table: the delayer steps aside and the next creature acts. `Delayed` is the only extra state, and it lasts at most a round. Rejected: keeping the turn on the mover and remembering the rows still owed one (hidden state across several `Next` calls). |
| D37 | **Ruleset files are JSON** (RS9), decoded with unknown fields refused, and the two built-ins are embedded JSON files read by the same parser. The file's fields mirror `combat.Ruleset`. | No new dependency, the same format as the state file, and a misspelt key fails at start-up instead of being silently ignored. Keeping the built-ins in the file form means the parser is exercised on every run and the built-ins serve as examples. Rejected: YAML or TOML (a dependency for a few fields), Go-only built-ins beside a separate file format. |
| D38 | **Every `*.json` in one folder is loaded at every start-up**: `dmct/rulesets` under the user's config folder, or `--rulesets DIR`. A missing default folder is fine; a missing `--rulesets` folder, a bad file, or a name that matches a built-in or another file (by the `--ruleset` matching rules) stops start-up with the file and the reason. | D35 stores only the name, so the definition must be found again on resume without the DM naming a file each time. Refusing a built-in's name keeps `5e` and `3.5e` meaning the same thing in every saved fight. Rejected: `--ruleset FILE` (resume would need the path again), files beside the state file (one set per campaign folder is rarely what's wanted, and `--rulesets .` gives it anyway), overriding built-ins. |
| D39 | **Snapshot undo.** Before each change, the session keeps a copy of the whole encounter, serialized as the state file is. Undo puts the copy back. | The encounter is already a plain, JSON-serializable value that `store` round-trips exactly, and it's small: a fight's state file is a few KB, plus its turn history. Every operation, present and future, is undoable for free. Rejected: an inverse for every operation (dozens of ops, and `Previous` already shows how much work reversing only durations is); extending `History` to cover every change (mixes turn rewind with general undo, and grows the state file). |
| D40 | **The stack lives in `session`, one entry per `Apply`.** Each entry holds the snapshots from before and after the change, its events and the log lines it wrote. A change with no events (a refused or no-op operation) adds nothing. A new change clears the redo side. Capped at 100 entries, oldest dropped. | `Apply` is already the single door for changes, and the TUI calls it exactly once per DM action (a group add, a multi-target command, a Delay with its effects), so "one step" matches what the DM did. `combat` stays free of undo. Two copies per step for 100 steps is a few MB at worst; to be measured on a long E2E fight in 13a. |
| D41 | **`p` stays.** It keeps meaning "step the turn back, keep what was done since". Undo of an `n` is plain undo of that step, and it also restores the durations because they are in the snapshot. | The two answer different mistakes: `p` is "I passed the turn too early, but the damage I entered since was right"; undo is "take back what I just did". Rejected: making `p` an alias for undo (it would discard edits made after the turn passed). |
| D42 | **The log stays append-only.** Undoing a step that wrote log lines appends `Undone: <its first line>` (and `+N more` if it wrote several); redoing it appends its lines again, marked `(redone)`. Steps that wrote nothing (Setup changes, HP arithmetic) log nothing either way. | The recap must not show things that didn't happen, and it can't be edited in place (D7). Logging only for logged steps keeps HP fixes as quiet as before. |
| D43 | **Ending combat is a barrier.** It clears the stack; so does starting `dmct`. | Ending already asks for confirmation, writes an archive and starts a new fight; undoing it would mean renaming the archive back and joining two logs. Restarts are rare mid-fight; persisting the stack would add a second file or bloat the state file (see open questions). |
| D44 | **Keys `u` (undo) and `U` (redo), plus `:undo` and `:redo`.** Both are free today. The status line says `Undid: Orc took 7 damage · U redoes` or `Nothing to undo`. `ctrl+r` also redoes, for vim habits. | `u`/`ctrl+r` is the vim pair the hotkeys already lean on (`jk`, `gG`). |
| D45 | **The dice roller is a tool beside the fight, not an operation.** A roll never goes through `session.Apply`: no state change, no autosave, no undo step, no log line. Revises D2's "no dice". | Typing `2d6+2+1d4` is faster than picking dice, and keeping it out of the encounter keeps R8's "no rules automation". Rejected: logging rolls (clutters the recap; the DM logs what matters with `l`); rolling straight into HP for now (needs resistance grammar, DR5). |
| D46 | **Grammar: a sum of `NdS` terms and whole numbers, with `+` and `-`.** A bare `d` means `1d`; spaces and case are ignored, but a space inside a term or between two unsigned terms (`2d6 3`) is refused. At most 100 dice in a term, 1000 sides, and 20 terms; a die has at least 2 sides. Parsed by hand in a pure `internal/dice` package. | Covers what's printed in stat blocks and spell text. The limits catch typos (`20d`, `1d2000`) and keep the breakdown short enough to read. Rejected: keep/drop, advantage, `d%` and multiplication (later grammar; see "Not in the roller" in REQUIREMENTS); a third-party dice library (a dependency for a 30-line parser). |
| D47 | **`r` opens a `Roll:` prompt; Enter on an empty prompt rolls the last expression again; `:roll EXPR` in the command bar.** The last expression lives in the TUI model only. | `r` is free and reads as "roll"; repeating an attack is the common case, and `r` Enter is two keys. Rejected: keeping the prompt open after a roll (the DM usually acts next); saving the last expression in the state file (it isn't fight state). |
| D48 | **The result is `EXPR = TOTAL · BREAKDOWN` on the status line**, the breakdown in set order: terms as typed, each term's faces in roll order (`[5 3] +2 [4]`), a subtracted dice term as `-[3]`. The breakdown is cut with `…` before the total is. | Set order lets the DM match each bracket to the term that made it. Total first means a narrow terminal never loses the number that matters. Rejected: sorted faces (hides which die was which). |

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
    Ruleset       string        // "5e", "3.5e" or a file's name; see Rulesets
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

- a combatant's own condition → that combatant, or the creature that caused it when one is named (D34)
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

- JSON, indented, with `schemaVersion` so the format can change later. Version 2 added `ruleset` and `nonlethal`; a version 1 file loads as `5e` and is written back as version 2.
- After every operation that changes state: marshal → write `<path>.tmp` → `fsync` → `rename` onto `<path>`.
- On startup, if the state file exists and `Status != Ended`, the app resumes. Otherwise it starts in Setup.
- **End combat** (D13) saves the final state, then renames the file to `dmct-state-<YYYYMMDD-HHMMSS>.json` next to it. The next combat starts in Setup with a new state file.
- The state stores `LogPath`, so a resumed encounter keeps writing to the same log even if the date has changed.
- A state file that can't be parsed, has a newer `schemaVersion`, or names a ruleset this build doesn't know, is refused and left untouched for the DM to look at.
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
| `r` | Roll dice (D47) |
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
| `:roll [DICE]` | `:roll 2d6+2+1d4` · `:roll` repeats the last |

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

Milestones 8–12 are the [Rulesets](#rulesets) work; 13a–13b are [Undo and redo](#undo-and-redo); 14a–14b are the [Dice roller](#dice-roller).

## Rulesets

Milestones 8–12 are built. The requirements are [Rulesets](REQUIREMENTS.md#rulesets) (gaps G1–G10, sub-features RS1–RS10). v1 hard-coded the 5e answers in three places: `health.go` (clamp at 0), `Combatant.Down()` (one state at HP ≤ 0) and the condition list. Milestone 8 moved the condition list into the ruleset, and milestone 9 the other two.

### Built so far (milestone 8)

- `combat.Ruleset` has a `Name` and a `Conditions` list, with two built-ins, `5e` and `3.5e`. `LookupRuleset` ignores case and the trailing "e".
- `Encounter.Ruleset` holds the **name**; `Encounter.Rules()` resolves it. `SetRuleset` is refused outside Setup and emits `RulesetChanged`, which isn't logged (nothing in Setup is, apart from notes).
- The state file's `schemaVersion` went to 2. Load fills in `5e` for a file without the field and refuses a name it doesn't know.
- `--ruleset` applies to a new fight or one still in Setup. Against a fight in progress with a different ruleset, start-up is refused with `session.ErrRulesetInUse`, and the message doesn't tell the DM to fix the state file. With no flag, a new fight takes the ruleset of the fight that just ended.
- The title line shows the ruleset on the right, and drops it when the turn holder's name needs the room.
- The condition picker and `:cond` take their presets and spelling from `Rules()`.

### Built so far (milestone 9)

```go
type Ruleset struct {
    Name       string        // "5e", "3.5e"
    HPFloor    int           // 5e: 0; 3.5e: -10
    States     []HealthState // ordered; first whose range holds HP wins
    Conditions []string
}

type HealthState struct {
    Name     string // "down", "disabled", "dying", "dead"; used in the log and status line
    Tag      string // short form for the HP column ("dsbld"); defaults to Name
    Min, Max *int   // inclusive HP range; nil = open
    Acts     bool   // still takes turns normally (3.5 disabled)
    Remind   bool   // status line points it out when its turn starts (3.5 dying)
}
```

- Damage and set-HP stop at `HPFloor`; heal counts up from the real value. The add form, `:add`, `:hp` and `=N` accept a current HP down to the floor.
- `Combatant.Down()` is gone. `Encounter.HealthState(c)` returns the state or nil, and the `Downed`/`Revived` events became one `HealthStateChanged{From, To, HP}`, emitted whenever the state's name changes.
- The log keeps the v1 wording for 5e ("dropped to 0 HP", "is back above 0 HP") and writes "Orc is dying (-7 HP)" for any other state.
- In the table, a state with `Acts` highlights only the HP cell; any other dims the row as `DOWN` always has. Dying also colors the HP cell and dead strikes the name through.
- The HP column is 11 wide at 80 columns and `DISABLED 0/22` doesn't fit, which is why that state has the tag `DSBLD`. Widening the column would have changed the 5e screens.
- The turn reminder is plain ("Orc is dying (-7 HP)") and names no rule, since a stable creature is dying by HP too.

### Built so far (milestone 10)

- `Ruleset.Nonlethal` (true for `3.5e`) and `Combatant.Nonlethal`, the running total. `NonlethalDamage` and `SetNonlethal` are refused with `ErrNoNonlethal` in a ruleset without it. Temp HP doesn't absorb nonlethal damage.
- Staggered and unconscious are `HealthState` values outside `Ruleset.States`, returned by `HealthState(c)` only when HP itself gives no state. So a dying creature with nonlethal damage shows as dying, and a move between an HP state and a nonlethal one is a single `HealthStateChanged`.
- `Heal` removes up to `n` nonlethal damage, even when HP is already full; `Healed.NonlethalHealed` says how much.
- `SetMaxHP` works in every ruleset and brings HP down to a lower max. `AddTempHP` is the `+N` form; it emits `TempHPSet` with `Added` set.
- The log gains "Orc max HP 10 → 6", "Orc is staggered (10 HP, 10 nonlethal)" and "Orc is no longer staggered". Nonlethal arithmetic and temp HP aren't logged, like other HP arithmetic.
- The HP cell appends `nl6`. In `3.5e` the cell is often cut off (`STAGR 10/1…`), so the status line now spells out the selected row's HP whenever its cell is cut, as it already did for the name, conditions and note.
- Hotkeys `D` (nonlethal; `=N` sets the total) and `M` (max HP, prefilled). The `t` prompt no longer prefills the current temp HP, since `+N` typed after it would read as `5+3`; the label shows it instead.

### Built so far (milestone 11a)

- `Condition.SourceID` (D34). `AddConditionFrom` takes a source; a source that is the bearer is stored as none, and an unknown one is refused. `ConditionApplied` carries the source's name.
- `tick` on a combatant's turn now walks every combatant's conditions and counts down those anchored to it: its own unsourced ones, and any whose source it is. Expired ones are recorded under their bearer, so `Previous` puts them back in place. A restored condition whose source has left is restored unsourced, as effects are.
- `Remove` clears the source of the conditions the leaver caused.
- The log writes "Dazed applied to Goblin by Mira (1 round)"; the table shows `Dazed (1 by Mira)`.
- After the duration, `c` asks "counts down on whose turn?": `s` for the turn holder, `n` or enter for the bearer. `:cond TARGETS NAME [ROUNDS] by SOURCE [-- NOTE]` names any combatant.

### Built so far (milestone 11b)

- `Ruleset.FixedAnchor` (true for `3.5e`). With it, `tick` counts every effect down on its own row (D32), and a combatant's turn no longer touches the effects it is the source of. A sourced condition still follows its source's turn: it has no row to pin.
- `Order` sorts by a key (initiative, group, slot, lane, insertion). A sourced effect tied with its live source takes the source's slot in the combatant group, in the lane ahead of it, so it sits between any earlier-added tied combatant and its source. Every other row gets the v1 key, so `5e` order is unchanged. An effect whose source has left falls back to sorting after the combatants at its count.
- `SetInitiative(id, v, SourceMove)`. With `FixedAnchor`, on a combatant that is the source of effects and a new value, `AskToMoveEffects` returns `*MoveEffectsError` and changes nothing (D33). `MoveEffects` shifts each of its effects by the same amount and emits an `InitiativeChanged` for each; `LeaveEffects` changes the combatant alone. In `5e` the choice is ignored.
- The `i` prompt and `:init` both ask "Mialee is the source of Fog Cloud: move them too?", `y` move or `n` leave; esc changes nothing.
- The turn stays with its row on an initiative edit, as before. So moving the turn holder later in the order and then pressing `n` starts the next round; for Delay, the DM passes the turn first and sets the new count when the creature steps in (README).


### Built so far (milestone 11c)

- `SetInitiative` on the turn holder during combat compares the order before and after. If the row now sorts after the row that followed it, the edit is a Delay (D36): the mover joins `Encounter.Delayed` and `advance` starts the next row's turn, with a `TurnRecord` pushed so `p` can undo it. Moving earlier, moving later without passing anyone, editing another row, and edits in Setup keep the turn where it is.
- `advance` skips `tick` for a row in `Delayed` and takes it off the list; a new round clears the list. `TurnRecord.PrevDelayed` lets `Previous` restore it.
- The TUI reports the pass and selects the new turn holder, for both `i` and `:init`. The E2E 3.5e story delays on the caster's own turn.

### Built so far (milestone 12)

- `rulesets/5e.json` and `rulesets/3.5e.json` are embedded in `internal/combat` and parsed by `ParseRuleset` at package init (a bad built-in panics, which a test covers). `Ruleset` and `HealthState` gained JSON tags (D37).
- `ParseRuleset` refuses unknown fields and trailing data, and checks: a name of 1–10 characters with no spaces, `hpFloor` ≤ 0, each state named, not `staggered`/`unconscious`, a label of at most 5 characters (the HP column), `min ≤ max`, not wholly below the floor, no repeated names and no overlapping ranges; conditions trimmed, not blank and not repeated. Names and conditions compare without case.
- `UseRulesets` replaces the file rulesets, refusing a clash with a built-in or another file; `LookupRuleset` and `RulesetNames` cover both, built-ins first. `UnknownRulesetError` wraps `ErrUnknownRuleset` and lists the choices; `store.Load` and `SetRuleset` use it.
- `store.LoadRulesets(dir)` reads `*.json` in name order, ignoring other files and folders, registers them, and on any error names the file and leaves only the built-ins loaded.
- `main` loads the folder before checking `--ruleset` (D38). A state file in a ruleset that isn't loaded is refused as before, with a hint naming the folder.
- The log no longer assumes a state ends at 0 HP: leaving a state for none at or below 0 HP writes "Orc is no longer down", and "dropped to 0 HP" is kept for a `down` state at exactly 0. Both built-ins log as before.
- The E2E harness points `HOME` at a temp folder after the build, so no test reads the real config folder.

### Build order

Each step keeps `5e` behavior identical to v1, checked by the existing unit, golden-screen and E2E suites.

| Milestone | Sub-features | Notes |
|---|---|---|
| 8. ✅ Ruleset core | RS1, RS8 | Adds the `Ruleset` type with the two built-ins, the Setup flag and command, the header label, and per-ruleset condition lists. No behavior change yet beyond the picker. |
| 9. ✅ HP model | RS2, RS3 | Floor and health states, the event change, table styling, log wording. This is the part that makes 3.5 usable (G1–G3). |
| 10. ✅ HP extras | RS4, RS5, RS6 | Nonlethal, max HP edit, `+N` temp HP. Independent of each other; can be split. |
| 11a. ✅ Condition sources | RS7 (G8) | `Condition.SourceID`, ticking on the source's turn, rewind and removal. Affects 5e too. |
| 11b. ✅ Fixed-count effects | RS7 (G7) | `FixedAnchor`, pinned effect rows, the move-effects question on initiative edits. The riskiest step: it touches `tick` and ordering. |
| 11c. ✅ Delay without lost turns | RS10 (G10) | Action item from 11b: an initiative edit that moves the turn holder later must not skip the rows in between. Touches `SetInitiative`, `Next` and rewind; affects 5e too (D36). |
| 12. ✅ Ruleset files | RS9 | File format, validation, built-ins moved to the same form. The state file keeps only the ruleset's name (D35). |

Milestones 8 and 9 alone are enough to run a 3.5 fight; 10–12 remove workarounds.

## Undo and redo

Built as milestones 13a–13b, for [Undo and redo](REQUIREMENTS.md#undo-and-redo) (UR1–UR5). The decisions are D39–D44: snapshots (D39), one step per `Apply` in `session` (D40), `p` kept (D41), the log (D42), End and restart as barriers (D43), and the keys (D44).

### How it works

```go
// internal/session
type step struct {
    before, after []byte         // encounter JSON either side of the change
    events        []combat.Event // what it emitted; the UI names the step and picks the row from these
    logged        []string       // journal.Texts of what it wrote to the log; nil if nothing
}

func (s *Session) Undo() (Outcome, error) // ErrNothingToUndo when empty; Outcome.Undid
func (s *Session) Redo() (Outcome, error) // ErrNothingToRedo when empty; Outcome.Redid
func (s *Session) UndoDepth() (undo, redo int)
```

- **Apply** marshals the encounter before running the op. If the op fails or emits nothing, the copy is dropped. Otherwise the step is pushed after the save and log, with `after` marshalled then.
- **Undo** writes the undo line at the current turn, then unmarshals `before`, sets the clock, keeps the current `LogPath` (once a log is bound it stays bound, so the undo line and later entries land in the same file), saves, and writes the undo line. The journal writer is re-synced to `ResumeFor` the restored encounter, so undoing back into an earlier round and passing the turn again writes that round's heading again, under the undo line. That's accurate: it's where the fight picks up.
- **Redo** re-appends the step's events through the journal with each entry marked `(redone)` and stamped with the time of the redo, then restores `after`. Writing first, with the writer still where the undo left it, writes any round heading the step started again.
- **Undo line.** `Undone: <first logged line>`, plus `(+N more)` when the step wrote several. A step that only started a round counts as writing `Round N started`.
- **Summaries.** Most changes write no log line (HP arithmetic, temp HP, notes on rows), so the summary can't come from the log. A new `journal.Summary(evs []combat.Event) string` phrases the first event of any kind in the log's wording, adding "and N more" for groups and multi-target changes. That needs a phrase for each event type that the log skips today; it's most of the milestone's work and is tested event by event.
- **TUI.** `u`/`U` call `Undo`/`Redo` like any other op, go through `track` for the save and log banners, and move the selection to the first row named in the step's events if it still exists (D22 otherwise). The help overlay lists both keys; the hint line doesn't, since it's full at 80 columns.
- **Ruleset files.** A snapshot carries only the ruleset's name (D35), and files load once at start-up, so restoring one can't name a ruleset that isn't loaded.

### Built so far (milestone 13a)

- `session.Apply` snapshots the encounter before every op and records a step when it emits events; a change that ends combat clears both stacks instead. `Undo`, `Redo` and `UndoDepth` as above, `UndoLimit = 100`.
- `journal.Texts`, `Writer.Entry`, `Writer.WriteRedone` and `Writer.Resync`; `combat.Encounter.Meta` stamps the undo line.
- Tested by applying 31 kinds of change in a 3.5e fight (Setup changes, HP, nonlethal, conditions with and without sources, effects and targets, a Delay, `n`, `p`, a new round, ending an effect, removal) and checking after each that undo gives back the state before byte for byte and redo the state after; then undoing all the way to an empty Setup and redoing it all.

### Built so far (milestone 13b)

- `journal.Summary(evs)`: a change of turn reads "Turn passed to Orc" or "Round 2 started: Mira's turn"; anything else is its first event's wording plus "(and N more)" for later events of the same kind (`Goblin x4`, `:dmg 1-4 7`). Logged events reuse the log's wording through `logText`, pulled out of the renderer; HP arithmetic, initiative, notes on rows and the ruleset have words of their own. A test checks every event type.
- `u`, `U`, `ctrl+r`, `:undo`, `:redo` (command kinds `Undo`/`Redo`). The status line reads "Undid: … · U redoes" or "Redid: … · u undoes", unless a save or log recovery has something to report. A change of turn selects the turn holder; anything else selects the first row its events name that still exists.
- The help overlay has the keys and commands; the hint line doesn't (D44). The `:ruleset` help now mentions ruleset files.
- E2E story: undo a group add, a Delay that moved its effects, an ended effect; redo and undo again; a new change drops the redo; the log lines; a restart has nothing to undo.

### Build order

| Milestone | Covers | Notes |
|---|---|---|
| 13a. ✅ Undo stack | UR1, UR2, UR4, UR5 | `session` stack, `Undo`/`Redo`, log lines and journal re-sync, the barrier at End. Tested in `session` with every kind of operation: apply, undo, redo, compare the encounter JSON byte for byte. |
| 13b. ✅ Undo in the TUI | UR3 | `journal.Summary`, keys and commands, status line, selection, help overlay, golden screens and an E2E story (undo a group add, a Delay that moved effects, and an ended effect; redo one; a new change drops the redo). |

## Dice roller

Milestones 14a–14b, for [Dice roller](REQUIREMENTS.md#dice-roller) (DR1–DR4). The decisions are D45–D48: a tool beside the fight (D45), the grammar (D46), the keys (D47) and the output (D48). 14a is built; 14b is next.

### How it works

```go
// internal/dice (pure: no I/O, no global random source)
type Term struct {
    Sign  int  // +1 or -1
    Count int  // 0 for a whole-number term
    Sides int
    Value int  // the number, for a whole-number term
}
type Expr struct{ Terms []Term }

type Rolled struct {
    Term  Term
    Faces []int // in roll order; nil for a whole-number term
}
type Result struct {
    Expr  Expr
    Total int
    Terms []Rolled // in the order typed
}

func Parse(s string) (Expr, error)
func (e Expr) String() string            // normalised: "d20 + 5" → "1d20+5"
func (e Expr) Roll(r *rand.Rand) Result  // math/rand/v2
func (r Result) Breakdown() string       // "[5 3] +2 [4]"
```

- **Parsing.** Lower-case and strip spaces, except that a space between two digits or `d`s (`2d6 3`, `2 d6`) is refused rather than read as `2d63`; then read terms left to right: an optional sign (required after the first term), then digits, then optionally `d` and digits. A missing count before `d` is 1. Errors name what was wrong and where (`missing sides after "4d"`, `100 dice at most in a term`). A plain number with no dice (`7`) is accepted and rolls to itself; it's harmless. A whole-number term is at most 10000.
- **Randomness.** The TUI model makes one `*rand.Rand` (PCG) seeded from the clock; tests swap in a seeded source and check exact results. Faces come from `r.IntN(sides)+1`.
- **TUI.** `r` opens a `Roll:` prompt (D24). Enter parses and rolls; a parse error keeps the prompt open with the reason in the hint line (D25). An empty Enter rolls the last expression, or says "Nothing to roll yet". `:roll EXPR` builds a command kind `Roll` that the TUI handles itself, like `Undo`, without calling `Apply`. The result is a normal status message, so a save or log banner (D31) is unaffected.
- **Width.** The status line keeps `EXPR = TOTAL` whole and trims the breakdown from the right with `…` (D48). The 100-dice cap keeps the full text under a few hundred characters.

### Built so far (milestone 14a)

- `internal/dice`: `Parse`, `Expr.String`, `Expr.Roll`, `Result.String`, with the faces kept for 14b. Tested for every refusal, the limits, signs, a bare `d`, split terms, and seeded rolls.
- `r` prompt (its hint names the expression enter alone repeats), `:roll [DICE]` (command kind `Roll`), help overlay and README. Like `u`, `r` isn't in the hint line, which is full at 80 columns.
- Golden screens for the prompt and the result; an E2E story: roll, fix a typo in place, repeat with enter, `:roll`, then check that undo skips the rolls and the log has none.

### Build order

| Milestone | Covers | Notes |
|---|---|---|
| 14a. ✅ Dice roller | DR1, DR2, DR3 | `internal/dice` with `Parse`, `Roll`, `String` and table-driven tests (every refusal, `d` without a count, signs, limits, seeded rolls); `r` prompt, empty-Enter repeat, `:roll`, help overlay and README; golden screen and an E2E story (roll, a refused expression fixed in place, repeat). Status line shows `EXPR = TOTAL`. |
| 14b. Breakdown | DR4 | `Result.Breakdown`, the `· [5 3] +2 [4]` suffix and its truncation at 80 columns and below; golden screens for a long roll (`20d6`) and a narrow terminal. |

Later, not designed: dice in the `d`/`h` prompts with resistance, vulnerability and immunity grammar (DR5).

## Open questions

The end-combat, duration, and condition-list questions were settled as D13, D12, and D14. Open for the ruleset work:

1. ~~**Fixed-count ticking (RS7).**~~ Settled as D32.
2. ~~**Correcting an initiative versus Delay/Ready.**~~ Settled as D33.
3. ~~**State-file format.**~~ Settled as D35: only the name.
4. ~~**HP floor in 3.5.**~~ Settled: HP stops at -10, since anything lower is equally dead.
5. ~~**Delay mechanism (RS10).**~~ Settled as D36.

For undo and redo, all settled 2026-11-18: no undo history across a restart and no undo past End (D43); undo and redo are marked in the log (D42); the history keeps 100 steps (D40).

For the dice roller, settled 2026-11-19: rolls are not logged (D45); the breakdown is in set order (D48); rolling in the damage prompt waits for resistance grammar (DR5).
