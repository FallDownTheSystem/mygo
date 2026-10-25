package editor

import (
	"fmt"
	"strconv"

	"github.com/egoist/mygo/internal/l10n"
	"github.com/egoist/mygo/ui"
)

// The three-way merge editor, as Fork's: the versions merged side by
// side above, ours on the left, each conflict's lines of them aligned,
// with a checkbox above each that takes its lines into the result; the
// result below, edited as any text, its conflicts tinted by how they
// are resolved.

const (
	mergeVersionBarH = 26 // the bar naming a version, or the result
	mergeHeaderH     = 28 // the bar above a conflict
)

// mergeSide shows a version of a merge (0 ours, 1 theirs), or its result
// (2), in an editor.
type mergeSide struct {
	m    *MergeState
	v    int
	blks []block
}

func (s *mergeSide) hidden() []lineSpan { return nil }
func (s *mergeSide) blocks() []block    { return s.blks }

func (s *mergeSide) words(int) []diffRange { return nil }
func (s *mergeSide) number(line int) int   { return line + 1 }

func (s *mergeSide) blockInput(*editor, *block, ui.InputEvent, ui.Rect) bool { return false }
func (s *mergeSide) dragLines(*editor, ui.InputEvent) bool                   { return false }

// lines returns the lines of conflict i the side shows, to exclusive.
func (s *mergeSide) lines(i int) (from, to int) {
	c := &s.m.conflicts[i]
	switch s.v {
	case 0:
		return c.inOurs[0], c.inOurs[1]
	case 1:
		return c.inTheirs[0], c.inTheirs[1]
	}
	snap := s.m.Result.Snapshot()
	from = snap.LineOf(min(c.start, snap.Len()))
	if c.end <= c.start {
		return from, from
	}
	return from, snap.LineOf(min(c.end-1, snap.Len())) + 1
}

// conflictAt returns the conflict whose lines hold line on the side, or
// -1.
func (s *mergeSide) conflictAt(line int) int {
	for i := range s.m.conflicts {
		if from, to := s.lines(i); line >= from && line < to {
			return i
		}
	}
	return -1
}

func (s *mergeSide) lineStyle(line int) lineLook {
	var st lineLook
	i := s.conflictAt(line)
	if i < 0 {
		return st
	}
	ed := &s.m.eds[s.v]
	dc := ed.theme.diff(isDark(ed.theme.Background))
	ch := s.m.choices[i]
	switch {
	case ch == MergeUnresolved:
		st.bg, st.gutter, st.number = dc.deletedLine, dc.deletedGutter, dc.deleted
	case ch == MergeEdited && s.v == 2:
		st.bg = mixLab(ed.theme.Background, ed.theme.Modified, 0.16)
		st.gutter = mixLab(ed.theme.Background, ed.theme.Modified, 0.28)
		st.number = ed.theme.Modified
	case s.v == 2:
		st.bg, st.gutter, st.number = dc.addedLine, dc.addedGutter, dc.added
	default:
		ours, theirs := includes(ch)
		if s.v == 0 && ours || s.v == 1 && theirs {
			st.bg, st.gutter, st.number = dc.addedLine, dc.addedGutter, dc.added
		} else {
			// Left out of the result: faint.
			st.bg = mixLab(ed.theme.Background, ed.theme.Text, 0.04)
		}
	}
	return st
}

// paintBlock paints a conflict's bar, the band its controls are on, and
// a spacer's hatching, as a diff's fillers.
func (s *mergeSide) paintBlock(p *ui.Painter, ed *editor, b *block, r ui.Rect) {
	ct := ed.theme
	switch b.kind {
	case blockHeader:
		p.Fill(r, mixLab(ct.Background, ct.Text, 0.05), 0)
	case blockSpacer:
		text := ui.Rect{X: r.X + ed.gutterW, Y: r.Y, W: r.W - ed.gutterW, H: r.H}
		p.Clip(text, 0, func() {
			c := ct.Text.Alpha(0.07)
			for x := text.X - r.H; x < text.X+text.W; x += 8 {
				p.Line(x, r.Y+r.H, x+r.H, r.Y, 1, c)
			}
		})
	}
}

