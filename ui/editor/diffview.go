package editor

import (
	"slices"
	"strconv"

	"github.com/egoist/mygo/ui"
)

// DiffState is a diff viewer's state, kept by the app across
// frames: the texts it compares, its options, and what it keeps of its
// own, as the lines expanded and its scroll position. Zero options are
// the defaults, those of Pierre's diffs.
type DiffState struct {
	// Old and New are the texts compared; empty buffers are made when
	// nil. New is the one edited when Editable, and can be the buffer of
	// an editor's too: an edit in either shows in both.
	Old, New *Buffer
	// Layout shows the texts side by side, or one over the other.
	Layout DiffLayout
	// Editable lets the new text be edited where it shows, with all an
	// editor does; the diff follows as it is typed.
	Editable bool
	// Inline highlights what changed within changed lines.
	Inline DiffInline
	// Indicators mark changed lines: with bars, + and -, or nothing.
	Indicators DiffIndicators
	// ContextLines is how many unchanged lines show around changes, 3
	// when 0, none when negative; the others collapse into separators
	// that expand them, ExpansionLines (100 when 0) a click, all of them
	// with Shift. ExpandUnchanged shows them all.
	ContextLines    int
	ExpansionLines  int
	ExpandUnchanged bool
	// NoBackground leaves changed lines without their tint, and
	// NoLineNumbers the gutter without numbers.
	NoBackground  bool
	NoLineNumbers bool
	// Wrap wraps long lines rather than scrolling them sideways.
	Wrap bool
	// FitContent makes the viewer as high as its lines, as a file of a
	// list of them, rather than scrolling them itself.
	FitContent bool
	// Font, LineHeight, TabSize and Theme are those of State.
	Font       ui.Font
	LineHeight float32
	TabSize    int
	Theme      *Theme
	// Bindings add key bindings to the editors of the sides.
	Bindings []Binding
	// HunkActions shows Revert and Keep on the change under the pointer,
	// or the caret's, which put the old lines back in New, or make the
	// new lines Old's, as an agent's changes are reviewed.
	HunkActions bool
	// LineSelection lets the numbers of lines be clicked and dragged to
	// select lines, which OnLineSelection reports, with the range the line
	// pressed or dragged to is in: Shift extends the selection from where
	// it started, and Cmd (Ctrl off macOS) adds a line or takes it away.
	// In a unified view, the lines go in the order they show, so that the
	// deleted lines and the added ones of a change select together.
	LineSelection   bool
	OnLineSelection func(DiffLineRange)
	// OnGutterButton shows a + on the gutter of the line under the
	// pointer, as to comment on it, and is called with the line when it
	// is clicked.
	OnGutterButton func(DiffLineRange)
	// Annotations show what RenderAnnotation builds under lines, as
	// comments of a review, as high as each asks; the other side of a
	// split view leaves the room empty.
	Annotations      []DiffAnnotation
	RenderAnnotation func(c *ui.Context, a DiffAnnotation)

	// RenderSeparator, RenderHunkActions and RenderGutterButton build the
	// viewer's separators, the actions on a change and the gutter's
	// button in place of its own (DefaultSeparator, DefaultHunkActions
	// and DefaultGutterButton, which they can call to wrap them): each is
	// given the element the viewer places, to style and fill, and what
	// the part shows and does. A separator is as wide as the viewer, less
	// the room of the scroll bar's track while the viewer scrolls, and 32
	// DIPs high.
	RenderSeparator    func(c *ui.Context, sep ui.Element, s *DiffSeparator)
	RenderHunkActions  func(c *ui.Context, bar ui.Element, a *DiffHunkActions)
	RenderGutterButton func(c *ui.Context, btn ui.Element, line DiffLineRange)
	// RenderFindBar and RenderGoToLine build the bars of the sides'
	// editors, as State's.
	RenderFindBar  func(c *ui.Context, bar ui.Element, f *FindBar)
	RenderGoToLine func(c *ui.Context, bar ui.Element, g *GoToLineBar)

	model   diffModel
	eds     [2]editor // the old side's and the new side's
	sides   [2]diffSide
	oldView view // the old text, for its lines in a unified view
	scroll  sharedScroll
	// revealed are the unchanged lines expanded, in the old text, which
	// edits of the new text leave where they are; revealedVer is the old
	// text's version they are of.
	revealed    []lineSpan
	revealedVer uint64
	revealedOf  *Buffer
	expandV     int
	// selected are the lines selected by their numbers, by side and
	// line, none overlapping or touching another of its side; selAnchor
	// is the line the selection extends from, and selBase what stays
	// selected besides, as a Cmd-click leaves it. lineDrag is set while
	// a press on a number drags.
	selected  []DiffLineRange
	selAnchor DiffLineRange
	selBase   []DiffLineRange
	lineDrag  bool
	// oldActive is set while the old side of a split view has the
	// keyboard, and stays after it goes, until the new side has it.
	oldActive bool
	prepared  diffPrepared
	// blocksV counts the blocks prepare made, and built is the geometry
	// the separators and buttons were built for, which a frame changing
	// it builds again (diffGeom).
	blocksV int
	built   diffGeom
	gapList []diffGap
	partial *diffPartial
}

