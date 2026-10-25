package editor

import (
	"strings"

	"github.com/egoist/mygo/internal/diff"
	"github.com/egoist/mygo/ui"
)

// MergeState is a three-way merge editor's state, kept by the app across
// frames (MergeView): the two versions a merge joins, and the file being
// merged, with the conflict markers git writes where they disagree, which
// the editor resolves, a conflict at a time, by either version, both, or
// an edit by hand.
//
// The file goes on from what it holds, not from the versions merged
// again, so that what was resolved in it already stays: the conflicts are
// its marker blocks, in git's three styles (merge, diff3 and zdiff3).
type MergeState struct {
	// Ours and Theirs are the versions merged, whole: the index's stages 2
	// and 3 of the file in a merge. Empty buffers are made when nil.
	Ours, Theirs *Buffer
	// Result is the file being merged, edited: its conflicts are the
	// marker blocks it holds when the editor first has it, or when it is
	// set again (Buffer.Reset).
	Result *Buffer
	// OursLabel and TheirsLabel name the versions, as branches; empty,
	// the names the first conflict's markers give, else "Ours" and
	// "Theirs".
	OursLabel, TheirsLabel string
	// Font, LineHeight, TabSize, Theme and Bindings are those of State.
	// Wrap wraps the result's long lines; the versions' scroll sideways,
	// as their lines are aligned.
	Font       ui.Font
	LineHeight float32
	TabSize    int
	Theme      *Theme
	Bindings   []Binding
	Wrap       bool
	// Split is the part of the height the versions take, above the
	// result: 0.5 when 0.
	Split float32
	// ResultHandle, when set, is bound to the result's editor, which the
	// app focuses by it.
	ResultHandle *ui.Handle

	conflicts []mergeConflict
	// parsed is the result the conflicts are of, and ver its version they
	// were mapped to; oursOf and theirsOf the versions their places in
	// them were found in.
	parsed            *Buffer
	ver               uint64
	oursOf, theirsOf  *Buffer
	oursVer, theirVer uint64
	labels            [2]string

	eds    [3]editor // ours, theirs, the result
	sides  [3]mergeSide
	scroll sharedScroll
	// choices are how the conflicts are resolved as the view was built.
	choices []MergeChoice
	// prepared is what the editors' blocks were made for.
	prepared mergePrepared
	built    mergeGeom
	// goTo is the conflict to scroll to once the editors are laid out,
	// -1 for none: the first, as the editor first shows.
	goTo int
}

// MergeChoice is how a conflict of a merge is resolved: what of the
// versions its place in the result holds.
type MergeChoice uint8

const (
	// MergeUnresolved: its conflict markers, as git wrote them.
	MergeUnresolved MergeChoice = iota
	// MergeOurs and MergeTheirs: one version's lines.
	MergeOurs
	MergeTheirs
	// MergeOursThenTheirs and MergeTheirsThenOurs: both, one after the
	// other.
	MergeOursThenTheirs
	MergeTheirsThenOurs
	// MergeNeither: none of their lines.
	MergeNeither
	// MergeEdited: lines edited by hand, none of the above.
	MergeEdited
)

// MergeVersion is a version of a merge: ours or theirs.
type MergeVersion uint8

const (
	MergeOursVersion MergeVersion = iota
	MergeTheirsVersion
)

// MergeConflict is a conflict of a merge, as Conflicts tells it.
type MergeConflict struct {
	Choice MergeChoice
	// Ours and Theirs are the versions' lines, and Base the lines they
	// both changed when the markers have them (HasBase, diff3 and
	// zdiff3), each line with its ending.
	Ours, Base, Theirs string
	HasBase            bool
	// Start and End are its lines in the result, End past the last;
	// equal when it holds none.
	Start, End int
}

// mergeConflict is a conflict: its texts, and its place in the result and
// in the versions.
type mergeConflict struct {
	ours, base, theirs string
	hasBase            bool
	markers            string // its marker block, as git wrote it
	// start and end are its bytes in the result.
	start, end int
	// inOurs and inTheirs are its lines in the versions, from and to
	// exclusive.
	inOurs, inTheirs [2]int
	// checks are what its checkboxes show, for them to write at the
	// click; then holds the choice they made, the one that's first of
	// both.
	checks [2]bool
}

