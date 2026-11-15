package ride

import (
	"strings"

	"github.com/OsirisRoman/sri-ecuador-go/internal/pdf"
)

// Geometría de la página, en puntos.
const (
	margen    = 28.0
	anchoUtil = pdf.Ancho - 2*margen
	pie       = pdf.Alto - margen
	relleno   = 4.0 // espacio interior de cajas y celdas
	grosor    = 0.5
	separa    = 8.0 // separación vertical entre bloques
)

// par es un dato rotulado: «Identificación: 1790011674001».
type par struct{ rotulo, valor string }

type alineacion int

const (
	izquierda alineacion = iota
	derecha
	centro
)

// columna describe una columna de una tabla; ancho es una fracción del
// ancho de la tabla.
type columna struct {
	titulo string
	ancho  float64
	alinea alineacion
}

// lienzo dibuja bloques de arriba abajo y salta de página cuando hace falta.
type lienzo struct {
	d *pdf.Documento
	y float64
}

func (l *lienzo) asegurar(alto float64) {
	if l.y+alto > pie && l.y > margen {
		l.d.NuevaPagina()
		l.y = margen
	}
}

func interlinea(tam float64) float64 { return tam * 1.3 }

// parrafo escribe texto ajustado al ancho, con la primera línea base en
// y+tam, y devuelve el alto ocupado. Si dibujar es false solo mide.
func (l *lienzo) parrafo(dibujar bool, x, y, ancho float64, texto string, f pdf.Fuente, tam float64, al alineacion) float64 {
	if texto == "" {
		return 0
	}
	lineas := pdf.Partir(texto, f, tam, ancho)
	if dibujar {
		for i, ln := range lineas {
			base := y + tam + float64(i)*interlinea(tam)
			switch al {
			case derecha:
				l.d.TextoDerecha(x+ancho, base, ln, f, tam)
			case centro:
				l.d.TextoCentrado(x+ancho/2, base, ln, f, tam)
			default:
				l.d.Texto(x, base, ln, f, tam)
			}
		}
	}
	return float64(len(lineas)) * interlinea(tam)
}

// parRotulado escribe «Rótulo: valor» con el rótulo en negrita y el valor
// ajustado al espacio restante; devuelve el alto.
func (l *lienzo) parRotulado(dibujar bool, x, y, ancho float64, p par, tam float64) float64 {
	rot := p.rotulo
	if rot != "" && !strings.HasSuffix(rot, ".") {
		rot += ":"
	}
	wr := pdf.AnchoTexto(rot, pdf.Negrita, tam)
	if wr > ancho*0.8 {
		// Rótulo muy largo: el valor va debajo.
		h := l.parrafo(dibujar, x, y, ancho, rot, pdf.Negrita, tam, izquierda)
		return h + l.parrafo(dibujar, x, y+h, ancho, p.valor, pdf.Normal, tam, izquierda)
	}
	if dibujar && rot != "" {
		l.d.Texto(x, y+tam, rot, pdf.Negrita, tam)
	}
	if rot != "" {
		wr += 4
	}
	return max(l.parrafo(dibujar, x+wr, y, ancho-wr, p.valor, pdf.Normal, tam, izquierda), interlinea(tam))
}

// filasPares dibuja filas de pares (cada fila reparte el ancho por igual
// entre sus pares) y devuelve el alto.
func (l *lienzo) filasPares(dibujar bool, x, y, ancho float64, filas [][]par, tam float64) float64 {
	y0 := y
	for _, fila := range filas {
		if len(fila) == 0 {
			continue
		}
		w := ancho / float64(len(fila))
		h := 0.0
		for i, p := range fila {
			h = max(h, l.parRotulado(dibujar, x+float64(i)*w, y, w-relleno, p, tam))
		}
		y += h + 2
	}
	return y - y0
}

// caja dibuja un recuadro a todo el ancho con filas de pares.
func (l *lienzo) caja(filas [][]par) {
	const tam = 8
	h := l.filasPares(false, 0, 0, anchoUtil-2*relleno, filas, tam) + 2*relleno
	l.asegurar(h)
	l.d.Rect(margen, l.y, anchoUtil, h, grosor)
	l.filasPares(true, margen+relleno, l.y+relleno, anchoUtil-2*relleno, filas, tam)
	l.y += h + separa
}

// tabla dibuja una tabla a partir de x con el ancho dado. Repite la
// cabecera al saltar de página.
func (l *lienzo) tabla(x, ancho float64, cols []columna, filas [][]string) {
	const tam, tamCab = 7, 7
	total := 0.0
	for _, c := range cols {
		total += c.ancho
	}
	w := make([]float64, len(cols))
	for i, c := range cols {
		w[i] = ancho * c.ancho / total
	}
	altoFila := func(celdas []string, f pdf.Fuente, t float64) float64 {
		h := interlinea(t)
		for i := range cols {
			if i < len(celdas) {
				h = max(h, l.parrafo(false, 0, 0, w[i]-2*relleno, celdas[i], f, t, izquierda))
			}
		}
		return h + 2*relleno - 2
	}
	titulos := make([]string, len(cols))
	for i, c := range cols {
		titulos[i] = c.titulo
	}
	hCab := altoFila(titulos, pdf.Negrita, tamCab)
	cabecera := func() {
		l.d.RectRelleno(x, l.y, ancho, hCab, 0.9)
		cx := x
		for i := range cols {
			l.d.Rect(cx, l.y, w[i], hCab, grosor)
			l.parrafo(true, cx+relleno, l.y+relleno-1, w[i]-2*relleno, titulos[i], pdf.Negrita, tamCab, centro)
			cx += w[i]
		}
		l.y += hCab
	}
	primera := hCab
	if len(filas) > 0 {
		primera += altoFila(filas[0], pdf.Normal, tam)
	}
	l.asegurar(primera)
	cabecera()
	for _, fila := range filas {
		h := altoFila(fila, pdf.Normal, tam)
		if l.y+h > pie {
			l.d.NuevaPagina()
			l.y = margen
			cabecera()
		}
		cx := x
		for i, c := range cols {
			l.d.Rect(cx, l.y, w[i], h, grosor)
			if i < len(fila) {
				l.parrafo(true, cx+relleno, l.y+relleno-1, w[i]-2*relleno, fila[i], pdf.Normal, tam, c.alinea)
			}
			cx += w[i]
		}
		l.y += h
	}
}
