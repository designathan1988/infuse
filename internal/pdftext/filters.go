package pdftext

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"encoding/ascii85"
	"errors"
	"fmt"
	"io"
)

// decode runs a stream through its filter chain.
func (d *Doc) decode(dict map[string]any, data []byte) ([]byte, error) {
	filters := d.chain(dict["Filter"])
	parms := d.chain(dict["DecodeParms"])
	var err error
	for i, f := range filters {
		n, _ := f.(name)
		fn, ok := filterTable[n]
		if !ok {
			return nil, fmt.Errorf("unsupported stream filter %s", string(n))
		}
		if data, err = fn(data); err != nil {
			return nil, err
		}
		if i < len(parms) {
			data = d.unpredict(data, parms[i])
		}
	}
	return data, nil
}

// chain normalizes a filter entry: absent, one value, or an array of values.
func (d *Doc) chain(v any) []any {
	switch r := d.resolve(v).(type) {
	case nil:
		return nil
	case []any:
		out := make([]any, 0, len(r))
		for _, e := range r {
			out = append(out, d.resolve(e))
		}
		return out
	default:
		return []any{r}
	}
}

var filterTable = map[name]func([]byte) ([]byte, error){
	"FlateDecode":     inflate,
	"Fl":              inflate,
	"ASCIIHexDecode":  asciiHex,
	"AHx":             asciiHex,
	"ASCII85Decode":   ascii85Decode,
	"A85":             ascii85Decode,
	"RunLengthDecode": runLength,
	"RL":              runLength,
}

// inflate reads a zlib stream, keeping what came out before any damage at the end of it: a truncated
// checksum loses no text.
func inflate(b []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(b))
	if err != nil {
		if len(b) < 2 {
			return nil, err
		}
		fr := flate.NewReader(bytes.NewReader(b[2:])) // a header some writers get wrong
		out, ferr := io.ReadAll(fr)
		if len(out) == 0 {
			return nil, errors.Join(err, ferr)
		}
		return out, nil
	}
	defer r.Close()
	out, err := io.ReadAll(r)
	if err != nil && len(out) == 0 {
		return nil, err
	}
	return out, nil
}

func asciiHex(b []byte) ([]byte, error) {
	l := lexer{b: append(append([]byte{'<'}, b...), '>')}
	return l.readHex(), nil
}

func ascii85Decode(b []byte) ([]byte, error) {
	if i := bytes.Index(b, []byte("~>")); i >= 0 {
		b = b[:i]
	}
	out := make([]byte, len(b)*4/5+8)
	n, _, err := ascii85.Decode(out, bytes.TrimPrefix(bytes.TrimSpace(b), []byte("<~")), true)
	return out[:n], err
}

func runLength(b []byte) ([]byte, error) {
	var out []byte
	for i := 0; i < len(b); {
		n := int(b[i])
		i++
		switch {
		case n == 128:
			return out, nil
		case n < 128:
			end := min(i+n+1, len(b))
			out = append(out, b[i:end]...)
			i = end
		case i < len(b):
			out = append(out, bytes.Repeat(b[i:i+1], 257-n)...)
			i++
		}
	}
	return out, nil
}

// unpredict reverses the PNG predictors object streams and cross-reference streams may use.
func (d *Doc) unpredict(data []byte, parm any) []byte {
	p := d.dict(parm)
	if p == nil || d.number(p["Predictor"]) < 10 {
		return data
	}
	columns := int(d.number(p["Columns"]))
	if columns <= 0 {
		columns = 1
	}
	row := columns + 1
	out := make([]byte, 0, len(data))
	prev := make([]byte, columns)
	for i := 0; i+row <= len(data); i += row {
		tag := data[i]
		cur := append([]byte{}, data[i+1:i+row]...)
		pngRow(tag, cur, prev)
		out = append(out, cur...)
		prev = cur
	}
	return out
}

// pngRow undoes one row's filter; these streams use one byte per sample.
func pngRow(tag byte, cur, prev []byte) {
	for j := range cur {
		var left, up, upLeft byte
		if j > 0 {
			left, upLeft = cur[j-1], prev[j-1]
		}
		up = prev[j]
		switch tag {
		case 1:
			cur[j] += left
		case 2:
			cur[j] += up
		case 3:
			cur[j] += byte((int(left) + int(up)) / 2)
		case 4:
			cur[j] += paeth(left, up, upLeft)
		}
	}
}

func paeth(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	pa, pb, pc := abs(p-int(a)), abs(p-int(b)), abs(p-int(c))
	if pa <= pb && pa <= pc {
		return a
	}
	if pb <= pc {
		return b
	}
	return c
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
