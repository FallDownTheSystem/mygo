package ui

// Files dragged from other apps, such as Finder or Explorer, drop on the
// elements that ask for them with DroppedFiles. The window takes them only
// over such an element, or anywhere when the window has OnFileDrop
// listeners, which get what no element takes.

// DroppedFiles returns the paths of the files dropped on the element since
// the last frame, nil when none, and makes the element take files dragged
// over it.
func (e *node) DroppedFiles() []string {
	e.flags |= flagDropTarget
	if len(e.st.dropped) > 0 {
		e.c.rt.consumed = true
	}
	return e.st.dropped
}

// FileDragOver reports whether files dragged from another app are over
// the element, which takes them if they are dropped, for showing it would.
func (e *node) FileDragOver() bool {
	e.flags |= flagDropTarget
	return e.c.rt.dropOver == e.id
}

// FileDragAt returns where files dragged from another app are, in the
// element's local axes, and whether they are within its bounds, whichever
// element there takes them: for showing what dropping them on it, or on
// what it holds, would do, as by the part of it they are over.
func (e *node) FileDragAt() (x, y float32, over bool) {
	rt := e.c.rt
	if !e.hasState() || !rt.fileDragging {
		return 0, 0, false
	}
	s := e.st
	x, y = s.local(rt.fileX, rt.fileY)
	return x, y, rt.contains(s, Rect{s.x, s.y, s.w, s.h}, rt.fileX, rt.fileY)
}

// dropTarget returns the innermost element at (x, y) that takes files.
func (rt *engine) dropTarget(x, y float32) *state {
	for _, id := range rt.hitChain(x, y) {
		if s := rt.states[id]; s != nil && s.flags&flagDropTarget != 0 && s.flags&flagDisabled == 0 {
			return s
		}
	}
	return nil
}

// fileDrag follows files dragged over (x, y), or out of the window for
// negative coordinates, and reports whether an element takes them there.
func (rt *engine) fileDrag(x, y float32) bool {
	var over uint64
	in := x >= 0 && y >= 0
	if in {
		if s := rt.dropTarget(x, y); s != nil {
			over = s.id
		}
	}
	// Where they are shows as they move (FileDragAt).
	if in != rt.fileDragging || in && (x != rt.fileX || y != rt.fileY) {
		rt.fileDragging, rt.fileX, rt.fileY = in, x, y
		rt.requestFrame()
	}
	if over != rt.dropOver {
		rt.dropOver = over
		rt.requestFrame()
	}
	return over != 0
}

// fileDrop drops files at (x, y), and reports whether an element took them.
func (rt *engine) fileDrop(x, y float32, files []string) bool {
	rt.fileDrag(-1, -1)
	s := rt.dropTarget(x, y)
	if s == nil || len(files) == 0 {
		return false
	}
	s.dropped = append(s.dropped, files...)
	rt.requestFrame()
	return true
}