// conflictMarker reports whether line, its ending left out, is a marker
// of n of c, followed by a label or nothing when label, else by nothing
// but blanks.
func conflictMarker(line string, c byte, n int, label bool) (string, bool) {
	if len(line) < n || strings.Count(line[:n], string(c)) != n || len(line) > n && line[n] == c {
		return "", false
	}
	rest := line[n:]
	if rest == "" {
		return "", true
	}
	if label && rest[0] == ' ' {
		return rest[1:], true
	}
	return "", strings.TrimSpace(rest) == ""
}

// trimEnding returns a line without its ending.
func trimEnding(line string) string {
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
}

// splitLines splits a text into its lines, each with its ending; the
// last without one when the text doesn't end in one.
func splitLines(s string) []string {
	var out []string
	for s != "" {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			out = append(out, s)
			break
		}
		out = append(out, s[:i+1])
		s = s[i+1:]
	}
	return out
}

// parseConflicts returns the marker blocks of a text: <<<<<<< ours,
// ||||||| base (diff3 and zdiff3), ======= and >>>>>>> theirs, of 7
// markers or more, as git's conflict-marker-size; and the labels of the
// first. A block that doesn't end is text.
func parseConflicts(text string) ([]mergeConflict, [2]string) {
	var out []mergeConflict
	var labels [2]string
	lines := splitLines(text)
	off := 0
	offs := make([]int, len(lines)+1)
	for i, l := range lines {
		offs[i] = off
		off += len(l)
	}
	offs[len(lines)] = off
	for i := 0; i < len(lines); i++ {
		line := trimEnding(lines[i])
		n := len(line) - len(strings.TrimLeft(line, "<"))
		if n < 7 {
			continue
		}
		oursLabel, ok := conflictMarker(line, '<', n, true)
		if !ok {
			continue
		}
		var c mergeConflict
		part := 0 // 0 ours, 1 base, 2 theirs
		var b [3]strings.Builder
		theirsLabel := ""
		end := -1
		for j := i + 1; j < len(lines); j++ {
			l := trimEnding(lines[j])
			if _, ok := conflictMarker(l, '<', n, true); ok {
				break // another block starts: this one doesn't end
			}
			if _, ok := conflictMarker(l, '|', n, true); ok && part == 0 {
				part, c.hasBase = 1, true
				continue
			}
			if _, ok := conflictMarker(l, '=', n, false); ok && part < 2 {
				part = 2
				continue
			}
			if label, ok := conflictMarker(l, '>', n, true); ok && part == 2 {
				theirsLabel, end = label, j
				break
			}
			b[part].WriteString(lines[j])
		}
		if end < 0 {
			continue
		}
		c.ours, c.base, c.theirs = b[0].String(), b[1].String(), b[2].String()
		c.start, c.end = offs[i], offs[end+1]
		c.markers = text[c.start:c.end]
		if len(out) == 0 {
			labels = [2]string{oursLabel, theirsLabel}
		}
		out = append(out, c)
		i = end
	}
	return out, labels
}

// ensure makes the buffers, and the conflicts of the result: parsed when
// it is new or set again, else mapped through its edits.
func (m *MergeState) ensure() {
	if m.Ours == nil {
		m.Ours = NewBuffer("")
	}
	if m.Theirs == nil {
		m.Theirs = NewBuffer("")
	}
	if m.Result == nil {
		m.Result = NewBuffer("")
	}
	ver := m.Result.Version()
	if m.parsed == m.Result && ver == m.ver {
		m.place()
		return
	}
	reparse := m.parsed != m.Result
	if !reparse {
		entries, _, ok := m.Result.entriesSince(m.ver)
		reparse = !ok
		for _, e := range entries {
			// A text Reset is a new one, whose markers are its conflicts.
			if reparse = reparse || e.reset; reparse {
				break
			}
			for i := range m.conflicts {
				c := &m.conflicts[i]
				c.start, c.end = mapRange(c.start, c.end, e.changes)
			}
		}
	}
	if reparse {
		text := m.Result.String()
		m.conflicts, m.labels = parseConflicts(text)
		m.parsed, m.oursOf, m.theirsOf = m.Result, nil, nil
	}
	m.ver = ver
	m.place()
}

