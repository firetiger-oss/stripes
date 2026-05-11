package table

import "github.com/firetiger-oss/stripes"

// detectNumericAlignment promotes left-aligned columns to right-aligned
// when their data cells all look numeric (per stripes.IsNumeric). Empty
// cells are skipped; a column needs at least one non-empty cell to be
// considered. Columns that are already right-aligned are left untouched.
//
// Used for slice-row tables, where every cell is statically a string (or
// erased through `any`) and content is the only signal available.
func detectNumericAlignment(rows [][]string, alignments []align) {
	for col := range alignments {
		if alignments[col] != alignLeft {
			continue
		}
		sawNumeric := false
		allNumeric := true
		for _, row := range rows {
			if col >= len(row) {
				continue
			}
			cell := row[col]
			if cell == "" {
				continue
			}
			if !stripes.IsNumeric(cell) {
				allNumeric = false
				break
			}
			sawNumeric = true
		}
		if sawNumeric && allNumeric {
			alignments[col] = alignRight
		}
	}
}
