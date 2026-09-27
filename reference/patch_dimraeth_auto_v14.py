#!/usr/bin/env python3
"""
Dimraeth automatic patcher v14.

No build SHA whitelist and no absolute GameAssembly.dll file offsets.

Patch locations are resolved at runtime by unique AOB signatures with
wildcards plus surrounding-instruction context. The script searches only
executable PE sections (normally .text + il2cpp).

Safety model:
  * every requested patch location must resolve to EXACTLY one match;
  * the bytes at the actual patch target are verified again;
  * every patch is planned/verified before anything is written;
  * if one signature is missing or ambiguous, the DLL is left unchanged.

Progression patches are enabled by default:
  - max level
  - editable attributes
  - no attribute cap
  - editable skill points

Optional:
  --loot-x3
  --guaranteed-item
  --guaranteed-equipment
  --ancient
  --stars N
  --gear-legality-bypass

Note: forcing star tiers that the current game content does not fully support
can still create unusable equipment. Auto-location only makes the binary
patch locations update-resistant; it does not change game-data limitations.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import shutil
import struct
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable


# ---------------------------------------------------------------------------
# Basic helpers
# ---------------------------------------------------------------------------

def hx(s: str) -> bytes:
    return bytes.fromhex(s)


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


@dataclass(frozen=True)
class Section:
    name: str
    raw_offset: int
    raw_size: int
    virtual_address: int
    virtual_size: int
    characteristics: int

    @property
    def raw_end(self) -> int:
        return self.raw_offset + self.raw_size

    @property
    def executable(self) -> bool:
        # IMAGE_SCN_MEM_EXECUTE
        return bool(self.characteristics & 0x20000000)


@dataclass(frozen=True)
class PEInfo:
    image_base: int
    sections: tuple[Section, ...]


def parse_pe(data: bytes) -> PEInfo:
    if len(data) < 0x100 or data[:2] != b"MZ":
        raise RuntimeError("Input is not a valid PE file (missing MZ header)")

    pe_off = struct.unpack_from("<I", data, 0x3C)[0]
    if pe_off + 24 > len(data) or data[pe_off:pe_off + 4] != b"PE\0\0":
        raise RuntimeError("Input is not a valid PE file (missing PE header)")

    coff = pe_off + 4
    number_of_sections = struct.unpack_from("<H", data, coff + 2)[0]
    size_of_optional_header = struct.unpack_from("<H", data, coff + 16)[0]

    opt = coff + 20
    if opt + size_of_optional_header > len(data):
        raise RuntimeError("Truncated PE optional header")

    magic = struct.unpack_from("<H", data, opt)[0]
    if magic == 0x20B:  # PE32+
        image_base = struct.unpack_from("<Q", data, opt + 24)[0]
    elif magic == 0x10B:  # PE32
        image_base = struct.unpack_from("<I", data, opt + 28)[0]
    else:
        raise RuntimeError(f"Unsupported PE optional-header magic 0x{magic:X}")

    section_table = opt + size_of_optional_header
    sections: list[Section] = []

    for i in range(number_of_sections):
        off = section_table + i * 40
        if off + 40 > len(data):
            raise RuntimeError("Truncated PE section table")

        name = data[off:off + 8].split(b"\0", 1)[0].decode(
            "ascii", errors="replace"
        )
        virtual_size, virtual_address, raw_size, raw_offset = struct.unpack_from(
            "<IIII", data, off + 8
        )
        characteristics = struct.unpack_from("<I", data, off + 36)[0]

        if raw_offset + raw_size > len(data):
            # Some PEs can have alignment padding beyond EOF. Clamp only if
            # the section actually starts inside the file.
            if raw_offset >= len(data):
                continue
            raw_size = len(data) - raw_offset

        sections.append(
            Section(
                name=name,
                raw_offset=raw_offset,
                raw_size=raw_size,
                virtual_address=virtual_address,
                virtual_size=virtual_size,
                characteristics=characteristics,
            )
        )

    return PEInfo(image_base=image_base, sections=tuple(sections))


# ---------------------------------------------------------------------------
# AOB signature engine
# ---------------------------------------------------------------------------

@dataclass(frozen=True)
class AOB:
    values: tuple[int, ...]
    mask: tuple[bool, ...]  # True = exact byte, False = wildcard

    @property
    def length(self) -> int:
        return len(self.values)


def parse_aob(pattern: str) -> AOB:
    values: list[int] = []
    mask: list[bool] = []

    for token in pattern.split():
        if token in {"?", "??"}:
            values.append(0)
            mask.append(False)
        else:
            if len(token) != 2:
                raise ValueError(f"Bad AOB token: {token!r}")
            values.append(int(token, 16))
            mask.append(True)

    if not values:
        raise ValueError("Empty AOB signature")

    return AOB(tuple(values), tuple(mask))


def _longest_exact_run(aob: AOB) -> tuple[int, bytes]:
    best_start = -1
    best = b""

    i = 0
    while i < aob.length:
        if not aob.mask[i]:
            i += 1
            continue

        j = i
        run = bytearray()
        while j < aob.length and aob.mask[j]:
            run.append(aob.values[j])
            j += 1

        if len(run) > len(best):
            best_start = i
            best = bytes(run)

        i = j

    if best_start < 0:
        raise ValueError("AOB cannot consist entirely of wildcards")

    return best_start, best


def aob_matches_at(data: bytes | bytearray, offset: int, aob: AOB) -> bool:
    if offset < 0 or offset + aob.length > len(data):
        return False

    for i, exact in enumerate(aob.mask):
        if exact and data[offset + i] != aob.values[i]:
            return False
    return True


def find_aob_in_range(
    data: bytes,
    aob: AOB,
    start: int,
    end: int,
) -> list[int]:
    anchor_rel, anchor = _longest_exact_run(aob)
    matches: list[int] = []

    pos = start
    while True:
        found = data.find(anchor, pos, end)
        if found < 0:
            break

        candidate = found - anchor_rel
        if (
            candidate >= start
            and candidate + aob.length <= end
            and aob_matches_at(data, candidate, aob)
        ):
            matches.append(candidate)

        pos = found + 1

    return matches


@dataclass(frozen=True)
class Match:
    file_offset: int
    section: Section
    rva: int
    va: int


class Resolver:
    def __init__(self, data: bytes):
        self.data = data
        self.pe = parse_pe(data)
        self.exec_sections = tuple(
            s for s in self.pe.sections if s.executable and s.raw_size > 0
        )

        if not self.exec_sections:
            raise RuntimeError("PE contains no executable sections")

        self._cache: dict[str, Match] = {}

    def resolve_unique(self, label: str, pattern: str) -> Match:
        cache_key = pattern
        if cache_key in self._cache:
            return self._cache[cache_key]

        aob = parse_aob(pattern)
        found: list[Match] = []

        for section in self.exec_sections:
            for off in find_aob_in_range(
                self.data, aob, section.raw_offset, section.raw_end
            ):
                rva = section.virtual_address + (off - section.raw_offset)
                found.append(
                    Match(
                        file_offset=off,
                        section=section,
                        rva=rva,
                        va=self.pe.image_base + rva,
                    )
                )

        if len(found) != 1:
            details = ""
            if found:
                details = "\nMatches:\n" + "\n".join(
                    f"  {m.section.name}: file=0x{m.file_offset:X}, "
                    f"VA=0x{m.va:X}"
                    for m in found[:20]
                )

            raise RuntimeError(
                f"{label}: expected exactly 1 AOB match, found {len(found)}."
                f"{details}\n"
                "This game build changed too much or the signature became "
                "ambiguous. No DLL modifications were written."
            )

        match = found[0]
        self._cache[cache_key] = match
        print(
            f"[FOUND] {label:<38} "
            f"{match.section.name:<8} "
            f"file=0x{match.file_offset:X} VA=0x{match.va:X}"
        )
        return match


# ---------------------------------------------------------------------------
# Stable signatures
#
# These deliberately include semantic context around each target. rel32 CALLs
# and RIP-relative addresses that naturally move between builds are `??`.
# ---------------------------------------------------------------------------

SIG_MAX_LEVEL_PAIR = (
    "83 FB 19 7C EA 33 D2 B9 19 00 00 00 "
    "E8 ?? ?? ?? ?? "
    "99 2B C2 D1 F8 48 63 C8 48 03 CF 48 8B 7C 24 38"
)

SIG_HIGHEST_LEVEL = (
    "BA 19 00 00 00 45 33 C0 2B D3 33 C9 "
    "E8 ?? ?? ?? ?? "
    "44 8B C8 45 33 C0 8B C5 99 2B C2 41 8B D1 D1 F8"
)

SIG_VALIDATE_LOADED_XP = (
    "40 53 55 56 57 41 56 "
    "48 81 EC B0 00 00 00 "
    "80 3D ?? ?? ?? ?? 00 "
    "48 8B F9 75 5B "
    "48 8D 0D ?? ?? ?? ?? "
    "E8 ?? ?? ?? ?? "
    "48 8D 0D"
)

SIG_VALIDATE_ATTRIBUTE_XP = (
    "48 89 4C 24 08 53 56 57 41 54 41 55 41 56 41 57 "
    "48 81 EC 30 01 00 00 "
    "0F 29 B4 24 20 01 00 00 "
    "4C 8B F1 "
    "80 3D ?? ?? ?? ?? 00 "
    "0F 85 BB 00 00 00"
)

SIG_VALIDATE_XP_INVARIANTS = (
    "48 89 4C 24 08 53 56 57 41 54 41 55 41 56 41 57 "
    "48 81 EC 50 01 00 00 "
    "0F 29 B4 24 40 01 00 00 "
    "48 8B F9 "
    "80 3D ?? ?? ?? ?? 00 "
    "0F 85 1B 01 00 00"
)

SIG_VALIDATE_XP_STATE = (
    "48 89 4C 24 08 53 56 57 41 54 41 55 41 56 41 57 "
    "48 81 EC D0 01 00 00 "
    "0F 29 B4 24 C0 01 00 00 "
    "48 8B F1 "
    "80 3D ?? ?? ?? ?? 00 "
    "0F 85 0B 02 00 00"
)

SIG_ATTRIBUTE_CAP_1 = (
    "45 33 C0 8B D3 49 8B CF "
    "E8 ?? ?? ?? ?? "
    "83 F8 63 0F 8D D8 FE FF FF "
    "48 8B 0D ?? ?? ?? ?? "
    "83 B9 E4 00 00 00 00"
)

SIG_ATTRIBUTE_CAP_2 = (
    "48 8B D0 48 8B CB 4C 8B F8 "
    "E8 ?? ?? ?? ?? "
    "83 F8 63 0F 8D 0A 01 00 00 "
    "48 89 6C 24 68 48 89 74 24 40"
)

SIG_ATTRIBUTE_CAP_3 = (
    "45 33 C0 8B D5 48 8B CF "
    "E8 ?? ?? ?? ?? "
    "44 8B F0 "
    "83 F8 63 0F 8D 74 04 00 00 "
    "4C 8B 05 ?? ?? ?? ?? 8D 50 01"
)

SIG_ATTRIBUTE_CAP_4 = (
    "33 C0 41 8B D5 48 8B CF "
    "E8 ?? ?? ?? ?? "
    "44 8B F0 "
    "83 F8 63 0F 8D FC 04 00 00 "
    "4C 8B 05 ?? ?? ?? ?? 8D 50 01"
)

SIG_SKILL_OVER_GRANT = (
    "40 53 48 83 EC 20 48 8B D9 41 3B D0 7E 42 0F 57 "
    "C0 0F 2F 81 F0 06 00 00 77 1F 33 C9 "
    "E8 ?? ?? ?? ?? "
    "F3 0F 5C 83 F0 06 00 00"
)

SIG_SKILL_RECONCILE = (
    "41 2B F7 "
    "E8 ?? ?? ?? ?? "
    "44 8B E8 3B F0 "
    "0F 84 99 02 00 00 "
    "48 8B 0D ?? ?? ?? ?? "
    "BA 04 00 00 00 "
    "E8 ?? ?? ?? ?? "
    "48 8B D8"
)

SIG_LOOT_MULTIPLIER = (
    "48 83 EC 28 33 D2 E8 25 F8 FF FF "
    "48 85 C0 74 0A F3 0F 10 40 40 48 83 C4 28 C3"
)

SIG_GUARANTEED_ITEM = (
    "45 33 C0 41 8B D7 48 8B CF "
    "E8 30 E1 FF FF "
    "44 3B F0 7D AB "
    "41 8D 55 01 45 33 C0 41 8B CC "
    "E8 ?? ?? ?? ?? "
    "45 33 F6"
)

SIG_GUARANTEED_EQUIPMENT = (
    "83 B9 E4 00 00 00 00 75 05 "
    "E8 ?? ?? ?? ?? "
    "33 D2 45 33 C0 0F 28 C6 "
    "E8 ?? ?? ?? ?? "
    "84 C0 0F 84 67 04 00 00 "
    "33 D2 48 8B CF"
)

# One signature identifies both the rarity and stars selections. Relative
# target positions within this semantic block are stable across the builds
# this patcher was developed against.
SIG_RETURN_RANDOM_RUNE = (
    "4C 8B 05 ?? ?? ?? ?? "
    "8B D6 49 8B CF "
    "E8 ?? ?? ?? ?? "
    "4C 8B 05 ?? ?? ?? ?? "
    "8B D7 49 8B CE 8B D8 "
    "E8 ?? ?? ?? ?? "
    "44 8B 84 24 50 01 00 00 "
    "48 8D 4C 24 30 "
    "44 8B CB "
    "48 C7 44 24 28 00 00 00 00 "
    "41 8B D4 "
    "89 44 24 20 "
    "E8 ?? ?? ?? ??"
)

SIG_GEAR_LEGALITY_INSPECT = (
    "40 55 53 48 8D 6C 24 D8 "
    "48 81 EC 28 01 00 00 "
    "80 3D ?? ?? ?? ?? 00 "
    "48 8B D9 75 5B "
    "48 8D 0D ?? ?? ?? ?? "
    "E8 ?? ?? ?? ?? "
    "48 8D 0D ?? ?? ?? ?? "
    "E8 ?? ?? ?? ??"
)


# ---------------------------------------------------------------------------
# Transactional patch planning
# ---------------------------------------------------------------------------

@dataclass(frozen=True)
class PlannedPatch:
    offset: int
    expected: AOB
    replacement: bytes
    name: str


class PatchPlanner:
    def __init__(self, original: bytes, resolver: Resolver):
        self.original = original
        self.resolver = resolver
        self.patches: list[PlannedPatch] = []

    def locate(
        self,
        label: str,
        signature: str,
        target_rel: int,
        expected_target: str,
        replacement: bytes,
    ) -> None:
        match = self.resolver.resolve_unique(label, signature)
        target = match.file_offset + target_rel
        expected = parse_aob(expected_target)

        if len(replacement) != expected.length:
            raise RuntimeError(
                f"{label}: replacement size {len(replacement)} != "
                f"target size {expected.length}"
            )

        if not aob_matches_at(self.original, target, expected):
            actual = self.original[target:target + expected.length]
            raise RuntimeError(
                f"{label}: context matched but patch-target verification failed "
                f"at 0x{target:X}.\n"
                f"Expected target: {expected_target}\n"
                f"Actual bytes:    {actual.hex(' ')}\n"
                "No DLL modifications were written."
            )

        self.patches.append(
            PlannedPatch(
                offset=target,
                expected=expected,
                replacement=replacement,
                name=label,
            )
        )

    def verify_no_overlap(self) -> None:
        ordered = sorted(self.patches, key=lambda p: p.offset)
        for left, right in zip(ordered, ordered[1:]):
            left_end = left.offset + len(left.replacement)
            if left_end > right.offset:
                raise RuntimeError(
                    f"Patch overlap: {left.name} and {right.name}. "
                    "No DLL modifications were written."
                )

    def apply(self) -> bytes:
        self.verify_no_overlap()
        data = bytearray(self.original)

        for p in sorted(self.patches, key=lambda p: p.offset):
            # Re-verify against the untouched original, not partially patched
            # bytes. Overlap has already been rejected.
            if not aob_matches_at(self.original, p.offset, p.expected):
                raise RuntimeError(f"{p.name}: target changed during planning")

            data[p.offset:p.offset + len(p.replacement)] = p.replacement
            print(f"[PATCH] {p.name}")

        return bytes(data)


def plan_progression(
    planner: PatchPlanner,
    max_level: int,
) -> None:
    if not 25 <= max_level <= 127:
        raise ValueError("--max-level must be between 25 and 127")

    if max_level != 25:
        imm8 = bytes([max_level])
        imm32 = struct.pack("<I", max_level)

        planner.locate(
            "XP level-loop cap",
            SIG_MAX_LEVEL_PAIR,
            0,
            "83 FB 19",
            b"\x83\xFB" + imm8,
        )
        planner.locate(
            "XP final cap",
            SIG_MAX_LEVEL_PAIR,
            7,
            "B9 19 00 00 00",
            b"\xB9" + imm32,
        )
        planner.locate(
            "Highest-level initialization",
            SIG_HIGHEST_LEVEL,
            0,
            "BA 19 00 00 00",
            b"\xBA" + imm32,
        )

    planner.locate(
        "ValidateLoadedXPData disabled",
        SIG_VALIDATE_LOADED_XP,
        0,
        "40 53 55 56 57 41 56 48",
        hx("C3 90 90 90 90 90 90 90"),
    )
    planner.locate(
        "ValidateAttributeXPConsistency disabled",
        SIG_VALIDATE_ATTRIBUTE_XP,
        0,
        "48 89 4C 24 08 53 56 57",
        hx("C3 90 90 90 90 90 90 90"),
    )
    planner.locate(
        "ValidateXPInvariants forced TRUE",
        SIG_VALIDATE_XP_INVARIANTS,
        0,
        "48 89 4C 24 08 53 56 57",
        hx("B0 01 C3 90 90 90 90 90"),
    )
    planner.locate(
        "ValidateXPState forced TRUE",
        SIG_VALIDATE_XP_STATE,
        0,
        "48 89 4C 24 08 53 56 57",
        hx("B0 01 C3 90 90 90 90 90"),
    )

    planner.locate(
        "CanReachNextLevel: attribute cap bypassed",
        SIG_ATTRIBUTE_CAP_1,
        13,
        "83 F8 63 0F 8D D8 FE FF FF",
        hx("83 F8 63") + b"\x90" * 6,
    )
    planner.locate(
        "TrySpendPoint: attribute cap bypassed",
        SIG_ATTRIBUTE_CAP_2,
        14,
        "83 F8 63 0F 8D 0A 01 00 00",
        hx("83 F8 63") + b"\x90" * 6,
    )
    planner.locate(
        "ApplyUpgradeAttributeInternal: attribute cap bypassed",
        SIG_ATTRIBUTE_CAP_3,
        16,
        "83 F8 63 0F 8D 74 04 00 00",
        hx("83 F8 63") + b"\x90" * 6,
    )
    planner.locate(
        "UpgradeAttributeServerRpc: attribute cap bypassed",
        SIG_ATTRIBUTE_CAP_4,
        16,
        "83 F8 63 0F 8D FC 04 00 00",
        hx("83 F8 63") + b"\x90" * 6,
    )

    planner.locate(
        "SkillPointOverGrantPersists disabled",
        SIG_SKILL_OVER_GRANT,
        0,
        "40 53 48 83 EC 20 48 8B D9 41 3B D0 7E 42 0F 57",
        hx("31 C0 C3") + b"\x90" * 13,
    )
    planner.locate(
        "Edited skill points preserved",
        SIG_SKILL_RECONCILE,
        13,
        "0F 84 99 02 00 00",
        hx("0F 8E 99 02 00 00"),
    )


def plan_optional(
    planner: PatchPlanner,
    *,
    loot_x3: bool,
    guaranteed_item: bool,
    guaranteed_equipment: bool,
    ancient: bool,
    stars: int | None,
    gear_legality_bypass: bool,
) -> None:
    if loot_x3:
        bits = struct.unpack("<I", struct.pack("<f", 3.0))[0]
        replacement = (
            b"\xB8" + struct.pack("<I", bits)
            + hx("66 0F 6E C0")
            + b"\xC3"
        )
        planner.locate(
            "LootChanceMultiplier = 3x",
            SIG_LOOT_MULTIPLIER,
            0,
            "48 83 EC 28 33 D2 E8 25 F8 FF",
            replacement,
        )

    if guaranteed_item:
        planner.locate(
            "Positive item drop chances guaranteed",
            SIG_GUARANTEED_ITEM,
            14,
            "44 3B F0 7D AB",
            hx("85 C0 7E AC 90"),
        )

    if guaranteed_equipment:
        planner.locate(
            "Positive equipment drop chances guaranteed",
            SIG_GUARANTEED_EQUIPMENT,
            14,
            "33 D2 45 33 C0 0F 28 C6 E8 ?? ?? ?? ??",
            hx("31 C0 0F 57 C0 0F 2F F0 0F 97 C0 90 90"),
        )

    if ancient:
        planner.locate(
            "ReturnRandomRuneData rarity -> Ancient",
            SIG_RETURN_RANDOM_RUNE,
            12,
            "E8 ?? ?? ?? ??",
            hx("B8 05 00 00 00"),
        )

    if stars is not None:
        if not 1 <= stars <= 9:
            raise ValueError("--stars must be between 1 and 9")
        internal = stars - 1
        planner.locate(
            f"ReturnRandomRuneData stars -> {stars}★",
            SIG_RETURN_RANDOM_RUNE,
            31,
            "E8 ?? ?? ?? ??",
            b"\xB8" + struct.pack("<I", internal),
        )

    if gear_legality_bypass:
        planner.locate(
            "GearLegality.Inspect always LEGAL",
            SIG_GEAR_LEGALITY_INSPECT,
            0,
            "40 55 53 48 8D 6C 24 D8",
            hx("31 C0 C3 90 90 90 90 90"),
        )


# ---------------------------------------------------------------------------
# Backup / restore
# ---------------------------------------------------------------------------

def manifest_path(dll: Path) -> Path:
    return dll.with_name(dll.name + ".dimraeth_patch.json")


def create_backup(dll: Path, clean_sha: str) -> Path:
    backup = dll.with_name(f"{dll.name}.original_{clean_sha[:8]}")
    if backup.exists():
        if sha256_file(backup) != clean_sha:
            raise RuntimeError(
                f"Backup path already exists but has different contents: "
                f"{backup}"
            )
    else:
        shutil.copy2(dll, backup)
    return backup


def write_manifest(
    dll: Path,
    backup: Path,
    original_sha: str,
    patched_sha: str,
) -> None:
    payload = {
        "backup": backup.name,
        "original_sha256": original_sha,
        "patched_sha256": patched_sha,
    }
    manifest_path(dll).write_text(
        json.dumps(payload, indent=2),
        encoding="utf-8",
    )


def restore_backup(dll: Path) -> None:
    manifest = manifest_path(dll)

    if manifest.exists():
        payload = json.loads(manifest.read_text(encoding="utf-8"))
        backup = dll.with_name(payload["backup"])

        if not backup.exists():
            raise FileNotFoundError(
                f"Backup referenced by manifest is missing: {backup}"
            )

        expected = payload.get("original_sha256")
        if expected and sha256_file(backup) != expected:
            raise RuntimeError("Backup SHA-256 does not match manifest")

        shutil.copy2(backup, dll)
        print(f"Restored: {backup} -> {dll}")
        print(f"SHA-256: {sha256_file(dll)}")
        return

    backups = sorted(
        dll.parent.glob(dll.name + ".original_*"),
        key=lambda p: p.stat().st_mtime,
        reverse=True,
    )

    if not backups:
        raise FileNotFoundError(
            f"No backup manifest and no {dll.name}.original_* backup found"
        )

    if len(backups) > 1:
        names = "\n".join(f"  {p.name}" for p in backups)
        raise RuntimeError(
            "Multiple backups found and no manifest says which one belongs "
            f"to the current installation:\n{names}"
        )

    shutil.copy2(backups[0], dll)
    print(f"Restored: {backups[0]} -> {dll}")
    print(f"SHA-256: {sha256_file(dll)}")


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def main() -> None:
    parser = argparse.ArgumentParser(
        description=(
            "Dimraeth v14 automatic AOB patcher "
            "(no build SHA whitelist / no absolute offsets)"
        )
    )

    parser.add_argument(
        "dll",
        nargs="?",
        type=Path,
        default=Path("GameAssembly.dll"),
    )
    parser.add_argument("--max-level", type=int, default=50)

    parser.add_argument("--loot-x3", action="store_true")
    parser.add_argument("--guaranteed-item", action="store_true")
    parser.add_argument("--guaranteed-equipment", action="store_true")
    parser.add_argument("--ancient", action="store_true")
    parser.add_argument("--stars", type=int, default=None)
    parser.add_argument("--gear-legality-bypass", action="store_true")

    parser.add_argument(
        "--output",
        type=Path,
        default=Path("GameAssembly_auto_v14.dll"),
    )

    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--install", action="store_true")
    mode.add_argument("--restore", action="store_true")
    mode.add_argument(
        "--scan-only",
        action="store_true",
        help="resolve and verify all requested signatures without writing",
    )

    args = parser.parse_args()

    if args.restore:
        restore_backup(args.dll)
        return

    original = args.dll.read_bytes()
    digest = sha256_bytes(original)

    print("Dimraeth automatic patcher v14")
    print(f"Input:   {args.dll}")
    print(f"SHA-256: {digest}")

    resolver = Resolver(original)
    print(
        "Executable sections: "
        + ", ".join(
            f"{s.name}[0x{s.raw_offset:X}:0x{s.raw_end:X}]"
            for s in resolver.exec_sections
        )
    )
    print()

    planner = PatchPlanner(original, resolver)

    # Resolve EVERYTHING before applying anything.
    plan_progression(planner, args.max_level)
    plan_optional(
        planner,
        loot_x3=args.loot_x3,
        guaranteed_item=args.guaranteed_item,
        guaranteed_equipment=args.guaranteed_equipment,
        ancient=args.ancient,
        stars=args.stars,
        gear_legality_bypass=args.gear_legality_bypass,
    )

    planner.verify_no_overlap()

    print()
    print(
        f"All requested signatures uniquely verified "
        f"({len(planner.patches)} patch targets)."
    )

    if args.scan_only:
        print("Scan-only mode: DLL was not modified.")
        return

    patched = planner.apply()
    patched_sha = sha256_bytes(patched)

    if args.install:
        backup = create_backup(args.dll, digest)
        args.dll.write_bytes(patched)
        write_manifest(
            args.dll,
            backup=backup,
            original_sha=digest,
            patched_sha=patched_sha,
        )
        out = args.dll
        print(f"Backup:  {backup}")
    else:
        args.output.write_bytes(patched)
        out = args.output

    print()
    print(f"Written: {out}")
    print(f"SHA-256: {patched_sha}")
    print()
    print("Optional loot patches enabled:")
    print(f"  loot x3:                {args.loot_x3}")
    print(f"  guaranteed item:        {args.guaranteed_item}")
    print(f"  guaranteed equipment:   {args.guaranteed_equipment}")
    print(f"  Ancient:                {args.ancient}")
    print(f"  Stars:                  {args.stars}")
    print(f"  Gear legality bypass:   {args.gear_legality_bypass}")


if __name__ == "__main__":
    main()