// buildBlocks builds the controls of the conflicts' bars in view: on a
// version's, the checkbox that takes its lines into the result; on the
// result's, how the conflict is resolved.
func (s *mergeSide) buildBlocks(c *ui.Context, ed *editor) {
	m := s.m
	t := c.Theme()
	tx := l10n.In(c.Locale())
	ours, theirs := m.Labels()
	ed.blocksInView(func(b *block, top float64) {
		if b.kind != blockHeader {
			return
		}
		i := b.hunk
		if i < 0 || i >= len(m.conflicts) {
			return
		}
		cf := &m.conflicts[i]
		bar := ui.Row(c).Key(fmt.Sprint("conflict ", i)).Absolute().Left(0).Right(ed.trackWidth()).Top(float32(top-ed.scrollY)).Height(b.height).
			AlignItems(ui.Center).Gap(t.Space(1)).Padding(0, t.Space(1.5)).Role(ui.RoleGroup).
			Label(tx.MergeConflictOf(i+1, len(m.conflicts))).Cursor(ui.CursorDefault).KeepFocus().HandleInput(shield)
		bar.Children(func() {
			small := max(11, t.FontSize-1)
			if s.v < 2 {
				in := [2]bool{}
				in[0], in[1] = includes(m.choices[i])
				name := ours
				if s.v == 1 {
					name = theirs
				}
				cf.checks[s.v] = in[s.v]
				v := MergeVersion(s.v)
				ui.Checkbox(c.Key("take"), &cf.checks[s.v], tx.MergeTake(name)).OnChange(func() { m.Toggle(i, v) })
			} else {
				ui.Text(c, mergeChoiceLabel(tx, m.choices[i], ours, theirs)).FontSize(small).TextColor(t.TextMuted).SingleLine()
			}
			ui.Box(c).Grow(1)
			ui.Text(c, tx.MergeConflictOf(i+1, len(m.conflicts))).FontSize(small).TextColor(t.TextMuted).SingleLine()
		})
	})
}

// mergeChoiceLabel says how a conflict is resolved.
func mergeChoiceLabel(tx l10n.Messages, ch MergeChoice, ours, theirs string) string {
	switch ch {
	case MergeOurs:
		return tx.MergeTook(ours)
	case MergeTheirs:
		return tx.MergeTook(theirs)
	case MergeOursThenTheirs:
		return tx.MergeTookBoth(ours, theirs)
	case MergeTheirsThenOurs:
		return tx.MergeTookBoth(theirs, ours)
	case MergeNeither:
		return tx.MergeTookNeither()
	case MergeEdited:
		return tx.MergeEdited()
	}
	return tx.MergeUnresolved()
}

// mergePrepared is what the blocks of the editors were made for.
type mergePrepared struct {
	ver, oursVer, theirVer uint64
	lh                     float32
	n                      int
	choices                string
}

// mergeGeom is what the bars' controls were built for.
type mergeGeom struct {
	totals  [3]float64
	scrolls [3]float64
}

