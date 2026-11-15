// Package pdf es un escritor mínimo de PDF 1.4: páginas A4, texto con las
// fuentes base Helvetica y Helvetica-Bold (codificación WinAnsi), líneas,
// rectángulos e imágenes. Cubre lo que necesita la representación impresa
// de un comprobante (RIDE) sin depender de bibliotecas externas.
//
// Las coordenadas se expresan en puntos (1/72 de pulgada) con el origen en
// la esquina superior izquierda de la página.
package pdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"
)

// Dimensiones de una página A4 en puntos.
const (
	Ancho = 595.28
	Alto  = 841.89
)

// Fuente identifica una de las fuentes disponibles.
type Fuente int

// Fuentes disponibles.
const (
	Normal Fuente = iota
	Negrita
)

// Documento acumula páginas y las serializa con Bytes.
type Documento struct {
	// Titulo y Autor son metadatos opcionales del PDF.
	Titulo, Autor string

	paginas  []*bytes.Buffer
	imagenes []imagen
}

type imagen struct {
	ancho, alto int
	datos       []byte
	// filtro y color describen datos: /FlateDecode con /DeviceRGB para los
	// píxeles comprimidos con zlib, o /DCTDecode para un JPEG tal cual.
	filtro, color string
}

// Nuevo crea un documento con una página en blanco.
func Nuevo() *Documento {
	d := &Documento{}
	d.NuevaPagina()
	return d
}

// NuevaPagina añade una página y la convierte en la actual.
func (d *Documento) NuevaPagina() {
	d.paginas = append(d.paginas, &bytes.Buffer{})
}

// Paginas devuelve el número de páginas.
func (d *Documento) Paginas() int { return len(d.paginas) }

func (d *Documento) p() *bytes.Buffer { return d.paginas[len(d.paginas)-1] }

// codificar convierte texto UTF-8 a WinAnsi (Windows-1252); los caracteres
// sin representación se sustituyen por '?'.
func codificar(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case r < 0x20:
			out = append(out, ' ')
		case r < 0x80, r >= 0xA0 && r <= 0xFF:
			out = append(out, byte(r))
		default:
			if b, ok := cp1252[r]; ok {
				out = append(out, b)
			} else {
				out = append(out, '?')
			}
		}
	}
	return out
}

// cp1252 son los caracteres de Windows-1252 situados entre 0x80 y 0x9F.
var cp1252 = map[rune]byte{
	'€': 0x80, '‚': 0x82, 'ƒ': 0x83, '„': 0x84, '…': 0x85, '†': 0x86, '‡': 0x87, 'ˆ': 0x88,
	'‰': 0x89, 'Š': 0x8A, '‹': 0x8B, 'Œ': 0x8C, 'Ž': 0x8E, '‘': 0x91, '’': 0x92, '“': 0x93,
	'”': 0x94, '•': 0x95, '–': 0x96, '—': 0x97, '˜': 0x98, '™': 0x99, 'š': 0x9A, '›': 0x9B,
	'œ': 0x9C, 'ž': 0x9E, 'Ÿ': 0x9F,
}

func anchos(f Fuente) *[256]uint16 {
	if f == Negrita {
		return &anchosHelveticaNegrita
	}
	return &anchosHelvetica
}

// AnchoTexto devuelve el ancho de s en puntos.
func AnchoTexto(s string, f Fuente, tam float64) float64 {
	a := anchos(f)
	n := 0
	for _, b := range codificar(s) {
		n += int(a[b])
	}
	return float64(n) * tam / 1000
}

// Partir divide s en líneas que caben en el ancho dado. Corta por espacios
// y, si una palabra sola no cabe, por caracteres.
func Partir(s string, f Fuente, tam, ancho float64) []string {
	var lineas []string
	for _, parrafo := range strings.Split(s, "\n") {
		actual := ""
		for _, palabra := range strings.Fields(parrafo) {
			for AnchoTexto(palabra, f, tam) > ancho && len([]rune(palabra)) > 1 {
				// La palabra no cabe ni sola: se corta.
				if actual != "" {
					lineas = append(lineas, actual)
					actual = ""
				}
				rs := []rune(palabra)
				n := len(rs) - 1
				for n > 1 && AnchoTexto(string(rs[:n]), f, tam) > ancho {
					n--
				}
				lineas = append(lineas, string(rs[:n]))
				palabra = string(rs[n:])
			}
			switch {
			case actual == "":
				actual = palabra
			case AnchoTexto(actual+" "+palabra, f, tam) <= ancho:
				actual += " " + palabra
			default:
				lineas = append(lineas, actual)
				actual = palabra
			}
		}
		lineas = append(lineas, actual)
	}
	return lineas
}

