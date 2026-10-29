//go:build windows && (amd64 || arm64)

package windows

import "testing"

// TestDIPRoundTrip checks that a window's position, saved in DIPs and
// restored, comes back where it was on a display left of the primary
// one, where coordinates are negative, as it does on the primary one.
func TestDIPRoundTrip(t *testing.T) {
	for _, dpi := range []int{96, 120, 144, 192} {
		for _, px := range []int32{-2000, -1999, -1, 0, 1, 1999, 2000} {
			// A DIP holds a pixel or more, so a position in one returns
			// to itself, from the second trip at the latest.
			got := toPx(toDIP(px, dpi), dpi)
			if again := toPx(toDIP(got, dpi), dpi); again != got {
				t.Errorf("dpi %d: %d moved to %d, then %d", dpi, px, got, again)
			}
			if d := got - px; d < -1 || d > 1 {
				t.Errorf("dpi %d: %d came back as %d", dpi, px, got)
			}
			// Mirrored, it rounds the same way.
			if m := toPx(toDIP(-px, dpi), dpi); m != -got {
				t.Errorf("dpi %d: %d came back as %d, its mirror as %d", dpi, px, got, m)
			}
		}
	}
}
