// Package ride genera la representación impresa de un comprobante
// electrónico (RIDE) en PDF, según los modelos del Anexo 2 de la ficha
// técnica v2.34, para los seis tipos de comprobante.
//
// El RIDE es lo que el receptor ve e imprime; el documento con validez
// tributaria es el XML autorizado. Genere el RIDE solo de comprobantes
// AUTORIZADOS: pase a Generar el documento que devuelve
// sri.Resultado.XMLAutorizado, que incluye el número y la fecha de
// autorización, y Generar se negará si su estado no es AUTORIZADO. También
// se acepta el XML del comprobante sin envoltura (por ejemplo, para una
// vista previa); en ese caso no figura la fecha de autorización y es
// responsabilidad de quien llama no entregarlo como comprobante válido.
//
// El PDF se escribe con un generador propio, sin dependencias externas:
// tamaño A4, fuentes base Helvetica (no se incrustan fuentes), código de
// barras Code 128 con la clave de acceso y logotipo opcional.
package ride

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // logotipos PNG
	"strings"
	"time"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/esquema"
	"github.com/OsirisRoman/sri-ecuador-go/internal/pdf"
	"github.com/OsirisRoman/sri-ecuador-go/retencion"
)

// Opciones ajusta Generar. El valor cero es válido.
type Opciones struct {
	// Logo es el logotipo del emisor en PNG o JPEG. Los JPEG se incrustan
	// tal cual; los PNG grandes se reducen. Si falta, su espacio queda
	// reservado con la leyenda «NO TIENE LOGO», como en los RIDE que genera
	// el SRI.
	Logo []byte
	// SinLeyendaLogo deja en blanco el espacio del logotipo cuando no hay
	// Logo, en vez de escribir «NO TIENE LOGO».
	SinLeyendaLogo bool
	// NumeroAutorizacion y FechaAutorizacion se usan cuando el documento no
	// trae la envoltura <autorizacion>. En el esquema offline el número de
	// autorización es la clave de acceso, que es el valor por defecto.
	NumeroAutorizacion string
	FechaAutorizacion  string
	// SinCodigoBarras omite el código de barras de la clave de acceso, que
	// es opcional (ficha técnica 9.20).
	SinCodigoBarras bool
}

// Errores de Generar.
var (
	// ErrNoAutorizado indica que la envoltura <autorizacion> recibida no
	// tiene el estado AUTORIZADO.
	ErrNoAutorizado = errors.New("ride: el comprobante no está autorizado")
	// ErrLogo indica que el logotipo no es una imagen PNG o JPEG legible.
	ErrLogo = errors.New("ride: logotipo ilegible")
)

// modelo es el contenido de un RIDE, independiente del tipo de comprobante.
type modelo struct {
	titulo                           string
	it                               comprobante.InfoTributaria
	dirEstablecimiento               string
	contribuyenteEspecial, obligado  string
	bloques                          []func(l *lienzo)
	adicional                        []comprobante.CampoAdicional
	pagos                            []comprobante.Pago
	totales                          []par
	numAutorizacion, fechaAutorizado string
	// nombre es el tipo en minúsculas («Nota de crédito») y receptor, a
	// quién va dirigido; los usa Resumir.
	nombre, receptor string
}

type envoltura struct {
	XMLName            xml.Name `xml:"autorizacion"`
	Estado             string   `xml:"estado"`
	NumeroAutorizacion string   `xml:"numeroAutorizacion"`
	FechaAutorizacion  string   `xml:"fechaAutorizacion"`
	Comprobante        string   `xml:"comprobante"`
}

// Generar devuelve el RIDE en PDF de un comprobante. doc es el documento
// <autorizacion> que devuelve sri.Resultado.XMLAutorizado o el XML del
// comprobante, firmado o no. op puede ser nil.
func Generar(doc []byte, op *Opciones) ([]byte, error) {
	if op == nil {
		op = &Opciones{}
	}
	m, err := leer(doc)
	if err != nil {
		return nil, err
	}
	if m.numAutorizacion == "" {
		m.numAutorizacion = op.NumeroAutorizacion
	}
	if m.numAutorizacion == "" {
		m.numAutorizacion = m.it.ClaveAcceso
	}
	if m.fechaAutorizado == "" {
		m.fechaAutorizado = fechaLegible(op.FechaAutorizacion)
	}
	logo, err := leerLogo(op.Logo)
	if err != nil {
		return nil, err
	}
	if logo == nil && !op.SinLeyendaLogo {
		logo = &logotipo{}
	}
	return m.dibujar(logo, !op.SinCodigoBarras), nil
}

