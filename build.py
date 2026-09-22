#!/usr/bin/env python3
"""Build native desktop applications and enforce a 5,000,000-byte size limit."""
import argparse
import os
import pathlib
import plistlib
import shutil
import subprocess
import sys

root = pathlib.Path(__file__).resolve().parent
out = root / 'dist'
out.mkdir(exist_ok=True)
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--target', choices=['all', 'macos-arm64', 'macos-amd64', 'windows-amd64'], default='all')
args = parser.parse_args()
go = os.environ.get('GO', 'go')
targets = [('darwin', 'arm64'), ('darwin', 'amd64'), ('windows', 'amd64')]
for system, arch in targets:
    tag = ('macos' if system == 'darwin' else system) + '-' + arch
    if args.target not in ('all', tag):
        continue
    if system == 'darwin' and sys.platform != 'darwin':
        sys.exit('macOS builds require macOS and Xcode Command Line Tools. Use --target windows-amd64 to build Windows only.')
    name = 'dimraeth-editor-' + tag + ('.exe' if system == 'windows' else '')
    target = out / name
    env = dict(os.environ, GOOS=system, GOARCH=arch, CGO_ENABLED='1' if system == 'darwin' else '0')
    if system == 'darwin':
        env['MACOSX_DEPLOYMENT_TARGET'] = '13.0'
        env['CGO_CFLAGS'] = '-O2 -g -mmacosx-version-min=13.0'
        env['CGO_LDFLAGS'] = '-O2 -g -mmacosx-version-min=13.0'
    flags = '-s -w' + (' -H=windowsgui' if system == 'windows' else '')
    subprocess.run([go, 'build', '-trimpath', '-buildvcs=false', '-ldflags=' + flags, '-o', str(target), '.'], cwd=root, env=env, check=True)
    if target.stat().st_size > 5_000_000:
        sys.exit(f'{name}: {target.stat().st_size:,} bytes exceeds the 5 MB limit')
    print(f'{name}: {target.stat().st_size:,} bytes', flush=True)
    if system == 'darwin':
        app = out / ('Dimraeth Editor ' + arch + '.app')
        macos = app / 'Contents' / 'MacOS'
        macos.mkdir(parents=True, exist_ok=True)
        shutil.copy2(target, macos / 'dimraeth-editor')
        resources = app / 'Contents' / 'Resources'
        resources.mkdir(parents=True, exist_ok=True)
        shutil.copy2(root / 'assets' / 'app.icns', resources / 'app.icns')
        with (app / 'Contents' / 'Info.plist').open('wb') as f:
            plistlib.dump({
                'CFBundleName': 'Dimraeth Editor',
                'CFBundleDisplayName': 'Dimraeth Editor',
                'CFBundleIdentifier': 'local.dimraeth.editor',
                'CFBundleVersion': '2',
                'CFBundleShortVersionString': '2.0',
                'CFBundleExecutable': 'dimraeth-editor',
                'CFBundleIconFile': 'app.icns',
                'CFBundlePackageType': 'APPL',
                'CFBundleDevelopmentRegion': 'en',
                'CFBundleLocalizations': ['en'],
                'LSMinimumSystemVersion': '13.0',
                'NSHighResolutionCapable': True,
            }, f)
        subprocess.run(['codesign', '--force', '--deep', '--sign', '-', str(app)], check=True)
        size = sum(p.stat().st_size for p in app.rglob('*') if p.is_file())
        if size > 5_000_000:
            sys.exit(f'{app.name}: bundle exceeds 5 MB ({size:,})')
        print(f'{app.name}: {size:,} bytes total', flush=True)
