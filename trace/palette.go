package trace

import (
	"fmt"
	"time"
)

// Base HSL coordinates for service colours. The hue is derived from
// a hash of the service name (full 360° range) so any number of
// services maps to a visually distinct hue without a hardcoded
// palette to outgrow. Saturation and lightness are tuned for
// readability on both light and dark terminal backgrounds.
//
// baseLightness sits above 0.5 so depth-0 spans land at an even
// brighter L; pushing the centre of the gradient up gives a more
// striking visual delta between root and deep-leaf spans without
// the dim end disappearing into the terminal background.
const (
	baseSaturation = 0.70
	baseLightness  = 0.60

	// depthLightnessShift is the half-range of the bright-to-dim
	// gradient applied across a trace's depth. Depth 0 lands at
	// baseLightness + depthLightnessShift; the deepest span lands
	// at baseLightness − depthLightnessShift. 0.20 puts the start
	// at L=0.80 (bright but still saturated) and the end at L=0.40
	// (clearly darker without being black).
	depthLightnessShift = 0.20

	// nameLightnessBoost brightens span-name TEXT above the bar's
	// bright end. Thin glyphs colour far less cell area than a solid
	// █ block, so the same RGB reads noticeably darker as text; this
	// boost compensates so a span's name visually matches its bar.
	// Applies to label text only — bars and legend swatches keep the
	// gradient's true bright end.
	nameLightnessBoost = 0.12

	// depthFadeScale shrinks how far both bars AND span-name text dim
	// with depth, relative to the full ±depthLightnessShift swing. At
	// 1.0 the deepest span reaches baseLightness − depthLightnessShift;
	// at 0.5 it only fades half that far. Softening keeps deep bars
	// (and especially deep thin text) legible while still signalling
	// depth.
	depthFadeScale = 0.5
)

// serviceHSL returns the base HSL coordinates for a service. The hue
// is derived from the FNV hash of the service name and mapped to a
// full 360° wheel, so any number of services produces stable,
// distinct hues without a hardcoded palette. Depth modulation then
// shifts the lightness ([lightnessAtFade]) without a hex round-trip.
func serviceHSL(serviceName string) (h, s, l float64) {
	if serviceName == "" {
		serviceName = "unknown_service"
	}
	h = float64(hashString(serviceName)%360) / 360
	return h, baseSaturation, baseLightness
}

// depthFade maps a span's depth within its service's [minDepth,
// maxDepth] window to a fade fraction: 0 at the shallowest (bright
// end), 1 at the deepest (dim end). A single-depth service returns
// 0 so its lone span pins to the bright end.
func depthFade(depth, minDepth, maxDepth int) float64 {
	if maxDepth <= minDepth {
		return 0
	}
	depth = max(minDepth, min(depth, maxDepth))
	return float64(depth-minDepth) / float64(maxDepth-minDepth)
}

// lightnessAtFade maps a fade fraction in [0,1] to a bar lightness:
// baseLightness + depthLightnessShift at fade 0 (shallowest), fading
// toward baseLightness − depthLightnessShift at fade 1 — scaled by
// [depthFadeScale] so the swing is softened. Shared by bars, span
// names (which add [nameLightnessBoost]), and the legend swatch so
// all three agree on the gradient.
func lightnessAtFade(fade float64) float64 {
	return baseLightness + depthLightnessShift*(1-2*depthFadeScale*fade)
}

// depthLightness returns a service's HSL coordinates with the bar
// lightness modulated for a span at `depth`.
func depthLightness(serviceName string, depth, minDepth, maxDepth int) (h, s, l float64) {
	h, s, _ = serviceHSL(serviceName)
	return h, s, lightnessAtFade(depthFade(depth, minDepth, maxDepth))
}

// serviceColorNameAtDepth returns the colour for a span's name text:
// the same depth-faded lightness as the bar plus [nameLightnessBoost],
// so the name fades in lockstep with its bar but stays a touch
// brighter to compensate for the perceptual dilution of thin glyphs
// vs solid █ blocks.
func serviceColorNameAtDepth(serviceName string, depth, minDepth, maxDepth int) string {
	h, s, _ := serviceHSL(serviceName)
	l := lightnessAtFade(depthFade(depth, minDepth, maxDepth)) + nameLightnessBoost
	return hslToHex(h, s, l)
}