// DiffLayout is how a diff viewer lays out its two texts.
type DiffLayout uint8

const (
	// DiffSplit shows the old text on the left and the new on the right,
	// their lines aligned.
	DiffSplit DiffLayout = iota
	// DiffUnified shows the new text, with the lines deleted from the
	// old above where they were.
	DiffUnified
)

// DiffInline is how a diff viewer highlights what changed within lines.
type DiffInline uint8

const (
	// DiffInlineWordAlt highlights words, joining those one character
	// apart, as "a b c" changed reads as one highlight.
	DiffInlineWordAlt DiffInline = iota
	// DiffInlineWord highlights each word changed.
	DiffInlineWord
	// DiffInlineWordLine highlights from the first word changed to the
	// last.
	DiffInlineWordLine
	// DiffInlineChar highlights characters.
	DiffInlineChar
	// DiffInlineNone highlights nothing within lines.
	DiffInlineNone
)

// DiffIndicators is how a diff viewer marks changed lines.
type DiffIndicators uint8

const (
	// DiffBars draws a bar at the gutter's edge, striped for deletions.
	DiffBars DiffIndicators = iota
	// DiffClassic shows + and - before the lines.
	DiffClassic
	// DiffNoIndicators marks lines by their background alone.
	DiffNoIndicators
)

// DiffSide is a side of a diff: the old text or the new.
type DiffSide uint8

const (
	DiffOld DiffSide = iota
	DiffNew
)

// DiffLineRange is lines Start to End, inclusive, from 0, of a side.
type DiffLineRange struct {
	Side       DiffSide
	Start, End int
}

// DiffAnnotation is room under a line of a side for what
// RenderAnnotation builds there, Height DIPs high. Key identifies it
// among the others.
type DiffAnnotation struct {
	Side   DiffSide
	Line   int
	Height float32
	Key    any
}

// DiffHunk is a change: the old lines OldStart to OldEnd, exclusive,
// replaced by the new lines NewStart to NewEnd.
type DiffHunk struct {
	OldStart, OldEnd, NewStart, NewEnd int
}

func (d *DiffState) context() int {
	switch {
	case d.ContextLines < 0:
		return 0
	case d.ContextLines == 0:
		return 3
	}
	return d.ContextLines
}

func (d *DiffState) expansionLines() int {
	if d.ExpansionLines <= 0 {
		return 100
	}
	return d.ExpansionLines
}

func (d *DiffState) ensure() {
	if d.Old == nil {
		d.Old = NewBuffer("")
	}
	if d.New == nil {
		d.New = NewBuffer("")
	}
	if pt := d.partial; pt != nil && (pt.old != d.Old || pt.new != d.New) {
		// Texts of the app's in place of a patch's.
		d.partial = nil
		d.prepared = diffPrepared{}
	}
	d.model.set(d.Old, d.New)
	d.model.update()
	// The lines revealed move with edits of the old text, as Keep makes,
	// and go with it.
	if d.revealedOf != d.Old {
		d.revealed, d.revealedOf, d.revealedVer = nil, d.Old, d.Old.Version()
		d.expandV++
	}
	if ver := d.Old.Version(); ver != d.revealedVer {
		if changes, _, ok := d.Old.changesSince(d.revealedVer); ok && len(d.revealed) > 0 {
			for _, cs := range changes {
				for i := range d.revealed {
					d.revealed[i] = lineSpan{mapLine(d.revealed[i].from, cs), mapLine(d.revealed[i].to, cs)}
				}
			}
		} else if !ok {
			d.revealed = nil
		}
		d.revealedVer = ver
		d.expandV++
	}
}

