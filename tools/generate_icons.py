#!/usr/bin/env python3
"""Generate custom multi-resolution Windows .ico and Go PE .syso resources.

Uses only the Python standard library so `tools/dev_check.ps1` and fresh clones
never require external C compilers or third-party imaging packages.
"""

from __future__ import annotations

import math
import struct
import zlib
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent


def clamp(val: float, lo: float = 0.0, hi: float = 1.0) -> float:
    return max(lo, min(hi, val))


def smoothstep(edge0: float, edge1: float, x: float) -> float:
    if edge0 == edge1:
        return 1.0 if x >= edge1 else 0.0
    t = clamp((x - edge0) / (edge1 - edge0))
    return t * t * (3.0 - 2.0 * t)


def lerp(a: float, b: float, t: float) -> float:
    return a + (b - a) * t


def blend_over(
    dst: tuple[float, float, float, float],
    src: tuple[float, float, float, float],
) -> tuple[float, float, float, float]:
    sr, sg, sb, sa = src
    if sa <= 0.0:
        return dst
    if sa >= 1.0:
        return (sr, sg, sb, 1.0)
    dr, dg, db, da = dst
    out_a = sa + da * (1.0 - sa)
    if out_a <= 1e-6:
        return (0.0, 0.0, 0.0, 0.0)
    out_r = (sr * sa + dr * da * (1.0 - sa)) / out_a
    out_g = (sg * sa + dg * da * (1.0 - sa)) / out_a
    out_b = (sb * sa + db * da * (1.0 - sa)) / out_a
    return (out_r, out_g, out_b, out_a)


def point_to_segment_dist(
    px: float, py: float, ax: float, ay: float, bx: float, by: float
) -> float:
    dx = bx - ax
    dy = by - ay
    length_sq = dx * dx + dy * dy
    if length_sq <= 1e-9:
        return math.hypot(px - ax, py - ay)
    t = clamp(((px - ax) * dx + (py - ay) * dy) / length_sq)
    proj_x = ax + t * dx
    proj_y = ay + t * dy
    return math.hypot(px - proj_x, py - proj_y)


def point_in_triangle(
    px: float,
    py: float,
    ax: float,
    ay: float,
    bx: float,
    by: float,
    cx: float,
    cy: float,
) -> float:
    """Signed distance to a 2D triangle (negative inside, positive outside)."""
    e0x, e0y = bx - ax, by - ay
    e1x, e1y = cx - bx, cy - by
    e2x, e2y = ax - cx, ay - cy
    v0x, v0y = px - ax, py - ay
    v1x, v1y = px - bx, py - by
    v2x, v2y = px - cx, py - cy

    d0 = point_to_segment_dist(px, py, ax, ay, bx, by)
    d1 = point_to_segment_dist(px, py, bx, by, cx, cy)
    d2 = point_to_segment_dist(px, py, cx, cy, ax, ay)
    min_d = min(d0, d1, d2)

    s = math.copysign(1.0, e0x * (cy - ay) - e0y * (cx - ax))
    c0 = s * (e0x * v0y - e0y * v0x)
    c1 = s * (e1x * v1y - e1y * v1x)
    c2 = s * (e2x * v2y - e2y * v2x)
    if c0 >= 0.0 and c1 >= 0.0 and c2 >= 0.0:
        return -min_d
    return min_d


