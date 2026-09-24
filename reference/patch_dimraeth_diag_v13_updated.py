#!/usr/bin/env python3
"""
Dimraeth diagnostic patcher v13 — updated for build 99bba4f3.

Supported clean GameAssembly.dll SHA-256:
99bba4f31626fd2b863dc0a31c2cf3babf5b2270f8682439dbdb3d57d50c1dcc

Matching global-metadata.dat SHA-256:
ca08f02c8a408840734a7b7824ea10e24e45c7b005c39af4a574c85cef4fdf54

Progression patches are always enabled:
  - max level
  - editable attributes
  - no attribute cap
  - editable skill points

Loot/equipment patches are OPTIONAL and independently selectable:
  --loot-x3
  --guaranteed-item
  --guaranteed-equipment
  --ancient
  --stars N
  --gear-legality-bypass

This is intended for isolating which patch prevents equipment from
entering the inventory.
"""

import argparse
import hashlib
import shutil
import struct
from pathlib import Path


SUPPORTED_SHA256 = (
    "99bba4f31626fd2b863dc0a31c2cf3babf5b2270f8682439dbdb3d57d50c1dcc"
)
BUILD_TAG = SUPPORTED_SHA256[:8]


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


def patch_exact(data, offset, expected, replacement, name):
    if len(expected) != len(replacement):
        raise ValueError(f"{name}: patch-size mismatch")

    actual = bytes(data[offset:offset + len(expected)])

    if actual != expected:
        raise RuntimeError(
            f"{name}: unexpected bytes at 0x{offset:X}\n"
            f"Expected: {expected.hex(' ')}\n"
            f"Actual:   {actual.hex(' ')}\n"
            "Start from the clean supported GameAssembly.dll."
        )

    data[offset:offset + len(expected)] = replacement
    print(f"[OK] {name}")


def patch_progression(data: bytearray, max_level: int):
    if not 25 <= max_level <= 127:
        raise ValueError("--max-level must be between 25 and 127")

    if max_level != 25:
        imm8 = bytes([max_level])
        imm32 = struct.pack("<I", max_level)

        patch_exact(
            data, 0x1168D81,
            hx("83 FB 19"),
            b"\x83\xFB" + imm8,
            f"XP level-loop cap -> {max_level}",
        )
        patch_exact(
            data, 0x1168D88,
            hx("B9 19 00 00 00"),
            b"\xB9" + imm32,
            f"XP final cap -> {max_level}",
        )
        patch_exact(
            data, 0x92C36D,
            hx("BA 19 00 00 00"),
            b"\xBA" + imm32,
            f"Highest-level initialization -> {max_level}",
        )

    patch_exact(
        data, 0x934330,
        hx("40 53 55 56 57 41 56 48"),
        hx("C3 90 90 90 90 90 90 90"),
        "ValidateLoadedXPData disabled",
    )
    patch_exact(
        data, 0x933C60,
        hx("48 89 4C 24 08 53 56 57"),
        hx("C3 90 90 90 90 90 90 90"),
        "ValidateAttributeXPConsistency disabled",
    )
    patch_exact(
        data, 0x934D40,
        hx("48 89 4C 24 08 53 56 57"),
        hx("B0 01 C3 90 90 90 90 90"),
        "ValidateXPInvariants forced TRUE",
    )
    patch_exact(
        data, 0x935ED0,
        hx("48 89 4C 24 08 53 56 57"),
        hx("B0 01 C3 90 90 90 90 90"),
        "ValidateXPState forced TRUE",
    )

    for off, exp, name in [
        (0x9BD3CF, "83 F8 63 0F 8D D8 FE FF FF", "CanReachNextLevel"),
        (0x9C12A1, "83 F8 63 0F 8D 0A 01 00 00", "TrySpendPoint"),
        (0xA20A06, "83 F8 63 0F 8D 74 04 00 00", "ApplyUpgradeAttributeInternal"),
        (0xA2CBF9, "83 F8 63 0F 8D FC 04 00 00", "UpgradeAttributeServerRpc"),
    ]:
        expected = hx(exp)
        patch_exact(
            data, off,
            expected,
            expected[:3] + b"\x90" * 6,
            f"{name}: attribute cap bypassed",
        )

    patch_exact(
        data, 0x9331E0,
        hx("40 53 48 83 EC 20 48 8B D9 41 3B D0 7E 42 0F 57"),
        hx("31 C0 C3 90 90 90 90 90 90 90 90 90 90 90 90 90"),
        "SkillPointOverGrantPersists disabled",
    )
    patch_exact(
        data, 0xA07996,
        hx("0F 84 99 02 00 00"),
        hx("0F 8E 99 02 00 00"),
        "Edited skill points preserved",
    )


def patch_loot_x3(data: bytearray):
    bits = struct.unpack("<I", struct.pack("<f", 3.0))[0]
    replacement = (
        b"\xB8" + struct.pack("<I", bits) +
        hx("66 0F 6E C0") +
        b"\xC3"
    )

    patch_exact(
        data,
        0xD25DD0,
        hx("48 83 EC 28 33 D2 E8 25 F8 FF"),
        replacement,
        "LootChanceMultiplier = 3x",
    )