// logotipo es la imagen que ocupa el espacio del logo: un JPEG que se
// incrusta sin recomprimir, una imagen decodificada o, si ambos faltan, la
// leyenda «NO TIENE LOGO».
type logotipo struct {
	jpeg        []byte
	ancho, alto int
	gris        bool
	img         image.Image
}

// maxLadoLogo es el mayor lado, en píxeles, con el que se incrusta un
// logotipo que hay que recomprimir: de sobra para los 8 cm que ocupa.
const maxLadoLogo = 900

func leerLogo(datos []byte) (*logotipo, error) {
	if len(datos) == 0 {
		return nil, nil
	}
	cfg, formato, err := image.DecodeConfig(bytes.NewReader(datos))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, fmt.Errorf("%w: %v", ErrLogo, err)
	}
	img, _, err := image.Decode(bytes.NewReader(datos))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLogo, err)
	}
	esJPEG := formato == "jpeg" && (cfg.ColorModel == color.YCbCrModel || cfg.ColorModel == color.GrayModel)
	recortada := recortar(img)
	if esJPEG && recortada.Bounds() == img.Bounds() && cfg.Width <= maxLadoLogo && cfg.Height <= maxLadoLogo {
		// Un JPEG sin márgenes y de tamaño razonable se incrusta tal cual.
		return &logotipo{jpeg: datos, ancho: cfg.Width, alto: cfg.Height, gris: cfg.ColorModel == color.GrayModel}, nil
	}
	img = reducir(recortada, maxLadoLogo)
	if esJPEG {
		// Los JPEG se vuelven a comprimir como JPEG para que el RIDE no
		// pese cientos de kilobytes.
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrLogo, err)
		}
		b := img.Bounds()
		return &logotipo{jpeg: buf.Bytes(), ancho: b.Dx(), alto: b.Dy()}, nil
	}
	return &logotipo{img: img}, nil
}

// recortar quita los márgenes en blanco o transparentes que rodean al
// logotipo, para que el dibujo —y no su lienzo vacío— llene el espacio
// reservado. Deja un margen del 2 % y devuelve img sin cambios si no hay
// nada que recortar o si la imagen está vacía.
func recortar(img image.Image) image.Image {
	b := img.Bounds()
	const umbral = 0xF000 // por encima se considera blanco
	vacio := func(x, y int) bool {
		r, g, bl, a := img.At(x, y).RGBA()
		return a < 0x1000 || (r >= umbral && g >= umbral && bl >= umbral)
	}
	// Se muestrea con un paso para no recorrer millones de píxeles.
	paso := max(max(b.Dx(), b.Dy())/600, 1)
	x0, y0, x1, y1 := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	for y := b.Min.Y; y < b.Max.Y; y += paso {
		for x := b.Min.X; x < b.Max.X; x += paso {
			if !vacio(x, y) {
				x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
			}
		}
	}
	if x1 < x0 || y1 < y0 {
		return img
	}
	m := max(b.Dx(), b.Dy())/50 + paso
	caja := image.Rect(x0-m, y0-m, x1+m+1, y1+m+1).Intersect(b)
	if caja == b {
		return img
	}
	type subimagen interface {
		SubImage(image.Rectangle) image.Image
	}
	if si, ok := img.(subimagen); ok {
		return si.SubImage(caja)
	}
	return img
}

// reducir devuelve img con su lado mayor limitado a lado píxeles,
// promediando los píxeles de origen que caen en cada píxel de destino.
func reducir(img image.Image, lado int) image.Image {
	b := img.Bounds()
	mayor := max(b.Dx(), b.Dy())
	if mayor <= lado {
		return img
	}
	w, h := max(b.Dx()*lado/mayor, 1), max(b.Dy()*lado/mayor, 1)
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		y0, y1 := b.Min.Y+y*b.Dy()/h, b.Min.Y+(y+1)*b.Dy()/h
		for x := 0; x < w; x++ {
			x0, x1 := b.Min.X+x*b.Dx()/w, b.Min.X+(x+1)*b.Dx()/w
			var r, g, bl, a, n uint64
			for sy := y0; sy < max(y1, y0+1); sy++ {
				for sx := x0; sx < max(x1, x0+1); sx++ {
					cr, cg, cb, ca := img.At(sx, sy).RGBA()
					r, g, bl, a, n = r+uint64(cr), g+uint64(cg), bl+uint64(cb), a+uint64(ca), n+1
				}
			}
			out.SetRGBA64(x, y, color.RGBA64{uint16(r / n), uint16(g / n), uint16(bl / n), uint16(a / n)})
		}
	}
	return out
}

