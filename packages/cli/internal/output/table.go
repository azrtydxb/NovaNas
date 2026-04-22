package output

import (
	"io"

	"github.com/olekukonko/tablewriter"
)

// Table writes a bordered text table with the given headers and rows.
func Table(w io.Writer, headers []string, rows [][]string) {
	t := tablewriter.NewWriter(w)
	t.SetHeader(headers)
	t.SetAutoWrapText(false)
	t.SetBorder(false)
	t.SetHeaderLine(true)
	t.SetCenterSeparator(" ")
	t.SetColumnSeparator(" ")
	t.SetRowSeparator(" ")
	t.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	t.SetAlignment(tablewriter.ALIGN_LEFT)
	for _, r := range rows {
		t.Append(r)
	}
	t.Render()
}
