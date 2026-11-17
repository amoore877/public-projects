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

- **The core isn't tied to any one game system.** It tracks HP, temp HP, conditions, round-based durations, and free-text notes. It doesn't roll dice or apply rules.
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

- Dice rolling (including initiative).
- Rules automation (concentration checks, death-save tracking, legendary/lair action rules).
- A player-facing view or masked HP.
- Multiplayer, networking, syncing.
- Importing stat blocks from external sources.
- Full undo/redo (only "previous turn" is in scope).

## Future

- Player-facing view (masked HP descriptors, second terminal or file output).
- Rulesets: make the tracker correct for systems other than D&D 5e, starting with D&D 3.5. See [Rulesets](#rulesets).
- Encounter templates: preload a prepared set of NPCs before combat starts.
- General undo stack.

### Rulesets

R8 says the core isn't tied to one game system. That holds for turn order, effects and the log, but a review against D&D 3.5 found places where v1 quietly assumes 5e. This section lists those gaps and the sub-features that close them. The build order is in [DESIGN.md](DESIGN.md#rulesets). RS1, RS2, RS3 and RS8 are built; the rest are planned.

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

Checked and found to need **no change**:

- **A caster dying or leaving.** Nothing ends automatically when a creature drops, in either edition. Removing the row asks keep-or-end (R5), and kept effects count down on their own row, which is the 3.5 rule (a spell outlives its caster, on the count it was cast).
- **Concentration.** Ending it is a manual dispel in both editions.
- **Round-based countdown.** Counting down at the start of the caster's turn is the same as 3.5's "ends just before the initiative count it began on", apart from G7.
- **Initiative.** Decimal tie-breaks and mid-combat edits already cover 3.5 ties, Delay and Ready.

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
- Hotkey and command to add nonlethal damage. Healing lowers the nonlethal total by the amount healed as well as raising HP.
- Derived states, shown and logged like RS3: **staggered** when nonlethal equals current HP, **unconscious** when it is higher.

#### RS5. Editing max HP (G5)

- Set max HP at any time, in every ruleset. If the new max is below current HP, current HP comes down to it.
- Raising max HP doesn't raise current HP; the DM heals if the rule says so.
- Logged ("Orc 1 max HP 30 → 24").

#### RS6. Adding to temp HP (G6)

- The temp HP prompt and command accept `+N` to add to the current value, alongside `N` to replace it. Available in every ruleset; nothing decides for the DM which sources stack.

#### RS7. Duration anchors (G7, G8)

- **Fixed-count effects (G7).** In `3.5e`, a sourced effect counts down at the initiative count it was started on, even if its source later moves. In `5e` it keeps following its source, as in v1.
- **Conditions with a source (G8).** In every ruleset, a condition can name the creature that caused it; it then counts down on that creature's turn (or count, per the rule above) instead of the bearer's. With no source named it behaves as in v1.
- If a condition's source leaves combat, the condition stays and counts down on the source's last initiative count, the same as a kept effect.

#### RS8. Condition lists (G9)

- Each ruleset has its own preset list for the picker. `3.5e`: Blinded, Confused, Cowering, Dazed, Dazzled, Deafened, Energy Drained, Entangled, Exhausted, Fascinated, Fatigued, Flat-Footed, Frightened, Grappling, Helpless, Invisible, Nauseated, Panicked, Paralyzed, Petrified, Pinned, Prone, Shaken, Sickened, Stable, Stunned, Turned, Unconscious, plus Concentrating.
- States that come from HP (disabled, dying, dead, staggered) are not in the list; RS3 and RS4 show them.
- Custom names are still always allowed.

#### RS9. Ruleset files

- A ruleset can be loaded from a file, so other systems (Pathfinder, 4e, home rules) need no code change. The file sets the name, HP floor, health-state thresholds, whether nonlethal is tracked, the effect anchoring rule, and the condition list.
- The two built-in rulesets are expressed in the same form, and a bad file is refused at start-up with the reason.

#### Still out of scope

Dice, the dying creature's HP loss and stabilization rolls, 5e death saves, massive-damage rules, and concentration checks. These stay with the DM under R8.
