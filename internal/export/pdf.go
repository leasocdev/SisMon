package export

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"sort"
	"strconv"

	"github.com/go-pdf/fpdf"

	"sismon/internal/stats"
)

//go:embed fonts/DejaVuSans.ttf fonts/DejaVuSans-Bold.ttf
var fonts embed.FS

const (
	pageW, pageH = 297.0, 210.0
	margin       = 10.0
	rowH         = 5.5
)

type rgb struct{ r, g, b int }

var stateColor = map[string]rgb{
	"up": {29, 117, 54}, "degraded": {154, 77, 0}, "down": {179, 38, 30}, "unknown": {90, 98, 108}, "error": {122, 61, 158},
}

var stateLabel = map[string]string{
	"up": "Funcțional", "degraded": "Degradat", "down": "Indisponibil", "unknown": "Fără date", "error": "Eroare monitor",
}

type col struct {
	title string
	w     float64
	align string
}

type pdfDoc struct{ *fpdf.Fpdf }

// fit truncates s with an ellipsis so it fits width w (mm).
func (p pdfDoc) fit(s string, w float64) string {
	if p.GetStringWidth(s) <= w-1.5 {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && p.GetStringWidth(string(r)+"…") > w-1.5 {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

func (p pdfDoc) need(h float64) bool { return p.GetY()+h > pageH-margin-8 }

func (p pdfDoc) heading(text string) {
	if p.need(rowH * 4) {
		p.AddPage()
	}
	p.Ln(3)
	p.SetFont("dejavu", "B", 11)
	p.CellFormat(0, 7, text, "", 1, "L", false, 0, "")
}

func (p pdfDoc) tableHeader(cols []col) {
	p.SetFont("dejavu", "B", 7.5)
	p.SetFillColor(221, 227, 234)
	for _, c := range cols {
		p.CellFormat(c.w, rowH+0.5, p.fit(c.title, c.w), "1", 0, c.align, true, 0, "")
	}
	p.Ln(-1)
	p.SetFont("dejavu", "", 7.5)
}

func (p pdfDoc) table(cols []col, rows [][]string, empty string) {
	p.tableHeader(cols)
	if len(rows) == 0 {
		p.CellFormat(0, rowH, empty, "1", 1, "L", false, 0, "")
		return
	}
	for _, r := range rows {
		if p.need(rowH) {
			p.AddPage()
			p.tableHeader(cols)
		}
		for i, c := range cols {
			v := ""
			if i < len(r) {
				v = r[i]
			}
			p.CellFormat(c.w, rowH, p.fit(v, c.w), "1", 0, c.align, false, 0, "")
		}
		p.Ln(-1)
	}
}

func pct(v *float64) string {
	if v == nil {
		return "—"
	}
	return comma(*v) + "%"
}

func dur(sec float64) string {
	m := int(sec/60 + 0.5)
	if m < 60 {
		return strconv.Itoa(m) + " min"
	}
	return fmt.Sprintf("%d h %02d min", m/60, m%60)
}

// StatsPDF writes an A4 landscape report.
func StatsPDF(w io.Writer, rep stats.Report, m Meta) error {
	f := fpdf.New("L", "mm", "A4", "")
	reg, _ := fonts.ReadFile("fonts/DejaVuSans.ttf")
	bold, _ := fonts.ReadFile("fonts/DejaVuSans-Bold.ttf")
	f.AddUTF8FontFromBytes("dejavu", "", reg)
	f.AddUTF8FontFromBytes("dejavu", "B", bold)
	f.SetMargins(margin, margin, margin)
	f.SetAutoPageBreak(false, margin)
	f.AliasNbPages("{nb}")
	f.SetTitle(m.Title, true)
	f.SetCreator(m.AppName, true)
	p := pdfDoc{f}
	f.SetFooterFunc(func() {
		f.SetY(pageH - margin - 4)
		f.SetFont("dejavu", "", 7)
		f.SetTextColor(90, 98, 108)
		f.CellFormat(0, 4, fmt.Sprintf("Pagina %d din {nb} · Ora: Europe/Chisinau · generat %s · %s", f.PageNo(), fmtTime(m.Generated), m.AppName), "", 0, "C", false, 0, "")
		f.SetTextColor(0, 0, 0)
	})
	f.AddPage()

	// title block
	x := margin
	if len(m.Logo) > 0 {
		opt := fpdf.ImageOptions{ImageType: "PNG", ReadDpi: false}
		f.RegisterImageOptionsReader("logo", opt, bytes.NewReader(m.Logo))
		if f.Ok() {
			info := f.GetImageInfo("logo")
			lw, lh := logoBox(info.Width(), info.Height())
			f.ImageOptions("logo", margin, margin, lw, lh, false, opt, 0, "")
			x = margin + lw + 4
		} else {
			f.ClearError() // an unreadable logo must not prevent the report
		}
	}
	f.SetXY(x, margin)
	f.SetFont("dejavu", "B", 15)
	f.CellFormat(0, 8, p.fit(m.Title, pageW-x-margin), "", 2, "L", false, 0, "")
	f.SetX(x)
	f.SetFont("dejavu", "", 9)
	f.CellFormat(0, 5, p.fit(m.AppName+" · Perioada: "+period(m)+" · Ora: Europe/Chisinau", pageW-x-margin), "", 2, "L", false, 0, "")
	if len(m.Filters) > 0 {
		s := ""
		for i, fl := range m.Filters {
			if i > 0 {
				s += " · "
			}
			s += fl.Label + ": " + fl.Value
		}
		f.SetX(x)
		f.CellFormat(0, 5, p.fit(s, pageW-x-margin), "", 2, "L", false, 0, "")
	}
	f.SetY(margin + 22)

	// devices
	p.heading("Echipamente")
	devCols := []col{{"Locație", 52, "L"}, {"Echipament", 52, "L"}, {"Host", 32, "L"}, {"Tip", 24, "L"}, {"Primar", 13, "C"},
		{"Disponibilitate", 22, "R"}, {"Acoperire", 18, "R"}, {"Latență", 16, "R"}, {"Pierderi", 15, "R"}, {"Incidente", 15, "R"}, {"MTTR", 18, "R"}}
	var rows [][]string
	for _, d := range rep.Devices {
		name := d.Name
		if d.Archived {
			name += " (arhivat)"
		}
		lat, loss, mt := "—", "—", "—"
		if d.LatencyMs != nil {
			lat = comma(*d.LatencyMs) + " ms"
		}
		if d.LossPct != nil {
			loss = comma(*d.LossPct) + "%"
		}
		if d.MTTRS != nil {
			mt = dur(*d.MTTRS)
		}
		rows = append(rows, []string{d.Location, name, d.Host, d.Kind, yesNo(d.Primary), pct(d.Availability), pct(d.Coverage), lat, loss, strconv.Itoa(d.Incidents), mt})
	}
	p.table(devCols, rows, "Nu există echipamente în perioada selectată.")

	// location timelines: each state where it happened in the period
	p.heading("Locații — cronologie în perioadă")
	f.SetFont("dejavu", "", 7.5)
	legendX := margin
	for _, k := range []string{"up", "degraded", "down", "unknown", "error"} {
		c := stateColor[k]
		f.SetFillColor(c.r, c.g, c.b)
		f.Rect(legendX, f.GetY()+1, 3, 3, "F")
		f.SetX(legendX + 4)
		f.CellFormat(30, 5, stateLabel[k], "", 0, "L", false, 0, "")
		legendX += 34
	}
	f.Ln(7)
	if len(rep.Locations) == 0 {
		f.CellFormat(0, rowH, "Nu există locații în perioada selectată.", "", 1, "L", false, 0, "")
	}
	const barX, barW = margin + 72, 160.0
	for _, l := range rep.Locations {
		if p.need(rowH + 1) {
			f.AddPage()
		}
		yy := f.GetY()
		name := l.Name
		if l.Archived {
			name += " (arhivat)"
		}
		f.CellFormat(70, rowH, p.fit(name, 70), "", 0, "L", false, 0, "")
		f.SetDrawColor(150, 150, 150)
		f.Rect(barX, yy+1, barW, rowH-2, "D")
		f.SetDrawColor(0, 0, 0)
		if span := float64(rep.To - rep.From); span > 0 {
			for _, s := range l.Segments {
				a := barX + barW*float64(max(s.Start, rep.From)-rep.From)/span
				b := barX + barW*float64(min(s.End, rep.To)-rep.From)/span
				if b <= a {
					continue
				}
				if s.State == "down" || s.State == "degraded" {
					b = max(b, a+0.4) // short outages stay visible
				}
				c := stateColor[string(s.State)]
				f.SetFillColor(c.r, c.g, c.b)
				f.Rect(a, yy+1, b-a, rowH-2, "F")
			}
		}
		f.SetXY(barX+barW+3, yy)
		f.CellFormat(0, rowH, p.fit(fmt.Sprintf("%s · %d căderi · %s", pct(l.Availability), l.Outages, dur(l.Durations.Down)), pageW-margin-barX-barW-3), "", 1, "L", false, 0, "")
	}
	if len(rep.Locations) > 0 {
		f.SetFont("dejavu", "", 7)
		f.SetX(barX)
		f.CellFormat(barW/2, 4, fmtMs(rep.From), "", 0, "L", false, 0, "")
		f.CellFormat(barW/2, 4, fmtMs(rep.To), "", 1, "R", false, 0, "")
	}

	// outage periods
	p.heading("Perioade de indisponibilitate")
	outCols := []col{{"Locație", 62, "L"}, {"Stare", 24, "L"}, {"Început", 30, "L"}, {"Sfârșit", 30, "L"}, {"Durată", 25, "R"}, {"Echipamente căzute", 106, "L"}}
	rows = rows[:0]
	shown := rep.Outages
	if len(shown) > MaxPDFIncidents {
		shown = shown[:MaxPDFIncidents]
	}
	for _, o := range shown {
		rows = append(rows, []string{o.Location, outageState(string(o.State)), fmtMs(o.Start), outageEnd(o, "în curs"), dur(o.DurationS), outageDevices(o)})
	}
	p.table(outCols, rows, "Nicio perioadă de indisponibilitate în perioada selectată.")
	if n := len(rep.Outages) - len(shown); n > 0 {
		f.SetFont("dejavu", "", 8)
		f.CellFormat(0, 6, fmt.Sprintf("… și încă %d perioade mai vechi — lista completă se află în exportul CSV sau Excel.", n), "", 1, "L", false, 0, "")
	}

	// incidents
	p.heading("Incidente")
	incCols := []col{{"Locație", 75, "L"}, {"Echipament", 70, "L"}, {"Început", 30, "L"}, {"Sfârșit", 30, "L"}, {"Durată", 25, "R"}, {"Stare", 47, "L"}}
	rows = rows[:0]
	incidents, omitted := pdfIncidents(rep.Incidents)
	for _, i := range incidents {
		end := "—"
		if i.EndedAt != nil {
			end = fmtMs(*i.EndedAt)
		}
		rows = append(rows, []string{i.Location, i.Device, fmtMs(i.ObservedAt), end, dur(i.DurationS), i.ReasonText})
	}
	p.table(incCols, rows, "Niciun incident în perioada selectată.")
	if omitted > 0 {
		f.SetFont("dejavu", "", 8)
		f.CellFormat(0, 6, fmt.Sprintf("… și încă %d incidente mai vechi — lista completă se află în exportul CSV sau Excel.", omitted), "", 1, "L", false, 0, "")
	}

	if err := f.Error(); err != nil {
		return err
	}
	return f.Output(w)
}

// MaxPDFIncidents bounds the incident table of the PDF (the full list is in
// the CSV/XLSX exports).
const MaxPDFIncidents = 2000

// pdfIncidents keeps the most recent MaxPDFIncidents incidents, in their order.
func pdfIncidents(rows []stats.IncidentRow) ([]stats.IncidentRow, int) {
	if len(rows) <= MaxPDFIncidents {
		return rows, 0
	}
	idx := make([]int, len(rows))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return rows[idx[a]].ObservedAt > rows[idx[b]].ObservedAt })
	keep := map[int]bool{}
	for _, i := range idx[:MaxPDFIncidents] {
		keep[i] = true
	}
	out := make([]stats.IncidentRow, 0, MaxPDFIncidents)
	for i, r := range rows {
		if keep[i] {
			out = append(out, r)
		}
	}
	return out, len(rows) - MaxPDFIncidents
}

// logoBox scales an image of wPx×hPx to fit 40×14 mm, keeping its proportions.
func logoBox(wPx, hPx float64) (float64, float64) {
	const maxW, maxH = 40.0, 14.0
	if wPx <= 0 || hPx <= 0 {
		return maxH, maxH
	}
	s := min(maxW/wPx, maxH/hPx)
	return wPx * s, hPx * s
}
