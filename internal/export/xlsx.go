package export

import (
	"io"

	"github.com/xuri/excelize/v2"

	"sismon/internal/stats"
	"sismon/internal/store"
)

type sheet struct {
	f          *excelize.File
	name       string
	row        int
	head, nums int
}

func newSheet(f *excelize.File, name string, first bool) (*sheet, error) {
	if first {
		if err := f.SetSheetName(f.GetSheetName(0), name); err != nil {
			return nil, err
		}
	} else if _, err := f.NewSheet(name); err != nil {
		return nil, err
	}
	head, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Color: []string{"DDE3EA"}, Pattern: 1}})
	if err != nil {
		return nil, err
	}
	numFmt := "0.00"
	nums, err := f.NewStyle(&excelize.Style{CustomNumFmt: &numFmt})
	if err != nil {
		return nil, err
	}
	return &sheet{f: f, name: name, head: head, nums: nums}, nil
}

// put writes one row; float64 and *float64 cells are numeric, nil pointers empty.
func (s *sheet) put(cells ...any) error {
	s.row++
	for i, v := range cells {
		ref, _ := excelize.CoordinatesToCellName(i+1, s.row)
		switch x := v.(type) {
		case *float64:
			if x == nil {
				continue
			}
			v = *x
		case string:
			if err := s.f.SetCellStr(s.name, ref, x); err != nil {
				return err
			}
			continue
		}
		if err := s.f.SetCellValue(s.name, ref, v); err != nil {
			return err
		}
		if _, ok := v.(float64); ok {
			s.f.SetCellStyle(s.name, ref, ref, s.nums)
		}
	}
	return nil
}

func (s *sheet) header(cols []string) error {
	cells := make([]any, len(cols))
	for i, c := range cols {
		cells[i] = c
	}
	if err := s.put(cells...); err != nil {
		return err
	}
	end, _ := excelize.CoordinatesToCellName(len(cols), s.row)
	s.f.SetCellStyle(s.name, "A1", end, s.head)
	s.f.SetPanes(s.name, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
	last, _ := excelize.ColumnNumberToName(len(cols))
	s.f.SetColWidth(s.name, "A", last, 16)
	s.f.SetColWidth(s.name, "A", "B", 24)
	return nil
}

func durations(d stats.Durations) []any {
	return []any{hours(d.Up), hours(d.Degraded), hours(d.Down), hours(d.Unknown), hours(d.Error)}
}

func perMinute(v *float64) *float64 {
	if v == nil {
		return nil
	}
	m := *v / 60
	return &m
}

// StatsXLSX writes the four sheets required by the spec.
func StatsXLSX(w io.Writer, rep stats.Report, m Meta) error {
	f := excelize.NewFile()
	defer f.Close()

	dev, err := newSheet(f, "Echipamente", true)
	if err != nil {
		return err
	}
	dev.header(deviceHeader)
	for _, d := range rep.Devices {
		name := d.Name
		if d.Archived {
			name += " (arhivat)"
		}
		cells := []any{Safe(d.Location), Safe(name), Safe(d.Host), Safe(d.Kind), yesNo(d.Primary), d.Availability, d.Coverage}
		cells = append(cells, durations(d.Durations)...)
		cells = append(cells, d.LatencyMs, d.LossPct, float64(d.Incidents), perMinute(d.MTTRS))
		if err := dev.put(cells...); err != nil {
			return err
		}
	}

	loc, err := newSheet(f, "Locații", false)
	if err != nil {
		return err
	}
	loc.header(locationHeader)
	for _, l := range rep.Locations {
		name := l.Name
		if l.Archived {
			name += " (arhivat)"
		}
		if err := loc.put(append([]any{Safe(name), l.Availability, l.Coverage}, durations(l.Durations)...)...); err != nil {
			return err
		}
	}

	inc, err := newSheet(f, "Incidente", false)
	if err != nil {
		return err
	}
	inc.header(incidentHeader)
	for _, i := range rep.Incidents {
		end := ""
		if i.EndedAt != nil {
			end = fmtMs(*i.EndedAt)
		}
		if err := inc.put(Safe(i.Location), Safe(i.Device), fmtMs(i.ObservedAt), fmtMs(i.ConfirmedAt), end, i.DurationS/60, Safe(i.ReasonText)); err != nil {
			return err
		}
	}

	per, err := newSheet(f, "Perioadă și filtre", false)
	if err != nil {
		return err
	}
	for _, r := range periodRows(m)[1:] {
		cells := make([]any, len(r))
		for i, c := range r {
			cells[i] = c
		}
		per.put(cells...)
	}
	per.f.SetColWidth(per.name, "A", "B", 32)
	f.SetActiveSheet(0)
	_, err = f.WriteTo(w)
	return err
}

// LocationsXLSX writes the import format with every cell stored as text.
func LocationsXLSX(w io.Writer, locs []store.Location) error {
	f := excelize.NewFile()
	defer f.Close()
	s, err := newSheet(f, "Locații", true)
	if err != nil {
		return err
	}
	text, err := f.NewStyle(&excelize.Style{NumFmt: 49})
	if err != nil {
		return err
	}
	s.header(locationsCols)
	for _, r := range locationRows(locs, false) {
		cells := make([]any, len(r))
		for i, c := range r {
			cells[i] = c
		}
		if err := s.put(cells...); err != nil {
			return err
		}
		end, _ := excelize.CoordinatesToCellName(len(r), s.row)
		start, _ := excelize.CoordinatesToCellName(1, s.row)
		f.SetCellStyle(s.name, start, end, text)
	}
	_, err = f.WriteTo(w)
	return err
}