// Hunks returns the changes between the texts.
func (d *DiffState) Hunks() []DiffHunk {
	d.ensure()
	out := make([]DiffHunk, len(d.model.hunks))
	for i, h := range d.model.hunks {
		out[i] = DiffHunk{h.A0, h.A1, h.B0, h.B1}
	}
	return out
}

// Stats returns how many lines the changes add and delete.
func (d *DiffState) Stats() (added, deleted int) {
	d.ensure()
	return d.model.stats()
}

// Revert puts the old lines of hunk i back in New.
func (d *DiffState) Revert(i int) {
	d.ensure()
	d.model.revert(i)
}

// Accept makes the new lines of hunk i Old's, so it is no longer a
// change.
func (d *DiffState) Accept(i int) {
	d.ensure()
	d.model.accept(i)
}

// ExpandAll shows every unchanged line; CollapseAll collapses them again
// into separators.
func (d *DiffState) ExpandAll() {
	d.ensure()
	d.ExpandUnchanged = true
}

func (d *DiffState) CollapseAll() {
	d.ExpandUnchanged = false
	d.revealed = nil
	d.expandV++
}

// SelectedLines returns the first of the ranges of lines selected by
// their numbers (SelectedRanges), and whether there are any.
func (d *DiffState) SelectedLines() (DiffLineRange, bool) {
	if len(d.selected) == 0 {
		return DiffLineRange{}, false
	}
	return d.selected[0], true
}

// SetSelectedLines selects a range of lines, as clicking their numbers
// does; ok false clears the selection.
func (d *DiffState) SetSelectedLines(r DiffLineRange, ok bool) {
	if !ok {
		d.SetSelectedRanges(nil)
		return
	}
	d.SetSelectedRanges([]DiffLineRange{r})
}

// SelectedRanges returns the lines selected by their numbers: the old
// side's ranges, then the new side's, each in order, none overlapping
// or touching another of its side.
func (d *DiffState) SelectedRanges() []DiffLineRange { return slices.Clone(d.selected) }

// SetSelectedRanges selects lines, as clicking and dragging their
// numbers does; none clears the selection.
func (d *DiffState) SetSelectedRanges(rs []DiffLineRange) {
	d.selected = normalRanges(rs)
	d.selBase, d.lineDrag = nil, false
	if len(d.selected) > 0 {
		r := d.selected[0]
		d.selAnchor = DiffLineRange{Side: r.Side, Start: r.Start, End: r.Start}
	}
}

// LineSelected reports whether a line of a side is selected by its
// number.
func (d *DiffState) LineSelected(side DiffSide, line int) bool {
	return rangeOf(d.selected, side, line) >= 0
}

// rangeOf returns the index of the range of rs holding a line of a side,
// or -1.
func rangeOf(rs []DiffLineRange, side DiffSide, line int) int {
	for i, r := range rs {
		if r.Side == side && line >= r.Start && line <= r.End {
			return i
		}
	}
	return -1
}