def sample_app_icon(u: float, v: float, aa: float) -> tuple[float, float, float, float]:
    """Sample normalized coordinates (u, v) in [-1, 1] for the Codex Auto Retry emblem."""
    # 1. Rounded squircle badge SDF
    half_size = 0.78
    corner_r = 0.24
    qx = abs(u) - (half_size - corner_r)
    qy = abs(v) - (half_size - corner_r)
    sdf_box = (
        math.hypot(max(qx, 0.0), max(qy, 0.0))
        + min(max(qx, qy), 0.0)
        - corner_r
    )

    color = (0.0, 0.0, 0.0, 0.0)

    # Outer drop shadow
    shadow_qx = abs(u) - (half_size - corner_r)
    shadow_qy = abs(v - 0.04) - (half_size - corner_r)
    sdf_shadow = (
        math.hypot(max(shadow_qx, 0.0), max(shadow_qy, 0.0))
        + min(max(shadow_qx, shadow_qy), 0.0)
        - corner_r
    )
    shadow_alpha = (1.0 - smoothstep(-0.02, 0.14, sdf_shadow)) * 0.42
    if shadow_alpha > 0.0:
        color = blend_over(color, (0.02, 0.04, 0.09, shadow_alpha))

    # Badge body
    badge_alpha = 1.0 - smoothstep(-aa, aa, sdf_box)
    if badge_alpha <= 0.0:
        return color

    # Deep slate/navy diagonal gradient with subtle cyan/emerald ambient glow
    grad_t = clamp((u + v + 1.6) / 3.2)
    bg_r = lerp(0.055, 0.110, grad_t)
    bg_g = lerp(0.090, 0.165, grad_t)
    bg_b = lerp(0.165, 0.275, grad_t)

    # Radial glow in the center
    r = math.hypot(u, v)
    glow = clamp(1.0 - r / 0.85) ** 2
    bg_r += 0.02 * glow
    bg_g += 0.14 * glow
    bg_b += 0.18 * glow

    # Inner glowing border ring
    border_dist = abs(sdf_box + 0.035) - 0.018
    border_alpha = (1.0 - smoothstep(-aa, aa, border_dist)) * 0.65
    border_t = clamp((u - v + 1.4) / 2.8)
    br_r = lerp(0.06, 0.16, border_t)
    br_g = lerp(0.78, 0.85, border_t)
    br_b = lerp(0.62, 0.96, border_t)

    badge_col = (bg_r, bg_g, bg_b, badge_alpha)
    color = blend_over(color, badge_col)
    if border_alpha > 0.0:
        color = blend_over(color, (br_r, br_g, br_b, border_alpha * badge_alpha))

    # 2. Dual circular retry arcs
    ring_r = 0.50
    ring_thickness = 0.088
    angle = math.degrees(math.atan2(v, u))  # -180 to 180 (0 = right, 90 = down)

    def arc_distance(ang_deg: float, start_deg: float, end_deg: float) -> float:
        # Normalize relative to start_deg in [0, 360)
        rel = (ang_deg - start_deg) % 360.0
        span = (end_deg - start_deg) % 360.0
        if rel <= span:
            return abs(r - ring_r)
        # Distance to the two arc endpoints
        a1 = math.radians(start_deg)
        a2 = math.radians(end_deg)
        d1 = math.hypot(u - ring_r * math.cos(a1), v - ring_r * math.sin(a1))
        d2 = math.hypot(u - ring_r * math.cos(a2), v - ring_r * math.sin(a2))
        return min(d1, d2)

    # Top-right arc: from -145 deg to -25 deg
    d_arc1 = arc_distance(angle, -145.0, -25.0) - ring_thickness
    # Bottom-left arc: from 35 deg to 155 deg
    d_arc2 = arc_distance(angle, 35.0, 155.0) - ring_thickness

    # Arrowhead 1 at -20 deg (pointing clockwise / down-right)
    ah1_tip = (0.53, -0.04)
    ah1_b1 = (0.31, -0.22)
    ah1_b2 = (0.63, -0.33)
    d_ah1 = point_in_triangle(u, v, ah1_tip[0], ah1_tip[1], ah1_b1[0], ah1_b1[1], ah1_b2[0], ah1_b2[1]) - 0.015

    # Arrowhead 2 at 160 deg (pointing clockwise / up-left)
    ah2_tip = (-0.53, 0.04)
    ah2_b1 = (-0.31, 0.22)
    ah2_b2 = (-0.63, 0.33)
    d_ah2 = point_in_triangle(u, v, ah2_tip[0], ah2_tip[1], ah2_b1[0], ah2_b1[1], ah2_b2[0], ah2_b2[1]) - 0.015

    d_arrows = min(d_arc1, d_arc2, d_ah1, d_ah2)
    arrow_alpha = (1.0 - smoothstep(-aa, aa, d_arrows)) * badge_alpha
    if arrow_alpha > 0.0:
        # Gradient from bright cyan (#38BDF8) to emerald (#10B981)
        at = clamp((v + 0.6) / 1.2)
        ar = lerp(0.22, 0.06, at)
        ag = lerp(0.82, 0.90, at)
        ab = lerp(0.98, 0.64, at)
        color = blend_over(color, (ar, ag, ab, arrow_alpha))

    # 3. Center lightning / auto-pulse bolt
    bolt_top = (0.05, -0.26)
    bolt_mid_l = (-0.14, 0.03)
    bolt_mid_r = (0.02, 0.03)
    bolt_bot = (-0.05, 0.26)
    bolt_mid_r2 = (0.14, -0.03)
    bolt_mid_l2 = (-0.02, -0.03)

    d_bolt1 = point_in_triangle(
        u, v, bolt_top[0], bolt_top[1], bolt_mid_l[0], bolt_mid_l[1], bolt_mid_r2[0], bolt_mid_r2[1]
    )
    d_bolt2 = point_in_triangle(
        u, v, bolt_bot[0], bolt_bot[1], bolt_mid_r[0], bolt_mid_r[1], bolt_mid_l2[0], bolt_mid_l2[1]
    )
    d_bolt = min(d_bolt1, d_bolt2) - 0.008
    bolt_alpha = (1.0 - smoothstep(-aa, aa, d_bolt)) * badge_alpha
    if bolt_alpha > 0.0:
        # Warm glowing white-cyan bolt
        color = blend_over(color, (0.94, 0.99, 1.0, bolt_alpha))

    return color


