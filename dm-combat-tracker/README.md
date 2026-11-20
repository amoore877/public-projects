# dm-combat-tracker

A terminal UI, written in Go, for running tabletop combat from the DM's side of the screen.

It tracks initiative order, whose turn it is, hit points, conditions, area effects, and per-combatant notes, and it keeps an append-only Markdown log of the fight that you can use when writing a session recap.

It's built with D&D in mind but isn't tied to any one ruleset: it doesn't apply rules. It has a [dice roller](#dice), but a roll never changes the fight by itself.

```
 Round 1 ─ Turn: Thorin                                                      5e
┌───┬────────┬──────────────┬──────┬─────────────┬────────────────┬────────────┐
│   │ Init   │ Name         │ Kind │ HP          │ Conditions     │ Note       │
├───┼────────┼──────────────┼──────┼─────────────┼────────────────┼────────────┤
│ ▶ │ 18     │ Thorin       │ PC   │ 31/44       │ Blessed (2)    │            │
│   │ 12.3   │ Goblin 1     │ NPC  │ 7/7         │ ◆ Spirit Guar… │ fleeing    │
│   │ 12.2   │ Goblin 2     │ NPC  │ DOWN 0/7    │                │            │
│   │ 12.1   │ Goblin 3     │ NPC  │ 3/7 ▼       │ Prone, ◆ Spir… │            │
│   │ 9 !    │ Mira         │ PC   │ 22/27 +5    │ Concentrating  │            │
│   │ 9 !    │ Bandit Capt… │ NPC  │ 65/65       │                │            │
│   │ 9      │ ◆ Spirit Gu… │ FX   │             │ 10 rds · 2 ta… │ 3d8 radia… │
└───┴────────┴──────────────┴──────┴─────────────┴────────────────┴────────────┘
 ! Tied initiative: enter a decimal (e.g. 15.2 / 15.1) to set the order
 n next · p prev · d/h dmg/heal · c/C cond · e effect · : cmd · ? help · q quit
```

## Install

You need Go 1.27 or later. From this folder:

```bash
go build -o dmct ./cmd/dmct
```

Or install it straight from GitHub:

```bash
go install github.com/amoore877/public-projects/dm-combat-tracker/cmd/dmct@latest
```

The result is a single static binary with no runtime dependencies. It runs on macOS and Linux.

## Run

```bash
dmct
```

```
dmct [--state FILE] [--log FILE] [--ruleset NAME] [--rulesets DIR]
```

| Flag | Default | What it is |
|---|---|---|
| `--state` | `./dmct-state.json` | The fight's state, saved after every change |
| `--log` | `./dmct-log-YYYY-MM-DD.md` | The Markdown combat log, dated the day combat starts |
| `--ruleset` | `5e` | The game ruleset, `5e`, `3.5e` or one from a file, for a fight that hasn't started. See [Rulesets](#rulesets) |
| `--rulesets` | see [Ruleset files](#ruleset-files) | The folder ruleset files are read from |

Run it in the folder you keep your campaign notes in, so the log lands next to them.

### Files

- **State file.** Saved after every change, atomically, so a crash or a closed laptop loses nothing. Start `dmct` again and it picks up exactly where it was: round, turn, HP, conditions, effects.
- **Archive.** Ending combat renames the state file to `dmct-state-YYYYMMDD-HHMMSS.json` and starts a fresh fight, so the final state of every fight is kept.
- **Log.** Append-only Markdown, never rewritten. Several fights on the same day go into the same file, each under its own `# Combat — …` heading.

If the state file can't be read (it's corrupt, or from a newer version of `dmct`), `dmct` refuses to start and leaves the file untouched. Fix or move it, or use `--state` to start a new fight in another file.

## Running a fight

1. **Add combatants** in Setup with `a`: a name, an initiative, HP, and PC or NPC. `Goblin x4` adds `Goblin 1` … `Goblin 4`.
2. **Start** with `S`. The log gets a "Combat started" line listing the turn order.
3. **Play.** `n` (or `space`) passes the turn; the selection follows the turn holder. Select another row with `↑↓` / `jk` to act on it: `d` damage, `h` heal, `c` add a condition, `e` start an effect, and so on.
4. **Note things down** with `l` as they happen. Notes are marked 📝 in the log so they stand out when you write the recap.
5. **End** with `E`. The state file is archived and a new fight begins in Setup.

You can quit with `q` at any point; everything is already saved.

### Undo

`u` takes back the last change, whatever it was: damage, a group add with the wrong count, an effect on the wrong targets, a Delay, a removed combatant along with the effects that went with it. The fight goes back exactly as it was, durations and turn included. Press it again to go further back, up to 100 changes. `U` (or `ctrl+r`) puts back what you undid; any new change drops what's left to redo. The status line says what was undone ("Undid: Orc took 7 damage") and the selection moves to that row.

