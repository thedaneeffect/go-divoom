#!/usr/bin/env python3
"""Split a horizontal 32x32 sprite sheet into an animated GIF for the Pixoo Max.

Frame count is inferred from the sheet width, so adding poses to the sheet needs
no code change. Transparency is flattened onto black because the device encoder
maps alpha <= 32 to black anyway; doing it here keeps the palette predictable.

Usage: sheet2gif.py SHEET.png [OUT.gif] [--fps 5]
"""
import argparse
import sys

from PIL import Image

CELL = 32


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("sheet")
    ap.add_argument("out", nargs="?")
    ap.add_argument("--fps", type=float, default=5.0)
    args = ap.parse_args()

    out = args.out or args.sheet.rsplit(".", 1)[0] + ".gif"
    sheet = Image.open(args.sheet).convert("RGBA")
    w, h = sheet.size
    if h != CELL or w % CELL:
        print(f"sheet must be a horizontal strip of {CELL}x{CELL} cells, got {w}x{h}",
              file=sys.stderr)
        return 1

    frames = []
    for i in range(w // CELL):
        cell = sheet.crop((i * CELL, 0, (i + 1) * CELL, CELL))
        black = Image.new("RGBA", cell.size, (0, 0, 0, 255))
        flat = Image.alpha_composite(black, cell)
        frames.append(flat.convert("P", palette=Image.ADAPTIVE))

    frames[0].save(out, save_all=True, append_images=frames[1:],
                   duration=round(1000 / args.fps), loop=0, disposal=1)
    print(f"{out}: {len(frames)} frames @ {args.fps:g} fps")
    return 0


if __name__ == "__main__":
    sys.exit(main())
