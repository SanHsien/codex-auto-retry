//go:build windows

package main

import (
	"encoding/binary"
	"math"
	"sync"
	"unsafe"
)

var (
	procCreateIconFromResourceEx = user32.NewProc("CreateIconFromResourceEx")
	trayIconCacheMu              sync.Mutex
	trayIconCache                = map[string]uintptr{}
)

type trayPalette struct {
	arcR0, arcG0, arcB0 float64
	arcR1, arcG1, arcB1 float64
	glowR, glowG, glowB float64
	mode                string
}

func paletteForVisual(visual string) trayPalette {
	switch visual {
	case "active":
		return trayPalette{
			arcR0: 0.02, arcG0: 0.71, arcB0: 0.83,
			arcR1: 0.22, arcG1: 0.88, arcB1: 0.99,
			glowR: 0.05, glowG: 0.22, glowB: 0.30,
			mode: "bolt",
		}
	case "waiting":
		return trayPalette{
			arcR0: 0.96, arcG0: 0.62, arcB0: 0.04,
			arcR1: 0.99, arcG1: 0.82, arcB1: 0.20,
			glowR: 0.24, glowG: 0.16, glowB: 0.02,
			mode: "clock",
		}
	case "paused":
		return trayPalette{
			arcR0: 0.50, arcG0: 0.58, arcB0: 0.68,
			arcR1: 0.72, arcG1: 0.78, arcB1: 0.86,
			glowR: 0.10, glowG: 0.13, glowB: 0.18,
			mode: "pause",
		}
	case "stopped":
		return trayPalette{
			arcR0: 0.94, arcG0: 0.27, arcB0: 0.27,
			arcR1: 0.98, arcG1: 0.52, arcB1: 0.52,
			glowR: 0.26, glowG: 0.05, glowB: 0.06,
			mode: "stop",
		}
	default: // "running"
		return trayPalette{
			arcR0: 0.06, arcG0: 0.73, arcB0: 0.51,
			arcR1: 0.20, arcG1: 0.91, arcB1: 0.67,
			glowR: 0.03, glowG: 0.22, glowB: 0.15,
			mode: "bolt",
		}
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func smoothstep01(edge0, edge1, x float64) float64 {
	if edge0 == edge1 {
		if x >= edge1 {
			return 1
		}
		return 0
	}
	t := clamp01((x - edge0) / (edge1 - edge0))
	return t * t * (3 - 2*t)
}

func lerpF(a, b, t float64) float64 {
	return a + (b-a)*t
}

func blendLayer(dr, dg, db, da, sr, sg, sb, sa float64) (float64, float64, float64, float64) {
	if sa <= 0 {
		return dr, dg, db, da
	}
	if sa >= 1 {
		return sr, sg, sb, 1
	}
	outA := sa + da*(1-sa)
	if outA <= 1e-6 {
		return 0, 0, 0, 0
	}
	outR := (sr*sa + dr*da*(1-sa)) / outA
	outG := (sg*sa + dg*da*(1-sa)) / outA
	outB := (sb*sa + db*da*(1-sa)) / outA
	return outR, outG, outB, outA
}

func segDist(px, py, ax, ay, bx, by float64) float64 {
	dx := bx - ax
	dy := by - ay
	l2 := dx*dx + dy*dy
	if l2 <= 1e-9 {
		return math.Hypot(px-ax, py-ay)
	}
	t := clamp01(((px-ax)*dx + (py-ay)*dy) / l2)
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy))
}

func triSDF(px, py, ax, ay, bx, by, cx, cy float64) float64 {
	d0 := segDist(px, py, ax, ay, bx, by)
	d1 := segDist(px, py, bx, by, cx, cy)
	d2 := segDist(px, py, cx, cy, ax, ay)
	minD := math.Min(d0, math.Min(d1, d2))

	e0x, e0y := bx-ax, by-ay
	e1x, e1y := cx-bx, cy-by
	e2x, e2y := ax-cx, ay-cy
	s := 1.0
	if e0x*(cy-ay)-e0y*(cx-ax) < 0 {
		s = -1.0
	}
	c0 := s * (e0x*(py-ay) - e0y*(px-ax))
	c1 := s * (e1x*(py-by) - e1y*(px-bx))
	c2 := s * (e2x*(py-cy) - e2y*(px-cx))
	if c0 >= 0 && c1 >= 0 && c2 >= 0 {
		return -minD
	}
	return minD
}

