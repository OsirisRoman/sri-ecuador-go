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
	_ "image/jpeg" // logotipos JPEG
	_ "image/png"  // logotipos PNG
	"strings"
	"time"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/esquema"
	"github.com/OsirisRoman/sri-ecuador-go/internal/pdf"
)

// Opciones ajusta Generar. El valor cero es válido.
type Opciones struct {
	// Logo es el logotipo del emisor en PNG o JPEG; si falta, su espacio
	// queda libre.
	Logo []byte
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
	var logo image.Image
	if len(op.Logo) > 0 {
		if logo, _, err = image.Decode(bytes.NewReader(op.Logo)); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrLogo, err)
		}
	}
	return m.dibujar(logo, !op.SinCodigoBarras), nil
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
	tipo, _, err := esquema.Identificar(doc)
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
		m, err = deRetencion(doc)
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

func (m *modelo) dibujar(logo image.Image, barras bool) []byte {
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
func (m *modelo) cabecera(l *lienzo, logo image.Image, barras bool) {
	const (
		hueco    = 10.0
		altoLogo = 78.0
		pad      = 8.0
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
		h := l.parrafo(dibujar, margen+pad, y+pad, wIzq-2*pad, m.it.RazonSocial, pdf.Negrita, 9, izquierda) + 4
		h += l.filasPares(dibujar, margen+pad, y+pad+h, wIzq-2*pad, datos, 7.5)
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
		y += l.parRotulado(dibujar, x, y, w, par{"R.U.C.", m.it.RUC}, 11) + 4
		y += l.parrafo(dibujar, x, y, w, m.titulo, pdf.Negrita, 11, izquierda) + 4
		y += l.parRotulado(dibujar, x, y, w, par{"No.", m.it.NumeroComprobante()}, 9) + 4
		y += l.parrafo(dibujar, x, y, w, "NÚMERO DE AUTORIZACIÓN", pdf.Normal, 9, izquierda)
		y += l.parrafo(dibujar, x, y, w, m.numAutorizacion, pdf.Normal, 8, izquierda) + 4
		if m.fechaAutorizado != "" {
			y += l.parRotulado(dibujar, x, y, w, par{"FECHA Y HORA DE AUTORIZACIÓN", m.fechaAutorizado}, 7.5) + 4
		}
		y += l.parRotulado(dibujar, x, y, w, par{"AMBIENTE", ambiente}, 8) + 2
		y += l.parRotulado(dibujar, x, y, w, par{"EMISIÓN", "NORMAL"}, 8) + 4
		y += l.parrafo(dibujar, x, y, w, "CLAVE DE ACCESO", pdf.Normal, 9, izquierda) + 2
		if barras && m.it.ClaveAcceso != "" {
			if dibujar {
				// Se dejan 10 módulos de silencio a cada lado.
				l.d.Code128(m.it.ClaveAcceso, x+8, y, w-16, 30)
			}
			y += 30 + 2
		}
		if dibujar {
			l.d.TextoCentrado(x+w/2, y+7, m.it.ClaveAcceso, pdf.Normal, 7)
		}
		y += interlinea(7)
		return y - y0 + pad
	}
	hDer := identificacion(false, 0)

	// Con logotipo, este ocupa la parte alta de la columna izquierda y el
	// emisor queda debajo; sin él, el recuadro del emisor llena la columna.
	alto := max(hDer, hEmisor)
	yEmisor, hCaja := l.y, alto
	if logo != nil {
		alto = max(hDer, altoLogo+hueco+hEmisor)
		l.d.Imagen(logo, margen, l.y, wIzq, alto-hEmisor-hueco)
		yEmisor, hCaja = l.y+alto-hEmisor, hEmisor
	}
	l.d.Rect(margen, yEmisor, wIzq, hCaja, grosor)
	emisor(true, yEmisor)
	l.d.Rect(xDer, l.y, wDer, alto, grosor)
	identificacion(true, l.y)
	l.y += alto + separa
}

// cierre dibuja la información adicional y las formas de pago a la
// izquierda y los totales a la derecha.
func (m *modelo) cierre(l *lienzo) {
	if len(m.adicional) == 0 && len(m.pagos) == 0 && len(m.totales) == 0 {
		return
	}
	const hueco, tam = 10.0, 7.5
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
		h := l.parrafo(dibujar, margen+relleno, y+relleno, wIzq-2*relleno, "Información Adicional", pdf.Negrita, 8, izquierda) + 2
		h += l.filasPares(dibujar, margen+relleno, y+relleno+h, wIzq-2*relleno, filas, tam)
		return h + 2*relleno
	}
	hAdic := adicional(false, 0)
	hPagos := 0.0
	if len(m.pagos) > 0 {
		hPagos = float64(len(m.pagos)+1)*(interlinea(7)+2*relleno) + 30 // holgura para textos partidos
		if hAdic > 0 {
			hPagos += separa
		}
	}
	hTot := float64(len(m.totales)) * (interlinea(tam) + 4)
	l.asegurar(max(hAdic+hPagos, hTot))
	y0 := l.y

	// Totales (derecha).
	y := y0
	for _, t := range m.totales {
		h := interlinea(tam) + 4
		l.d.Rect(xDer, y, wDer*0.62, h, grosor)
		l.d.Rect(xDer+wDer*0.62, y, wDer*0.38, h, grosor)
		f := pdf.Normal
		if t.rotulo == rotuloTotal {
			f = pdf.Negrita
		}
		l.d.Texto(xDer+relleno, y+tam+2, t.rotulo, f, tam)
		l.d.TextoDerecha(xDer+wDer-relleno, y+tam+2, t.valor, f, tam)
		y += h
	}
	finDer := y

	// Información adicional y pagos (izquierda).
	if hAdic > 0 {
		l.d.Rect(margen, l.y, wIzq, hAdic, grosor)
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
			if d, ok := comprobante.FormasPago[p.FormaPago]; ok {
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
