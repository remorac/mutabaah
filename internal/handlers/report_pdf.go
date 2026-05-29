package handlers

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jung-kurt/gofpdf"
	"github.com/remorac/mutabaah/internal/services"
)

const (
	pdfPageWidth  = 595.28
	pdfPageHeight = 841.89
	pdfMargin     = 42.0
)

type simplePDF struct {
	pdf *gofpdf.Fpdf
}

func newSimplePDF() (*simplePDF, error) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetCompression(false)

	regular, err := readReportPDFFont("Manrope-Regular.ttf")
	if err != nil {
		return nil, err
	}
	bold, err := readReportPDFFont("Manrope-Bold.ttf")
	if err != nil {
		return nil, err
	}
	pdf.AddUTF8FontFromBytes("Manrope", "", regular)
	pdf.AddUTF8FontFromBytes("Manrope", "B", bold)
	if err := pdf.Error(); err != nil {
		return nil, err
	}

	p := &simplePDF{pdf: pdf}
	p.newPage()
	return p, nil
}

func readReportPDFFont(name string) ([]byte, error) {
	for _, dir := range []string{
		filepath.Join("web", "static", "fonts"),
		filepath.Join("..", "..", "web", "static", "fonts"),
	} {
		path := filepath.Join(dir, name)
		b, err := os.ReadFile(path)
		if err == nil {
			return b, nil
		}
	}
	return nil, fmt.Errorf("read report PDF font %s: file not found", name)
}

func (p *simplePDF) newPage() {
	p.pdf.AddPage()
}

func (p *simplePDF) finish() ([]byte, error) {
	var out bytes.Buffer
	if err := p.pdf.Output(&out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (p *simplePDF) y(y float64) float64 {
	return pdfPageHeight - y
}

func (p *simplePDF) setTextColor(r, g, b float64) {
	p.pdf.SetTextColor(pdfColor(r), pdfColor(g), pdfColor(b))
}

func (p *simplePDF) setDrawColor(r, g, b float64) {
	p.pdf.SetDrawColor(pdfColor(r), pdfColor(g), pdfColor(b))
}

func (p *simplePDF) setFillColor(r, g, b float64) {
	p.pdf.SetFillColor(pdfColor(r), pdfColor(g), pdfColor(b))
}

func pdfColor(v float64) int {
	if v <= 1 {
		v *= 255
	}
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return int(v + 0.5)
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
	p.setTextColor(r, g, b)
	p.pdf.SetFont("Manrope", fontStyle(font), size)
	p.pdf.Text(x, p.y(y), value)
}

func (p *simplePDF) centeredText(x, y, width, size float64, value string) {
	p.centeredFontText("F1", x, y, width, size, value)
}

func (p *simplePDF) centeredBoldText(x, y, width, size float64, value string) {
	p.centeredFontText("F2", x, y, width, size, value)
}

func (p *simplePDF) centeredFontText(font string, x, y, width, size float64, value string) {
	value = sanitizePDFText(value)
	textWidth := p.textWidth(font, size, value)
	p.fontText(font, x+(width-textWidth)/2, y, size, value)
}

func (p *simplePDF) rightMutedText(x, y, width, size float64, value string) {
	value = sanitizePDFText(value)
	textWidth := p.textWidth("F1", size, value)
	p.colorFontText("F1", x+width-textWidth, y, size, value, 0.39, 0.45, 0.55)
}

func (p *simplePDF) textWidth(font string, size float64, value string) float64 {
	p.pdf.SetFont("Manrope", fontStyle(font), size)
	return p.pdf.GetStringWidth(sanitizePDFText(value))
}

func fontStyle(font string) string {
	if font == "F2" {
		return "B"
	}
	return ""
}

func (p *simplePDF) line(x1, y1, x2, y2 float64) {
	p.colorLine(x1, y1, x2, y2, 0.55, 0.60, 0.68)
}

func (p *simplePDF) colorLine(x1, y1, x2, y2 float64, r, g, b float64) {
	p.setDrawColor(r, g, b)
	p.pdf.Line(x1, p.y(y1), x2, p.y(y2))
}

func (p *simplePDF) mutedLine(x1, y1, x2, y2 float64) {
	p.colorLine(x1, y1, x2, y2, 0.86, 0.89, 0.93)
}

func (p *simplePDF) circle(x, y, radius float64, r, g, b float64) {
	p.setDrawColor(r, g, b)
	p.pdf.Circle(x, p.y(y), radius, "D")
}

func (p *simplePDF) rect(x, y, w, h float64, r, g, b float64) {
	if h < 0 {
		y += h
		h = -h
	}
	p.setFillColor(r, g, b)
	p.pdf.Rect(x, p.y(y+h), w, h, "F")
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
	p.setFillColor(r, g, b)
	p.pdf.RoundedRect(x, p.y(y+h), w, h, radius, "1234", "F")
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
	p.setFillColor(r, g, b)
	p.pdf.RoundedRectExt(x, p.y(y+h), w, h, radius, radius, 0, 0, "F")
}

func (p *simplePDF) strokeRect(x, y, w, h float64) {
	p.setDrawColor(0.82, 0.85, 0.89)
	p.pdf.Rect(x, p.y(y+h), w, h, "D")
}

func buildReportPDF(report reportData) ([]byte, error) {
	p, err := newSimplePDF()
	if err != nil {
		return nil, err
	}
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

	return p.finish()
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
	chipWidth := 60.0
	chipGap := 6.0
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
	nameWidth := p.textWidth("F1", 7, name)
	gapWidth := 5.5
	valueWidth := p.textWidth("F2", 7, value)
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
		headerLabelY := y + 12
		headerDateY := y + 1
		p.topRoundedRect(pdfMargin, y-4, pdfPageWidth-pdfMargin*2, rowHeight, 5, 0.78, 0.86, 0.96)
		p.centeredBoldText(pdfMargin, y+6, taskWidth, 9, "TASK")
		for i, day := range days {
			x := pdfMargin + taskWidth + float64(i)*dayWidth
			p.centeredBoldText(x, headerLabelY, dayWidth, 7, strings.ToUpper(day.Label))
			p.centeredText(x, headerDateY, dayWidth, 8, strings.ToUpper(day.ShortDate))
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