// mapRange maps the bytes start to end of a text through the changes of
// an edit of it, sorted, in the old text's bytes: an edit before them
// moves them, one in them grows or shrinks them; text put at their start
// goes in them, at their end after them, but in them when they are empty.
func mapRange(start, end int, changes []change) (int, int) {
	s := mapPoint(start, changes, false, false)
	e := mapPoint(end, changes, true, start == end)
	return s, max(s, e)
}

// mapPoint maps byte p through the changes of an edit: the start of a
// range, or its end (end), of an empty range (empty).
func mapPoint(p int, changes []change, end, empty bool) int {
	delta := 0
	for _, c := range changes {
		switch {
		case c.oldStart < c.oldEnd && c.oldEnd <= p, c.oldStart == c.oldEnd && c.oldStart < p:
			// Before p.
			delta = c.newEnd - c.oldEnd
			continue
		case c.oldStart > p:
			return p + delta
		case c.oldStart == p && c.oldStart == c.oldEnd && end && empty:
			// Text put in an empty range.
			return c.newEnd
		case c.oldStart == p:
			// Text put at p, or replacing what starts there.
			return c.newStart
		case end:
			// p in what the change replaced.
			return c.newEnd
		}
		return c.newStart
	}
	return p + delta
}

// place finds the conflicts' lines in the versions, when they are new:
// the result's lines, each conflict as the version's lines of it, line up
// with the version's but for what git merged without a conflict, which
// a diff of them finds.
func (m *MergeState) place() {
	oursVer, theirVer := m.Ours.Version(), m.Theirs.Version()
	if m.oursOf == m.Ours && m.theirsOf == m.Theirs && m.oursVer == oursVer && m.theirVer == theirVer {
		return
	}
	m.oursOf, m.theirsOf, m.oursVer, m.theirVer = m.Ours, m.Theirs, oursVer, theirVer
	text := m.Result.String()
	for v, buf := range []*Buffer{m.Ours, m.Theirs} {
		var proj []string
		starts := make([]int, len(m.conflicts))
		at := 0
		for i, c := range m.conflicts {
			start, end := min(c.start, len(text)), min(c.end, len(text))
			proj = append(proj, splitLines(text[at:max(at, start)])...)
			starts[i] = len(proj)
			part := c.ours
			if v == 1 {
				part = c.theirs
			}
			proj = append(proj, splitLines(part)...)
			at = max(at, end)
		}
		proj = append(proj, splitLines(text[at:])...)
		lines := splitLines(buf.String())
		hunks := diff.Lines(trimEndings(proj), trimEndings(lines))
		for i, c := range m.conflicts {
			part := c.ours
			if v == 1 {
				part = c.theirs
			}
			from := mapThrough(starts[i], hunks)
			to := min(from+len(splitLines(part)), len(lines))
			if v == 0 {
				m.conflicts[i].inOurs = [2]int{from, to}
			} else {
				m.conflicts[i].inTheirs = [2]int{from, to}
			}
		}
	}
}

// trimEndings returns lines without their endings, for a diff of texts
// whose lines end alike or not.
func trimEndings(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = trimEnding(l)
	}
	return out
}

// mapThrough maps line a of a diff's old text to its new one: past the
// hunks before it, or to the start of the hunk it is in.
func mapThrough(a int, hunks []diff.Hunk) int {
	delta := 0
	for _, h := range hunks {
		if a < h.A0 {
			break
		}
		if a < h.A1 {
			return h.B0
		}
		delta = h.B1 - h.A1
	}
	return a + delta
}

// choiceText returns what a choice puts in a conflict's place.
func (c *mergeConflict) choiceText(ch MergeChoice) string {
	join := func(a, b string) string {
		if a != "" && !strings.HasSuffix(a, "\n") && b != "" {
			a += "\n"
		}
		return a + b
	}
	switch ch {
	case MergeUnresolved:
		return c.markers
	case MergeOurs:
		return c.ours
	case MergeTheirs:
		return c.theirs
	case MergeOursThenTheirs:
		return join(c.ours, c.theirs)
	case MergeTheirsThenOurs:
		return join(c.theirs, c.ours)
	}
	return ""
}

// choice tells how a conflict is resolved by what its place holds.
func (c *mergeConflict) choice(text string) MergeChoice {
	held := text[min(c.start, len(text)):min(c.end, len(text))]
	for _, ch := range []MergeChoice{MergeUnresolved, MergeOurs, MergeTheirs, MergeOursThenTheirs, MergeTheirsThenOurs, MergeNeither} {
		if held == c.choiceText(ch) {
			return ch
		}
	}
	return MergeEdited
}

