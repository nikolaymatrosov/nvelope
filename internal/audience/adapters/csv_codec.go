package adapters

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// ImportRow is one decoded CSV row destined for a subscriber upsert.
type ImportRow struct {
	LineNum    int
	Email      string
	Name       string
	State      string
	Attributes map[string]any
}

// DecodeUpload decodes an uploaded import file. A .zip upload is unwrapped to
// its single contained CSV before decoding; any other upload is treated as a
// CSV. A header row is required.
func DecodeUpload(fileName string, data []byte) ([]ImportRow, error) {
	if strings.HasSuffix(strings.ToLower(fileName), ".zip") {
		csvBytes, err := unwrapZip(data)
		if err != nil {
			return nil, err
		}
		data = csvBytes
	}
	return decodeCSV(data)
}

// unwrapZip returns the bytes of the single CSV inside a ZIP archive.
func unwrapZip(data []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("reading zip upload: %w", err)
	}
	for _, f := range zr.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".csv") {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("opening zipped csv: %w", err)
			}
			defer func() { _ = rc.Close() }()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("zip upload contains no .csv file")
}

// decodeCSV parses CSV bytes into import rows. The first row is the header;
// reserved header names map to subscriber fields, all others to attributes.
func decodeCSV(data []byte) ([]ImportRow, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("reading csv header: %w", err)
	}
	for i := range header {
		header[i] = strings.TrimSpace(strings.ToLower(header[i]))
	}

	var rows []ImportRow
	line := 1
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading csv row: %w", err)
		}
		line++
		row := ImportRow{LineNum: line, Attributes: map[string]any{}}
		for i, col := range header {
			if i >= len(record) {
				continue
			}
			value := strings.TrimSpace(record[i])
			switch col {
			case "email":
				row.Email = value
			case "name":
				row.Name = value
			case "state":
				row.State = value
			default:
				if col != "" && value != "" {
					row.Attributes[col] = value
				}
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// EncodeCSV encodes subscribers as a CSV with a header row. The reserved
// columns come first; every custom-attribute key seen across the set follows,
// in a stable order.
func EncodeCSV(subscribers []*domain.Subscriber) ([]byte, error) {
	attrKeys := collectAttrKeys(subscribers)
	header := append([]string{"email", "name", "state"}, attrKeys...)

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(header); err != nil {
		return nil, fmt.Errorf("writing csv header: %w", err)
	}
	for _, s := range subscribers {
		record := []string{s.Email(), s.Name(), string(s.State())}
		for _, k := range attrKeys {
			if v, ok := s.Attributes().Get(k); ok {
				record = append(record, fmt.Sprintf("%v", v))
			} else {
				record = append(record, "")
			}
		}
		if err := w.Write(record); err != nil {
			return nil, fmt.Errorf("writing csv row: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("flushing csv: %w", err)
	}
	return buf.Bytes(), nil
}

// collectAttrKeys returns every custom-attribute key across the subscriber
// set, in sorted order for a stable CSV layout.
func collectAttrKeys(subscribers []*domain.Subscriber) []string {
	seen := map[string]bool{}
	for _, s := range subscribers {
		for k := range s.Attributes().Values() {
			seen[k] = true
		}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