def render_icon_bgra(size: int) -> bytes:
    """Render a size x size icon in top-down BGRA byte order using 2x2 supersampling."""
    out = bytearray(size * size * 4)
    aa = 1.6 / size
    offsets = (-0.25, 0.25)
    idx = 0
    for y in range(size):
        for x in range(size):
            acc_r = acc_g = acc_b = acc_a = 0.0
            for oy in offsets:
                v = ((y + 0.5 + oy) / size) * 2.0 - 1.0
                for ox in offsets:
                    u = ((x + 0.5 + ox) / size) * 2.0 - 1.0
                    sr, sg, sb, sa = sample_app_icon(u, v, aa)
                    acc_r += sr * sa
                    acc_g += sg * sa
                    acc_b += sb * sa
                    acc_a += sa
            a = acc_a / 4.0
            if a > 1e-5:
                r = clamp(acc_r / 4.0 / a)
                g = clamp(acc_g / 4.0 / a)
                b = clamp(acc_b / 4.0 / a)
            else:
                r = g = b = a = 0.0
            out[idx] = int(round(b * 255.0))
            out[idx + 1] = int(round(g * 255.0))
            out[idx + 2] = int(round(r * 255.0))
            out[idx + 3] = int(round(a * 255.0))
            idx += 4
    return bytes(out)