// Resumen son los datos con los que se suele presentar un comprobante, por
// ejemplo en el correo de entrega.
type Resumen struct {
	// Tipo legible: "Factura", "Nota de crédito", "Guía de remisión"…
	Tipo string
	// Numero con formato 001-001-000000001.
	Numero      string
	ClaveAcceso string
	// Emisor y Receptor son las razones sociales. En una guía de remisión
	// el receptor es el primer destinatario.
	Emisor, Receptor string
}

// Resumir extrae el Resumen de un comprobante; acepta los mismos
// documentos que Generar.
func Resumir(doc []byte) (Resumen, error) {
	m, err := leer(doc)
	if err != nil {
		return Resumen{}, err
	}
	return Resumen{Tipo: m.nombre, Numero: m.it.NumeroComprobante(), ClaveAcceso: m.it.ClaveAcceso,
		Emisor: m.it.RazonSocial, Receptor: m.receptor}, nil
}

// leer interpreta doc (con o sin envoltura de autorización) y compone el
// modelo del RIDE.
func leer(doc []byte) (*modelo, error) {
	var numAut, fechaAut string
	if raiz(doc) == "autorizacion" {
		var e envoltura
		if err := xml.Unmarshal(doc, &e); err != nil {
			return nil, fmt.Errorf("ride: autorización ilegible: %w", err)
		}
		if strings.TrimSpace(e.Estado) != "AUTORIZADO" {
			return nil, fmt.Errorf("%w: estado %q", ErrNoAutorizado, e.Estado)
		}
		doc = []byte(strings.TrimSpace(e.Comprobante))
		numAut, fechaAut = strings.TrimSpace(e.NumeroAutorizacion), strings.TrimSpace(e.FechaAutorizacion)
	}
	tipo, version, err := esquema.Identificar(doc)
	if err != nil {
		return nil, fmt.Errorf("ride: %w", err)
	}
	var m *modelo
	switch tipo {
	case esquema.Factura:
		m, err = deFactura(doc)
	case esquema.NotaCredito:
		m, err = deNotaCredito(doc)
	case esquema.NotaDebito:
		m, err = deNotaDebito(doc)
	case esquema.ComprobanteRetencion:
		if version == retencion.VersionV1 {
			m, err = deRetencionV1(doc)
		} else {
			m, err = deRetencion(doc)
		}
	case esquema.GuiaRemision:
		m, err = deGuia(doc)
	case esquema.LiquidacionCompra:
		m, err = deLiquidacion(doc)
	default:
		err = fmt.Errorf("tipo de comprobante %q sin RIDE", tipo)
	}
	if err != nil {
		return nil, fmt.Errorf("ride: %w", err)
	}
	m.numAutorizacion, m.fechaAutorizado = numAut, fechaLegible(fechaAut)
	return m, nil
}

// fechaLegible reescribe una fecha ISO 8601, que es como la entrega el
// servicio de autorización, en el formato dd/mm/aaaa hh:mm:ss de los RIDE
// del SRI. Cualquier otro texto se deja como está.
func fechaLegible(s string) string {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Format("02/01/2006 15:04:05")
	}
	return s
}

// raiz devuelve el nombre del elemento raíz de doc, o "".
func raiz(doc []byte) string {
	dec := xml.NewDecoder(bytes.NewReader(doc))
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		if se, ok := tok.(xml.StartElement); ok {
			return se.Name.Local
		}
	}
}

func (m *modelo) dibujar(logo *logotipo, barras bool) []byte {
	d := pdf.Nuevo()
	d.Titulo = fmt.Sprintf("%s %s", m.titulo, m.it.NumeroComprobante())
	d.Autor = m.it.RazonSocial
	l := &lienzo{d: d, y: margen}
	m.cabecera(l, logo, barras)
	for _, b := range m.bloques {
		b(l)
	}
	m.cierre(l)
	return d.Bytes()
}