func sampleTrayPixel(u, v, aa float64, pal trayPalette) (float64, float64, float64, float64) {
	halfSize := 0.84
	cornerR := 0.26
	qx := math.Abs(u) - (halfSize - cornerR)
	qy := math.Abs(v) - (halfSize - cornerR)
	sdfBox := math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - cornerR

	badgeAlpha := 1.0 - smoothstep01(-aa, aa, sdfBox)
	if badgeAlpha <= 0 {
		return 0, 0, 0, 0
	}

	gradT := clamp01((u + v + 1.6) / 3.2)
	r := math.Hypot(u, v)
	glow := math.Pow(clamp01(1.0-r/0.9), 2)
	bgR := lerpF(0.06, 0.12, gradT) + pal.glowR*glow
	bgG := lerpF(0.09, 0.17, gradT) + pal.glowG*glow
	bgB := lerpF(0.16, 0.26, gradT) + pal.glowB*glow

	cr, cg, cb, ca := bgR, bgG, bgB, badgeAlpha

	// Subtle colored outer rim
	rimDist := math.Abs(sdfBox+0.04) - 0.022
	rimAlpha := (1.0 - smoothstep01(-aa, aa, rimDist)) * 0.72 * badgeAlpha
	cr, cg, cb, ca = blendLayer(cr, cg, cb, ca, pal.arcR1, pal.arcG1, pal.arcB1, rimAlpha)

	// Dual circular retry arcs
	ringR := 0.53
	ringThick := 0.095
	ang := math.Atan2(v, u) * 180.0 / math.Pi
	arcDist := func(startDeg, endDeg float64) float64 {
		rel := math.Mod(ang-startDeg+360.0, 360.0)
		span := math.Mod(endDeg-startDeg+360.0, 360.0)
		if rel <= span {
			return math.Abs(r - ringR)
		}
		a1 := startDeg * math.Pi / 180.0
		a2 := endDeg * math.Pi / 180.0
		d1 := math.Hypot(u-ringR*math.Cos(a1), v-ringR*math.Sin(a1))
		d2 := math.Hypot(u-ringR*math.Cos(a2), v-ringR*math.Sin(a2))
		return math.Min(d1, d2)
	}

	dArc1 := arcDist(-145.0, -25.0) - ringThick
	dArc2 := arcDist(35.0, 155.0) - ringThick
	dAh1 := triSDF(u, v, 0.56, -0.04, 0.32, -0.23, 0.66, -0.35) - 0.015
	dAh2 := triSDF(u, v, -0.56, 0.04, -0.32, 0.23, -0.66, 0.35) - 0.015
	dArrows := math.Min(math.Min(dArc1, dArc2), math.Min(dAh1, dAh2))
	arrowAlpha := (1.0 - smoothstep01(-aa, aa, dArrows)) * badgeAlpha
	if arrowAlpha > 0 {
		at := clamp01((v + 0.65) / 1.3)
		ar := lerpF(pal.arcR1, pal.arcR0, at)
		ag := lerpF(pal.arcG1, pal.arcG0, at)
		ab := lerpF(pal.arcB1, pal.arcB0, at)
		cr, cg, cb, ca = blendLayer(cr, cg, cb, ca, ar, ag, ab, arrowAlpha)
	}

	// Center symbol according to visual mode
	var symDist float64 = 1.0
	switch pal.mode {
	case "pause":
		// Two vertical pause bars
		b1 := math.Max(math.Abs(u+0.095)-0.045, math.Abs(v)-0.18)
		b2 := math.Max(math.Abs(u-0.095)-0.045, math.Abs(v)-0.18)
		symDist = math.Min(b1, b2)
	case "clock":
		// Clock hands (vertical + right-down) + center dot
		h1 := segDist(u, v, 0, 0, 0, -0.19) - 0.045
		h2 := segDist(u, v, 0, 0, 0.15, 0.09) - 0.045
		symDist = math.Min(h1, h2)
	case "stop":
		// Exclamation mark
		bar := segDist(u, v, 0, -0.18, 0, 0.03) - 0.052
		dot := math.Hypot(u, v-0.17) - 0.055
		symDist = math.Min(bar, dot)
	default: // "bolt"
		t1 := triSDF(u, v, 0.05, -0.25, -0.14, 0.03, 0.14, -0.03) - 0.01
		t2 := triSDF(u, v, -0.05, 0.25, 0.02, 0.03, -0.02, -0.03) - 0.01
		symDist = math.Min(t1, t2)
	}
	symAlpha := (1.0 - smoothstep01(-aa, aa, symDist)) * badgeAlpha
	if symAlpha > 0 {
		cr, cg, cb, ca = blendLayer(cr, cg, cb, ca, 0.96, 0.99, 1.0, symAlpha)
	}
	return clamp01(cr), clamp01(cg), clamp01(cb), clamp01(ca)
}