`p` is different: it steps the turn back and keeps everything done since, which is what you want when you passed the turn too early.

The log is never rewritten. Undoing something that was logged adds an `Undone: …` line, and redoing it writes its lines again marked `(redone)`. Ending combat can't be undone (it asks first), and quitting clears the undo history.

### Dice

`r` opens a roll prompt. Type what the rulebook says and press enter: `4d6+4`, `2d6+2+1d4`, `1d8-1`. `d20` means `1d20`, and spaces and capitals don't matter (`2D6 + d4`). The status line shows the total, `2d6+2+1d4 = 14`. Enter on an empty prompt rolls the last expression again, so a repeated attack is `r` then enter. `:roll 2d6+3` does the same from the command bar, and `:roll` alone repeats.

A roll changes nothing: it isn't saved, logged, or undone. Limits are 100 dice in a term, 1000 sides, and 20 terms. Keep/drop (`4d6kh3`), advantage and `d%` aren't supported.

### Rulesets

The title line shows the fight's ruleset. Choose it before combat starts, with `--ruleset 3.5e` or `:ruleset 3.5e`; it's saved with the fight and can't change once combat has started. The next fight keeps the last one's ruleset.

The ruleset sets these:

| | `5e` | `3.5e` |
|---|---|---|
| Lowest HP | 0 | -10 |
| Health states | `DOWN` at 0 | `DSBLD` (disabled) at exactly 0, `DYING` from -1 to -9, `DEAD` at -10 |
| Nonlethal damage | not tracked | tracked: `STAGR` (staggered) when it equals HP, `UNCON` (unconscious) when it's higher |
| Sourced effects | count down on the source's turn | stay on the count they began on (see [Area effects](#area-effects)) |
| Condition list | D&D 5e | D&D 3.5 (Shaken, Sickened, Nauseated, Dazed, Fatigued and so on) |

It still applies no rules for you: a dying creature doesn't lose HP or roll to stabilize on its own. When a dying creature's turn starts, the status line says so, and Stable is a condition you add by hand.

#### Ruleset files

Other systems (Pathfinder, house rules) are added as JSON files, with no code change. `dmct` reads every `*.json` file in the rulesets folder each time it starts:

- `~/Library/Application Support/dmct/rulesets` on macOS
- `~/.config/dmct/rulesets` on Linux (or `$XDG_CONFIG_HOME/dmct/rulesets`)
- `%AppData%\dmct\rulesets` on Windows

or the folder given with `--rulesets DIR`. The default folder doesn't have to exist; a folder named with `--rulesets` does.

```json
{
  "name": "pf1",
  "hpFloor": -10,
  "nonlethal": true,
  "fixedAnchor": true,
  "states": [
    {"name": "disabled", "tag": "dsbld", "min": 0, "max": 0, "acts": true},
    {"name": "dying", "min": -9, "max": -1, "remind": true},
    {"name": "dead", "max": -10}
  ],
  "conditions": ["Shaken", "Sickened", "Staggered"]
}
```

| Field | Default | Meaning |
|---|---|---|
| `name` | required | What you pass to `--ruleset` and `:ruleset`. At most 10 characters, no spaces, and not `5e`, `3.5e` or another file's name. |
| `hpFloor` | `0` | The lowest HP a creature can have. 0 or below. |
| `states` | none | Health states by HP. `min` and `max` are inclusive; leave one out for no limit. The ranges can't overlap. `tag` is the short form in the HP column (at most 5 letters; defaults to `name`). `acts: true` highlights only the HP cell instead of dimming the row. `remind: true` points the state out when the creature's turn starts. `staggered` and `unconscious` are reserved for nonlethal damage. |
| `nonlethal` | `false` | Track nonlethal damage, with the staggered and unconscious states. |
| `fixedAnchor` | `false` | Sourced effects stay on the count they began on, as in 3.5e. |
| `conditions` | none | The condition picker's list. Custom names can always be typed. |

The built-in rulesets are written the same way: see [`internal/combat/rulesets`](internal/combat/rulesets) for both. A file with a mistake (an unknown field, overlapping states, a duplicate name) stops `dmct` at start-up with the file and the reason.

A fight saves only its ruleset's name. To resume a fight in a ruleset from a file, the file must still be there, and a change to the file applies to the fight from then on.

### Initiative and ties

Initiative is typed in by hand, as a decimal. Ties are settled by decimals: three creatures tied at 15 can be `15.3`, `15.2` and `15.1`. Exact ties between combatants are marked with `!` and a warning until you change one (`i`).

