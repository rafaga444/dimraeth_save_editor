# Build instructions

## Requirements

- Go 1.27.1, available on `PATH` or specified using the `GO` environment variable.
- Python 3.
- For macOS builds: macOS with Xcode Command Line Tools installed.

Run the following commands from the repository root (the `editor` directory).

## Build all targets on macOS

```sh
python3 build.py
```

This builds macOS Apple Silicon, macOS Intel, and Windows x64 applications.

## Build a single target

```sh
python3 build.py --target macos-arm64
python3 build.py --target macos-amd64
python3 build.py --target windows-amd64
```

On Windows, run:

```powershell
python build.py --target windows-amd64
```

macOS targets must be built on macOS.

## Use a custom Go installation

macOS:

```sh
GO=/path/to/go python3 build.py
```

Windows PowerShell:

```powershell
$env:GO = 'C:\Go\bin\go.exe'
python build.py --target windows-amd64
```

## Build output

Generated applications are written to `dist/`:

- `Dimraeth Editor arm64.app` — macOS Apple Silicon.
- `Dimraeth Editor amd64.app` — macOS Intel.
- `dimraeth-editor-windows-amd64.exe` — Windows x64.

The build helper signs macOS bundles locally and checks that each executable
and complete `.app` bundle is no larger than 5,000,000 bytes. The build fails
if this limit is exceeded.

Application icons and the Windows resource file are included in the repository;
normal builds do not require an image converter or Windows resource compiler.
Launch the `.app` bundle on macOS to use its Dock and Finder icon.

## Rebuild icons after replacing the source image

This optional asset step requires Pillow. Normal application builds do not.

```sh
python3 -m pip install Pillow
python3 assets/build_icons.py
```

## Rebuild Windows resources after replacing the icon or manifest

```sh
go run github.com/akavel/rsrc@v0.10.2 -arch amd64 -ico assets/app.ico -manifest assets/app.manifest -o resources_windows_amd64.syso
```

Commit the regenerated resource file together with its source assets. macOS
bundles use `assets/app.icns`; Windows uses `assets/app.ico` embedded in the
resource file. `assets/app-icon.png` is the source image for both icon formats.