func buildTrayIconDIB(visual string) []byte {
	const size = 32
	pal := paletteForVisual(visual)
	aa := 1.65 / float64(size)
	offsets := [2]float64{-0.25, 0.25}

	xorBytes := make([]byte, size*size*4)
	maskRowBytes := ((size + 31) / 32) * 4
	andBytes := make([]byte, maskRowBytes*size)

	for y := 0; y < size; y++ {
		dstY := size - 1 - y // Bottom-up DIB
		for x := 0; x < size; x++ {
			var accR, accG, accB, accA float64
			for _, oy := range offsets {
				v := ((float64(y)+0.5+oy)/float64(size))*2.0 - 1.0
				for _, ox := range offsets {
					u := ((float64(x)+0.5+ox)/float64(size))*2.0 - 1.0
					sr, sg, sb, sa := sampleTrayPixel(u, v, aa, pal)
					accR += sr * sa
					accG += sg * sa
					accB += sb * sa
					accA += sa
				}
			}
			a := accA / 4.0
			var r, g, b float64
			if a > 1e-5 {
				r = clamp01(accR / 4.0 / a)
				g = clamp01(accG / 4.0 / a)
				b = clamp01(accB / 4.0 / a)
			}
			idx := (dstY*size + x) * 4
			xorBytes[idx+0] = byte(math.Round(b * 255.0))
			xorBytes[idx+1] = byte(math.Round(g * 255.0))
			xorBytes[idx+2] = byte(math.Round(r * 255.0))
			alphaByte := byte(math.Round(a * 255.0))
			xorBytes[idx+3] = alphaByte
			if alphaByte == 0 {
				byteIdx := dstY*maskRowBytes + (x >> 3)
				bitIdx := uint(7 - (x & 7))
				andBytes[byteIdx] |= 1 << bitIdx
			}
		}
	}

	header := make([]byte, 40)
	binary.LittleEndian.PutUint32(header[0:4], 40)
	binary.LittleEndian.PutUint32(header[4:8], uint32(size))
	binary.LittleEndian.PutUint32(header[8:12], uint32(size*2))
	binary.LittleEndian.PutUint16(header[12:14], 1)
	binary.LittleEndian.PutUint16(header[14:16], 32)
	binary.LittleEndian.PutUint32(header[16:20], 0)
	binary.LittleEndian.PutUint32(header[20:24], uint32(len(xorBytes)+len(andBytes)))

	dib := make([]byte, 0, 40+len(xorBytes)+len(andBytes))
	dib = append(dib, header...)
	dib = append(dib, xorBytes...)
	dib = append(dib, andBytes...)
	return dib
}

func loadCustomTrayIcon(visual string) uintptr {
	trayIconCacheMu.Lock()
	defer trayIconCacheMu.Unlock()

	if cached, ok := trayIconCache[visual]; ok && cached != 0 {
		return cached
	}
	dib := buildTrayIconDIB(visual)
	hicon, _, _ := procCreateIconFromResourceEx.Call(
		uintptr(unsafe.Pointer(&dib[0])),
		uintptr(len(dib)),
		1,
		0x00030000,
		32,
		32,
		0,
	)
	if hicon != 0 {
		trayIconCache[visual] = hicon
	}
	return hicon
}