// Texto escribe s con la línea base en (x, y).
func (d *Documento) Texto(x, y float64, s string, f Fuente, tam float64) {
	if s == "" {
		return
	}
	var esc bytes.Buffer
	for _, b := range codificar(s) {
		if b == '(' || b == ')' || b == '\\' {
			esc.WriteByte('\\')
		}
		esc.WriteByte(b)
	}
	fmt.Fprintf(d.p(), "BT /F%d %.2f Tf %.2f %.2f Td (%s) Tj ET\n", int(f)+1, tam, x, Alto-y, esc.Bytes())
}

// TextoDerecha escribe s terminando en x.
func (d *Documento) TextoDerecha(x, y float64, s string, f Fuente, tam float64) {
	d.Texto(x-AnchoTexto(s, f, tam), y, s, f, tam)
}

// TextoCentrado escribe s centrado en x.
func (d *Documento) TextoCentrado(x, y float64, s string, f Fuente, tam float64) {
	d.Texto(x-AnchoTexto(s, f, tam)/2, y, s, f, tam)
}

// Linea traza un segmento.
func (d *Documento) Linea(x1, y1, x2, y2, grosor float64) {
	fmt.Fprintf(d.p(), "%.2f w %.2f %.2f m %.2f %.2f l S\n", grosor, x1, Alto-y1, x2, Alto-y2)
}

// Rect traza el contorno de un rectángulo cuya esquina superior izquierda
// es (x, y).
func (d *Documento) Rect(x, y, ancho, alto, grosor float64) {
	fmt.Fprintf(d.p(), "%.2f w %.2f %.2f %.2f %.2f re S\n", grosor, x, Alto-y-alto, ancho, alto)
}

// RectRelleno pinta un rectángulo con un gris entre 0 (negro) y 1 (blanco).
func (d *Documento) RectRelleno(x, y, ancho, alto, gris float64) {
	fmt.Fprintf(d.p(), "%.3f g %.2f %.2f %.2f %.2f re f 0 g\n", gris, x, Alto-y-alto, ancho, alto)
}

// Imagen dibuja img dentro del rectángulo dado, conservando la proporción y
// centrada. Las transparencias se componen sobre blanco.
func (d *Documento) Imagen(img image.Image, x, y, ancho, alto float64) {
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 || ancho <= 0 || alto <= 0 {
		return
	}
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Over)
	rgb := make([]byte, 0, b.Dx()*b.Dy()*3)
	for i := 0; i < len(rgba.Pix); i += 4 {
		rgb = append(rgb, rgba.Pix[i], rgba.Pix[i+1], rgba.Pix[i+2])
	}
	d.imagenes = append(d.imagenes, imagen{b.Dx(), b.Dy(), comprimir(rgb), "/FlateDecode", "/DeviceRGB"})
	d.colocar(b.Dx(), b.Dy(), x, y, ancho, alto)
}

// ImagenJPEG dibuja un JPEG incrustándolo tal cual, sin recomprimirlo.
// gris indica que tiene un solo canal; si no, debe ser de tres (YCbCr o
// RGB). anchoPx y altoPx son sus dimensiones en píxeles.
func (d *Documento) ImagenJPEG(jpeg []byte, anchoPx, altoPx int, gris bool, x, y, ancho, alto float64) {
	if anchoPx <= 0 || altoPx <= 0 || ancho <= 0 || alto <= 0 {
		return
	}
	color := "/DeviceRGB"
	if gris {
		color = "/DeviceGray"
	}
	d.imagenes = append(d.imagenes, imagen{anchoPx, altoPx, jpeg, "/DCTDecode", color})
	d.colocar(anchoPx, altoPx, x, y, ancho, alto)
}

