package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Roblox .mesh to Wavefront OBJ. Input is untrusted: every count is checked against the real length.
// Versions 1 (text) to 4 (binary, skinning data skipped) are read; 5 and newer give a clear error.
// shortcut: v4 layout follows the public format notes, not yet checked against a real v4 file.

var errMesh = errors.New("file ini bukan mesh Roblox yang bisa dibaca")

var meshTriple = regexp.MustCompile(`\[([^\]]*)\]`)

type meshData struct {
	pos, nrm [][3]float32
	uv       [][2]float32
	faces    [][3]uint32
}

// MeshToOBJ converts the bytes of a Roblox mesh to OBJ text.
func MeshToOBJ(b []byte) ([]byte, error) {
	if !bytes.HasPrefix(b, []byte("version ")) {
		return nil, errMesh
	}
	nl := bytes.IndexByte(b, '\n')
	if nl < 0 {
		return nil, errMesh
	}
	ver := strings.TrimSpace(string(b[8:nl]))
	body := b[nl+1:]
	var m *meshData
	var err error
	switch {
	case strings.HasPrefix(ver, "1."):
		m, err = meshV1(ver, string(body))
	case strings.HasPrefix(ver, "2."), strings.HasPrefix(ver, "3."), strings.HasPrefix(ver, "4."):
		m, err = meshBinary(ver[0], body)
	default:
		return nil, fmt.Errorf("mesh versi %s belum didukung; simpan sebagai .mesh mentah", ver)
	}
	if err != nil {
		return nil, err
	}
	var o bytes.Buffer
	o.WriteString("# Rokit: dikonversi dari mesh Roblox\n")
	f := func(v float32) string { return strconv.FormatFloat(float64(v), 'g', 7, 32) }
	for _, p := range m.pos {
		fmt.Fprintf(&o, "v %s %s %s\n", f(p[0]), f(p[1]), f(p[2]))
	}
	for _, t := range m.uv {
		fmt.Fprintf(&o, "vt %s %s\n", f(t[0]), f(1-t[1]))
	}
	for _, n := range m.nrm {
		fmt.Fprintf(&o, "vn %s %s %s\n", f(n[0]), f(n[1]), f(n[2]))
	}
	for _, t := range m.faces {
		o.WriteString("f")
		for _, i := range t {
			fmt.Fprintf(&o, " %d/%d/%d", i+1, i+1, i+1)
		}
		o.WriteByte('\n')
	}
	return o.Bytes(), nil
}

// meshV1 reads the text format: a face count line, then three [x,y,z] groups per vertex (position, normal, uv).
func meshV1(ver, body string) (*meshData, error) {
	line, rest, _ := strings.Cut(body, "\n")
	faces, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || faces <= 0 || faces > 5_000_000 {
		return nil, errMesh
	}
	g := meshTriple.FindAllStringSubmatch(rest, -1)
	if len(g) != faces*9 {
		return nil, errMesh
	}
	scale := float32(1)
	if ver == "1.00" {
		scale = 0.5 // version 1.00 stores positions at double size
	}
	m := &meshData{}
	for i := 0; i < len(g); i += 3 {
		var v [3][3]float32
		for k := 0; k < 3; k++ {
			parts := strings.Split(g[i+k][1], ",")
			if len(parts) < 3 {
				return nil, errMesh
			}
			for j := 0; j < 3; j++ {
				x, err := strconv.ParseFloat(strings.TrimSpace(parts[j]), 32)
				if err != nil || math.IsNaN(x) || math.IsInf(x, 0) {
					return nil, errMesh
				}
				v[k][j] = float32(x)
			}
		}
		m.pos = append(m.pos, [3]float32{v[0][0] * scale, v[0][1] * scale, v[0][2] * scale})
		m.nrm = append(m.nrm, v[1])
		m.uv = append(m.uv, [2]float32{v[2][0], v[2][1]})
		if n := uint32(len(m.pos)); n%3 == 0 {
			m.faces = append(m.faces, [3]uint32{n - 3, n - 2, n - 1})
		}
	}
	return m, nil
}

// meshBinary reads versions 2, 3 and 4 (first LOD only; skinning data skipped).
func meshBinary(major byte, d []byte) (*meshData, error) {
	u16 := func(o int) int { return int(binary.LittleEndian.Uint16(d[o:])) }
	u32 := func(o int) int { return int(binary.LittleEndian.Uint32(d[o:])) }
	var hdr, vs, fs, nv, nf, nLod, bones int
	switch major {
	case '2':
		if len(d) < 12 {
			return nil, errMesh
		}
		hdr, vs, fs, nv, nf = u16(0), int(d[2]), int(d[3]), u32(4), u32(8)
	case '3':
		if len(d) < 16 {
			return nil, errMesh
		}
		hdr, vs, fs, nv, nf = u16(0), int(d[2]), int(d[3]), u32(8), u32(12)
		nLod = u16(6)
	default:
		if len(d) < 24 {
			return nil, errMesh
		}
		hdr, vs, fs, nv, nf, nLod, bones = u16(0), 40, 12, u32(4), u32(8), u16(12), u16(14)
	}
	if hdr < 12 || hdr > len(d) || vs < 32 || fs < 12 || nv <= 0 || nf <= 0 || nv > len(d)/vs || nf > len(d)/fs {
		return nil, errMesh
	}
	vEnd := hdr + nv*vs
	fStart := vEnd
	if bones > 0 {
		fStart += nv * 8
	}
	fEnd := fStart + nf*fs
	if fEnd > len(d) {
		return nil, errMesh
	}
	m := &meshData{}
	for i := 0; i < nv; i++ {
		o := hdr + i*vs
		fl := func(k int) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(d[o+4*k:])) }
		m.pos = append(m.pos, [3]float32{fl(0), fl(1), fl(2)})
		m.nrm = append(m.nrm, [3]float32{fl(3), fl(4), fl(5)})
		m.uv = append(m.uv, [2]float32{fl(6), fl(7)})
	}
	lo, hi := 0, nf
	if nLod >= 2 && fEnd+nLod*4 <= len(d) {
		a, b := u32(fEnd), u32(fEnd+4)
		if a <= b && b <= nf {
			lo, hi = a, b
		}
	}
	for i := lo; i < hi; i++ {
		o := fStart + i*fs
		t := [3]uint32{binary.LittleEndian.Uint32(d[o:]), binary.LittleEndian.Uint32(d[o+4:]), binary.LittleEndian.Uint32(d[o+8:])}
		if int(t[0]) >= nv || int(t[1]) >= nv || int(t[2]) >= nv {
			return nil, errMesh
		}
		m.faces = append(m.faces, t)
	}
	return m, nil
}
