package export

import (
	"encoding/csv"
	"io"
	"strconv"
	"strings"

	"sismon/internal/stats"
	"sismon/internal/store"
)

// Column titles shared by the CSV and XLSX statistics exports.
var (
	deviceHeader = []string{"Locație", "Echipament", "Host", "Tip", "Primar", "Disponibilitate (%)", "Acoperire (%)",
		"Funcțional (h)", "Degradat (h)", "Indisponibil (h)", "Fără date (h)", "Eroare monitor (h)", "Latență medie (ms)", "Pierderi (%)", "Incidente", "MTTR (min)"}
	locationHeader = []string{"Locație", "Disponibilitate (%)", "Acoperire (%)", "Funcțional (h)", "Degradat (h)", "Indisponibil (h)", "Fără date (h)", "Eroare monitor (h)"}
	incidentHeader = []string{"Locație", "Echipament", "Început", "Confirmat", "Sfârșit", "Durată (min)", "Stare"}
	locationsCols  = []string{"Locație", "Adresă", "Latitudine", "Longitudine", "Echipament", "Host", "Tip", "Primar", "Observații"}
)

func newCSV(w io.Writer) (*csv.Writer, error) {
	if _, err := io.WriteString(w, "\xEF\xBB\xBF"); err != nil {
		return nil, err
	}
	c := csv.NewWriter(w)
	c.Comma = ';'
	c.UseCRLF = true
	return c, nil
}

func durationCells(d stats.Durations) []string {
	return []string{comma(hours(d.Up)), comma(hours(d.Degraded)), comma(hours(d.Down)), comma(hours(d.Unknown)), comma(hours(d.Error))}
}

func minutes(v *float64) string {
	if v == nil {
		return ""
	}
	return comma(*v / 60)
}

func periodRows(m Meta) [][]string {
	rows := [][]string{{"Perioadă și filtre"}, {"Raport", Safe(m.Title)}, {"Perioada", period(m)}, {"Ora", "Europe/Chisinau"}}
	for _, f := range m.Filters {
		rows = append(rows, []string{Safe(f.Label), Safe(f.Value)})
	}
	return append(rows, []string{"Generat", fmtTime(m.Generated)})
}

func deviceRow(d stats.DeviceRow) []string {
	name := d.Name
	if d.Archived {
		name += " (arhivat)"
	}
	row := []string{Safe(d.Location), Safe(name), Safe(d.Host), Safe(d.Kind), yesNo(d.Primary), num(d.Availability), num(d.Coverage)}
	row = append(row, durationCells(d.Durations)...)
	return append(row, num(d.LatencyMs), num(d.LossPct), strconv.Itoa(d.Incidents), minutes(d.MTTRS))
}

func locationRow(l stats.LocationRow) []string {
	name := l.Name
	if l.Archived {
		name += " (arhivat)"
	}
	return append([]string{Safe(name), num(l.Availability), num(l.Coverage)}, durationCells(l.Durations)...)
}

func incidentRow(i stats.IncidentRow) []string {
	end := ""
	if i.EndedAt != nil {
		end = fmtMs(*i.EndedAt)
	}
	return []string{Safe(i.Location), Safe(i.Device), fmtMs(i.ObservedAt), fmtMs(i.ConfirmedAt), end, comma(i.DurationS / 60), Safe(i.ReasonText)}
}

// StatsCSV writes the report as blocks separated by an empty row.
func StatsCSV(w io.Writer, rep stats.Report, m Meta) error {
	c, err := newCSV(w)
	if err != nil {
		return err
	}
	rows := periodRows(m)
	rows = append(rows, nil, []string{"Echipamente"}, deviceHeader)
	for _, d := range rep.Devices {
		rows = append(rows, deviceRow(d))
	}
	rows = append(rows, nil, []string{"Locații"}, locationHeader)
	for _, l := range rep.Locations {
		rows = append(rows, locationRow(l))
	}
	rows = append(rows, nil, []string{"Incidente"}, incidentHeader)
	for _, i := range rep.Incidents {
		rows = append(rows, incidentRow(i))
	}
	for _, r := range rows {
		if r == nil {
			r = []string{""}
		}
		if err := c.Write(r); err != nil {
			return err
		}
	}
	c.Flush()
	return c.Error()
}

func coord(v *float64, comma bool) string {
	if v == nil {
		return ""
	}
	s := strconv.FormatFloat(*v, 'f', -1, 64)
	if comma {
		// Excel with Romanian settings would read "29.66" as a date
		s = strings.Replace(s, ".", ",", 1)
	}
	return s
}

// locationRows are the rows of the import format (values unchanged).
func locationRows(locs []store.Location, decimalComma bool) [][]string {
	var rows [][]string
	for _, l := range locs {
		for _, d := range l.Devices {
			rows = append(rows, []string{l.Name, l.Address, coord(l.Lat, decimalComma), coord(l.Lon, decimalComma), d.Name, d.Host, d.Kind, yesNo(d.Primary), l.Notes})
		}
	}
	return rows
}

// LocationsCSV writes the locations in the import format, re-importable as is.
func LocationsCSV(w io.Writer, locs []store.Location) error {
	c, err := newCSV(w)
	if err != nil {
		return err
	}
	if err := c.Write(locationsCols); err != nil {
		return err
	}
	if err := c.WriteAll(locationRows(locs, true)); err != nil {
		return err
	}
	return c.Error()
}