// normalRanges sorts ranges by side and line, joining those of a side
// that overlap or touch, each from its start to its end.
func normalRanges(rs []DiffLineRange) []DiffLineRange {
	var out []DiffLineRange
	for _, r := range rs {
		if r.End < r.Start {
			r.Start, r.End = r.End, r.Start
		}
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b DiffLineRange) int {
		if a.Side != b.Side {
			return int(a.Side) - int(b.Side)
		}
		return a.Start - b.Start
	})
	merged := out[:0]
	for _, r := range out {
		if n := len(merged); n > 0 && merged[n-1].Side == r.Side && r.Start <= merged[n-1].End+1 {
			merged[n-1].End = max(merged[n-1].End, r.End)
			continue
		}
		merged = append(merged, r)
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

// withoutLine returns rs without a line of a side.
func withoutLine(rs []DiffLineRange, side DiffSide, line int) []DiffLineRange {
	var out []DiffLineRange
	for _, r := range rs {
		if r.Side != side || line < r.Start || line > r.End {
			out = append(out, r)
			continue
		}
		if r.Start < line {
			out = append(out, DiffLineRange{Side: side, Start: r.Start, End: line - 1})
		}
		if line < r.End {
			out = append(out, DiffLineRange{Side: side, Start: line + 1, End: r.End})
		}
	}
	return out
}

// GoToHunk puts the caret of the active side (ActiveSide) at the start
// of hunk i and scrolls it to the middle of the view.
func (d *DiffState) GoToHunk(i int) {
	d.ensure()
	if i < 0 || i >= len(d.model.hunks) {
		return
	}
	h := d.model.hunks[i]
	side := d.ActiveSide()
	ed := d.editorOf(side)
	line := h.B0
	if side == DiffOld {
		line = h.A0
	}
	r := &ed.view.text
	off := r.LineStart(min(line, r.LineCount()-1))
	ed.setSels([]sel{ed.newSel(off, off)}, revealCenter)
}

// Run runs a command in the editor of the active side (ActiveSide), as
// its key would, and reports whether it did something: Find opens its
// find bar, as an app's menu item does.
func (d *DiffState) Run(cmd Command) bool {
	ed := d.editorOf(d.ActiveSide())
	return ed.retry(func() bool { return ed.run(cmd) })
}

// Find opens the find bar of the active side's editor (ActiveSide) with
// query, and selects its first match after the caret.
func (d *DiffState) Find(query string) {
	ed := d.editorOf(d.ActiveSide())
	f := &ed.find
	f.open, f.query, f.stale = true, query, true
	f.step(ed, 1)
}

// ActiveSide returns the side that has the keyboard, or had it last: the
// new side before either had it, and in a unified view. Cursor,
// CursorPos, CurrentHunk, GoToHunk, Run and Find act in its editor.
func (d *DiffState) ActiveSide() DiffSide {
	if d.oldActive && d.Layout == DiffSplit {
		return DiffOld
	}
	return DiffNew
}

// Cursor returns the offset of the newest caret of the active side, in
// its text.
func (d *DiffState) Cursor() int {
	return d.editorOf(d.ActiveSide()).primary().head
}

// CursorPos returns the line and column of the newest caret of the
// active side, in its text.
func (d *DiffState) CursorPos() Pos {
	ed := d.editorOf(d.ActiveSide())
	return posOf(ed.view.text, ed.primary().head)
}

// CurrentHunk returns the hunk the caret of the active side is in, with
// inside true, as for Revert; else the last hunk before the caret, as
// "2 of 5" counts changes and Next Change steps from (GoToHunk(i+1)), or
// -1 before the first. On a side the hunk has no lines of, as the new
// side of a deletion, the caret is in it on the line where its lines
// were.
func (d *DiffState) CurrentHunk() (i int, inside bool) {
	side := d.ActiveSide()
	ed := d.editorOf(side)
	line := ed.view.text.LineOf(ed.primary().head)
	if j := d.model.hunkAt(line, side == DiffOld); j >= 0 {
		return j, true
	}
	i = -1
	for j, h := range d.model.hunks {
		start := h.B0
		if side == DiffOld {
			start = h.A0
		}
		if start > line {
			break
		}
		i = j
	}
	return i, false
}

// editorOf returns the editor of a side, made ready to act in before the
// viewer is first built.
func (d *DiffState) editorOf(side DiffSide) *editor {
	d.ensure()
	ed, buf := &d.eds[side], d.New
	if side == DiffOld {
		buf = d.Old
	}
	if ed.view.buf != buf || ed.sels == nil {
		ed.init(buf, d.sideOptions(side == DiffOld), ui.LightTheme())
	}
	ed.sync()
	return ed
}

// sideOptions are the options of the editor of the old side, or the new.
func (d *DiffState) sideOptions(old bool) options {
	// Both gutters as wide, so that both sides wrap alike.
	digits := max(3, len(strconv.Itoa(max(d.Old.LineCount(), d.New.LineCount()))))
	o := options{
		readOnly: old || !d.Editable, font: d.Font, lineHeight: d.LineHeight, tabSize: d.TabSize,
		wrap: d.Wrap, lineNumbers: LineNumbersOn, whitespace: WhitespaceNone, autoClose: true,
		theme: d.Theme, bindings: d.Bindings, fitContent: d.FitContent, minDigits: digits,
		renderFindBar: d.RenderFindBar, renderGoToLine: d.RenderGoToLine,
	}
	if d.NoLineNumbers {
		o.lineNumbers = LineNumbersOff
	}
	o.diffSide = diffNewSide
	if old {
		o.diffSide = diffOldSide
	}
	return o
}

// DiffView builds the diff viewer.
func DiffView(c *ui.Context, d *DiffState) ui.Element {
	d.ensure()
	t := c.Theme()
	ct := d.Theme.resolve(t)
	opts := d.sideOptions
	d.sides[0] = diffSide{d: d, old: true, hide: d.sides[0].hide, blks: d.sides[0].blks}
	d.sides[1] = diffSide{d: d, unified: d.Layout == DiffUnified, hide: d.sides[1].hide, blks: d.sides[1].blks}
	if d.Layout == DiffUnified {
		d.oldView.setTheme(ct)
		d.oldView.setBuffer(d.Old)
		d.oldView.sync(nil)
		o := opts(false)
		o.decor = &d.sides[1]
		if d.Indicators == DiffClassic {
			o.signs = true
		}
		root := ui.Box(c).Direction(ui.LeftToRight).Clip()
		root.Draw(func(p *ui.Painter, r ui.Rect) {
			ed := &d.eds[1]
			ed.layout(r.W, r.H, p.Scale())
			d.oldView.setStyle(ed.view.font, ed.o.lineHeight, p.Scale(), ed.tabSize())
			d.oldView.setWrap(ed.view.wrap, ed.view.wrapWidth)
			d.prepare()
			ed.layout(r.W, r.H, p.Scale())
			d.rebuildIfMoved()
		})
		root.Children(func() {
			ed := d.eds[1].build(c, d.New, o)
			ed.Fill()
			if d.FitContent {
				ed.FillWidth()
			}
			d.buildSeparators(c, ct)
		})
		if d.FitContent {
			d.fit()
			root.Height(float32(d.eds[1].view.heights.total()))
		}
		return root
	}
	// Side by side: two editors sharing their scroll.
	d.scroll.eds = []*editor{&d.eds[0], &d.eds[1]}
	// The old text on the left, as code reads left to right.
	row := ui.Row(c).Direction(ui.LeftToRight).Clip()
	row.Draw(func(p *ui.Painter, r ui.Rect) {
		w := (r.W - 1) / 2
		for i := range d.eds {
			d.eds[i].layout(w, r.H, p.Scale())
		}
		d.prepare()
		for i := range d.eds {
			d.eds[i].layout(w, r.H, p.Scale())
		}
		if !d.FitContent {
			d.scroll.set(d.scroll.y)
			// The new side reveals its carets for both.
			ed := &d.eds[1]
			if ed.reveal != revealNone {
				ed.doReveal()
				d.scroll.set(ed.scrollY)
			}
			if ed0 := &d.eds[0]; ed0.reveal != revealNone {
				ed0.doReveal()
				d.scroll.set(ed0.scrollY)
			}
		}
		d.rebuildIfMoved()
	})
	row.Children(func() {
		for i, b := range []*Buffer{d.Old, d.New} {
			o := opts(i == 0)
			o.decor = &d.sides[i]
			o.sharedScroll = &d.scroll
			if d.FitContent {
				o.sharedScroll = nil
			}
			o.noScrollbar = i == 0
			if d.Indicators == DiffClassic {
				o.signs = true
			}
			ed := d.eds[i].build(c, b, o)
			ed.Grow(1).Basis(0).FillHeight()
			if i == 0 {
				ui.Box(c).Width(1).FillHeight().Background(ct.Background.Mix(t.Border, 0.6))
			}
			if d.eds[i].focused {
				d.oldActive = i == 0
			}
		}
		d.buildSeparators(c, ct)
	})
	if d.FitContent {
		d.fit()
		row.Height(float32(max(d.eds[0].view.heights.total(), d.eds[1].view.heights.total())))
	}
	return row
}

// fit lays out a diff that fits its content as its Draw does, its
// unchanged lines folded, at the size its editors were built at. A diff
// not drawn yet, as one below the view of a scroll container, is then as
// high as it shows, not as all its lines: else it would shrink as it came
// into view, and the container's end with it, under the user scrolling.
func (d *DiffState) fit() {
	eds := d.eds[:]
	if d.Layout == DiffUnified {
		eds = d.eds[1:]
		ed := &d.eds[1]
		d.oldView.setStyle(ed.view.font, ed.o.lineHeight, max(1, ed.view.scale), ed.tabSize())
		d.oldView.setWrap(ed.view.wrap, ed.view.wrapWidth)
	}
	d.prepare()
	for i := range eds {
		ed := &eds[i]
		w := ed.r.W
		if w <= 0 {
			w = 600
		}
		ed.layout(w, ed.viewH, max(1, ed.view.scale))
	}
}

// expand reveals lines of the gap starting at old line a: at its top,
// below the change before it (dir 1), at its bottom (dir -1), or all.
func (d *DiffState) expand(a int, dir int, all bool) {
	for _, g := range d.gapList {
		if g.a != a || g.hidden == 0 {
			continue
		}
		from, to := g.hideA, g.hideA+g.hidden-1
		n := d.expansionLines()
		switch {
		case all:
		case dir > 0:
			to = min(to, from+n-1)
		default:
			from = max(from, to-n+1)
		}
		d.revealed = mergeSpans(append(d.revealed, lineSpan{from, to}))
		d.expandV++
		return
	}
}

// revealedAt returns the last line of the lines revealed holding old line
// a, or -1.
func (d *DiffState) revealedAt(a int) int {
	for _, sp := range d.revealed {
		if a >= sp.from && a <= sp.to {
			return sp.to
		}
	}
	return -1
}

// revealedBefore returns the first line of the lines revealed holding old
// line a, or -1.
func (d *DiffState) revealedBefore(a int) int {
	for _, sp := range d.revealed {
		if a >= sp.from && a <= sp.to {
			return sp.from
		}
	}
	return -1
}

// selectLine takes a press on the number of a line: it selects the line,
// or with Shift the lines from the anchor to it, or with Cmd adds it or
// takes it away; a press on the only line selected clears the selection.
// A drag from it goes on with dragLine.
func (d *DiffState) selectLine(side DiffSide, line int, mods ui.Modifiers) {
	at := DiffLineRange{Side: side, Start: line, End: line}
	d.lineDrag = true
	switch {
	case mods&ui.Shift != 0 && len(d.selected) > 0:
		d.selected = normalRanges(append(slices.Clone(d.selBase), d.span(d.selAnchor, at)...))
	case mods&ui.Cmd != 0 && d.LineSelected(side, line):
		d.selected = withoutLine(d.selected, side, line)
		d.selBase, d.selAnchor, d.lineDrag = d.selected, at, false
	case mods&ui.Cmd != 0:
		d.selBase, d.selAnchor = slices.Clone(d.selected), at
		d.selected = normalRanges(append(slices.Clone(d.selected), at))
	case len(d.selected) == 1 && d.selected[0] == at:
		d.selected, d.selBase, d.lineDrag = nil, nil, false
	default:
		d.selected, d.selBase, d.selAnchor = []DiffLineRange{at}, nil, at
	}
	d.reportSelection(side, line)
}

// dragLine extends the selection a press on a number started to the line
// the pointer is over.
func (d *DiffState) dragLine(side DiffSide, line int) {
	if !d.lineDrag {
		return
	}
	sel := normalRanges(append(slices.Clone(d.selBase), d.span(d.selAnchor, DiffLineRange{Side: side, Start: line, End: line})...))
	if slices.Equal(sel, d.selected) {
		return
	}
	d.selected = sel
	d.reportSelection(side, line)
}

// reportSelection tells OnLineSelection of the range a line is in, when
// it is selected.
func (d *DiffState) reportSelection(side DiffSide, line int) {
	if d.OnLineSelection == nil {
		return
	}
	if i := rangeOf(d.selected, side, line); i >= 0 {
		d.OnLineSelection(d.selected[i])
	}
}

// span returns the lines from line a to line b, each a range of one line
// of its side, as they show: those of a side between them, in a split
// view, where lines of the two sides don't select together (only b
// then); in a unified view, the new lines and the deleted ones between
// them, the deleted lines of a change above its first new line.
func (d *DiffState) span(a, b DiffLineRange) []DiffLineRange {
	if !d.sides[1].unified {
		if a.Side != b.Side {
			return []DiffLineRange{{Side: b.Side, Start: b.Start, End: b.Start}}
		}
		return []DiffLineRange{{Side: a.Side, Start: min(a.Start, b.Start), End: max(a.Start, b.Start)}}
	}
	ka, kb := d.showOrder(a), d.showOrder(b)
	if kb.less(ka) {
		ka, kb = kb, ka
	}
	var out []DiffLineRange
	for _, h := range d.model.hunks {
		for o := h.A0; o < h.A1; o++ {
			if k := (showKey{h.B0, 0, o}); !k.less(ka) && !kb.less(k) {
				out = append(out, DiffLineRange{Side: DiffOld, Start: o, End: o})
			}
		}
	}
	for n := ka.line; n <= kb.line; n++ {
		if k := (showKey{n, 1, n}); !k.less(ka) && !kb.less(k) {
			out = append(out, DiffLineRange{Side: DiffNew, Start: n, End: n})
		}
	}
	return out
}

// showKey orders the lines of a unified view as they show: by the new
// line they show at or above, the deleted lines (new 0) above it in
// their order.
type showKey struct{ line, new, at int }

func (k showKey) less(o showKey) bool {
	if k.line != o.line {
		return k.line < o.line
	}
	if k.new != o.new {
		return k.new < o.new
	}
	return k.at < o.at
}

// showOrder returns where a line shows in a unified view: an old line of
// a change above the change's first new line, an unchanged old one as
// the new line it is.
func (d *DiffState) showOrder(r DiffLineRange) showKey {
	if r.Side == DiffNew {
		return showKey{r.Start, 1, r.Start}
	}
	for _, h := range d.model.hunks {
		if r.Start >= h.A0 && r.Start < h.A1 {
			return showKey{h.B0, 0, r.Start}
		}
	}
	if n, ok := d.model.newLineOf(r.Start); ok {
		return showKey{n, 1, n}
	}
	return showKey{r.Start, 1, r.Start}
}

// activeHunk returns the hunk under the pointer on either side, or else
// the one holding the new side's newest caret, or -1.
func (d *DiffState) activeHunk() int {
	for i := range d.eds {
		ed := &d.eds[i]
		if !ed.pointerIn || ed.hoverLine < 0 {
			continue
		}
		side := &d.sides[i]
		if i == 1 && side.unified {
			if j := d.model.hunkAtNew(ed.hoverLine); j >= 0 {
				return j
			}
			continue
		}
		if j := side.hunkOf(ed.hoverLine); j >= 0 {
			return j
		}
		if i == 1 {
			if j := d.model.hunkAtNew(ed.hoverLine); j >= 0 {
				return j
			}
		}
	}
	ed := &d.eds[1]
	if ed.focused && len(ed.sels) > 0 {
		line := ed.view.text.LineOf(ed.primary().head)
		return d.model.hunkAtNew(line)
	}
	return -1
}

// isDark reports whether a background is dark.
func isDark(c ui.Color) bool {
	l := 0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)
	return l < 128
}

// diffGeom is what places the elements a diff view builds over its lines,
// as its separators, which the view builds from the last frame's layout:
// the blocks, the wraps of the lines, the height and the scroll.
type diffGeom struct {
	blocks         int
	wrapsA, wrapsB uint64
	total, scroll  float64
}

func (d *DiffState) geom() diffGeom {
	a := &d.eds[0].view
	if d.Layout == DiffUnified {
		a = &d.oldView
	}
	b := &d.eds[1]
	return diffGeom{d.blocksV, a.heights.wraps.version, b.view.heights.wraps.version, b.view.heights.total(), b.scrollY}
}

// rebuildIfMoved asks for the view to be built again when laying it out
// for painting moved what its elements were built for, as expanding
// lines from a separator's button does: else the separator stays until
// something else builds the view, as the pointer moving.
func (d *DiffState) rebuildIfMoved() {
	if d.geom() != d.built {
		d.eds[1].services.Invalidate()
	}
}
