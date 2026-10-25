package editor

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// A merge whose result git merged: theirs' D, which ours didn't change,
// merged in, and the line both changed a conflict.
func testMerge() *MergeState {
	return &MergeState{
		Ours:   NewBuffer("a\nours1\nc\nd\nx\n"),
		Theirs: NewBuffer("a\ntheirs1\nc\nD\nx\n"),
		Result: NewBuffer("a\n<<<<<<< HEAD\nours1\n=======\ntheirs1\n>>>>>>> feature\nc\nD\nx\n"),
	}
}

func TestMergeParse(t *testing.T) {
	m := testMerge()
	cs := m.Conflicts()
	if len(cs) != 1 {
		t.Fatalf("conflicts %+v", cs)
	}
	c := cs[0]
	if c.Choice != MergeUnresolved || c.Ours != "ours1\n" || c.Theirs != "theirs1\n" || c.HasBase || c.Start != 1 || c.End != 6 {
		t.Errorf("conflict %+v", c)
	}
	if ours, theirs := m.Labels(); ours != "HEAD" || theirs != "feature" {
		t.Errorf("labels %q %q", ours, theirs)
	}
	if m.Unresolved() != 1 {
		t.Errorf("unresolved %d", m.Unresolved())
	}

	// diff3's base, a longer marker, CRLF, and a block that never ends.
	m = &MergeState{Result: NewBuffer("<<<<<<<<< ours\r\no\r\n||||||||| base\r\nb\r\n=========\r\nt\r\n>>>>>>>>> theirs\r\n<<<<<<< x\r\nloose\r\n")}
	cs = m.Conflicts()
	if len(cs) != 1 || !cs[0].HasBase || cs[0].Base != "b\n" || cs[0].Ours != "o\n" || cs[0].Theirs != "t\n" {
		t.Errorf("diff3 conflicts %+v", cs)
	}
}

// The versions' lines of a conflict are found past what git merged
// without one.
func TestMergePlace(t *testing.T) {
	m := &MergeState{
		Ours:   NewBuffer("a\nours-only\nb\nours1\nc\n"),
		Theirs: NewBuffer("a\nb\ntheirs1\ntheirs2\nc\n"),
		Result: NewBuffer("a\nours-only\nb\n<<<<<<< HEAD\nours1\n=======\ntheirs1\ntheirs2\n>>>>>>> feature\nc\n"),
	}
	m.ensure()
	c := m.conflicts[0]
	if c.inOurs != [2]int{3, 4} || c.inTheirs != [2]int{2, 4} {
		t.Errorf("in ours %v, in theirs %v", c.inOurs, c.inTheirs)
	}
}

func TestMergeChoose(t *testing.T) {
	m := testMerge()
	text := func() string { return m.Result.String() }
	choice := func() MergeChoice { return m.Conflicts()[0].Choice }
	for _, step := range []struct {
		v    MergeVersion
		want MergeChoice
		text string
	}{
		{MergeOursVersion, MergeOurs, "a\nours1\nc\nD\nx\n"},
		{MergeTheirsVersion, MergeOursThenTheirs, "a\nours1\ntheirs1\nc\nD\nx\n"},
		{MergeOursVersion, MergeTheirs, "a\ntheirs1\nc\nD\nx\n"},
		{MergeOursVersion, MergeTheirsThenOurs, "a\ntheirs1\nours1\nc\nD\nx\n"},
		{MergeTheirsVersion, MergeOurs, "a\nours1\nc\nD\nx\n"},
		{MergeOursVersion, MergeNeither, "a\nc\nD\nx\n"},
	} {
		m.Toggle(0, step.v)
		if choice() != step.want || text() != step.text {
			t.Fatalf("toggling %d: %v %q, want %v %q", step.v, choice(), text(), step.want, step.text)
		}
	}
	// Taking a version into a conflict that holds neither puts it there.
	m.Toggle(0, MergeTheirsVersion)
	if choice() != MergeTheirs || text() != "a\ntheirs1\nc\nD\nx\n" {
		t.Fatalf("from neither: %v %q", choice(), text())
	}

	// Edits before the conflict move it; one in it is a hand's.
	m.Result.Edit(Edit{Start: 0, End: 0, Text: "new\n"})
	if c := m.Conflicts()[0]; c.Choice != MergeTheirs || c.Start != 2 || c.End != 3 {
		t.Fatalf("after an edit above: %+v", c)
	}
	at := strings.Index(text(), "theirs1") + len("theirs")
	m.Result.Edit(Edit{Start: at, End: at, Text: "!"})
	if c := m.Conflicts()[0]; c.Choice != MergeEdited {
		t.Fatalf("after an edit in it: %+v", c)
	}
	// Undo takes the edits back, and the choices with them.
	for m.Result.CanUndo() {
		m.Result.Undo()
	}
	if choice() != MergeUnresolved || m.Unresolved() != 1 {
		t.Fatalf("undone: %v %q", choice(), text())
	}

	// A text Reset is new: its markers are its conflicts.
	m.Result.Reset("<<<<<<< a\n1\n=======\n2\n>>>>>>> b\nmid\n<<<<<<< a\n3\n=======\n4\n>>>>>>> b\n")
	if cs := m.Conflicts(); len(cs) != 2 || cs[1].Ours != "3\n" || cs[1].Start != 6 {
		t.Fatalf("reset: %+v", cs)
	}
	m.ChooseAll(MergeTheirs)
	if text() != "2\nmid\n4\n" || m.Unresolved() != 0 {
		t.Fatalf("all theirs: %q", text())
	}
	// A conflict that is the whole file stays one when it is resolved.
	m.Result.Reset("<<<<<<< a\n1\n=======\n2\n>>>>>>> b\n")
	m.Choose(0, MergeOurs)
	if cs := m.Conflicts(); len(cs) != 1 || cs[0].Choice != MergeOurs || text() != "1\n" {
		t.Fatalf("whole file: %+v %q", cs, text())
	}
}

