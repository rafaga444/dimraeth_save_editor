# Dimraeth Save Workshop

A native desktop application for macOS (Apple Silicon and Intel) and Windows x64.
The interface, application messages, source comments, and documentation are in English.
The application uses Cocoa controls on macOS and Win32 controls on Windows. It runs
in its own desktop window, with no browser, web server, or network connection.

## Launch

Ready-to-run applications are in `dist/`. macOS builds require macOS 13 or later:

- `Dimraeth Editor arm64.app`: macOS Apple Silicon.
- `Dimraeth Editor amd64.app`: macOS Intel.
- `dimraeth-editor-windows-amd64.exe`: Windows x64.

Open the application for your platform. Python and Go are not required to run it.
Use the **Save editor** and **Loot patcher** tabs to switch tools. Closing the window
exits the application; unsaved changes prompt you before they are discarded.
The macOS application also supports Command-Q to quit.

The macOS bundles have local ad-hoc signatures, without Apple notarization.
The Windows executable does not have a publisher signature. Standard operating
system dialogs may follow your system language. User-provided save values, such as
character names, are preserved in their original language.

## Save editor

1. Select the game installation folder containing `GameAssembly.dll` using **Browse…**.
   The catalog loads automatically. If you enter the path manually, click **Load item catalog**.
2. Select a `.jrf` save using **Browse…**, or enter its full path and click **Open**.
3. Choose a **Parameter section** or use **Search parameters** to find a field.
   Select a row, edit its value on the right, and click **Apply value**. Boolean fields
   use a dropdown. Nested objects and array entries are available as individual fields.
   Null values are read-only. Large integer values retain their full precision.
4. Search for an item by name, enum symbol, or ID, choose it, enter the quantity,
   and click **Add item**. Select an inventory row to edit its quantity.
5. **Create backup** copies the current file on disk, including its existing encryption.
   It does not include edits that have not been saved.
6. **Save** applies the current field value, encrypts the save, verifies decryption,
   creates an automatic backup, and replaces the opened file at its original location.

Editing the path input alone does not change the save destination. Open the new file
first. Pending field edits are applied when switching fields or saving. Edits remain
in memory until you click **Save**.

Backups are stored beside the original file with a `.backup-DATE-TIME` suffix.
Close the game before saving. If the file has changed on disk since it was opened,
writing is blocked; reopen it before editing again. To restore a backup, close the game
and copy the chosen backup over the original file.

### Inventory behavior

New items occupy empty inventory slots without increasing the backpack size.
For tools and refillable items already present in the save, the editor copies known
properties, restores durability, and allocates one slot per instance. For a newly
encountered ID, it creates a basic ItemEntry. The game's
`Inventory.ValidateInventorySubkinds` function normalizes type and durability on load.
This function was inspected in the supplied DLL; loading an edited save inside the
game has not been tested.

Stack limits from Unity assets are not extracted, so an ordinary entry can exceed its
in-game stack limit. ItemType entries and RuneData instances are separate entities:
adding an ItemType does not generate new equipment with randomized stats. Existing
equipment can be edited through its fields.

## Item catalog

The parser reads type definitions, field definitions, and constant values from
`global-metadata.dat`, including IL2CPP compressed int32 values. Metadata versions
29 and 31 are supported. It selects the `ItemType` enum in the empty namespace of
`Assembly-CSharp.dll`, ignoring unrelated enums with the same name.

The PE64 parser then finds the Assembly-CSharp code-generation module inside
`GameAssembly.dll` and compares its method count with the metadata. The DLL is read
as data; its code is never loaded or executed.

The verified installation contains **393 items**, metadata v31, and 39,212
Assembly-CSharp methods. Display names are enum symbols with spaces inserted, such
as `CookedBeastMeat` becoming `Cooked Beast Meat`. Localized names stored in Unity
assets are not extracted. Unsupported metadata or an ambiguous installation folder
produces an error instead of a guessed catalog.

## Loot patcher

Set **Drop chance multiplier** and **Rune and equipment rarity**, then click **Save**.
A backup is created before the selected installation's `GameAssembly.dll` is replaced.

The multiplier replaces the return value of
`DifficultyManager.GetLootChanceMultiplier` with a constant positive finite float32,
matching the original Python patcher's behavior. It replaces the normal calculation
based on difficulty.

Available rarities:

| Rarity | Value |
| --- | --- |
| Common | 0 |
| Uncommon | 1 |
| Rare | 2 |
| Mythical | 3 |
| Heroic | 4 |
| Ancient | 5 |

The rarity patch affects `Runes.GenerateRuneData` and `Runes.GenerateRune`, which
create runes and equipment. It does not change the rarity of other item categories.
All three patch locations are validated before writing. Both original bytes and
existing patches of this format are recognized, allowing repeat changes to the
multiplier or rarity. Unknown DLL versions are rejected.

The patcher targets the Windows x64 DLL, including an installation running through
CrossOver on macOS. Native Mach-O game libraries are not supported.

## Build from source

The build uses Go 1.27.1 with no external Go dependencies.
macOS builds require macOS and Xcode Command Line Tools; the Cocoa bridge uses CGO.
Windows builds use Win32 directly and have CGO disabled.
Python 3 is needed only to run the build helper.

From this directory on macOS:

```sh
python3 build.py
```

To specify a Go installation:

```sh
GO=/path/to/go python3 build.py
```

Build an individual target:

```sh
python3 build.py --target macos-arm64
python3 build.py --target macos-amd64
python3 build.py --target windows-amd64
```

On Windows, use `python build.py --target windows-amd64`. If necessary, set the Go
path in PowerShell with `$env:GO = 'C:\Go\bin\go.exe'`.

The helper creates the macOS `.app` bundles, signs them locally, and enforces a
**5,000,000-byte limit for each executable and each complete .app bundle**.
The build fails if the limit is exceeded. No executable packer is used.

## Validation

Run the tests on macOS with Xcode Command Line Tools installed:

```sh
go test ./...
# Include the metadata/DLL integration test against an installed game:
DIMRAETH_TEST_GAME='/path/to/Dimraeth' go test -v ./...
```

Tests cover encryption, HMAC and corruption detection, large-number preservation,
backups, conflicting file changes, item insertion without expanding the inventory,
full-inventory rejection, and the DLL patch bytes. When the supplied Python-patched
DLL fixture is present, its output is compared byte for byte. Fixture-based tests
are skipped if their input files are absent. Writes use temporary copies; original
save files and installed game files are read only during tests.

Native macOS UI checks and cross-platform build checks are performed separately.
The Windows executable is cross-compiled; a native Windows runtime test has not been
performed. An in-game check of edited saves has not been performed.

The IL2CPP format implementation was checked against
[Il2CppDumper MetadataClass.cs](https://github.com/Perfare/Il2CppDumper/blob/master/Il2CppDumper/Il2Cpp/MetadataClass.cs)
and [BinaryReaderExtensions.cs](https://github.com/Perfare/Il2CppDumper/blob/master/Il2CppDumper/Extensions/BinaryReaderExtensions.cs).
