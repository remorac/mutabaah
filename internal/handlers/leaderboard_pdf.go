package handlers

import "fmt"

func buildLeaderboardPDF(view leaderboardPageView) ([]byte, error) {
	p, err := newSimplePDF()
	if err != nil {
		return nil, err
	}

	y := pdfPageHeight - pdfMargin
	y = drawLeaderboardPDFHeader(p, view, y)
	y -= 40

	y = drawLeaderboardPDFRows(p, "Task Done", "Ranked by completed task count", view.PrimaryRows, y, leaderboardPDFCompletedColumns)
	y -= 26

	if y < pdfMargin+120 {
		p.newPage()
		y = pdfPageHeight - pdfMargin
	}
	drawLeaderboardPDFRows(p, "Best Streak", "Ranked by best fully completed day streak", view.StreakRows, y, leaderboardPDFStreakColumns)

	return p.finish()
}

func leaderboardPDFFilename(view leaderboardPageView) string {
	return fmt.Sprintf("leaderboard-%s-%s.pdf", view.Period, view.DateValue)
}

func drawLeaderboardPDFHeader(p *simplePDF, view leaderboardPageView, y float64) float64 {
	left := pdfMargin
	width := pdfPageWidth - pdfMargin*2
	top := y + 8
	height := 92.0
	bottom := top - height

	p.roundedRect(left, bottom, width, height, 6, 0.95, 0.98, 1.00)
	p.topRoundedRect(left, top-18, width, 18, 6, 0.31, 0.48, 0.65)
	p.text(left+16, top-13, 10, "MUTABA'AH YAUMIYAH")
	p.text(left+16, top-46, 20, "Leaderboard")
	p.text(left+16, top-65, 11, fmt.Sprintf("%s | %s", view.PeriodLabel, view.RangeLabel))

	return bottom - 10
}

type leaderboardPDFColumns int

const (
	leaderboardPDFCompletedColumns leaderboardPDFColumns = iota
	leaderboardPDFStreakColumns
)

func drawLeaderboardPDFRows(p *simplePDF, title, subtitle string, rows []leaderboardRowView, y float64, columns leaderboardPDFColumns) float64 {
	p.text(pdfMargin, y, 14, title)
	p.mutedText(pdfMargin, y-13, 9, subtitle)
	y -= 44

	drawHeader := func() {
		p.topRoundedRect(pdfMargin, y-4, pdfPageWidth-pdfMargin*2, 24, 5, 0.78, 0.86, 0.96)
		p.centeredBoldText(pdfMargin, y+4, 42, 8, "RANK")
		p.boldText(pdfMargin+52, y+4, 8, "USER")
		switch columns {
		case leaderboardPDFCompletedColumns:
			p.centeredBoldText(314, y+4, 58, 8, "DONE")
			p.centeredBoldText(380, y+4, 58, 8, "DUE")
			p.centeredBoldText(448, y+4, 64, 8, "COMPLETE")
		case leaderboardPDFStreakColumns:
			p.centeredBoldText(448, y+4, 76, 8, "STREAK")
		}
		y -= 24
	}

	drawHeader()
	if len(rows) == 0 {
		p.text(pdfMargin, y+2, 9, "No task occurrences found for this period.")
		return y - 26
	}

	for i, row := range rows {
		if y < pdfMargin+30 {
			p.newPage()
			y = pdfPageHeight - pdfMargin
			p.text(pdfMargin, y, 14, title)
			p.mutedText(pdfMargin, y-13, 9, subtitle)
			y -= 44
			drawHeader()
		}

		if row.IsCurrent {
			p.rect(pdfMargin, y-4, pdfPageWidth-pdfMargin*2, 28, 0.90, 0.95, 1.00)
		} else if i%2 == 0 {
			p.rect(pdfMargin, y-4, pdfPageWidth-pdfMargin*2, 28, 0.96, 0.98, 1.00)
		}

		p.centeredBoldText(pdfMargin, y+5, 42, 9, fmt.Sprintf("#%d", row.Rank))
		name := truncatePDFText(row.UserName, 32)
		p.text(pdfMargin+52, y+6, 10, name)

		switch columns {
		case leaderboardPDFCompletedColumns:
			p.centeredBoldText(314, y+5, 58, 10, fmt.Sprintf("%d", row.Completed))
			p.centeredText(380, y+5, 58, 10, fmt.Sprintf("%d", row.Due))
			p.centeredBoldText(448, y+5, 64, 10, fmt.Sprintf("%d%%", row.Percent))
		case leaderboardPDFStreakColumns:
			p.centeredBoldText(448, y+5, 76, 10, fmt.Sprintf("%d days", row.BestStreak))
		}

		p.line(pdfMargin, y-4, pdfPageWidth-pdfMargin, y-4)
		y -= 28
	}

	return y
}
