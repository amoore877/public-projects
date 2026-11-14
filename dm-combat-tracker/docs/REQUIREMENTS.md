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
- Ruleset preset files (condition lists for other systems).
- Encounter templates: preload a prepared set of NPCs before combat starts.
- General undo stack.