// Conflicts returns the conflicts of the result, in its order.
func (m *MergeState) Conflicts() []MergeConflict {
	m.ensure()
	text := m.Result.String()
	snap := m.Result.Snapshot()
	out := make([]MergeConflict, len(m.conflicts))
	for i := range m.conflicts {
		c := &m.conflicts[i]
		out[i] = MergeConflict{Choice: c.choice(text), Ours: c.ours, Base: c.base, Theirs: c.theirs, HasBase: c.hasBase,
			Start: snap.LineOf(c.start), End: snap.LineOf(c.start)}
		if c.end > c.start {
			out[i].End = snap.LineOf(c.end-1) + 1
		}
	}
	return out
}

// Unresolved returns how many conflicts still hold their markers.
func (m *MergeState) Unresolved() int {
	n := 0
	for _, c := range m.Conflicts() {
		if c.Choice == MergeUnresolved {
			n++
		}
	}
	return n
}

// Labels returns the names of the versions: OursLabel and TheirsLabel,
// else the markers', else "Ours" and "Theirs".
func (m *MergeState) Labels() (ours, theirs string) {
	m.ensure()
	ours, theirs = m.OursLabel, m.TheirsLabel
	if ours == "" {
		ours = m.labels[0]
	}
	if theirs == "" {
		theirs = m.labels[1]
	}
	if ours == "" {
		ours = "Ours"
	}
	if theirs == "" {
		theirs = "Theirs"
	}
	return ours, theirs
}

// Choose resolves conflict i by choice, in the result, as an edit that
// undoes; MergeEdited does nothing.
func (m *MergeState) Choose(i int, choice MergeChoice) {
	m.ensure()
	if i < 0 || i >= len(m.conflicts) || choice == MergeEdited {
		return
	}
	c := &m.conflicts[i]
	m.Result.Edit(Edit{Start: c.start, End: c.end, Text: c.choiceText(choice)})
	m.ensure()
}

// ChooseAll resolves every conflict by choice, as one edit to undo.
func (m *MergeState) ChooseAll(choice MergeChoice) {
	m.ensure()
	if choice == MergeEdited || len(m.conflicts) == 0 {
		return
	}
	edits := make([]Edit, len(m.conflicts))
	for i := range m.conflicts {
		c := &m.conflicts[i]
		edits[i] = Edit{Start: c.start, End: c.end, Text: c.choiceText(choice)}
	}
	m.Result.Edit(edits...)
	m.ensure()
}

// Toggle takes a version's lines into conflict i, or out of it, as its
// checkbox: with the other's, after them; without either, the conflict
// holds neither. A conflict edited by hand takes the version alone.
func (m *MergeState) Toggle(i int, v MergeVersion) {
	m.ensure()
	if i < 0 || i >= len(m.conflicts) {
		return
	}
	c := &m.conflicts[i]
	ours, theirs := includes(c.choice(m.Result.String()))
	first := ours && !theirs
	if v == MergeOursVersion {
		ours = !ours
	} else {
		theirs = !theirs
	}
	var ch MergeChoice
	switch {
	case ours && theirs && first:
		ch = MergeOursThenTheirs
	case ours && theirs:
		ch = MergeTheirsThenOurs
	case ours:
		ch = MergeOurs
	case theirs:
		ch = MergeTheirs
	default:
		ch = MergeNeither
	}
	m.Choose(i, ch)
}

// includes tells which versions' lines a choice holds.
func includes(ch MergeChoice) (ours, theirs bool) {
	switch ch {
	case MergeOurs:
		return true, false
	case MergeTheirs:
		return false, true
	case MergeOursThenTheirs, MergeTheirsThenOurs:
		return true, true
	}
	return false, false
}

// GoTo scrolls the versions and the result to conflict i, and puts the
// result's caret at its start.
func (m *MergeState) GoTo(i int) {
	m.ensure()
	if i < 0 || i >= len(m.conflicts) {
		return
	}
	m.goTo = i
	ed := &m.eds[2]
	if ed.view.buf == m.Result && ed.sels != nil {
		ed.setSels([]sel{ed.newSel(m.conflicts[i].start, m.conflicts[i].start)}, revealNone)
	}
}
