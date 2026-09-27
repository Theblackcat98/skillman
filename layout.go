package main

// Layout ownership. This file is the only place that turns a terminal
// size into frame geometry.
//
// It used not to be. paneW(), previewWidth() and mainBoxH() lived in
// model.go and view.go recomputed the same arithmetic on its own, so the
// drawing code and the sizing code could disagree — which is exactly how
// the off-by-one in audit B2 happened (review F3, A12). One owner means
// a change to the frame shape happens here once.

import "fmt"

// layout is the resolved geometry of one frame. It is computed from the
// model and read, never mutated.
type layout struct {
	width, height int
	narrow        bool
	showList      bool
	showPreview   bool

	// Outer box widths, and the content cells inside the borders.
	listW, prevW int
	listInner    int
	prevInner    int
	boxH         int // outer height of the main box
	bodyH        int // content lines inside the main box

	// Help overlay, sized from its own content so a short keymap is a
	// short box rather than a box full of padding.
	helpW     int
	helpH     int
	helpInner int
	helpBodyH int

	row rowPlan
}

// layout resolves the frame. The frame is header + box + status + footer,
// so the box takes the remaining rows exactly; the old height-4 left one
// row permanently unused (audit B16).
func (m Model) layout() layout {
	l := layout{width: m.width, height: m.height, narrow: m.width < narrowBreak}

	l.boxH = l.height - 3
	if l.boxH < 5 {
		l.boxH = 5
	}
	l.bodyH = contentH(l.boxH)

	if l.narrow {
		// One pane at a time, full width; tab toggles.
		l.listW, l.prevW = l.width, l.width
		l.showList, l.showPreview = !m.showPreviewOv, m.showPreviewOv
	} else {
		// joinH puts a single space between the panes, so the panes plus
		// the gap must add up to exactly the width for a flush frame.
		l.listW = (l.width - 1) / 2
		l.prevW = l.width - 1 - l.listW
		l.showList, l.showPreview = true, true
	}
	l.listInner = innerOf(l.listW)
	l.prevInner = innerOf(l.prevW)
	l.row = planRow(l.listInner)

	l.helpW = l.width - 4
	if l.helpW > 78 {
		l.helpW = 78
	}
	if l.helpW < 20 {
		l.helpW = 20
	}
	l.helpH = len(m.helpLines()) + 3
	if l.helpH > l.height-1 {
		l.helpH = l.height - 1
	}
	l.helpInner = innerOf(l.helpW)
	l.helpBodyH = contentH(l.helpH)
	return l
}

// narrowBreak is the width under which the two panes no longer both fit.
// Below it the frame shows one pane and tab switches.
const narrowBreak = 70

// previewWrap is the width the markdown renderer wraps at. It is the
// preview box interior, with a floor so a very narrow pane still renders
// readable prose.
//
// The floor is 10, not the old hardcoded 20. The narrow pane interior can
// be 18, so wrapping at 20 made box() cut two cells off every single
// preview line (review F3).
func (l layout) previewWrap() int {
	if l.prevInner < 10 {
		return 10
	}
	return l.prevInner
}

// contentH is how many body lines box() renders for a box of height h:
// the title line and the two borders are not content.
func contentH(h int) int {
	v := h - 3
	if v < 1 {
		v = 1
	}
	return v
}

// innerOf is the content width inside a box of outer width w. box()
// reserves one cell per side for its borders; layout is where that is
// decided, so drawing and sizing read the same number.
func innerOf(w int) int {
	if w < 4 {
		return 2
	}
	return w - 2
}

// rowPlan is the cell budget for one skill row, resolved once per frame
// so renderRow does no arithmetic of its own.
type rowPlan struct {
	inner    int
	marker   int
	name     int
	severity int
	category int
	// mark is true when severity has to shrink to a single character
	// because the name needs the cells more.
	mark bool
}

const (
	rowMarkerW  = 2  // "> " or "  "
	rowSepW     = 1  // gap between columns
	rowBadgeW   = 6  // "[warn]", "[err]", or "[ok]  "
	rowNameCap  = 22 // a longer name does not need the room
	rowNameMin  = 12 // below this the name stops identifying the skill
	rowCatCap   = 12
	rowCatMin   = 4
	rowMinNameW = 1
)

// planRow divides the list interior between the cursor marker, the name,
// the severity indicator and the category.
//
// Priority is marker, then name, then severity, then category. The old
// layout fixed the name column first and gave severity a flat 6 cells, so
// at 20 columns the name got 6 cells and every row read "skill-": four
// different skills looked identical (review E1). It also passed
// interior-4 to renderRow and never used the 4 cells, so the pane was
// 4 cells narrower than the frame it drew.
func planRow(inner int) rowPlan {
	r := rowPlan{inner: inner, marker: rowMarkerW, severity: rowBadgeW}
	if inner < 1 {
		return r
	}
	// Severity as a word costs 6 cells. When that leaves too little to
	// identify a skill, spend 1 cell on a mark instead and give the rest
	// to the name.
	if inner-rowMarkerW-rowBadgeW < rowNameMin {
		r.severity = 1
		r.mark = true
	}

	name := inner - r.marker - r.severity - rowSepW
	if name > rowNameCap {
		name = rowNameCap
	}
	if name < rowMinNameW {
		name = rowMinNameW
	}
	r.name = name

	// The category is the first thing to go, and only when the name is
	// comfortably legible and at least rowCatMin cells are spare.
	if name >= rowNameMin {
		spare := inner - r.marker - r.severity - rowSepW - name - rowSepW
		if spare > rowCatCap {
			spare = rowCatCap
		}
		if spare >= rowCatMin {
			r.category = spare
		}
	}
	return r
}

// severityMark is the one-cell severity indicator used when the row
// budget cannot afford a word. A clean skill shows nothing, so any mark
// in that position means a problem — which is the property that matters
// when the pane is 18 cells wide.
func severityMark(s Severity) string {
	switch s {
	case SevErr:
		return "x"
	case SevWarn:
		return "!"
	default:
		return " "
	}
}

// severityCells is what a row spends on severity: the mark or the badge.
func (r rowPlan) severityCells(s Severity) string {
	if r.mark {
		return severityMark(s)
	}
	return "[" + s.String() + "]"
}

// helpTitle is the overlay title, carrying the visible range once the
// keymap is taller than the box. Before this the help silently cut off
// at short heights with no way to see the rest (review E2).
func (l layout) helpTitle(vp viewportRange) string {
	if vp.total <= l.helpBodyH {
		return "help · ?"
	}
	end := vp.offset + l.helpBodyH
	if end > vp.total {
		end = vp.total
	}
	return fmt.Sprintf("help · ?  %d–%d/%d", vp.offset+1, end, vp.total)
}

// viewportRange is what the help title needs to know about a viewport.
type viewportRange struct{ offset, total int }