// prepare makes the editors' blocks: a bar above each conflict, and
// spacers that align the versions' lines of each conflict and of the
// lines between them, the shorter side's below its lines.
func (m *MergeState) prepare() {
	lh := m.eds[2].view.lineHeight
	key := mergePrepared{m.Result.Version(), m.Ours.Version(), m.Theirs.Version(), lh, len(m.conflicts), fmt.Sprint(m.choices)}
	if m.prepared == key {
		return
	}
	m.prepared = key
	bars := float32(mergeHeaderH)
	var blk [3][]block
	counts := [2]int{m.Ours.LineCount(), m.Theirs.LineCount()}
	// at attaches a block to the end of lines from to to of version v:
	// below its last, or above the line after them when there are none.
	at := func(v, from, to int, b block) {
		switch {
		case to > from:
			b.line, b.below = to-1, true
		case from < counts[v]:
			b.line = from
		default:
			b.line, b.below = max(counts[v]-1, 0), true
		}
		blk[v] = append(blk[v], b)
	}
	align := func(i int, spans [2][2]int) {
		h := [2]float32{float32(spans[0][1]-spans[0][0]) * lh, float32(spans[1][1]-spans[1][0]) * lh}
		for v := range 2 {
			if extra := h[1-v] - h[v]; extra > 0.5 {
				at(v, spans[v][0], spans[v][1], block{kind: blockSpacer, height: extra, from: 1, hunk: i})
			}
		}
	}
	prev := [2]int{}
	for i := range m.conflicts {
		c := &m.conflicts[i]
		spans := [2][2]int{c.inOurs, c.inTheirs}
		align(-1, [2][2]int{{prev[0], spans[0][0]}, {prev[1], spans[1][0]}})
		for v := range 2 {
			b := block{kind: blockHeader, height: bars, hunk: i}
			if spans[v][0] < counts[v] {
				b.line = spans[v][0]
			} else {
				b.line, b.below = max(counts[v]-1, 0), true
			}
			blk[v] = append(blk[v], b)
		}
		align(i, spans)
		prev = [2]int{spans[0][1], spans[1][1]}
	}
	side := &mergeSide{m: m, v: 2}
	n := m.Result.LineCount()
	for i := range m.conflicts {
		from, _ := side.lines(i)
		b := block{kind: blockHeader, height: bars, hunk: i}
		if from < n {
			b.line = from
		} else {
			b.line, b.below = max(n-1, 0), true
		}
		blk[2] = append(blk[2], b)
	}
	for v := range 3 {
		m.sides[v].blks = blk[v]
	}
}

// options are those of the editor of a version, or of the result (v 2).
func (m *MergeState) options(v int) options {
	digits := max(3, len(strconv.Itoa(max(m.Ours.LineCount(), m.Theirs.LineCount(), m.Result.LineCount()))))
	return options{
		readOnly: v < 2, font: m.Font, lineHeight: m.LineHeight, tabSize: m.TabSize,
		wrap: v == 2 && m.Wrap, lineNumbers: LineNumbersOn, whitespace: WhitespaceNone, autoClose: true,
		theme: m.Theme, bindings: m.Bindings, minDigits: digits, decor: &m.sides[v],
	}
}

