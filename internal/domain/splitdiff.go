package domain

// SplitCell is one side of an aligned row. Filled is false when the side has no line for the row.
type SplitCell struct {
	Text   string
	Number int
	Kind   DiffLineKind
	Filled bool
}

// SplitRow aligns one old line with one new line.
type SplitRow struct {
	Old SplitCell
	New SplitCell
}

// AlignedHunk is one hunk with its rows already aligned side by side.
type AlignedHunk struct {
	Hunk DiffHunk
	Rows []SplitRow
}

// AlignDiff aligns every hunk of d: a context line goes on both sides and each run of removed and
// added lines is paired by index, so the two columns read in parallel. The pairing is positional,
// not textual, which keeps it deterministic and cheap.
func AlignDiff(d Diff) []AlignedHunk {
	aligned := make([]AlignedHunk, 0, len(d.Hunks))

	for _, hunk := range d.Hunks {
		aligned = append(aligned, AlignedHunk{Hunk: hunk, Rows: alignHunk(hunk)})
	}

	return aligned
}

func alignHunk(hunk DiffHunk) []SplitRow {
	rows := make([]SplitRow, 0, len(hunk.Lines))

	for index := 0; index < len(hunk.Lines); {
		if hunk.Lines[index].Kind == Context {
			rows = append(rows, contextRow(hunk.Lines[index]))
			index++

			continue
		}

		end := index
		for end < len(hunk.Lines) && hunk.Lines[end].Kind != Context {
			end++
		}

		rows = append(rows, changeRows(hunk.Lines[index:end])...)
		index = end
	}

	return rows
}

// contextRow shows the same line on both sides.
func contextRow(line DiffLine) SplitRow {
	cell := SplitCell{Text: line.Text, Number: line.OldLine, Kind: Context, Filled: true}

	return SplitRow{
		Old: cell,
		New: SplitCell{Text: line.Text, Number: line.NewLine, Kind: Context, Filled: true},
	}
}

// changeRows pairs the removed lines with the added ones by position; the longer list keeps its
// rows with the other side empty.
func changeRows(lines []DiffLine) []SplitRow {
	old, new := splitSides(lines)
	count := max(len(old), len(new))

	rows := make([]SplitRow, 0, count)
	for index := 0; index < count; index++ {
		row := SplitRow{}
		if index < len(old) {
			row.Old = cellOf(old[index])
		}
		if index < len(new) {
			row.New = cellOf(new[index])
		}

		rows = append(rows, row)
	}

	return rows
}

// splitSides separates a change block into its old and new lines; a "no newline" marker follows the
// side of the line it belongs to.
func splitSides(lines []DiffLine) ([]DiffLine, []DiffLine) {
	var old, new []DiffLine

	last := Context
	for _, line := range lines {
		switch line.Kind {
		case Removed:
			old = append(old, line)
			last = Removed
		case Added:
			new = append(new, line)
			last = Added
		default:
			switch last {
			case Removed:
				old = append(old, line)
			case Added:
				new = append(new, line)
			default:
				old = append(old, line)
				new = append(new, line)
			}
		}
	}

	return old, new
}

func cellOf(line DiffLine) SplitCell {
	cell := SplitCell{Text: line.Text, Kind: line.Kind, Filled: true}

	switch line.Kind {
	case Removed:
		cell.Number = line.OldLine
	case Added:
		cell.Number = line.NewLine
	default:
		cell.Number = max(line.OldLine, line.NewLine)
	}

	return cell
}
