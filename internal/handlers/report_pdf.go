package handlers

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/remorac/mutabaah/internal/services"
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

	objectCount := 4 + len(p.pages)*2
	offsets := make([]int, objectCount+1)
	writeObj := func(id int, body string) {
		offsets[id] = out.Len()
		_, _ = fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", id, body)
	}

	kids := make([]string, 0, len(p.pages))
	for i := range p.pages {
		kids = append(kids, fmt.Sprintf("%d 0 R", 5+i*2))
	}

	writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObj(2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(p.pages)))
	writeObj(3, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	writeObj(4, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>")

	for i, stream := range p.pages {
		pageID := 5 + i*2
		contentID := pageID + 1
		writeObj(pageID, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] /Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> /Contents %d 0 R >>", pdfPageWidth, pdfPageHeight, contentID))
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
	p.fontText("F1", x, y, size, value)
}

func (p *simplePDF) mutedText(x, y, size float64, value string) {
	p.colorFontText("F1", x, y, size, value, 0.39, 0.45, 0.55)
}

func (p *simplePDF) boldText(x, y, size float64, value string) {
	p.fontText("F2", x, y, size, value)
}

func (p *simplePDF) fontText(font string, x, y, size float64, value string) {
	p.colorFontText(font, x, y, size, value, 0.09, 0.12, 0.16)
}

func (p *simplePDF) colorFontText(font string, x, y, size float64, value string, r, g, b float64) {
	value = sanitizePDFText(value)
	_, _ = fmt.Fprintf(&p.buf, "%.3f %.3f %.3f rg BT /%s %.2f Tf %.2f %.2f Td (%s) Tj ET\n", r, g, b, font, size, x, y, escapePDFString(value))
}

func (p *simplePDF) centeredText(x, y, width, size float64, value string) {
	p.centeredFontText("F1", x, y, width, size, value)
}

func (p *simplePDF) centeredBoldText(x, y, width, size float64, value string) {
	p.centeredFontText("F2", x, y, width, size, value)
}

func (p *simplePDF) centeredFontText(font string, x, y, width, size float64, value string) {
	value = sanitizePDFText(value)
	textWidth := float64(len(value)) * size * 0.48
	p.fontText(font, x+(width-textWidth)/2, y, size, value)
}

func (p *simplePDF) rightMutedText(x, y, width, size float64, value string) {
	value = sanitizePDFText(value)
	textWidth := float64(len(value)) * size * 0.48
	p.colorFontText("F1", x+width-textWidth, y, size, value, 0.39, 0.45, 0.55)
}

func (p *simplePDF) line(x1, y1, x2, y2 float64) {
	_, _ = fmt.Fprintf(&p.buf, "0.55 0.60 0.68 RG %.2f %.2f m %.2f %.2f l S\n", x1, y1, x2, y2)
}

func (p *simplePDF) colorLine(x1, y1, x2, y2 float64, r, g, b float64) {
	_, _ = fmt.Fprintf(&p.buf, "%.3f %.3f %.3f RG %.2f %.2f m %.2f %.2f l S\n", r, g, b, x1, y1, x2, y2)
}

func (p *simplePDF) mutedLine(x1, y1, x2, y2 float64) {
	p.colorLine(x1, y1, x2, y2, 0.86, 0.89, 0.93)
}

func (p *simplePDF) circle(x, y, radius float64, r, g, b float64) {
	k := radius * 0.5522847498
	_, _ = fmt.Fprintf(
		&p.buf,
		"%.3f %.3f %.3f RG %.2f %.2f m %.2f %.2f %.2f %.2f %.2f %.2f c %.2f %.2f %.2f %.2f %.2f %.2f c %.2f %.2f %.2f %.2f %.2f %.2f c %.2f %.2f %.2f %.2f %.2f %.2f c S\n",
		r, g, b,
		x+radius, y,
		x+radius, y+k, x+k, y+radius, x, y+radius,
		x-k, y+radius, x-radius, y+k, x-radius, y,
		x-radius, y-k, x-k, y-radius, x, y-radius,
		x+k, y-radius, x+radius, y-k, x+radius, y,
	)
}

func (p *simplePDF) rect(x, y, w, h float64, r, g, b float64) {
	if h < 0 {
		y += h
		h = -h
	}
	_, _ = fmt.Fprintf(&p.buf, "%.3f %.3f %.3f rg %.2f %.2f %.2f %.2f re f\n", r, g, b, x, y, w, h)
}

func (p *simplePDF) roundedRect(x, y, w, h, radius float64, r, g, b float64) {
	if h < 0 {
		y += h
		h = -h
	}
	if radius > w/2 {
		radius = w / 2
	}
	if radius > h/2 {
		radius = h / 2
	}
	k := radius * 0.5522847498
	_, _ = fmt.Fprintf(
		&p.buf,
		"%.3f %.3f %.3f rg %.2f %.2f m %.2f %.2f l %.2f %.2f %.2f %.2f %.2f %.2f c %.2f %.2f l %.2f %.2f %.2f %.2f %.2f %.2f c %.2f %.2f l %.2f %.2f %.2f %.2f %.2f %.2f c %.2f %.2f l %.2f %.2f %.2f %.2f %.2f %.2f c f\n",
		r, g, b,
		x+radius, y,
		x+w-radius, y,
		x+w-radius+k, y, x+w, y+radius-k, x+w, y+radius,
		x+w, y+h-radius,
		x+w, y+h-radius+k, x+w-radius+k, y+h, x+w-radius, y+h,
		x+radius, y+h,
		x+radius-k, y+h, x, y+h-radius+k, x, y+h-radius,
		x, y+radius,
		x, y+radius-k, x+radius-k, y, x+radius, y,
	)
}

func (p *simplePDF) topRoundedRect(x, y, w, h, radius float64, r, g, b float64) {
	if h < 0 {
		y += h
		h = -h
	}
	if radius > w/2 {
		radius = w / 2
	}
	if radius > h {
		radius = h
	}
	k := radius * 0.5522847498
	_, _ = fmt.Fprintf(
		&p.buf,
		"%.3f %.3f %.3f rg %.2f %.2f m %.2f %.2f l %.2f %.2f %.2f %.2f %.2f %.2f c %.2f %.2f l %.2f %.2f %.2f %.2f %.2f %.2f c %.2f %.2f l h f\n",
		r, g, b,
		x, y,
		x, y+h-radius,
		x, y+h-radius+k, x+radius-k, y+h, x+radius, y+h,
		x+w-radius, y+h,
		x+w-radius+k, y+h, x+w, y+h-radius+k, x+w, y+h-radius,
		x+w, y,
	)
}

func (p *simplePDF) strokeRect(x, y, w, h float64) {
	_, _ = fmt.Fprintf(&p.buf, "0.82 0.85 0.89 RG %.2f %.2f %.2f %.2f re S\n", x, y, w, h)
}

func buildReportPDF(report reportData) ([]byte, error) {
	p := newSimplePDF()
	y := pdfPageHeight - pdfMargin

	y = drawReportPDFHeader(p, report, y)
	y -= 6

	if len(report.Bars) > 0 {
		y = drawReportPDFChart(p, report.Bars, y)
		y -= 14
	} else {
		p.text(pdfMargin, y, 11, "No task occurrences found for this report.")
		y -= 32
	}

	if y < pdfMargin+80 {
		p.newPage()
		y = pdfPageHeight - pdfMargin
	}
	drawReportPDFTableTitle(p, pdfMargin, y, pdfPageWidth-pdfMargin*2, report.WeekLabel, report.MonthLabel)
	y -= 34
	drawReportPDFMatrixTable(p, report.WeekLabel, report.MonthLabel, report.WeekDays, report.TaskRows, y)

	return p.finish(), nil
}

func drawReportPDFHeader(p *simplePDF, report reportData, y float64) float64 {
	left := pdfMargin
	width := pdfPageWidth - pdfMargin*2
	top := y + 8
	height := 82.0
	bottom := top - height

	p.roundedRect(left, bottom, width, height, 6, 0.95, 0.98, 1.00)
	p.topRoundedRect(left, top-18, width, 18, 6, 0.31, 0.48, 0.65)
	p.text(left+16, top-13, 10, "MUTABA'AH YAUMIYAH")
	headerName := report.SelectedUserName
	if headerName == "" {
		headerName = "Report"
	}
	p.text(left+16, top-46, 20, headerName)
	p.text(left+16, top-64, 11, report.WeekRangeLabel)

	chipY := top - 66
	chipWidth := 72.0
	chipGap := 8.0
	drawReportPDFChip(p, left+width-chipWidth*3-chipGap*2-16, chipY, chipWidth, "Completion", fmt.Sprintf("%d%%", report.TotalPct), 0.90, 0.95, 1.00)
	drawReportPDFChip(p, left+width-chipWidth*2-chipGap-16, chipY, chipWidth, "Done", fmt.Sprintf("%d", report.TotalDone), 0.88, 0.96, 0.88)
	drawReportPDFChip(p, left+width-chipWidth-16, chipY, chipWidth, "Due", fmt.Sprintf("%d", report.TotalDue), 1.00, 0.95, 0.82)

	return bottom - 10
}

func drawReportPDFChip(p *simplePDF, x, y, width float64, label, value string, r, g, b float64) {
	p.roundedRect(x, y, width, 36, 5, r, g, b)
	p.mutedText(x+8, y+23, 8, label)
	p.boldText(x+8, y+7, 13, value)
}

type reportPDFChartBar struct {
	Label     string
	SubLabel  string
	Completed int
	Total     int
	Percent   int
}

func drawReportPDFChart(p *simplePDF, bars []reportBarView, y float64) float64 {
	chartBars := aggregateReportPDFBars(bars)
	totalBars := len(chartBars)
	maxBars := min(len(chartBars), 24)
	chartBars = chartBars[:maxBars]

	axisLabelWidth := 26.0
	left := pdfMargin + axisLabelWidth
	bottom := y - 155
	width := pdfPageWidth - pdfMargin*2 - axisLabelWidth
	height := 150.0
	p.strokeRect(left, bottom, width, height)
	for tick := 0; tick <= 100; tick += 10 {
		tickY := bottom + height*float64(tick)/100.0
		if tick > 0 && tick < 100 {
			p.mutedLine(left, tickY, left+width, tickY)
		}
		p.text(pdfMargin, tickY-3, 7, fmt.Sprintf("%d", tick))
	}

	gap := 6.0
	barWidth := (width - gap*float64(maxBars+1)) / float64(maxBars)
	if barWidth < 4 {
		barWidth = 4
	}
	for i, bar := range chartBars {
		x := left + gap + float64(i)*(barWidth+gap)
		barHeight := height * float64(bar.Percent) / 100.0
		if barHeight > 0 {
			p.topRoundedRect(x, bottom, barWidth, barHeight, 3, 0.31, 0.48, 0.65)
		}
		if maxBars <= 14 || i%2 == 0 {
			drawReportPDFChartLabel(p, x+barWidth/2, bottom-13, bar)
		}
	}

	if totalBars > maxBars {
		p.text(left, bottom-37, 9, fmt.Sprintf("showing first %d of %d groups", maxBars, totalBars))
	}

	return bottom - 28
}

func drawReportPDFChartLabel(p *simplePDF, centerX, y float64, bar reportPDFChartBar) {
	name := truncatePDFText(bar.Label, 8)
	value := fmt.Sprintf("(%d%%)", bar.Percent)
	nameWidth := float64(len(name)) * 7 * 0.48
	gapWidth := 7 * 0.72
	valueWidth := float64(len(value)) * 7 * 0.48
	x := centerX - (nameWidth+gapWidth+valueWidth)/2
	p.text(x, y, 7, name)
	p.boldText(x+nameWidth+gapWidth, y, 7, value)
}

func drawReportPDFTableTitle(p *simplePDF, x, y, width float64, weekLabel, monthLabel string) {
	titleInset := 6.0
	p.text(x, y, 14, weekLabel)
	p.rightMutedText(x, y, width-titleInset, 14, monthLabel)
}

func drawReportPDFMatrixTable(p *simplePDF, weekLabel, monthLabel string, days []reportWeekDayView, rows []reportTaskRowView, y float64) {
	rowHeight := 30.0
	taskWidth := 158.0
	dayWidth := (pdfPageWidth - pdfMargin*2 - taskWidth) / 7.0

	drawHeader := func() {
		p.topRoundedRect(pdfMargin, y-4, pdfPageWidth-pdfMargin*2, rowHeight, 5, 0.78, 0.86, 0.96)
		p.centeredBoldText(pdfMargin, y+8, taskWidth, 9, "TASK")
		for i, day := range days {
			x := pdfMargin + taskWidth + float64(i)*dayWidth
			p.centeredText(x, y+15, dayWidth, 7, strings.ToUpper(day.Label))
			p.centeredBoldText(x, y+5, dayWidth, 8, strings.ToUpper(day.ShortDate))
		}
		y -= rowHeight
	}

	drawHeader()
	if len(rows) == 0 {
		p.text(pdfMargin, y+2, 9, "No task occurrences found for this week.")
		return
	}
	for rowIndex, row := range rows {
		if y < pdfMargin+rowHeight {
			p.newPage()
			y = pdfPageHeight - pdfMargin
			drawReportPDFTableTitle(p, pdfMargin, y, pdfPageWidth-pdfMargin*2, weekLabel, monthLabel)
			y -= 22
			drawHeader()
		}

		if rowIndex%2 == 0 {
			p.rect(pdfMargin, y-4, pdfPageWidth-pdfMargin*2, rowHeight, 0.96, 0.98, 1.00)
		}
		p.text(pdfMargin+4, y+14, 9, truncatePDFText(row.TaskName, 24))
		if row.Description != "" {
			p.mutedText(pdfMargin+4, y+3, 7, truncatePDFText(row.Description, 30))
		}
		for i, cell := range row.Cells {
			if !cell.Scheduled {
				continue
			}
			x := pdfMargin + taskWidth + float64(i)*dayWidth
			r, g, b := reportPDFCellBackground(cell.Status)
			p.roundedRect(x+3, y+3, dayWidth-6, rowHeight-12, 4, r, g, b)
			drawReportPDFStatusIcon(p, cell.Status, x+dayWidth/2, y+12)
		}
		p.line(pdfMargin, y-4, pdfPageWidth-pdfMargin, y-4)
		y -= rowHeight
	}
}

func reportPDFCellBackground(status string) (float64, float64, float64) {
	switch status {
	case string(services.StatusCompleted):
		return 0.82, 0.94, 0.84
	case string(services.StatusMissed):
		return 0.98, 0.82, 0.82
	case string(services.StatusExempt):
		return 0.98, 0.86, 0.94
	default:
		return 0.90, 0.92, 0.95
	}
}

func drawReportPDFStatusIcon(p *simplePDF, status string, cx, cy float64) {
	switch status {
	case string(services.StatusCompleted):
		p.colorLine(cx-3.8, cy-0.2, cx-1.2, cy-3.8, 0.12, 0.55, 0.22)
		p.colorLine(cx-1.2, cy-3.8, cx+3.8, cy+3.8, 0.12, 0.55, 0.22)
	case string(services.StatusMissed):
		p.colorLine(cx-3.8, cy-3.8, cx+3.8, cy+3.8, 0.72, 0.12, 0.12)
		p.colorLine(cx-3.8, cy+3.8, cx+3.8, cy-3.8, 0.72, 0.12, 0.12)
	case string(services.StatusExempt):
		p.circle(cx-1.1, cy, 3.9, 0.70, 0.18, 0.45)
		p.circle(cx+1.4, cy+0.8, 3.8, 0.98, 0.86, 0.94)
	default:
		p.circle(cx, cy, 3.8, 0.39, 0.45, 0.55)
	}
}

func aggregateReportPDFBars(bars []reportBarView) []reportPDFChartBar {
	indexes := map[string]int{}
	out := make([]reportPDFChartBar, 0)
	for _, bar := range bars {
		key := bar.Label + "\x00" + bar.SubLabel
		idx, ok := indexes[key]
		if !ok {
			idx = len(out)
			indexes[key] = idx
			out = append(out, reportPDFChartBar{Label: bar.Label, SubLabel: bar.SubLabel})
		}
		out[idx].Completed += bar.Completed
		out[idx].Total += bar.Total
		out[idx].Percent = percent(out[idx].Completed, out[idx].Total)
	}
	return out
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
