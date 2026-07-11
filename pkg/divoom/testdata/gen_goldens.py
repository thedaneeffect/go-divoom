"""Generate golden byte fixtures for go-divoom from hass-divoom reference code."""
import sys, types, json

# Stub PIL so importing divoom.py works without Pillow.
for m in ("PIL",):
    mod = types.ModuleType(m)
    for sub in ("Image", "ImageDraw", "ImageFont"):
        setattr(mod, sub, types.SimpleNamespace())
    sys.modules[m] = mod

sys.path.insert(0, "/tmp/hass-divoom/custom_components/divoom/devices")
import importlib.util
spec = importlib.util.spec_from_file_location(
    "divoom", "/tmp/hass-divoom/custom_components/divoom/devices/divoom.py"
)
divoom = importlib.util.module_from_spec(spec)
spec.loader.exec_module(divoom)

class Fake(divoom.Divoom):
    def __init__(self, screensize):
        self.type = "Fake"
        self.screensize = screensize
        self.chunksize = 200
        self.escapePayload = False

def hexs(b):
    return bytes(b).hex()

out = {}

d32 = Fake(32)
d16 = Fake(16)  # only for non-flag paths (16x16 image on Max uses screensize-independent frame funcs)

# 1. Framing: brightness 10 and 100 (hardware-validated already)
out["msg_brightness_10"] = hexs(d32.make_message([0x04, 0x00, 0x74, 0x0A]))
out["msg_brightness_100"] = hexs(d32.make_message([0x04, 0x00, 0x74, 0x64]))

# 2. Checksum overflow -> u32LE (payload sum >= 65535)
big = [0xFF] * 260  # sum = 66300 > 65535
out["msg_checksum_u32"] = hexs(d32.make_message(big))[:20] + "..."
out["msg_checksum_u32_full_len"] = len(d32.make_message(big))
out["msg_checksum_u32_tail"] = hexs(d32.make_message(big)[-6:])

# 3. process_pixels: bit packing
# palette of 1 color, 16 pixels -> bitsPerPixel 1
out["pack_1color_16px"] = hexs(d32.process_pixels([0]*16, [[0,0,0]]))
# 2 colors alternating, 16 pixels -> 1 bpp: 0101.. LSB-first per byte
out["pack_2color_alt_16px"] = hexs(d32.process_pixels([0,1]*8, [[0,0,0],[255,255,255]]))
# 3 colors -> 2 bpp, pixels 0,1,2,0,1,2,0,1 (8 px = 16 bits = 2 bytes)
out["pack_3color_8px"] = hexs(d32.process_pixels([0,1,2,0,1,2,0,1], [[1,1,1]]*3))
# 5 colors -> 3 bpp, pixels 0..4,0,1,2 (8 px = 24 bits = 3 bytes)
out["pack_5color_8px"] = hexs(d32.process_pixels([0,1,2,3,4,0,1,2], [[1,1,1]]*5))

# 4. process_frame: 16x16 single-color red frame, single image (no flags, timeCode 0)
pixels16 = [0]*256
colors_red = [[255,0,0]]
out["frame16_red_single"] = hexs(d32.process_frame(pixels16, colors_red, 1, 1, 0, False))

# 5. process_frame: 32x32 2-color checkerboard, single frame, WITH flags (0x03, colorCount u16)
pix32 = [ (x+y) % 2 for y in range(32) for x in range(32) ]
colors_bw = [[0,0,0],[255,255,255]]
out["frame32_checker_single"] = hexs(d32.process_frame(pix32, colors_bw, 2, 1, 0, True))

# 6. make_frame on the 16x16 frame
frame16 = d32.process_frame(pixels16, colors_red, 1, 1, 0, False)
mf, mflen = d32.make_frame(frame16)
out["makeframe16_red"] = hexs(mf)
out["makeframe16_red_len"] = mflen

# 7. single-image command payload (set image 0x44): framepart with fixed prefix
fp = d32.make_framepart(mflen, -1, mf)
cmdpayload = [len(fp)+3 & 0xFF, (len(fp)+3) >> 8, 0x44] + fp
out["setimage16_red_full_message"] = hexs(d32.make_message(cmdpayload))

# 8. animation: two 16x16 frames (red, black) on Max (screensize 32 -> lsum u32LE + index u16LE)
f1 = d32.process_frame([0]*256, [[255,0,0]], 1, 2, 500, False)
f2 = d32.process_frame([0]*256, [[0,0,0]], 1, 2, 500, False)
parts, total = [], 0
for f in (f1, f2):
    b, l = d32.make_frame(f)
    parts += b
    total += l
chunks = list(d32.chunks(parts, d32.chunksize))
out["anim_2frames_total_lsum"] = total
out["anim_2frames_nchunks"] = len(chunks)
msgs = []
for i, ch in enumerate(chunks):
    fp = d32.make_framepart(total, i, ch)
    cp = list((len(fp)+3).to_bytes(2, "little")) + [0x49] + fp
    msgs.append(hexs(d32.make_message(cp)))
out["anim_2frames_messages"] = msgs

# 9. flag frames prepended for 32x32 content
ff1, _ = d32.make_frame([0x00, 0x00, 0x05, 0x00, 0x00])
ff2, _ = d32.make_frame([0x00, 0x00, 0x06, 0x00, 0x00, 0x00])
out["flag_frame_1"] = hexs(ff1)
out["flag_frame_2"] = hexs(ff2)

# 9b. wide (32x32) animation: checkerboard + solid red, 250ms -> flag frames
# prepended, wide counters (u32LE total + u16LE index), two 200-byte chunks
pix_checker = [(x + y) % 2 for y in range(32) for x in range(32)]
wf1 = d32.process_frame(pix_checker, [[0, 0, 0], [255, 255, 255]], 2, 2, 250, True)
wf2 = d32.process_frame([0] * 1024, [[255, 0, 0]], 1, 2, 250, True)
parts, total = [], 0
for raw in ([0x00, 0x00, 0x05, 0x00, 0x00], [0x00, 0x00, 0x06, 0x00, 0x00, 0x00]):
    b, l = d32.make_frame(raw)
    parts += b
    total += l
for f in (wf1, wf2):
    b, l = d32.make_frame(f)
    parts += b
    total += l
msgs = []
for i, ch in enumerate(d32.chunks(parts, d32.chunksize)):
    fp = d32.make_framepart(total, i, ch)
    cp = list((len(fp) + 3).to_bytes(2, "little")) + [0x49] + fp
    msgs.append(hexs(d32.make_message(cp)))
out["anim32_2frames_total"] = total
out["anim32_2frames_nchunks"] = len(msgs)
out["anim32_2frames_messages"] = msgs

# 10. view/light/clock command payloads (args only, for command-builder tests)
out["cmd_light_white_100_on"] = hexs(d32.make_message([len([0x01,0xFF,0xFF,0xFF,100,0x01,0x01,0x00,0x00,0x00])+3, 0x00, 0x45, 0x01,0xFF,0xFF,0xFF,100,0x01,0x01,0x00,0x00,0x00]))
out["cmd_clock_style2_24h"] = hexs(d32.make_message([len([0x00,0x01,0x02,0x01,0x00,0x00,0x00])+3, 0x00, 0x45, 0x00,0x01,0x02,0x01,0x00,0x00,0x00]))
out["cmd_datetime_2026_07_11_15_30_45"] = hexs(d32.make_message([len([26,20,7,11,15,30,45])+3, 0x00, 0x18, 26,20,7,11,15,30,45]))

print(json.dumps(out, indent=1))