// cabecera dibuja el logotipo y los datos del emisor a la izquierda y la
// identificación del comprobante a la derecha.
func (m *modelo) cabecera(l *lienzo, logo *logotipo, barras bool) {
	const (
		hueco    = 12.0
		altoLogo = 120.0
		pad      = 12.0
	)
	wIzq := anchoUtil*0.48 - hueco/2
	wDer := anchoUtil - wIzq - hueco
	xDer := margen + wIzq + hueco

	// --- Emisor (izquierda) ---
	var datos [][]par
	if m.it.NombreComercial != "" && m.it.NombreComercial != m.it.RazonSocial {
		datos = append(datos, []par{{"", m.it.NombreComercial}})
	}
	datos = append(datos, []par{{"Dirección Matriz", m.it.DirMatriz}})
	if m.dirEstablecimiento != "" {
		datos = append(datos, []par{{"Dirección Sucursal", m.dirEstablecimiento}})
	}
	if m.contribuyenteEspecial != "" {
		datos = append(datos, []par{{"Contribuyente Especial Nro", m.contribuyenteEspecial}})
	}
	if m.obligado != "" {
		datos = append(datos, []par{{"OBLIGADO A LLEVAR CONTABILIDAD", m.obligado}})
	}
	if m.it.AgenteRetencion != "" {
		datos = append(datos, []par{{"Agente de Retención Resolución No.", m.it.AgenteRetencion}})
	}
	if m.it.ContribuyenteRimpe != "" {
		datos = append(datos, []par{{"", m.it.ContribuyenteRimpe}})
	}
	emisor := func(dibujar bool, y float64) float64 {
		h := l.parrafo(dibujar, margen+pad, y+pad, wIzq-2*pad, m.it.RazonSocial, pdf.Negrita, 11, izquierda) + 6
		h += l.filasPares(dibujar, margen+pad, y+pad+h, wIzq-2*pad, datos, 9)
		return h + 2*pad
	}
	hEmisor := emisor(false, 0)

	// --- Comprobante (derecha) ---
	ambiente := "PRUEBAS"
	if m.it.Ambiente == comprobante.Produccion {
		ambiente = "PRODUCCIÓN"
	}
	identificacion := func(dibujar bool, y float64) float64 {
		x, w, y0 := xDer+pad, wDer-2*pad, y
		y += pad
		y += l.parRotulado(dibujar, x, y, w, par{"R.U.C.", m.it.RUC}, 13) + 7
		y += l.parrafo(dibujar, x, y, w, m.titulo, pdf.Negrita, 14, izquierda) + 7
		y += l.parRotulado(dibujar, x, y, w, par{"No.", m.it.NumeroComprobante()}, 11) + 9
		y += l.parrafo(dibujar, x, y, w, "NÚMERO DE AUTORIZACIÓN", pdf.Normal, 10, izquierda) + 1
		y += l.parrafo(dibujar, x, y, w, m.numAutorizacion, pdf.Normal, 8.5, izquierda) + 8
		if m.fechaAutorizado != "" {
			y += l.parRotulado(dibujar, x, y, w, par{"FECHA Y HORA DE AUTORIZACIÓN", m.fechaAutorizado}, 8.5) + 8
		}
		y += l.parRotulado(dibujar, x, y, w, par{"AMBIENTE", ambiente}, 9.5) + 5
		y += l.parRotulado(dibujar, x, y, w, par{"EMISIÓN", "NORMAL"}, 9.5) + 9
		y += l.parrafo(dibujar, x, y, w, "CLAVE DE ACCESO", pdf.Normal, 10, izquierda) + 4
		if barras && m.it.ClaveAcceso != "" {
			if dibujar {
				// Se dejan 10 módulos de silencio a cada lado.
				l.d.Code128(m.it.ClaveAcceso, x+8, y, w-16, 42)
			}
			y += 42 + 3
		}
		if dibujar {
			l.d.TextoCentrado(x+w/2, y+8, m.it.ClaveAcceso, pdf.Normal, 8)
		}
		y += interlinea(8)
		return y - y0 + pad
	}
	hDer := identificacion(false, 0)

	// El logotipo (o la leyenda que reserva su espacio) ocupa la parte alta
	// de la columna izquierda y el emisor queda debajo; si se pidió dejarlo
	// en blanco sin leyenda, el recuadro del emisor llena la columna.
	alto := max(hDer, hEmisor)
	yEmisor, hCaja := l.y, alto
	if logo != nil {
		alto = max(hDer, altoLogo+hueco+hEmisor)
		hLogo := alto - hEmisor - hueco
		switch {
		case logo.jpeg != nil:
			l.d.ImagenJPEG(logo.jpeg, logo.ancho, logo.alto, logo.gris, margen, l.y, wIzq, hLogo)
		case logo.img != nil:
			l.d.Imagen(logo.img, margen, l.y, wIzq, hLogo)
		default:
			l.d.Color(0.85, 0, 0)
			l.d.TextoCentrado(margen+wIzq/2, l.y+hLogo/2+8, "NO TIENE LOGO", pdf.Negrita, 26)
			l.d.Negro()
		}
		yEmisor, hCaja = l.y+alto-hEmisor, hEmisor
	}
	l.d.RectRedondeado(margen, yEmisor, wIzq, hCaja, radio, grosor)
	emisor(true, yEmisor)
	l.d.RectRedondeado(xDer, l.y, wDer, alto, radio, grosor)
	identificacion(true, l.y)
	l.y += alto + separa
}