To **Delay** (or Ready), change the initiative of the creature whose turn it is to the count it will act on. If that moves it past the next creature, the turn passes on ("Mira steps aside · turn passes to Orc"), the creatures in between act, and its turn comes again at the new count in the same round. Its durations don't count down a second time that round. `p` steps back over the pass; the new initiative stays.

A group shares the initiative you give it, with decimals filled in to keep its members in order: `Goblin x4` at `12` becomes `12.4` … `12.1`. You can also give each member its own initiative, comma-separated: `14, 9, 11, 12`.

### HP

`d` takes damage from temp HP first, then HP, down to 0 (-10 in `3.5e`). `h` heals up to max HP, counting from where HP really is, so healing 5 at -7 gives -2. In either prompt, `=N` sets HP to exactly N (`=-4` in `3.5e`). `t` sets temp HP: `N` replaces what's there and `+N` adds to it, for sources that stack. `M` edits max HP: a lower max brings HP down with it, a higher one doesn't heal. When adding, HP can be `MAX` or `CURRENT/MAX`, with `+N` on the end for temp HP (`31/44+5`). In `3.5e` the current HP can be negative (`-4/22`).

In `3.5e`, `D` adds nonlethal damage (`=N` sets the total). It's kept as a separate total, shown as `nl6` after HP, and temp HP doesn't absorb it. Healing lowers it by the amount healed, even at full HP. When the HP cell is too narrow for all of this, the status line spells out the selected creature's HP.

A creature at half HP or less is marked `▼`, and one at 0 shows `DOWN` and is dimmed. Downed creatures stay in the order until you remove them (`x`), so PCs still get their turns for death saves.

In `3.5e` the HP cell shows `DSBLD`, `DYING` or `DEAD` instead. A disabled creature can still act, so only its HP cell is highlighted; dying and dead rows are dimmed, and a dead creature's name is struck through. Each change is logged ("Orc is dying (-7 HP)").

### Conditions and durations

`c` adds a condition: pick from the ruleset's list (tab completes) or type your own, then give an optional duration in rounds and an optional note (`DC 14 CON save ends`). For a timed condition during combat it also asks whose turn it counts down on: `s` if the creature whose turn it is caused it, `n` (or enter) for the creature that has it. `C` removes one.

A duration counts down **at the start of its owner's turn** and expires at 0, with a log entry. So a 1-round condition on a goblin lasts until the start of that goblin's next turn. A condition with a source counts down at the start of the **source's** turn instead, so Mira's 1-round daze lasts until her next turn and the table shows `Dazed (1 by Mira)`. If the source leaves combat, the condition goes back to counting on its owner's turn. Anything set up before combat starts counts from its owner's first turn after the opening one. For "until the end of its turn", remove it by hand.

`p` steps back a turn to fix a mis-click. It puts back any durations the step counted down (and anything that expired), but not HP or other edits.

### Area effects

`e` starts an effect such as Spirit Guardians or Wall of Fire. You give it a name, a source (the selected combatant, or none for lair and environment effects, which then need their own initiative), a duration, and its targets, ticked off on the table itself with `space`.

The effect gets its own `◆` row, right after its source's turn, as a reminder to apply it. Each target shows it as a linked `◆` condition. Its duration counts down at the start of its **source's** turn, so a spell cast on Mira's turn for 10 rounds ends as her 10th turn after that begins. An effect with no source counts down on its own row's turn.

- `T` on the effect row changes its targets as creatures move in or out.
- `x` on the effect row ends it early (dispelled, concentration broken), with an optional reason for the log.
- Removing a creature that is the source of effects asks whether to keep them or end them.

In `3.5e` an effect stays on the initiative count it began on, as the 3.5 rules have it. Its row sorts just before its source, and it counts down on its own row, so it ends just before that count comes up. If the source later Delays or Readies, the effect doesn't follow. Changing the initiative of a creature that is the source of effects asks whether to move them too: `y` for a mistyped initiative (they shift by the same amount), `n` for Delay or Ready.

### The command bar

`:` opens a command line for exact or bulk input. Names match loosely: `goblin 2`, `gob2` and `g2` all mean Goblin 2, and `band` means Bandit Captain. If a name could mean more than one creature, the command is refused with the candidates listed. `TARGETS` is a comma-separated list. `↑↓` browse earlier commands and `tab` completes the command name.