// colocar pinta la última imagen registrada dentro del rectángulo,
// conservando la proporción y centrada.
func (d *Documento) colocar(anchoPx, altoPx int, x, y, ancho, alto float64) {
	escala := min(ancho/float64(anchoPx), alto/float64(altoPx))
	w, h := float64(anchoPx)*escala, float64(altoPx)*escala
	x += (ancho - w) / 2
	y += (alto - h) / 2
	fmt.Fprintf(d.p(), "q %.2f 0 0 %.2f %.2f %.2f cm /Im%d Do Q\n", w, h, x, Alto-y-h, len(d.imagenes))
}

// Color fija el color de relleno (texto y rectángulos rellenos) con
// componentes RGB entre 0 y 1; Negro lo restablece.
func (d *Documento) Color(r, g, b float64) { fmt.Fprintf(d.p(), "%.3f %.3f %.3f rg\n", r, g, b) }

// Negro restablece el color de relleno.
func (d *Documento) Negro() { d.p().WriteString("0 g\n") }

func comprimir(b []byte) []byte {
	var buf bytes.Buffer
	z := zlib.NewWriter(&buf)
	z.Write(b)
	z.Close()
	return buf.Bytes()
}

// cadenaPDF escribe s como cadena de texto de metadatos (UTF-16BE).
func cadenaPDF(s string) string {
	var sb strings.Builder
	sb.WriteString("<FEFF")
	for _, r := range s {
		if r > 0xFFFF {
			r = '?'
		}
		fmt.Fprintf(&sb, "%04X", r)
	}
	sb.WriteString(">")
	return sb.String()
}

// Bytes serializa el documento.
func (d *Documento) Bytes() []byte {
	var out bytes.Buffer
	var pos []int
	obj := func(cuerpo string, flujo []byte) int {
		pos = append(pos, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s", len(pos), cuerpo)
		if flujo != nil {
			out.WriteString("\nstream\n")
			out.Write(flujo)
			out.WriteString("\nendstream")
		}
		out.WriteString("\nendobj\n")
		return len(pos)
	}
	out.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")

	// Numeración: 1 catálogo, 2 árbol de páginas, 3 y 4 fuentes, luego las
	// imágenes y, por cada página, su contenido y su objeto.
	nImg := len(d.imagenes)
	primeraPag := 5 + nImg
	var hijos strings.Builder
	for i := range d.paginas {
		fmt.Fprintf(&hijos, "%d 0 R ", primeraPag+2*i+1)
	}
	obj("<< /Type /Catalog /Pages 2 0 R >>", nil)
	obj(fmt.Sprintf("<< /Type /Pages /Kids [ %s] /Count %d >>", hijos.String(), len(d.paginas)), nil)
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>", nil)
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>", nil)
	var xobj strings.Builder
	for i, im := range d.imagenes {
		n := obj(fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace %s /BitsPerComponent 8 /Filter %s /Length %d >>",
			im.ancho, im.alto, im.color, im.filtro, len(im.datos)), im.datos)
		fmt.Fprintf(&xobj, "/Im%d %d 0 R ", i+1, n)
	}
	recursos := fmt.Sprintf("<< /Font << /F1 3 0 R /F2 4 0 R >> /XObject << %s>> >>", xobj.String())
	for _, p := range d.paginas {
		c := comprimir(p.Bytes())
		n := obj(fmt.Sprintf("<< /Filter /FlateDecode /Length %d >>", len(c)), c)
		obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] /Resources %s /Contents %d 0 R >>", Ancho, Alto, recursos, n), nil)
	}
	info := obj(fmt.Sprintf("<< /Title %s /Author %s /Producer (sri-ecuador-go) >>", cadenaPDF(d.Titulo), cadenaPDF(d.Autor)), nil)

	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(pos)+1)
	for _, p := range pos {
		fmt.Fprintf(&out, "%010d 00000 n \n", p)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R /Info %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(pos)+1, info, xref)
	return out.Bytes()
}