// MergeView builds the three-way merge editor.
func MergeView(c *ui.Context, m *MergeState) ui.Element {
	m.ensure()
	m.choices = m.currentChoices()
	t := c.Theme()
	ct := m.Theme.resolve(t)
	tx := l10n.In(c.Locale())
	for v := range 3 {
		m.sides[v] = mergeSide{m: m, v: v, blks: m.sides[v].blks}
	}
	m.scroll.eds = []*editor{&m.eds[0], &m.eds[1]}
	ours, theirs := m.Labels()
	border := ct.Background.Mix(t.Border, 0.6)
	versionBar := func(label string) {
		bar := ui.Row(c).Height(mergeVersionBarH).Shrink(0).AlignItems(ui.Center).Padding(0, t.Space(1.5)).
			Background(mixLab(ct.Background, ct.Text, 0.03))
		bar.Children(func() {
			ui.Text(c, label).FontSize(max(11, t.FontSize-1)).FontWeight(600).TextColor(t.TextMuted).SingleLine().Ellipsis("…").Shrink(1)
		})
	}
	split := m.Split
	if split <= 0 || split >= 1 {
		split = 0.5
	}
	root := ui.Column(c).Clip()
	root.Children(func() {
		top := ui.Column(c).Grow(split).Basis(0).Clip()
		top.Children(func() {
			labels := ui.Row(c).Direction(ui.LeftToRight).Shrink(0)
			labels.Children(func() {
				for i, label := range []string{ours, theirs} {
					col := ui.Column(c).Grow(1).Basis(0)
					col.Children(func() { versionBar(label) })
					if i == 0 {
						ui.Box(c).Width(1).FillHeight().Background(border)
					}
				}
			})
			row := ui.Row(c).Direction(ui.LeftToRight).Grow(1).Basis(0).Clip()
			row.Draw(func(p *ui.Painter, r ui.Rect) {
				w := (r.W - 1) / 2
				for i := range 2 {
					m.eds[i].layout(w, r.H, p.Scale())
				}
				m.prepare()
				for i := range 2 {
					m.eds[i].layout(w, r.H, p.Scale())
				}
				m.scroll.set(m.scroll.y)
				m.scrollToConflict(true)
				m.rebuildIfMoved()
			})
			row.Children(func() {
				for i, b := range []*Buffer{m.Ours, m.Theirs} {
					o := m.options(i)
					o.sharedScroll = &m.scroll
					o.noScrollbar = i == 0
					ed := m.eds[i].build(c, b, o)
					ed.Grow(1).Basis(0).FillHeight()
					if i == 0 {
						ui.Box(c).Width(1).FillHeight().Background(border)
					}
				}
			})
		})
		ui.Box(c).Height(1).Shrink(0).FillWidth().Background(border)
		bottom := ui.Column(c).Grow(1 - split).Basis(0).Clip()
		bottom.Children(func() {
			versionBar(tx.MergeResult())
			box := ui.Box(c).Grow(1).Basis(0).Clip()
			box.Draw(func(p *ui.Painter, r ui.Rect) {
				ed := &m.eds[2]
				ed.layout(r.W, r.H, p.Scale())
				m.prepare()
				ed.layout(r.W, r.H, p.Scale())
				m.scrollToConflict(false)
				m.rebuildIfMoved()
			})
			box.Children(func() {
				ed := m.eds[2].build(c, m.Result, m.options(2)).Fill()
				if m.ResultHandle != nil {
					ed.Bind(m.ResultHandle)
				}
			})
		})
	})
	return root
}

// currentChoices returns how each conflict is resolved, by what the
// result holds.
func (m *MergeState) currentChoices() []MergeChoice {
	text := m.Result.String()
	out := make([]MergeChoice, len(m.conflicts))
	for i := range m.conflicts {
		out[i] = m.conflicts[i].choice(text)
	}
	return out
}

// scrollToConflict scrolls the versions (top) or the result to the
// conflict GoTo asked for, its bar a line below the top, once they are
// laid out; the result's last, as it goes after the versions.
func (m *MergeState) scrollToConflict(top bool) {
	i := m.goTo
	if i < 0 || i >= len(m.conflicts) {
		return
	}
	if top {
		ed := &m.eds[0]
		g := ed.view.heights.geometry(min(m.conflicts[i].inOurs[0], max(m.Ours.LineCount()-1, 0)))
		m.scroll.set(g.top - float64(ed.view.lineHeight))
		return
	}
	ed := &m.eds[2]
	from, _ := (&mergeSide{m: m, v: 2}).lines(i)
	g := ed.view.heights.geometry(min(from, max(m.Result.LineCount()-1, 0)))
	ed.setScroll(g.top - float64(ed.view.lineHeight))
	m.goTo = -1
}

// geom is what the bars' controls are built at.
func (m *MergeState) geom() mergeGeom {
	var g mergeGeom
	for i := range m.eds {
		g.totals[i], g.scrolls[i] = m.eds[i].view.heights.total(), m.eds[i].scrollY
	}
	return g
}

// rebuildIfMoved asks for the view to be built again when laying it out
// moved what the bars' controls were built for.
func (m *MergeState) rebuildIfMoved() {
	if g := m.geom(); g != m.built {
		m.built = g
		m.eds[2].services.Invalidate()
	}
}