// serviceColorAtDepth returns the hex colour for a span's bar at
// `depth` within its own service's [minDepth, maxDepth] range. The
// gradient is per-service (see [depthLightness]): every service
// uses its full bright→dim spectrum, even when it only appears in
// one corner of the tree. Hue and saturation are constant so the
// service identity stays recognisable across depths.
func serviceColorAtDepth(serviceName string, depth, minDepth, maxDepth int) string {
	h, s, l := depthLightness(serviceName, depth, minDepth, maxDepth)
	return hslToHex(h, s, l)
}

// hslToHex converts HSL (each component in [0, 1]) to a `#rrggbb`
// truecolour string. h wraps modulo 1; s and l are clamped to
// [0, 1]. Charmbracelet's colorprofile downsamples the truecolour
// output for terminals that only support 256/16/none.
func hslToHex(h, s, l float64) string {
	if h < 0 {
		h -= float64(int(h)) - 1
	} else if h >= 1 {
		h -= float64(int(h))
	}
	if s < 0 {
		s = 0
	} else if s > 1 {
		s = 1
	}
	if l < 0 {
		l = 0
	} else if l > 1 {
		l = 1
	}
	var r, g, b float64
	if s == 0 {
		r, g, b = l, l, l // grey
	} else {
		var q float64
		if l < 0.5 {
			q = l * (1 + s)
		} else {
			q = l + s - l*s
		}
		p := 2*l - q
		r = hueToRGB(p, q, h+1.0/3)
		g = hueToRGB(p, q, h)
		b = hueToRGB(p, q, h-1.0/3)
	}
	return fmt.Sprintf("#%02x%02x%02x",
		byte(r*255+0.5),
		byte(g*255+0.5),
		byte(b*255+0.5),
	)
}

// hueToRGB is the standard helper used by [hslToHex]. t is the
// shifted hue (h ± 1/3 for the red/blue channels, h for green).
func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t += 1
	}
	if t > 1 {
		t -= 1
	}
	switch {
	case t < 1.0/6:
		return p + (q-p)*6*t
	case t < 1.0/2:
		return q
	case t < 2.0/3:
		return p + (q-p)*(2.0/3-t)*6
	}
	return p
}

// durationUnit identifies a single time unit used to render every
// duration within a trace. Selecting the unit once per trace (via
// [pickDurationUnit] over the trace's total duration) keeps every
// span's duration in the same column-aligned unit, so the eye can
// compare values without re-parsing the suffix on each row.
type durationUnit int

const (
	unitNS durationUnit = iota
	unitUS
	unitMS
	unitS
	unitMin
	unitH
)

// suffix returns the rendered suffix for the unit ("s", "ms", …).
func (u durationUnit) suffix() string {
	switch u {
	case unitNS:
		return "ns"
	case unitUS:
		return "µs"
	case unitMS:
		return "ms"
	case unitS:
		return "s"
	case unitMin:
		return "min"
	case unitH:
		return "h"
	}
	return ""
}

// pickDurationUnit picks a single unit appropriate for rendering
// every duration in a trace whose total span covers d. The
// threshold for switching to a larger unit is 1 of the next unit —
// e.g. ≥1s switches to seconds — except minutes, which only kick in
// at ≥10 minutes so 1m34s still renders in seconds ("94s") instead
// of "1.57min".
func pickDurationUnit(d time.Duration) durationUnit {
	abs := d
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs >= time.Hour:
		return unitH
	case abs >= 10*time.Minute:
		return unitMin
	case abs >= time.Second:
		return unitS
	case abs >= time.Millisecond:
		return unitMS
	case abs >= time.Microsecond:
		return unitUS
	}
	return unitNS
}

// formatDurationAs renders d in the supplied unit with a single
// decimal digit and the unit's suffix appended. unitNS is the only
// exception: nanoseconds render as an integer count since fractional
// nanoseconds are below the protobuf input's own resolution.
// Negative durations carry a leading minus.
func formatDurationAs(d time.Duration, u durationUnit) string {
	if u == unitNS {
		return fmt.Sprintf("%dns", d.Nanoseconds())
	}
	neg := ""
	if d < 0 {
		neg = "-"
		d = -d
	}
	var v float64
	switch u {
	case unitUS:
		v = float64(d) / float64(time.Microsecond)
	case unitMS:
		v = float64(d) / float64(time.Millisecond)
	case unitS:
		v = d.Seconds()
	case unitMin:
		v = d.Minutes()
	case unitH:
		v = d.Hours()
	}
	return fmt.Sprintf("%s%.1f%s", neg, v, u.suffix())
}
