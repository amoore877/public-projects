# dm-combat-tracker

A terminal UI, written in Go, for running tabletop combat from the DM's side of the screen.

It tracks initiative order, whose turn it is, hit points, conditions, area effects, and per-combatant notes, and it keeps an append-only Markdown log of the fight that you can use when writing a session recap.

It's built with D&D in mind but isn't tied to any one ruleset: it doesn't roll dice or apply rules.

```
 Round 1 ─ Turn: Thorin
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
dmct [--state FILE] [--log FILE]
```

| Flag | Default | What it is |
|---|---|---|
| `--state` | `./dmct-state.json` | The fight's state, saved after every change |
| `--log` | `./dmct-log-YYYY-MM-DD.md` | The Markdown combat log, dated the day combat starts |

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

### Initiative and ties

Initiative is typed in by hand, as a decimal. Ties are settled by decimals: three creatures tied at 15 can be `15.3`, `15.2` and `15.1`. Exact ties between combatants are marked with `!` and a warning until you change one (`i`).

A group shares the initiative you give it, with decimals filled in to keep its members in order: `Goblin x4` at `12` becomes `12.4` … `12.1`. You can also give each member its own initiative, comma-separated: `14, 9, 11, 12`.

### HP

`d` takes damage from temp HP first, then HP, down to 0. `h` heals up to max HP. In either prompt, `=N` sets HP to exactly N. `t` sets temp HP, replacing what's there (temp HP doesn't stack). When adding, HP can be `MAX` or `CURRENT/MAX`.

A creature at half HP or less is marked `▼`, and one at 0 shows `DOWN` and is dimmed. Downed creatures stay in the order until you remove them (`x`), so PCs still get their turns for death saves.

### Conditions and durations

`c` adds a condition: pick from the D&D 5e list (tab completes) or type your own, then give an optional duration in rounds and an optional note (`DC 14 CON save ends`). `C` removes one.

A duration counts down **at the start of its owner's turn** and expires at 0, with a log entry. So a 1-round condition on a goblin lasts until the start of that goblin's next turn. Anything set up before combat starts counts from its owner's first turn after the opening one. For "until the end of its turn", remove it by hand.

`p` steps back a turn to fix a mis-click. It puts back any durations the step counted down (and anything that expired), but not HP or other edits.

### Area effects

`e` starts an effect such as Spirit Guardians or Wall of Fire. You give it a name, a source (the selected combatant, or none for lair and environment effects, which then need their own initiative), a duration, and its targets, ticked off on the table itself with `space`.

The effect gets its own `◆` row, right after its source's turn, as a reminder to apply it. Each target shows it as a linked `◆` condition. The effect's duration counts down at the start of its own row's turn.

- `T` on the effect row changes its targets as creatures move in or out.
- `x` on the effect row ends it early (dispelled, concentration broken), with an optional reason for the log.
- Removing a creature that is the source of effects asks whether to keep them or end them.

### The command bar

`:` opens a command line for exact or bulk input. Names match loosely: `goblin 2`, `gob2` and `g2` all mean Goblin 2, and `band` means Bandit Captain. If a name could mean more than one creature, the command is refused with the candidates listed. `TARGETS` is a comma-separated list. `↑↓` browse earlier commands and `tab` completes the command name.

| Command | Example | Does |
|---|---|---|
| `:add NAME [xN] INIT HP [pc\|npc]` | `:add Goblin x4 12 7` | Add a combatant or a numbered group |
| `:rm TARGET` | `:rm goblin 2` | Remove a combatant |
| `:dmg TARGETS N` | `:dmg g1,g3 9` | Damage |
| `:heal TARGETS N` | `:heal thorin 8` | Heal |
| `:hp TARGET N` | `:hp g2 3` | Set HP directly |
| `:temp TARGETS N` | `:temp mira 5` | Set temp HP |
| `:cond TARGETS NAME [ROUNDS] [-- NOTE]` | `:cond thorin blessed 10` | Add a condition |
| `:uncond TARGET NAME` | `:uncond g3 prone` | Remove a condition |
| `:fx NAME [ROUNDS] [from SOURCE] [at INIT] [on TARGETS]` | `:fx Spirit Guardians 10 from mira on g1,g3` | Start an effect |
| `:fxadd FX TARGETS` / `:fxrm FX TARGETS` | `:fxrm spirit g1` | Change an effect's targets |
| `:end FX [-- REASON]` | `:end spirit -- concentration broken` | End an effect early |
| `:init TARGET VALUE` | `:init g2 12.35` | Set initiative |
| `:note TARGET TEXT` | `:note g1 fleeing` | Set the side note (blank clears it) |
| `:log TEXT` | `:log Thorin shoves the goblin off the bridge` | Write a log entry |
| `:next` `:prev` `:start` `:endcombat` | | Turn controls |

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
| `t` | Set temp HP |
| `c` / `C` | Add / remove a condition |
| `e` | New effect |
| `T` | Change an effect's targets |
| `i` | Edit initiative |
| `N` | Edit the side note |
| `l` | Write a log entry |
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