def patch_guaranteed_item(data: bytearray):
    patch_exact(
        data,
        0xBC79E0,
        hx("44 3B F0 7D AB"),
        hx("85 C0 7E AC 90"),
        "Positive item drop chances guaranteed",
    )


def patch_guaranteed_equipment(data: bytearray):
    patch_exact(
        data,
        0xBC7F74,
        hx("33 D2 45 33 C0 0F 28 C6 E8 FF 48 56 00"),
        hx("31 C0 0F 57 C0 0F 2F F0 0F 97 C0 90 90"),
        "Positive equipment drop chances guaranteed",
    )


def patch_ancient(data: bytearray):
    # ReturnRandomRuneData selects rarity by indexing the `rarities` array:
    #
    #   call array_get(rarities, selectedIndex)   ; EAX = chosen Rarity
    #   mov  ebx,eax                              ; save full 32-bit value
    #
    # v12 incorrectly changed `mov ebx,eax` to `mov bl,5`, which left the
    # upper 24 bits of EBX untouched. That passed a garbage Rarity value to
    # GenerateRuneData and explains the white/invalid drops and pickup failure.
    #
    # Correct fix: replace the array_get call itself with:
    #
    #   mov eax, 5
    #
    # The original `mov ebx,eax` then copies a clean Ancient value.
    patch_exact(
        data,
        0x10BE2C1,
        hx("E8 0A 48 FA 00"),
        hx("B8 05 00 00 00"),
        "ReturnRandomRuneData rarity selection -> Ancient",
    )


def patch_stars(data: bytearray, stars: int):
    if not 1 <= stars <= 9:
        raise ValueError("--stars must be between 1 and 9")

    # Stars enum is zero-based:
    #   1★=0, 2★=1, ... 6★=5, ... 9★=8
    internal = stars - 1

    # ReturnRandomRuneData selects Stars with a second array_get call:
    #
    #   call array_get(stars, selectedIndex)  ; EAX = chosen Stars
    #
    # Replace that 5-byte CALL directly with `mov eax, internal`.
    # Everything afterwards remains original, including the full-width
    # stack argument store passed to GenerateRuneData.
    patch_exact(
        data,
        0x10BE2D4,
        hx("E8 F7 47 FA 00"),
        b"\xB8" + struct.pack("<I", internal),
        f"ReturnRandomRuneData star selection -> {stars}★",
    )


def patch_gear_legality(data: bytearray, stars: int | None):
    # Force GearLegality.Inspect to return legal.
    patch_exact(
        data,
        0x104FB20,
        hx("40 55 53 48 8D 6C 24 D8"),
        hx("31 C0 C3 90 90 90 90 90"),
        "GearLegality.Inspect always LEGAL",
    )


def backup_path(dll: Path) -> Path:
    return dll.with_name(f"{dll.name}.original_{BUILD_TAG}")


def main():
    parser = argparse.ArgumentParser(
        description="Dimraeth diagnostic patcher v13 for build 99bba4f3"
    )
    parser.add_argument(
        "dll", nargs="?", type=Path, default=Path("GameAssembly.dll")
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
        default=Path("GameAssembly_diag_v13_99bba4f3.dll"),
    )

    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--install", action="store_true")
    mode.add_argument("--restore", action="store_true")

    args = parser.parse_args()
    bak = backup_path(args.dll)

    if args.restore:
        if not bak.exists():
            raise FileNotFoundError(bak)
        if sha256_file(bak) != SUPPORTED_SHA256:
            raise RuntimeError("Backup is not the clean supported DLL")
        shutil.copy2(bak, args.dll)
        print(f"Restored clean DLL: {args.dll}")
        return

    original = args.dll.read_bytes()
    digest = sha256_bytes(original)

    print(f"Input SHA-256: {digest}")

    if digest != SUPPORTED_SHA256:
        raise RuntimeError(
            "Start from the clean GameAssembly.dll.\n"
            f"Expected: {SUPPORTED_SHA256}\n"
            f"Actual:   {digest}"
        )

    data = bytearray(original)

    # Always-safe progression modifications.
    patch_progression(data, args.max_level)

    if args.loot_x3:
        patch_loot_x3(data)

    if args.guaranteed_item:
        patch_guaranteed_item(data)

    if args.guaranteed_equipment:
        patch_guaranteed_equipment(data)

    if args.ancient:
        patch_ancient(data)

    if args.stars is not None:
        patch_stars(data, args.stars)

    if args.gear_legality_bypass:
        patch_gear_legality(data, args.stars)

    patched = bytes(data)

    if args.install:
        if not bak.exists():
            shutil.copy2(args.dll, bak)
        args.dll.write_bytes(patched)
        out = args.dll
    else:
        args.output.write_bytes(patched)
        out = args.output

    print()
    print(f"Written: {out}")
    print(f"SHA-256: {sha256_file(out)}")
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
