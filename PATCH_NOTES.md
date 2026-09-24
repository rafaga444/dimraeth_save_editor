# Patch notes — September 24, 2026

- Updated every patch offset and the supported DLL SHA-256 for build 99bba4f3.
- Updated relative CALL instruction bytes for RuneDropCheck and ReturnRandomRuneData (rarity and stars).

# Patch notes — September 22, 2026

- Fixed a startup crash caused by oversized skill-point patch padding; return patches now pad to the original instruction length automatically.
- Updated all patch offsets and instructions to diagnostic v13.
- Fixed rarity writes and zero-based stars; star selection is limited to 1–3.
- RNG eliminator now combines guaranteed positive-chance item/equipment drops and the gear-legality bypass.
- Updated editable attributes, attribute cap, skill points, and level cap patches; existing settings are read from the DLL.

# Patch notes — September 21, 2026

- Fixed the Windows gray overlay and renamed Loot patcher to Patcher.
- Added RNG eliminator and stars 1–9, with level requirements in the dropdown.
- Added Editable attributes, No attribute cap, Editable skill points, and Max level cap (25–127).
- Attribute edits now warn and automatically enable Editable attributes; apply it with Save in Patcher.
- Added level/XP synchronization and removed duplicate star descriptions.