def encode_dib_frame(size: int, bgra_top_down: bytes) -> bytes:
    """Encode an ICO/RT_ICON BMP DIB payload (BITMAPINFOHEADER + bottom-up BGRA + AND mask)."""
    xor_bytes = bytearray(size * size * 4)
    row_bytes = size * 4
    for y in range(size):
        src_start = (size - 1 - y) * row_bytes
        dst_start = y * row_bytes
        xor_bytes[dst_start : dst_start + row_bytes] = bgra_top_down[
            src_start : src_start + row_bytes
        ]

    # AND mask rows are padded to 32-bit (4-byte) boundaries
    mask_row_bytes = ((size + 31) // 32) * 4
    and_bytes = bytearray(mask_row_bytes * size)
    for y in range(size):
        src_y = size - 1 - y
        for x in range(size):
            alpha = bgra_top_down[(src_y * size + x) * 4 + 3]
            if alpha == 0:
                byte_index = y * mask_row_bytes + (x >> 3)
                bit_index = 7 - (x & 7)
                and_bytes[byte_index] |= 1 << bit_index

    header = struct.pack(
        "<IIIHHIIIIII",
        40,  # biSize
        size,  # biWidth
        size * 2,  # biHeight (XOR + AND)
        1,  # biPlanes
        32,  # biBitCount
        0,  # biCompression (BI_RGB)
        len(xor_bytes) + len(and_bytes),  # biSizeImage
        0,
        0,
        0,
        0,
    )
    return header + bytes(xor_bytes) + bytes(and_bytes)


def encode_png_frame(size: int, bgra_top_down: bytes) -> bytes:
    """Encode a size x size RGBA PNG image using standard library zlib."""
    raw_rows = bytearray()
    for y in range(size):
        raw_rows.append(0)  # Filter type 0 (None)
        row_start = y * size * 4
        for x in range(size):
            px = row_start + x * 4
            b, g, r, a = bgra_top_down[px : px + 4]
            raw_rows.extend((r, g, b, a))

    def png_chunk(chunk_type: bytes, data: bytes) -> bytes:
        crc = zlib.crc32(chunk_type + data) & 0xFFFFFFFF
        return struct.pack(">I", len(data)) + chunk_type + data + struct.pack(">I", crc)

    ihdr = struct.pack(">IIBBBBB", size, size, 8, 6, 0, 0, 0)
    idat = zlib.compress(bytes(raw_rows), level=9)
    return (
        b"\x89PNG\r\n\x1a\n"
        + png_chunk(b"IHDR", ihdr)
        + png_chunk(b"IDAT", idat)
        + png_chunk(b"IEND", b"")
    )


def build_ico_and_frames() -> tuple[bytes, list[tuple[int, bytes]]]:
    sizes = [16, 24, 32, 48, 256]
    frames: list[tuple[int, bytes]] = []
    for sz in sizes:
        bgra = render_icon_bgra(sz)
        payload = encode_png_frame(sz, bgra) if sz == 256 else encode_dib_frame(sz, bgra)
        frames.append((sz, payload))

    count = len(frames)
    ico_header = struct.pack("<HHH", 0, 1, count)
    dir_entries = bytearray()
    offset = 6 + count * 16
    for sz, payload in frames:
        b_width = 0 if sz >= 256 else sz
        b_height = 0 if sz >= 256 else sz
        dir_entries.extend(
            struct.pack(
                "<BBBBHHII",
                b_width,
                b_height,
                0,
                0,
                1,
                32,
                len(payload),
                offset,
            )
        )
        offset += len(payload)

    ico_bytes = ico_header + bytes(dir_entries) + b"".join(p for _, p in frames)
    return ico_bytes, frames


def build_coff_syso(frames: list[tuple[int, bytes]]) -> bytes:
    """Build a Windows AMD64 COFF (.syso) file containing RT_ICON (3) and RT_GROUP_ICON (14)."""
    # Build GRPICONDIR for RT_GROUP_ICON
    grp_data = bytearray(struct.pack("<HHH", 0, 1, len(frames)))
    for idx, (sz, payload) in enumerate(frames, start=1):
        b_width = 0 if sz >= 256 else sz
        b_height = 0 if sz >= 256 else sz
        grp_data.extend(
            struct.pack(
                "<BBBBHHIH",
                b_width,
                b_height,
                0,
                0,
                1,
                32,
                len(payload),
                idx,  # nID (1-based RT_ICON resource ID)
            )
        )

    # Resource tree structure in .rsrc:
    # Level 1 (Root): 2 types -> Type 3 (RT_ICON) and Type 14 (RT_GROUP_ICON)
    # Level 2 (Type 3): N named/ID entries (ID 1..N)
    # Level 2 (Type 14): 1 ID entry (ID 1)
    # Level 3 (Language): N+1 entries (Lang 0x0409 = 1033)
    # Followed by N+1 IMAGE_RESOURCE_DATA_ENTRY (16 bytes each)
    # Followed by aligned raw resource payloads and 1 COFF relocation per data entry.
    n_icons = len(frames)
    total_resources = n_icons + 1

    # Calculate directory sizes:
    # Root dir: 16 + 2 * 8 = 32 bytes (offset 0)
    # Type 3 dir: 16 + n_icons * 8 (offset 32)
    # Type 14 dir: 16 + 1 * 8 = 24 (offset 32 + 16 + n_icons * 8)
    # Lang dirs (total_resources of them): each 16 + 1 * 8 = 24 bytes
    # Data entries (total_resources of them): each 16 bytes
    off_type3 = 32
    off_type14 = off_type3 + 16 + n_icons * 8
    off_lang_start = off_type14 + 24
    off_data_entries = off_lang_start + total_resources * 24
    off_payload_start = off_data_entries + total_resources * 16

    rsrc = bytearray()

    # Level 1: Root directory (2 ID entries: 3 and 14)
    rsrc.extend(struct.pack("<IIHHHH", 0, 0, 0, 0, 0, 2))
    rsrc.extend(struct.pack("<II", 3, 0x80000000 | off_type3))
    rsrc.extend(struct.pack("<II", 14, 0x80000000 | off_type14))

    # Level 2: Type 3 (RT_ICON) directory
    rsrc.extend(struct.pack("<IIHHHH", 0, 0, 0, 0, 0, n_icons))
    for i in range(n_icons):
        lang_dir_off = off_lang_start + i * 24
        rsrc.extend(struct.pack("<II", i + 1, 0x80000000 | lang_dir_off))

    # Level 2: Type 14 (RT_GROUP_ICON) directory (ID 1)
    rsrc.extend(struct.pack("<IIHHHH", 0, 0, 0, 0, 0, 1))
    grp_lang_dir_off = off_lang_start + n_icons * 24
    rsrc.extend(struct.pack("<II", 1, 0x80000000 | grp_lang_dir_off))

    # Level 3: Language directories (Lang 0x0409)
    for i in range(total_resources):
        data_entry_off = off_data_entries + i * 16
        rsrc.extend(struct.pack("<IIHHHH", 0, 0, 0, 0, 0, 1))
        rsrc.extend(struct.pack("<II", 0x0409, data_entry_off))

    assert len(rsrc) == off_data_entries

    # Prepare payloads and data entries
    payloads = [p for _, p in frames] + [bytes(grp_data)]
    payload_blob = bytearray()
    relocations = bytearray()
    data_entries = bytearray()

    for i, data in enumerate(payloads):
        data_offset = off_payload_start + len(payload_blob)
        data_entry_off = off_data_entries + i * 16
        # IMAGE_RESOURCE_DATA_ENTRY: OffsetToData, Size, CodePage, Reserved
        data_entries.extend(struct.pack("<IIII", data_offset, len(data), 0, 0))
        # COFF Relocation for OffsetToData (VirtualAddress = data_entry_off, SymbolTableIndex = 0, Type = IMAGE_REL_AMD64_ADDR32NB = 3)
        relocations.extend(struct.pack("<IIH", data_entry_off, 0, 3))
        payload_blob.extend(data)
        # Pad payload to 8-byte alignment
        while len(payload_blob) % 8 != 0:
            payload_blob.append(0)

    rsrc.extend(data_entries)
    rsrc.extend(payload_blob)

    # COFF Header (20 bytes) + 1 Section Header (40 bytes) = 60 bytes
    coff_header_size = 20
    section_header_size = 40
    rsrc_file_offset = coff_header_size + section_header_size
    reloc_file_offset = rsrc_file_offset + len(rsrc)
    symtab_file_offset = reloc_file_offset + len(relocations)

    # IMAGE_FILE_MACHINE_AMD64 = 0x8664
    coff_header = struct.pack(
        "<HHIIIHH",
        0x8664,  # Machine (AMD64)
        1,  # NumberOfSections
        0,  # TimeDateStamp
        symtab_file_offset,  # PointerToSymbolTable
        1,  # NumberOfSymbols
        0,  # SizeOfOptionalHeader
        0,  # Characteristics
    )

    # Section Header for ".rsrc"
    # Characteristics: IMAGE_SCN_CNT_INITIALIZED_DATA (0x40) | IMAGE_SCN_MEM_READ (0x40000000) = 0x40000040
    section_header = struct.pack(
        "<8sIIIIIIHHI",
        b".rsrc\x00\x00\x00",
        0,  # VirtualSize
        0,  # VirtualAddress
        len(rsrc),  # SizeOfRawData
        rsrc_file_offset,  # PointerToRawData
        reloc_file_offset,  # PointerToRelocations
        0,  # PointerToLinenumbers
        total_resources,  # NumberOfRelocations
        0,  # NumberOfLinenumbers
        0x40000040,  # Characteristics
    )

    # 1 COFF Symbol (18 bytes) for ".rsrc" section (SectionNumber = 1, StorageClass = IMAGE_SYM_CLASS_STATIC = 3)
    symbol_table = struct.pack(
        "<8sihHBB",
        b".rsrc\x00\x00\x00",
        0,  # Value
        1,  # SectionNumber
        0,  # Type
        3,  # StorageClass (STATIC)
        0,  # NumberOfAuxSymbols
    )

    # Empty string table (4 bytes: length = 4)
    string_table = struct.pack("<I", 4)

    return coff_header + section_header + bytes(rsrc) + bytes(relocations) + symbol_table + string_table


def main() -> int:
    ico_bytes, frames = build_ico_and_frames()
    syso_bytes = build_coff_syso(frames)

    ico_targets = [
        ROOT / "assets" / "app-icon.ico",
        ROOT / "scripts" / "source" / "ui" / "app-icon.ico",
        ROOT / "release" / "windows" / "app-icon.ico",
    ]
    for path in ico_targets:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(ico_bytes)
        print(f"Wrote {path.relative_to(ROOT)} ({len(ico_bytes)} bytes)")

    syso_targets = [
        ROOT / "scripts" / "source" / "rsrc_windows_amd64.syso",
        ROOT / "scripts" / "installer" / "rsrc_windows_amd64.syso",
    ]
    for path in syso_targets:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(syso_bytes)
        print(f"Wrote {path.relative_to(ROOT)} ({len(syso_bytes)} bytes)")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
