#!/usr/bin/env python3
"""Package app-icon.png as ICO and ICNS. Requires Pillow only for asset updates."""
import io
import pathlib
import struct

from PIL import Image

assets = pathlib.Path(__file__).resolve().parent
source = Image.open(assets / 'app-icon.png').convert('RGBA')
source.save(assets / 'app.ico', sizes=[(n, n) for n in (16, 24, 32, 48, 64, 128, 256)])

# Include standard and Retina representations, capped at 512 physical pixels
# to keep complete application bundles below the repository's 5 MB limit.
chunks = []
for kind, size in [
    (b'icp4', 16), (b'icp5', 32), (b'icp6', 64), (b'ic07', 128),
    (b'ic08', 256), (b'ic09', 512), (b'ic11', 32), (b'ic12', 64),
    (b'ic13', 256), (b'ic14', 512),
]:
    output = io.BytesIO()
    source.resize((size, size), Image.Resampling.LANCZOS).save(output, format='PNG', optimize=True)
    png = output.getvalue()
    chunks.append(kind + struct.pack('>I', len(png) + 8) + png)
body = b''.join(chunks)
(assets / 'app.icns').write_bytes(b'icns' + struct.pack('>I', len(body) + 8) + body)