// cierre dibuja la información adicional y las formas de pago a la
// izquierda y los totales a la derecha.
func (m *modelo) cierre(l *lienzo) {
	if len(m.adicional) == 0 && len(m.pagos) == 0 && len(m.totales) == 0 {
		return
	}
	const hueco, tam = 12.0, tamTexto - 0.5
	wIzq := anchoUtil*0.56 - hueco/2
	wDer := anchoUtil - wIzq - hueco
	xDer := margen + wIzq + hueco

	var filas [][]par
	for _, c := range m.adicional {
		filas = append(filas, []par{{c.Nombre, c.Valor}})
	}
	adicional := func(dibujar bool, y float64) float64 {
		if len(filas) == 0 {
			return 0
		}
		h := l.parrafo(dibujar, margen+relleno, y+relleno, wIzq-2*relleno, "Información Adicional", pdf.Negrita, tamTexto, izquierda) + 4
		h += l.filasPares(dibujar, margen+relleno, y+relleno+h, wIzq-2*relleno, filas, tam)
		return h + 2*relleno
	}
	hAdic := adicional(false, 0)
	hPagos := 0.0
	if len(m.pagos) > 0 {
		hPagos = float64(len(m.pagos)+1)*(interlinea(tamTabla)+2*relleno) + 36 // holgura para textos partidos
		if hAdic > 0 {
			hPagos += separa
		}
	}
	hFilaTot := interlinea(tam) + 7
	hTot := float64(len(m.totales)) * hFilaTot
	l.asegurar(max(hAdic+hPagos, hTot))
	y0 := l.y

	// Totales (derecha).
	y := y0
	for i, t := range m.totales {
		h := hFilaTot
		if i > 0 {
			l.d.Linea(xDer, y, xDer+wDer, y, grosor)
		}
		f := pdf.Normal
		if t.rotulo == rotuloTotal {
			f = pdf.Negrita
		}
		l.d.Texto(xDer+relleno, y+tam+5, t.rotulo, f, tam)
		l.d.TextoDerecha(xDer+wDer-relleno, y+tam+5, t.valor, f, tam)
		y += h
	}
	if len(m.totales) > 0 {
		l.d.Linea(xDer+wDer*0.62, y0, xDer+wDer*0.62, y, grosor)
		l.d.RectRedondeado(xDer, y0, wDer, y-y0, radio, grosor)
	}
	finDer := y

	// Información adicional y pagos (izquierda).
	if hAdic > 0 {
		l.d.RectRedondeado(margen, l.y, wIzq, hAdic, radio, grosor)
		adicional(true, l.y)
		l.y += hAdic + separa
	}
	if len(m.pagos) > 0 {
		var fp [][]string
		conPlazo := false
		for _, p := range m.pagos {
			conPlazo = conPlazo || p.Plazo != nil
		}
		for _, p := range m.pagos {
			nombre := p.FormaPago
			if d, ok := comprobante.FormaPagoDe(p.FormaPago); ok {
				nombre += " - " + d
			}
			fila := []string{nombre, p.Total.String()}
			if conPlazo {
				plazo := ""
				if p.Plazo != nil {
					plazo = strings.TrimSpace(p.Plazo.String() + " " + p.UnidadTiempo)
				}
				fila = append(fila, plazo)
			}
			fp = append(fp, fila)
		}
		cols := []columna{{"Forma de pago", 0.7, izquierda}, {"Valor", 0.3, derecha}}
		if conPlazo {
			cols = []columna{{"Forma de pago", 0.58, izquierda}, {"Valor", 0.2, derecha}, {"Plazo", 0.22, centro}}
		}
		l.tabla(margen, wIzq, cols, fp)
	}
	l.y = max(l.y, finDer) + separa
}

const rotuloTotal = "VALOR TOTAL"

// nombreDocumento traduce un código de la Tabla 3 (y afines) a su nombre.
func nombreDocumento(cod string) string {
	switch cod {
	case comprobante.CodFactura:
		return "FACTURA"
	case comprobante.CodLiquidacionCompra:
		return "LIQUIDACIÓN DE COMPRA"
	case comprobante.CodNotaCredito:
		return "NOTA DE CRÉDITO"
	case comprobante.CodNotaDebito:
		return "NOTA DE DÉBITO"
	case comprobante.CodGuiaRemision:
		return "GUÍA DE REMISIÓN"
	case comprobante.CodComprobanteRetencion:
		return "COMPROBANTE DE RETENCIÓN"
	case "":
		return ""
	}
	return "DOCUMENTO " + cod
}
