package ui

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// A view sees a press go on until it is let go, and draws again as it
// ends, though nothing it shows looks at the press: as a page taking the
// keyboard back once a divider's drag ends.
func TestPointerHeld(t *testing.T) {
	var held []bool
	tt := coreNewTester(func(c *context) {
		held = append(held, c.PointerHeld())
		coreBox(c).Width(100).Height(100).KeepFocus().Pressed()
	}, 200, 200)
	if held[len(held)-1] {
		t.Fatal("held with nothing pressed")
	}
	tt.Press(50, 50)
	tt.Frame()
	if !held[len(held)-1] {
		t.Fatal("not held while pressed")
	}
	n := len(held)
	tt.Release(50, 50)
	if len(held) == n || held[len(held)-1] {
		t.Fatalf("no frame, or still held, as the press ended: %v", held[n:])
	}

	// The window losing the keyboard ends the press too.
	tt.Press(50, 50)
	tt.Frame()
	n = len(held)
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceBlur})
	if len(held) == n || held[len(held)-1] {
		t.Fatalf("no frame, or still held, as the window lost the keyboard: %v", held[n:])
	}

	// Outside any element, nothing is pressed.
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceFocus})
	tt.Press(150, 150)
	tt.Frame()
	if held[len(held)-1] {
		t.Error("held with no element pressed")
	}
	tt.Release(150, 150)
}
