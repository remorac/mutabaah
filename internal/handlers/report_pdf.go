package handlers

import (
	"bytes"
	"fmt"
	"strings"
)

const (
	pdfPageWidth  = 595.28
	pdfPageHeight = 841.89
	pdfMargin     = 42.0
)

type simplePDF struct {
	pages []string
	buf   strings.Builder
}

func newSimplePDF() *simplePDF {
	p := &simplePDF{}
	p.newPage()
	return p
}

func (p *simplePDF) newPage() {
	if p.buf.Len() > 0 {
		p.pages = append(p.pages, p.buf.String())
		p.buf.Reset()
	}
}

func (p *simplePDF) finish() []byte {
	if p.buf.Len() > 0 || len(p.pages) == 0 {
		p.pages = append(p.pages, p.buf.String())
		p.buf.Reset()
	}

	var out bytes.Buffer
	_, _ = out.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")

	objectCount := 3 + len(p.pages)*2
	offsets := make([]int, objectCount+1)
	writeObj := func(id int, body string) {
		offsets[id] = out.Len()
		_, _ = fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", id, body)
	}

	kids := make([]string, 0, len(p.pages))
	for i := range p.pages {
		kids = append(kids, fmt.Sprintf("%d 0 R", 4+i*2))
	}

	writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObj(2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(p.pages)))
	writeObj(3, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")

	for i, stream := range p.pages {
		pageID := 4 + i*2
		contentID := pageID + 1
		writeObj(pageID, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>", pdfPageWidth, pdfPageHeight, contentID))
		writeObj(contentID, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	}

	xref := out.Len()
	_, _ = fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", objectCount+1)
	for i := 1; i <= objectCount; i++ {
		_, _ = fmt.Fprintf(&out, "%010d 00000 n \n", offsets[i])
	}
	_, _ = fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", objectCount+1, xref)
	return out.Bytes()
}

func (p *simplePDF) text(x, y, size float64, value string) {
	value = sanitizePDFText(value)
	_, _ = fmt.Fprintf(&p.buf, "0.09 0.12 0.16 rg BT /F1 %.2f Tf %.2f %.2f Td (%s) Tj ET\n", size, x, y, escapePDFString(value))
}

func (p *simplePDF) line(x1, y1, x2, y2 float64) {
	_, _ = fmt.Fprintf(&p.buf, "0.55 0.60 0.68 RG %.2f %.2f m %.2f %.2f l S\n", x1, y1, x2, y2)
}

func (p *simplePDF) rect(x, y, w, h float64, r, g, b float64) {
	if h < 0 {
		y += h
		h = -h
	}
	_, _ = fmt.Fprintf(&p.buf, "%.3f %.3f %.3f rg %.2f %.2f %.2f %.2f re f\n", r, g, b, x, y, w, h)
}

func (p *simplePDF) strokeRect(x, y, w, h float64) {
	_, _ = fmt.Fprintf(&p.buf, "0.82 0.85 0.89 RG %.2f %.2f %.2f %.2f re S\n", x, y, w, h)
}

func buildReportPDF(report reportData) ([]byte, error) {
	p := newSimplePDF()
	y := pdfPageHeight - pdfMargin

	p.text(pdfMargin, y, 18, "Mutaba'ah Report")
	y -= 22
	p.text(pdfMargin, y, 11, fmt.Sprintf("%s | Grouped by weeks", report.MonthLabel))
	y -= 25
	if report.SelectedUserName != "" {
		p.text(pdfMargin, y, 11, fmt.Sprintf("User: %s", report.SelectedUserName))
		y -= 22
	}
	p.text(pdfMargin, y, 11, fmt.Sprintf("Group by tasks: %s", reportGroupByTasksLabel(report.GroupByTasks)))
	y -= 22

	summary := fmt.Sprintf("Completion %d%%    Done %d    Due %d", report.TotalPct, report.TotalDone, report.TotalDue)
	p.text(pdfMargin, y, 12, summary)
	y -= 35

	if len(report.Bars) > 0 {
		y = drawReportPDFChart(p, report.Bars, y)
		y -= 26
	} else {
		p.text(pdfMargin, y, 11, "No task occurrences found for this report.")
		y -= 26
	}

	p.text(pdfMargin, y, 14, "Tabulation")
	y -= 20
	drawReportPDFTable(p, report.Bars, y)

	return p.finish(), nil
}

type reportPDFChartBar struct {
	Label     string
	SubLabel  string
	TaskName  string
	Completed int
	Missed    int
	Total     int
}

func drawReportPDFChart(p *simplePDF, bars []reportBarView, y float64) float64 {
	chartBars := aggregateReportPDFBars(bars)
	totalBars := len(chartBars)
	maxBars := min(len(chartBars), 24)
	chartBars = chartBars[:maxBars]

	left := pdfMargin
	bottom := y - 170
	width := pdfPageWidth - pdfMargin*2
	height := 150.0
	p.strokeRect(left, bottom, width, height)
	p.text(left, y+10, 14, "Chart")

	maxTotal := 1
	for _, bar := range chartBars {
		if bar.Total > maxTotal {
			maxTotal = bar.Total
		}
	}

	gap := 4.0
	barWidth := (width - gap*float64(maxBars+1)) / float64(maxBars)
	if barWidth < 4 {
		barWidth = 4
	}
	for i, bar := range chartBars {
		x := left + gap + float64(i)*(barWidth+gap)
		completedHeight := height * float64(bar.Completed) / float64(maxTotal)
		missedHeight := height * float64(bar.Missed) / float64(maxTotal)
		p.rect(x, bottom, barWidth, completedHeight, 0.31, 0.48, 0.65)
		p.rect(x, bottom+completedHeight, barWidth, missedHeight, 0.82, 0.84, 0.86)
		if maxBars <= 14 || i%2 == 0 {
			label := bar.Label
			if bar.TaskName != "" {
				label = label + "/" + bar.TaskName
			}
			p.text(x, bottom-13, 7, truncatePDFText(label, 12))
		}
	}

	p.rect(left, bottom-38, 8, 8, 0.31, 0.48, 0.65)
	p.text(left+13, bottom-37, 9, "completed")
	p.rect(left+82, bottom-38, 8, 8, 0.82, 0.84, 0.86)
	p.text(left+95, bottom-37, 9, "missed")
	if totalBars > maxBars {
		p.text(left+160, bottom-37, 9, fmt.Sprintf("showing first %d of %d groups", maxBars, totalBars))
	}

	return bottom - 48
}

func drawReportPDFTable(p *simplePDF, bars []reportBarView, y float64) {
	rowHeight := 18.0
	colX := []float64{pdfMargin, 104, 192, 282, 372, 418, 470, 522}
	headers := []string{"Label", "Detail", "Task", "User", "Done", "Missed", "Due", "%"}

	drawHeader := func() {
		p.rect(pdfMargin, y-4, pdfPageWidth-pdfMargin*2, rowHeight, 0.78, 0.86, 0.96)
		for i, header := range headers {
			p.text(colX[i], y+2, 9, header)
		}
		y -= rowHeight
	}

	drawHeader()
	for rowIndex, bar := range bars {
		if y < pdfMargin+rowHeight {
			p.newPage()
			y = pdfPageHeight - pdfMargin
			p.text(pdfMargin, y, 14, "Tabulation")
			y -= 22
			drawHeader()
		}

		if rowIndex%2 == 0 {
			p.rect(pdfMargin, y-4, pdfPageWidth-pdfMargin*2, rowHeight, 0.96, 0.98, 1.00)
		}
		missed := bar.Total - bar.Completed
		values := []string{
			truncatePDFText(bar.Label, 11),
			truncatePDFText(bar.SubLabel, 13),
			truncatePDFText(bar.TaskName, 13),
			truncatePDFText(bar.UserName, 13),
			fmt.Sprintf("%d", bar.Completed),
			fmt.Sprintf("%d", missed),
			fmt.Sprintf("%d", bar.Total),
			fmt.Sprintf("%d%%", bar.Percent),
		}
		for i, value := range values {
			p.text(colX[i], y+2, 9, value)
		}
		p.line(pdfMargin, y-4, pdfPageWidth-pdfMargin, y-4)
		y -= rowHeight
	}
}

func aggregateReportPDFBars(bars []reportBarView) []reportPDFChartBar {
	indexes := map[string]int{}
	out := make([]reportPDFChartBar, 0)
	for _, bar := range bars {
		key := bar.Label + "\x00" + bar.SubLabel + "\x00" + bar.TaskName
		idx, ok := indexes[key]
		if !ok {
			idx = len(out)
			indexes[key] = idx
			out = append(out, reportPDFChartBar{Label: bar.Label, SubLabel: bar.SubLabel, TaskName: bar.TaskName})
		}
		out[idx].Completed += bar.Completed
		out[idx].Missed += bar.Total - bar.Completed
		out[idx].Total += bar.Total
	}
	return out
}

func reportGroupByTasksLabel(groupByTasks bool) string {
	if groupByTasks {
		return "yes"
	}
	return "no"
}

func truncatePDFText(s string, limit int) string {
	s = sanitizePDFText(s)
	if len(s) <= limit {
		return s
	}
	if limit <= 1 {
		return s[:limit]
	}
	return s[:limit-1] + "."
}

func sanitizePDFText(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 32 && r <= 126 {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func escapePDFString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "(", `\(`)
	s = strings.ReplaceAll(s, ")", `\)`)
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