// The ranges of conflicts map through edits around, in and at them.
func TestMergeMapRange(t *testing.T) {
	for _, tc := range []struct {
		name               string
		start, end         int
		c                  change
		wantStart, wantEnd int
	}{
		{"before", 10, 20, change{oldStart: 2, oldEnd: 4, newStart: 2, newEnd: 7}, 13, 23},
		{"after", 10, 20, change{oldStart: 25, oldEnd: 25, newStart: 25, newEnd: 30}, 10, 20},
		{"put at the start", 10, 20, change{oldStart: 10, oldEnd: 10, newStart: 10, newEnd: 13}, 10, 23},
		{"put at the end", 10, 20, change{oldStart: 20, oldEnd: 20, newStart: 20, newEnd: 23}, 10, 20},
		{"replaced whole", 10, 20, change{oldStart: 10, oldEnd: 20, newStart: 10, newEnd: 12}, 10, 12},
		{"cut across the start", 10, 20, change{oldStart: 5, oldEnd: 15, newStart: 5, newEnd: 5}, 5, 10},
		{"put in an empty one", 10, 10, change{oldStart: 10, oldEnd: 10, newStart: 10, newEnd: 14}, 10, 14},
		{"deleted before, up to it", 10, 20, change{oldStart: 4, oldEnd: 10, newStart: 4, newEnd: 4}, 4, 14},
	} {
		s, e := mapRange(tc.start, tc.end, []change{tc.c})
		if s != tc.wantStart || e != tc.wantEnd {
			t.Errorf("%s: %d-%d, want %d-%d", tc.name, s, e, tc.wantStart, tc.wantEnd)
		}
	}
}

// The view shows the conflict on all three, and its checkboxes resolve it.
func TestMergeView(t *testing.T) {
	m := testMerge()
	m.OursLabel, m.TheirsLabel = "main", "feature"
	tt := ui.NewTester(func(c *ui.Context) { MergeView(c, m).Fill() }, 1000, 700)
	tt.Frame()
	tt.Frame()
	for _, want := range []string{"main", "feature", "Result", "Conflict 1 of 1", "Unresolved", "Use main's lines", "Use feature's lines"} {
		if !tt.HasText(want) {
			t.Errorf("no %q; texts %q", want, tt.Texts())
		}
	}
	if err := tt.Click("Use feature's lines"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if c := m.Conflicts()[0]; c.Choice != MergeTheirs || m.Result.String() != "a\ntheirs1\nc\nD\nx\n" {
		t.Fatalf("after the checkbox: %+v %q", c, m.Result.String())
	}
	tt.Frame()
	if !tt.HasText("Uses feature's lines") {
		t.Errorf("the result's bar doesn't say; texts %q", tt.Texts())
	}
	// The versions' conflict lines line up.
	top := func(v, line int) float64 { return m.eds[v].view.heights.geometry(line).top }
	if a, b := top(0, m.conflicts[0].inOurs[0]), top(1, m.conflicts[0].inTheirs[0]); a != b {
		t.Errorf("the conflict starts at %v in ours, %v in theirs", a, b)
	}
}
