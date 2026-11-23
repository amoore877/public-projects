# Requirements

## Story

In D&D (and similar games), running combat means keeping track of a lot at once: several NPCs, the turn order, the PCs, everyone's health and status effects, area spells and abilities that last several rounds, and notable moments you'll want for the post-session recap. Paper and spreadsheets fall behind once a fight gets busy.

## Users

- **Primary:** the DM, running the tool on their own screen during play.
- **Out of scope for now:** a player-facing view (a masked HP display or a second screen). See [Future](#future).

## Functional requirements

### R1. Combat table

The main screen is a table showing every combatant and every active effect in initiative order:

| Column | Notes |
|---|---|
| Turn marker | Clearly shows whose turn it is. |
| Initiative | Decimal value, as entered (see R2). |
| Name | Auto-numbered for groups (R3). |
| Kind | PC / NPC / Effect. |
| HP | `current / max`, plus temp HP when present. Visual state for bloodied (≤ half) and down (0). |
| Conditions | Name and remaining duration, if any. Includes conditions linked to an area effect (R5). |
| Notes | A short free-text side note for each row. |

The header shows the current **round number**.

### R2. Initiative and turn order

- Initiative is **entered by hand**. The app does not roll dice.
- Initiative is a **decimal number**, which is how ties get broken: three combatants tied at 15 can be entered as `15.3`, `15.2`, `15.1` and run in that order.
- The order is sorted by initiative, highest first. If two values are exactly equal, the combatant added first goes first, and a warning asks the DM to enter a decimal to settle it.
- A combatant's initiative can be edited mid-combat; the table re-sorts and the current turn stays on the same combatant.
- **Advance turn** moves to the next row; wrapping past the bottom starts a new round.
- **Previous turn** steps back, to fix mis-clicks. It restores the turn and round number but doesn't undo other changes.

### R3. Adding and removing combatants mid-combat

- Add a combatant at any time, with a name, kind, initiative, max HP (and optionally current HP and temp HP), and notes. It's slotted into the order without changing whose turn it is.
- **Group add:** `Goblin ×6` creates `Goblin 1` … `Goblin 6`, each with its own HP, conditions, and notes. By default they share the entered initiative, with decimals filled in automatically to keep them in order (e.g. `12.6` … `12.1`); the DM can choose separate initiatives instead.
- Remove a combatant at any time. If it was their turn, the turn passes to the next combatant. Removing a combatant also removes them from the targets of any area effects.
- A combatant at 0 HP stays in the table, marked as down, until it is removed explicitly. PCs at 0 HP still need their turns (for death saves and the like).

### R4. Health and conditions

- Damage, heal, set HP directly, and set temp HP.
- Damage comes out of temp HP first. Healing can't go above max HP. HP doesn't go below 0.
- Add or remove conditions on a combatant. A condition has:
  - a name (pick from a preset list or type your own),
  - an optional duration in rounds,
  - an optional short note (e.g. "DC 14 CON save ends").
- Timed conditions count down automatically and are removed when they expire, with a log entry.
- Edit the per-combatant side note.

### R5. Area effects (spells and abilities)

Area effects show up as **their own row in the table and as linked conditions on the combatants they affect**.

- An effect has a name, a source (a combatant, or none for environmental/lair effects), an initiative position (by default just after its source), an optional duration in rounds, and a list of affected combatants.
- The effect row shows its remaining duration and comes up in initiative order, as a reminder to apply its per-turn behavior.
- Each affected combatant shows a linked condition naming the effect. It is backed by the effect itself, not stored as a separate copy.
- Add or remove targets while the effect is active, for creatures that move into or out of the area.
- **End or dispel early** is an explicit control. Ending an effect removes its row and every linked condition at once, and writes a log entry ("Wall of Fire dispelled, round 4").
- Effects with a duration end automatically when it runs out, in the same way.
- If the source is removed from combat, the DM is asked whether to keep or end their effects. Nothing is ended without asking.

### R6. Combat log

- An **append-only Markdown file**, readable outside the app without any tools.
- **Manual entries:** the DM can write a one-line note at any time.
- **Automatic entries** for key events:
  - combat started / ended
  - each new round (as a section heading)
  - combatant added / removed
  - combatant dropped to 0 HP, or brought back above 0
  - condition applied / expired / removed
  - effect started / ended early / expired
- Every entry is stamped with the **round**, **whose turn it was**, and the **local time**.
- The app never rewrites or truncates existing log content.

### R7. Persistence

- Combat state **autosaves** to a state file after every change.
- Starting the app with an existing state file picks up exactly where it left off (round, turn, HP, conditions, effects), so a crash or closed laptop costs nothing.
- Writes are atomic, so a crash mid-write can't corrupt the state file.

### R8. Ruleset scope

- **The core isn't tied to any one game system.** It tracks HP, temp HP, conditions, round-based durations, and free-text notes. It doesn't apply rules, and dice it rolls never change the fight on their own (the [dice roller](#dice-roller) only shows a result).
- The D&D 5e condition list is the **default preset** for the condition picker (Blinded, Charmed, Deafened, Exhaustion, Frightened, Grappled, Incapacitated, Invisible, Paralyzed, Petrified, Poisoned, Prone, Restrained, Stunned, Unconscious, plus Concentrating). Custom condition names are always allowed.

## Interaction requirements

- Fully usable from the keyboard.
- **Single-key hotkeys** act on the selected row (next turn, damage, heal, add condition, note, …).
- A **`:` command bar** handles faster, more specific input (`:dmg goblin 2 7`, `:add Goblin x6 12 7hp`).
- A help overlay (`?`) lists every key and command.
- Works in a standard 80×24 terminal, using extra width when it's there.

## Non-functional requirements

- A single static Go binary; no runtime dependencies.
- Runs on macOS and Linux (Windows nice-to-have).
- The domain logic (turn order, HP, durations, effects) can be unit-tested without the UI.

## Out of scope (v1)

- Dice rolling (including initiative). A standalone roller came after v1: see [Dice roller](#dice-roller). Initiative is still entered by hand.
- Rules automation (concentration checks, death-save tracking, legendary/lair action rules).
- A player-facing view or masked HP.
- Multiplayer, networking, syncing.
- Importing stat blocks from external sources.
- Full undo/redo (only "previous turn" is in scope). Built after v1: see [Undo and redo](#undo-and-redo).

## Future

- Player-facing view (masked HP descriptors, second terminal or file output).
- ✅ Rulesets: make the tracker correct for systems other than D&D 5e, starting with D&D 3.5. See [Rulesets](#rulesets).
- Encounter templates: preload a prepared set of NPCs before combat starts.
- ✅ Undo and redo of any change. See [Undo and redo](#undo-and-redo).
- Dice roller: type an expression like `2d6+2+1d4` and see the total and each die. See [Dice roller](#dice-roller).
- Dice in damage and healing: roll and apply in one step, with halving, doubling and immunity per damage type and per target. See [Dice in damage and healing](#dice-in-damage-and-healing).
- ✅ Carry over PCs between fights: keep the party's state when ending combat instead of re-entering them every encounter. See [Carry over PCs between fights](#carry-over-pcs-between-fights).
- ✅ Quick initiative pass: step through every combatant entering initiatives back-to-back without repeated navigation. See [Quick initiative pass](#quick-initiative-pass).
- ✅ Fuzzy jump: jump table selection by typing part of a row's name. See [Fuzzy jump](#fuzzy-jump).
- ✅ Rename: rename a combatant or effect in place mid-fight. See [Rename](#rename).
- ✅ Counters / pips (resources): attach lightweight named counters to combatants (Legendary Actions, Reactions, Legendary Resistances, ammo) with optional turn-start reset. See [Counters and resources](#counters-and-resources).
- ✅ Round countdowns: standalone reminders that tick at the start of each new round and alert + log when due. See [Round countdowns](#round-countdowns).
- ✅ End-of-turn durations: allow timed conditions and area effects to count down at the end of their anchor's turn (`1e`) alongside start-of-turn defaults. See [End-of-turn durations](#end-of-turn-durations).

### Rulesets

R8 says the core isn't tied to one game system. That holds for turn order, effects and the log, but a review against D&D 3.5 found places where v1 quietly assumes 5e. This section lists those gaps and the sub-features that close them. The build order is in [DESIGN.md](DESIGN.md#rulesets). RS1–RS10 are built.

A **ruleset** is a per-encounter choice that sets the HP rules, the health states, the duration anchoring and the condition list. It still rolls no dice and automates no rules (R8).

#### Gaps found (3.5 against v1)

| # | Area | v1 behavior | What 3.5 needs | Blocks 3.5? |
|---|---|---|---|---|
| G1 | Negative HP | Damage, set-HP and the add form clamp or refuse below 0. | HP runs to -10: -1 to -9 is dying, -10 is dead. | **Yes** |
| G2 | Healing from below 0 | A creature that should be at -7 sits at 0, so healing 5 gives 5. | Healing 5 at -7 gives -2. | **Yes** (follows from G1) |
| G3 | Health states | One "down" state at HP ≤ 0. | Three: disabled at exactly 0 (still acts), dying below 0, dead at -10. | **Yes** |
| G4 | Nonlethal damage | No field; it can only go in the note. | A separate running total, compared with current HP. | No, but awkward |
| G5 | Max HP changes | Max HP can't be edited after a combatant is added. | Con damage, negative levels and rage change max HP mid-fight. (5e needs it too, less often.) | No |
| G6 | Temp HP | Setting temp HP replaces the old value. | Temp HP from different sources stacks. | No (the DM can add it up) |
| G7 | Duration anchor after an initiative change | A sourced effect counts down on its caster's turn, wherever the caster now is in the order. | Delay and Ready move the caster for good, but the effect still ends on the count it began on. Durations drift by part of a round. | No |
| G8 | Conditions from another creature | A condition counts down on its bearer's turn, so a "1 round" daze expires as the bearer's turn starts, before it costs them anything. Affects 5e as well. | The duration is counted on the originator's turn. Today's workaround is a sourced effect with one target. | No |
| G9 | Condition list | The picker offers only the 5e list. Custom names can be typed. | Shaken, Sickened, Nauseated, Dazed, Fatigued, Staggered and so on. | No |
| G10 | Moving the turn holder later | An initiative edit leaves the turn on the edited row. Moving the turn holder to a later count and pressing `n` starts the next round, so every creature between the old and new count loses its turn this round. Affects 5e as well. | Delay: the creature steps aside, those after it act, and it acts at its new count in the same round. The workaround is to press `n` first and set the new count when it steps in. | No, but easy to get wrong |

Checked and found to need **no change**:

- **A caster dying or leaving.** Nothing ends automatically when a creature drops, in either edition. Removing the row asks keep-or-end (R5), and kept effects count down on their own row, which is the 3.5 rule (a spell outlives its caster, on the count it was cast).
- **Concentration.** Ending it is a manual dispel in both editions.
- **Round-based countdown.** Counting down at the start of the caster's turn is the same as 3.5's "ends just before the initiative count it began on", apart from G7.
- **Initiative.** Decimal tie-breaks and mid-combat edits already cover 3.5 ties and Ready. Delay is covered by RS10.

#### RS1. Choosing a ruleset

- The ruleset is chosen during Setup (a start-up flag and a command) and saved in the state file. Built in: `5e` (the default, identical to v1) and `3.5e`.
- The header shows the ruleset. A state file saved before rulesets existed loads as `5e`.
- It can't be changed once combat has started, since HP already recorded would change meaning.
- A new fight keeps the ruleset of the fight that ended before it, unless the start-up flag says otherwise.

#### RS2. HP below zero (G1, G2)

- The ruleset sets an **HP floor**: 0 for 5e, -10 for 3.5e (anything lower is equally dead).
- Damage, heal and set-HP work on the real value, so healing from -7 counts up from -7. The add form accepts a negative current HP when the ruleset allows it.
- The HP column shows negative values as they are (`-4/22`).

#### RS3. Health states (G3)

- The ruleset defines named states by HP threshold, replacing the single "down" flag:
  - `5e`: **down** at 0.
  - `3.5e`: **disabled** at exactly 0, **dying** from -1 to -9, **dead** at -10.
- Each state has its own look in the table, and each move between states is logged ("Goblin 2 is dying"). Stable is a condition the DM adds by hand (RS8).
- "Bloodied" stays a display-only state in every ruleset.
- No automation: the app doesn't take the 1 HP a round from a dying creature or roll to stabilize. When a dying creature's turn starts, the status line reminds the DM.

#### RS4. Nonlethal damage (G4)

- A per-combatant **nonlethal** total, in rulesets that use it (`3.5e`), shown beside HP when above 0.
- Hotkey and command to add nonlethal damage, or set the total to fix a mistake. Healing lowers the nonlethal total by the amount healed as well as raising HP, even at full HP. Temp HP doesn't absorb it.
- Derived states, shown and logged like RS3: **staggered** when nonlethal equals current HP, **unconscious** when it is higher. A state from HP (disabled, dying, dead) takes precedence.

#### RS5. Editing max HP (G5)

- Set max HP at any time, in every ruleset. If the new max is below current HP, current HP comes down to it.
- Raising max HP doesn't raise current HP; the DM heals if the rule says so.
- Logged ("Orc 1 max HP 30 → 24").

#### RS6. Adding to temp HP (G6)

- The temp HP prompt and command accept `+N` to add to the current value, alongside `N` to replace it. Available in every ruleset; nothing decides for the DM which sources stack.

#### RS7. Duration anchors (G7, G8)

- **Fixed-count effects (G7).** In `3.5e`, a sourced effect counts down at the initiative count it was started on, even if its source later moves (Delay, Ready). Its row stays on that count (D32), and changing the source's initiative asks whether its effects move too (D33). In `5e` it keeps following its source, as in v1.
- **Conditions with a source (G8).** In every ruleset, a condition can name the creature that caused it; it then counts down on that creature's turn (or count, per the rule above) instead of the bearer's. With no source named it behaves as in v1.
- If a condition's source leaves combat, the condition stays and counts down on its bearer's turn again. A condition has no row of its own to hold the source's old count, so it can drift by part of a round; the DM can adjust the remaining rounds by hand (D34).

#### RS8. Condition lists (G9)

- Each ruleset has its own preset list for the picker. `3.5e`: Blinded, Confused, Cowering, Dazed, Dazzled, Deafened, Energy Drained, Entangled, Exhausted, Fascinated, Fatigued, Flat-Footed, Frightened, Grappling, Helpless, Invisible, Nauseated, Panicked, Paralyzed, Petrified, Pinned, Prone, Shaken, Sickened, Stable, Stunned, Turned, Unconscious, plus Concentrating.
- States that come from HP (disabled, dying, dead, staggered) are not in the list; RS3 and RS4 show them.
- Custom names are still always allowed.

#### RS9. Ruleset files

- A ruleset can be loaded from a file, so other systems (Pathfinder, 4e, home rules) need no code change. The file sets the name, HP floor, health-state thresholds, whether nonlethal is tracked, the effect anchoring rule, and the condition list.
- The two built-in rulesets are expressed in the same form, and a bad file is refused at start-up with the reason.

#### RS10. Delay without lost turns (G10)

- Changing the turn holder's initiative to a later count during its turn must not skip the creatures between the old and new count: they still act this round, and the moved creature acts at its new count in the same round, once.
- Moving the turn holder earlier, or editing any other row, behaves as today: the turn stays where it is.
- Its durations count down once that round, at the start of its first turn, not again at its new count.
- Rewind (`p`) still steps back one turn at a time, and the log records the change as today.
- Applies in every ruleset. In `3.5e` the D33 "move them too?" question is still asked first.

#### Still out of scope

Dice, the dying creature's HP loss and stabilization rolls, 5e death saves, massive-damage rules, and concentration checks. These stay with the DM under R8.

### Undo and redo

`p` steps the turn back, but nothing else can be taken back. A mistyped damage amount can be fixed by hand. A mistake that touches several things can't be fixed that way: a group add with the wrong count, an effect on the wrong targets, a Delay to the wrong count, an ended effect, a removed combatant whose conditions and effects went with it. The DM then has to rebuild state from memory in the middle of a fight. UR1–UR5 are built; the design is in [DESIGN.md](DESIGN.md#undo-and-redo).

#### UR1. Undo

- One key takes back the last change, whatever it was, and leaves the fight exactly as it was before it: HP, conditions, effects, initiative, the turn, the round and the counted-down durations.
- One change is one DM action, however many rows it touched: `Goblin x4`, `:dmg 1-4 7`, a Delay that moves its effects too, and removing a combatant and ending its effects are each one step.
- Pressing it again takes back the change before that, back to at least the last 100 changes.
- Changes made in Setup can be undone too, including the ruleset choice and starting combat.

#### UR2. Redo

- A second key puts back what was undone, one step at a time, in order.
- Any new change after an undo drops what's left to redo.

#### UR3. Feedback

- The status line names what was undone or redone ("Undid: Orc took 7 damage"), including changes that are never logged, such as HP arithmetic.
- The selection moves to the row the step changed, if it's still there.
- With nothing to undo or redo, the status line says so, and nothing changes.

#### UR4. Log and state

- The log stays append-only. Undoing a change that wrote log lines adds a line saying it was undone, and redoing it writes its lines again. Undoing a change that wrote nothing logs nothing.
- The state file is saved after an undo or redo, as after any change.

#### UR5. Boundaries

- Ending combat (already behind a confirmation, and archiving the state file) can't be undone; the undo history starts again with the new fight.
- `p` keeps its meaning: it steps the turn back and keeps changes made since. Undo is strictly last-in, first-out.
- The history lasts while `dmct` runs. Quitting and starting again resumes the fight with nothing to undo.

### Dice roller

Picking dice from a menu is slow at the table. The DM should be able to type what the rulebook says (`4d6+4`, `2d6+2+1d4`) and see the result at once, without leaving the tracker. The roller is a tool beside the fight: it rolls and shows, and nothing in the fight changes.

#### DR1. Expressions

- An expression is a sum of dice terms (`NdS`) and whole numbers, joined by `+` or `-`: `4d6+4`, `2d6+2+1d4`, `1d8-1`, `10-1d4`.
- `d` with no count means one die: `d20+5` is `1d20+5`.
- Spaces and letter case don't matter: `2D6 + 1d4` works. A space that splits a term or joins two terms with no sign (`2d6 3`) is refused, not read as `2d63`.
- Input that isn't an expression is refused with the reason, and the text stays so it can be fixed.

#### DR2. Rolling

- One key opens a roll prompt; Enter rolls. Enter on an empty prompt rolls the last expression again.
- The command bar rolls too (`:roll 2d6+2+1d4`).
- Rolling works in Setup and in combat, and never changes the fight: no save, no undo step, no log line.

#### DR3. Result

- The status line shows the expression and its total: `2d6+2+1d4 = 14`.

#### DR4. Breakdown

- After the total, the status line shows what each die rolled, in set order: terms in the order they were typed, and each term's dice in the order they were rolled. Flat numbers appear with their sign: `2d6+2+1d4 = 14 · [5 3] +2 [4]`.
- The total always shows; on a narrow terminal the breakdown is cut short with `…` first.

#### DR5. Dice in damage and healing

Moved to its own section: [Dice in damage and healing](#dice-in-damage-and-healing) (DD1–DD6).

#### Not in the roller

Keep/drop (`4d6kh3`), advantage and disadvantage, `d%`, and rolling initiative for combatants. Each could be added to the grammar later without changing DR1–DR4. (Multiplying, dividing and brackets came with [DD1](#dd1-modifiers).)

### Dice in damage and healing

Rolling with `r` and then typing the total into `d` is two steps for every hit. The amount prompts should take the dice directly. Damage is often not one number, though: a hit can be half fire and half poison, one target saved and another is immune to one of them. The DM should be able to say all of that in the amount they type, without the tracker storing resistances or knowing what fire is (R8).

#### DD1. Modifiers

- `/N` divides, rounding down; `*N` multiplies. `/2` is resistance or a save for half, `*2` is vulnerability, `*0` is immunity.
- They follow the usual order of arithmetic: a modifier applies to the term just before it, and brackets group. `2d6+1d4/2` is 2d6 plus half of 1d4; `(2d6+3)/2` halves the whole hit; `2d6+3/2` is 2d6+1.
- Rounding happens where the division is, so `1d6/2+1d6/2` can differ from `(2d6)/2`.
- The `r` roller reads the same grammar.

#### DD2. Parts

- A word after a term or bracket names a **part**: everything since the previous name, or the start. `1d6+3 fire + 1d6 poison` has a fire part (1d6+3) and a poison part (1d6).
- A modifier right after a name applies to that whole part: `1d6+3 fire/2 + 1d6 poison*2`.
- Names are any word of letters (`fire`, `slashing`, `blarg`); the tracker only matches them to each other. `d` followed by a digit is always a die.
- Without names, the whole amount is one part.

#### DD3. Amounts with dice

- The damage, heal, temp HP and nonlethal prompts (`d`, `h`, `t`, `D`) and their commands (`:dmg`, `:heal`, `:temp`, `:nl`) take an expression wherever they take a number today. A plain number with modifiers works too: `24/2`.
- In an amount, each bracket group and each part stops at 0, so resistance 10 (`(1d6-10) fire + 1d6 poison`) can't eat into the poison, and an amount is never negative.
- Setting HP (`=N`, `:hp`) and max HP (`M`, `:maxhp`) stay plain numbers.
- A command with several targets rolls once, and every target takes the same amount (unless DD4 says otherwise).
- One amount is one change: one undo step, and redo gives back the same roll.

#### DD4. A different amount for each target

- In a command, each target can carry its own modifiers: `g2/2` for every part, or `g3[fire*0]` for a named part, or both: `mira[/2 poison*2]`.
- A target's modifiers apply after the amount's own, left to right. A modifier without a name applies to each part separately, and each part rounds down on its own and stops at 0.
- A name that isn't a part of the amount is refused.

#### DD5. What the DM sees

- The status line says what each target took, then the roll: `Goblin 1 took 14 damage · 2d6+3 = 14 · [5 6] +3`.
- With several targets: `8d6 = 29 · Goblin 1 29, Goblin 2 14, Goblin 3 0 · [faces]`. With named parts, each target's split follows its number when it differs from the roll's: `Goblin 2 4 (fire 2, poison 2)`.
- The faces come last, so a narrow terminal cuts them first.
- Damage and healing stay out of the combat log, as before; the undo summary names what each target took.

#### DD6. Repeating

- Enter on an empty amount prompt is still refused, so a stray key never deals damage.
- `↑` in an amount prompt brings back earlier amounts. Amounts are kept apart from `r`, so `r` then enter still repeats the attack roll and not the damage.

#### Later

Picking targets on the table and marking each one saved, resistant, vulnerable or immune with a key, instead of typing brackets.

### Carry over PCs between fights

In a dungeon crawl or multi-encounter session, re-entering four to six PCs (with their remaining HP, temp HP, and lingering conditions) at the start of every fight is tedious. Ending combat should let the DM carry the party straight into the next fight's Setup. The design is in [DESIGN.md](DESIGN.md#carry-over-pcs-between-fights).

#### CP1. What carries over

- Only combatants with kind `PC` carry over. NPCs and area effects are dropped.
- Every PC carries over, including downed or dead ones, preserving their previous `AddedSeq` order.
- Each carried-over PC keeps its `Name`, `Kind`, `HP`, `MaxHP`, `TempHP`, `Nonlethal` (for rulesets that track it), `Note`, and **indefinite conditions** (those with no round duration, with any `SourceID` cleared).
- Timed conditions (those with a remaining round count) are dropped.

#### CP2. Initiative

- Every carried-over PC starts the new Setup with initiative `0`.
- Existing exact-tie warnings (`!`) highlight multiple PCs at `0` so the DM remembers to enter fresh initiatives before starting combat.

#### CP3. Ending combat

- When the fight has at least one PC, `E` asks whether to keep PCs or clear everything: `[k] keep PCs`, `[c] clear all`, `esc` cancel.
- When there are no PCs in the fight, `E` stays a plain `[y]/[n]` confirmation.
- In the command bar, `:endcombat` carries PCs over by default; `:endcombat clear` ends combat and starts an empty Setup.

#### CP4. Persistence and undo

- The finished fight is still archived as before (`dmct-state-YYYYMMDD-HHMMSS.json`).
- When PCs are carried over, the new Setup encounter (with the same ruleset and carried-over PCs) is saved to the state file immediately after archiving so quitting right away doesn't lose the party.
- When starting empty (`c` or `:endcombat clear`, or when no PCs existed), no new state file is written until the first Setup edit, as before.
- Ending combat remains an undo barrier (UR5): the undo stack starts fresh for the new Setup.

### Quick initiative pass

When PCs carry over at initiative `0`, or at the start of a new round in systems that re-roll initiative, pressing `j`, `i`, number, Enter for every row is slow. A single key should walk down the roster prompting only for each combatant's initiative. The design is in [DESIGN.md](DESIGN.md#quick-initiative-pass).

#### QI1. Scope and order

- Available in both Setup and Active combat, via hotkey `I` or command `:initall`.
- Visits every **combatant** row (skipping effect rows) in the table order at the moment the pass begins, so live re-sorting never skips or repeats a combatant.
- If there are no combatants, warns and does nothing.

#### QI2. Stepping through combatants

- As each combatant is prompted, the table selection highlights that row and the prompt shows progress and current initiative, e.g. `Initiative (1/4) Thorin (now 0):`.
- Pressing Enter with a new value applies it immediately and re-sorts the table live before advancing to the next combatant.
- Pressing Enter on an empty prompt (or entering the same value) keeps the combatant's current initiative unchanged and advances to the next.
- An invalid value keeps the prompt open with the reason so it can be fixed in place.

#### QI3. Stopping early and Active combat

- Pressing `esc` (or `ctrl+c`) stops the pass and keeps every initiative already entered.
- During Active combat, the pass updates initiatives directly without triggering Delay turn-passing or per-row move-effects prompts; the current turn stays on the same row.

#### QI4. Undo and persistence

- Each applied value autosaves immediately as usual (D6).
- All changes made during one pass collapse into a **single undo step** (UR1): pressing `u` once restores every combatant edited during the pass, and `U` puts all of them back.

### Fuzzy jump

In large fights with fifteen or more rows (combatants and area effects), tapping `j` or `k` to reach a target is slow. Pressing `/` should let the DM jump straight to any row by typing part of its name. The design is in [DESIGN.md](DESIGN.md#fuzzy-jump).

#### FJ1. Matching

- Works in both Setup and Active combat, across every row in the table (combatants and effects).
- Matching is case-insensitive and ranked in three tiers, keeping table order within each tier:
  1. Prefix match (ignoring spaces, so `band` matches Bandit Captain).
  2. Substring match (`cap` matches Bandit Captain, `guard` matches Spirit Guardians).
  3. Subsequence (fuzzy) match (`bc` matches Bandit Captain, `g3` matches Goblin 3).

#### FJ2. Live navigation

- Selection jumps live to the best match as each character is typed, scrolling the table to keep it visible.
- `tab` / `↓` (`ctrl+n`) cycle to the next match; `shift+tab` / `↑` (`ctrl+p`) cycle to the previous match.
- The hint line shows the current match index and total (`1/3`), or `No match` when nothing matches (leaving selection on the starting row).

#### FJ3. Accepting and cancelling

- `enter` closes the prompt and keeps the selected row.
- `esc` (or `ctrl+c`) closes the prompt and restores the selection and scroll position from before `/` was pressed.
- Jumping changes no combat state: no save, no log line, no undo step.

### Rename

When a disguised enemy reveals its identity ("Hooded Figure" → "Strahd"), a goblin gets a distinguishing label ("Goblin 2" → "Goblin Boss"), or a name was mistyped, the DM should be able to rename the row in place without removing and re-adding it. The design is in [DESIGN.md](DESIGN.md#rename).

#### RN1. Scope

- Works on any row (combatant or area effect) in Setup or Active combat.
- Blank names are refused. Entering the exact same name is a no-op (no event, no log, no undo step).
- Because linked conditions and condition sources reference rows by ID, renaming automatically updates every linked effect and sourced condition across the table.

#### RN2. Controls

- Hotkey `R` opens a `Rename <name>:` prompt prefilled with the row's current name.
- Command `:rename TARGET NEWNAME` renames a single matching row (combatant or effect).

#### RN3. Log and undo

- During Active combat, renaming is written to the combat log (`Hooded Figure renamed to Strahd`) so later recap entries stay coherent. During Setup, it stays unlogged (D20).
- Fully undoable and redoable as one step (`u` / `U`), moving selection back to the renamed row.

### Counters and resources

Bosses and PCs often have per-turn or limited-use resources — Legendary Actions, Reactions, Legendary Resistances, ki points, spell slots, or arrows — that currently have to be scribbled into the free-text note. Generic named counters let the DM track any resource without baking system-specific rules into the tracker (R8). The design is in [DESIGN.md](DESIGN.md#counters-and-resources).

#### RP1. Bounded and unbounded counters

- Any combatant can hold any number of named counters (`Resources`), matched case-insensitively by name.
- A counter is either:
  - **Bounded** (`cur/max`, e.g. `LA 2/3`), clamped to `[0, max]`, with an optional **reset on turn start** flag (resets `cur` to `max` at the start of that combatant's turn).
  - **Unbounded** (`cur`, e.g. `Arrows 14`), clamped to `≥ 0`.
- Turn-start resets are recorded in `TurnRecord` so `p` (previous turn) restores the pre-reset value, and Delay (`Delayed`) skips resetting a second time in the same round.

#### RP2. Display

- Counters appear inline at the end of the **Conditions** column in compact brackets — `[LA 2/3↻]` (with `↻` when turn-reset is enabled), `[LR 3/3]`, `[Arrows 14]` — keeping the 80×24 column layout intact.
- When truncated in a narrow cell, the status line spells them out when the row is selected, just like conditions.

#### RP3. Controls

- Hotkey `P` on a selected combatant opens a single `Resource on <name>:` prompt (tab completes existing counter names on that row):
  - `NAME CUR/MAX [reset]` (or `NAME /MAX [reset]` starting full): create or update a bounded counter (`r` accepted as shorthand for `reset`).
  - `NAME N`: create an unbounded counter, or set an existing bounded counter's current value to `N`.
  - `NAME +N` / `NAME -N`: adjust an existing counter up or down.
  - `NAME rm`: remove an existing counter.
- Command bar `:res TARGET …` takes the same syntax after the target.

#### RP4. Carry-over, log, and undo

- When PCs carry over between fights (`E` → `k` or `:endcombat`), their counters carry over too: normal counters keep their current value, and turn-reset counters reset to their max.
- Counter edits are quiet like HP arithmetic (not written to the Markdown log), but fully undoable/redoable (`u`/`U`) with clear status summaries.

### Round countdowns

Fights often have timed events that aren't tied to any creature or targets — reinforcements arriving in 3 rounds, a ritual completing on round 5, a bridge collapsing in 2 rounds. Using an unsourced area effect for these adds noise to the turn order. Standalone round countdowns keep them visible without adding fake initiative rows. The design is in [DESIGN.md](DESIGN.md#round-countdowns).

#### RC1. Creating and cancelling countdowns

- A countdown (`Timer`) has a non-empty name and a remaining round count (`≥ 1`).
- Can be set either as **relative** (`in N rounds`, e.g. `3 Bridge collapses`) or by **target round** (`rN` / `@N`, e.g. `r5 Reinforcements arrive`, requiring a future round — `≥ 2` in Setup, `> Round` in Active).
- Can be cancelled early (`rm NAME`, matched case-insensitively by prefix or exact name).

#### RC2. Ticking and firing

- Countdowns tick once at the **start of each new round** (when advancing past the bottom of the turn order into Round `R+1`, never on the opening `Start`, matching D15).
- When a countdown reaches `0`, it is removed, emits an expiry event, shows a prominent alert on the status line (`⏱ Reinforcements arrive`), and writes an entry to the combat log (`⏱ Reinforcements arrive`).
- Rewinding across a round boundary with `p` restores ticked and expired countdowns, just as it does for conditions and effects.

#### RC3. Display and controls

- While any countdown is active, a compact banner line under the title shows every active countdown (`⏱ Reinforcements arrive (2 rds) · Bridge collapses (1 rd)`). When none are active, it takes zero lines.
- Hotkey `A` (alarm/timer) opens a `Timer:` prompt (`N TEXT`, `rN TEXT`, or `rm NAME`, with tab completion after `rm `).
- Command bar `:timer N TEXT`, `:timer rN TEXT`, and `:timer rm NAME` do the same.

#### RC4. Persistence, log, and undo

- Active countdowns autosave in the state file and do not carry over into the next fight.
- Setting, cancelling, and firing a countdown are logged during Active combat (`Timer set: …`, `Timer cancelled: …`, `⏱ …`), and all changes are undoable/redoable (`u`/`U`).

### End-of-turn durations

D12 counts durations down at the start of their anchor's turn, which covers "until the start of your next turn" cleanly, but many 5e abilities last "until the end of your turn" or "until the end of your next turn" (Dodge, Shield, Stunning Strike, Guiding Bolt, Mind Sliver). Adding an optional end-of-turn flag (`e`) lets the DM track both without removing conditions by hand. The design is in [DESIGN.md](DESIGN.md#end-of-turn-durations).

#### ET1. Syntax and scope

- Applies to both **timed conditions** and **timed area effects**.
- Anywhere a round count is typed — the `c` and `e` duration prompts, `:cond TARGETS NAME ROUNDS[e] ...`, and `:fx NAME ROUNDS[e] ...` — a trailing `e` (case-insensitive, e.g. `1e`, `2e`) marks the duration as **end-of-turn**. Plain numbers (`1`, `2`) keep start-of-turn behavior unchanged.
- Indefinite conditions and effects ignore end-of-turn.

#### ET2. Ticking and anchor rules

- An end-of-turn duration counts down when `Next` (`n` / `:next`) **leaves** its anchor's turn (before moving to the next row).
  - So `1e` anchored to the current turn holder expires at the end of **this** turn; `2e` ticks `2e → 1e` at the end of this turn and expires at the end of its **next** turn.
  - `1e` anchored to another creature expires at the end of that creature's upcoming turn.
- Anchoring follows the same rules as start-of-turn (D12, D32, D34):
  - Conditions anchor to their `SourceID` when set, otherwise their bearer. If the source leaves combat, the condition keeps `EndOfTurn` and anchors to its bearer.
  - Under `5e` (`!FixedAnchor`), sourced effects anchor to their source; unsourced effects anchor to their own row. Under `3.5e` (`FixedAnchor`), all effects anchor to their own row.
- Stepping aside on **Delay** (RS10) does not end the creature's turn, so its end-of-turn durations wait until its delayed turn ends.
- If an unsourced end-of-turn effect expires as `Next` leaves its own row, the turn still moves cleanly to the row that followed it (or wraps into the next round).

#### ET3. Rewind and undo

- `Previous` (`p`) restores end-of-turn decrements and expirations from the rewound turn, just as it does for start-of-turn ticks.
- Full snapshot undo/redo (`u`/`U`) works automatically.

#### ET4. Display and log

- The table marks end-of-turn durations with `e`: `Stunned (1e)`, `Dazed (1e by Mira)`, and on effect rows `1e rd · 2 targets` / `2e rds · 2 targets`.
- The combat log notes end-of-turn when applied/started (`Stunned applied to Goblin 1 (1 round, end of turn)`), and expirations are stamped with the leaving turn holder's round and name.