| Command | Example | Does |
|---|---|---|
| `:add NAME [xN] INIT HP [pc\|npc] [-- NOTE]` | `:add Goblin x4 12 7` · `:add Thorin 18 31/44+5 pc -- cursed` | Add a combatant or a numbered group |
| `:rm TARGET` | `:rm goblin 2` | Remove a combatant |
| `:dmg TARGETS N` | `:dmg g1,g3 9` | Damage |
| `:heal TARGETS N` | `:heal thorin 8` | Heal |
| `:hp TARGET N` | `:hp g2 3` | Set HP directly |
| `:temp TARGETS [+]N` | `:temp mira +5` | Set temp HP, or add to it with `+N` |
| `:nl TARGETS [=]N` | `:nl orc 6` | Nonlethal damage, or set the total with `=N` (`3.5e`) |
| `:maxhp TARGET N` | `:maxhp orc 24` | Set max HP |
| `:cond TARGETS NAME [ROUNDS] [by SOURCE] [-- NOTE]` | `:cond g3 dazed 1 by mira` | Add a condition; with a source it counts down on the source's turn |
| `:uncond TARGET NAME` | `:uncond g3 prone` | Remove a condition |
| `:fx NAME [ROUNDS] [from SOURCE] [at INIT] [on TARGETS]` | `:fx Spirit Guardians 10 from mira on g1,g3` | Start an effect |
| `:fxadd FX TARGETS` / `:fxrm FX TARGETS` | `:fxrm spirit g1` | Change an effect's targets |
| `:end FX [-- REASON]` | `:end spirit -- concentration broken` | End an effect early |
| `:init TARGET VALUE` | `:init g2 12.35` | Set initiative |
| `:note TARGET TEXT` | `:note g1 fleeing` | Set the side note (blank clears it) |
| `:log TEXT` | `:log Thorin shoves the goblin off the bridge` | Write a log entry |
| `:ruleset NAME` | `:ruleset 3.5e` | Choose the ruleset (before combat starts) |
| `:next` `:prev` `:start` `:endcombat` | | Turn controls |
| `:undo` `:redo` | | Take back the last change / make it again |
| `:roll [DICE]` | `:roll 2d6+2+1d4` | Roll dice; alone, roll the last again. See [Dice](#dice) |

A command with a mistake stays open with the reason, so you only need to fix the typo. A command on several targets checks all of them before changing anything.

### Keys

Keys act on the selected row (highlighted), which can differ from the row whose turn it is (`▶`).

| Key | Action |
|---|---|
| `n` / `space` | Next turn |
| `p` | Previous turn |
| `↑↓` / `j k` | Move the selection |
| `g` / `G` | First / last row |
| `a` | Add a combatant or group |
| `x` | Remove the combatant (asks first) / end the effect early |
| `d` / `h` | Damage / heal (`=N` sets HP) |
| `t` | Temp HP (`N` replaces, `+N` adds) |
| `D` | Nonlethal damage (`3.5e`; `=N` sets the total) |
| `M` | Edit max HP |
| `c` / `C` | Add / remove a condition |
| `e` | New effect |
| `T` | Change an effect's targets |
| `i` | Edit initiative |
| `N` | Edit the side note |
| `l` | Write a log entry |
| `u` / `U` | Undo / redo the last change (`ctrl+r` also redoes). See [Undo](#undo) |
| `r` | Roll dice; enter alone rolls the last again. See [Dice](#dice) |
| `:` | Command bar |
| `?` | Help: every key and command |
| `S` / `E` | Start / end combat |
| `q` | Quit |

In a prompt, `enter` accepts, `esc` cancels, `tab` completes, and the usual line-editing keys work (`ctrl+a/e/u/k/w`, arrows, paste).

## The combat log

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

Every line is stamped with the time, the round and whose turn it was. The log records combat starting and ending, each round, creatures added or removed, dropping to 0 HP or coming back, conditions and effects starting, changing, ending and expiring, rewound turns, and your notes. It leaves out plain turn changes and HP arithmetic. During Setup only your notes are logged.

## Terminal size

`dmct` fits a standard 80×24 terminal and uses extra width for the Name, Conditions and Note columns. In narrower terminals it drops the Note column, then the Kind column, then narrows the rest. When the selected row's name, conditions or note are cut off, the line under the table spells them out. Below 43×9 it asks for a bigger window.

## If saving fails

If the state file or the log can't be written (a full disk, a folder that went read-only), the fight carries on and a red banner under the title says what failed. `dmct` tries again after every change, and the banner clears once a write succeeds. A log that recovers tells you how many changes it missed. While autosave is failing, `q` asks before quitting, since changes made since the failure aren't on disk.

## Docs

- [docs/REQUIREMENTS.md](docs/REQUIREMENTS.md): what it must do
- [docs/DESIGN.md](docs/DESIGN.md): how it's built, and why
