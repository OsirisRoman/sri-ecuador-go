// Package esquema valida comprobantes electrónicos contra los esquemas XSD
// oficiales del SRI, que van incrustados en el binario: la validación es real
// (modelos de contenido, patrones, facetas numéricas), se hace sin conexión y
// en Go puro, sin cgo ni libxml2.
//
// # Procedencia de los esquemas
//
// Los archivos de xsd/ son los que publica el SRI en
// https://www.sri.gob.ec/facturacion-electronica ("Esquemas XSD y XML"),
// copiados sin modificación; sus resúmenes SHA-256 están en
// xsd/PROCEDENCIA.md y una prueba impide que cambien inadvertidamente.
// xmldsig-core-schema.xsd es el esquema de XMLDSig del W3C, que los XSD del
// SRI importan pero el SRI no distribuye.
//
// # Diferencias conocidas entre los XSD publicados y la ficha técnica
//
// Los XSD publicados no siempre siguen el ritmo de la ficha técnica. Cuando
// un XSD rechaza algo que la ficha exige, el servidor del SRI evidentemente
// usa un esquema más reciente que el publicado, y validar con el archivo
// literal produciría falsos rechazos. Esas diferencias se corrigen en
// memoria al cargar el esquema y se enumeran en Ajustes; ValidarEstricto
// usa los archivos tal cual.
package esquema

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"
	"testing/fstest"

	"github.com/cB-Sigma/xsd"

	"github.com/OsirisRoman/sri-ecuador-go/internal/xmldom"
)

//go:embed xsd/*.xsd
var archivos embed.FS

// Tipo identifica un tipo de comprobante por el nombre de su elemento raíz.
type Tipo string

// Tipos de comprobante con esquema disponible.
const (
	Factura              Tipo = "factura"
	NotaCredito          Tipo = "notaCredito"
	NotaDebito           Tipo = "notaDebito"
	GuiaRemision         Tipo = "guiaRemision"
	ComprobanteRetencion Tipo = "comprobanteRetencion"
	LiquidacionCompra    Tipo = "liquidacionCompra"
)

type clave struct {
	tipo    Tipo
	version string
}

// catalogo asocia cada (tipo, versión) con su archivo XSD.
//
// Para notas de crédito y débito el SRI publica dos familias que no son
// compatibles entre sí: NotaCredito_V*/NotaDebito_V* admiten
// <agenteRetencion> y <contribuyenteRimpe> (Anexos 21 y 22) pero no
// <valorDevolucionIva>; NOTA_CREDITO-*/NOTA_DEBITO-* ("con el campo
// Devolución IVA", Anexo 20) hacen lo contrario. Se usa la primera familia;
// la segunda queda incrustada para cuando se implementen esos comprobantes
// y se determine con pruebas reales cuál aplica el servidor.
var catalogo = map[clave]string{
	{Factura, "1.0.0"}:              "factura_V1.0.0.xsd",
	{Factura, "1.1.0"}:              "factura_V1.1.0.xsd",
	{Factura, "2.0.0"}:              "factura_V2.0.0.xsd",
	{Factura, "2.1.0"}:              "factura_V2.1.0.xsd",
	{NotaCredito, "1.0.0"}:          "NotaCredito_V1.0.0.xsd",
	{NotaCredito, "1.1.0"}:          "NotaCredito_V1.1.0.xsd",
	{NotaDebito, "1.0.0"}:           "NotaDebito_V1.0.0.xsd",
	{GuiaRemision, "1.0.0"}:         "GuiaRemision_V1.0.0.xsd",
	{GuiaRemision, "1.1.0"}:         "GuiaRemision_V1.1.0.xsd",
	{ComprobanteRetencion, "1.0.0"}: "ComprobanteRetencion_V1.0.0.xsd",
	{ComprobanteRetencion, "2.0.0"}: "ComprobanteRetencion_V2.0.0.xsd",
	{LiquidacionCompra, "1.0.0"}:    "LiquidacionCompra_V1.0.0.xsd",
	{LiquidacionCompra, "1.1.0"}:    "LiquidacionCompra_V1.1.0.xsd",
}

// Ajuste describe una corrección aplicada en memoria a un XSD publicado.
type Ajuste struct {
	// Motivo explica la diferencia y cita la norma que la respalda.
	Motivo string
	// Original y Reemplazo son el fragmento del XSD publicado y el que se
	// usa en su lugar.
	Original, Reemplazo string
}

// Ajustes son las correcciones que Validar aplica a los XSD publicados.
// ValidarEstricto no las aplica.
var Ajustes = []Ajuste{
	{
		Motivo: `El XSD publicado solo admite la leyenda "CONTRIBUYENTE RÉGIMEN RIMPE" en ` +
			`<contribuyenteRimpe>, pero el Anexo 22 de la ficha técnica v2.34 (pág. 129-130) exige ` +
			`"CONTRIBUYENTE NEGOCIO POPULAR - RÉGIMEN RIMPE" a los negocios populares.`,
		Original:  `<xsd:pattern value="CONTRIBUYENTE RÉGIMEN RIMPE"/>`,
		Reemplazo: `<xsd:pattern value="CONTRIBUYENTE RÉGIMEN RIMPE"/><xsd:pattern value="CONTRIBUYENTE NEGOCIO POPULAR - RÉGIMEN RIMPE"/>`,
	},
}

// Errores de uso.
var (
	// ErrTipoDesconocido indica que el elemento raíz no es un comprobante
	// conocido.
	ErrTipoDesconocido = errors.New("esquema: tipo de comprobante desconocido")
	// ErrVersionDesconocida indica que no existe XSD para esa versión del
	// comprobante (el SRI respondería con el error 36 o 48).
	ErrVersionDesconocida = errors.New("esquema: versión de comprobante sin XSD")
)

// Problema es un incumplimiento del esquema.
type Problema struct {
	// Linea del documento, si se conoce.
	Linea int
	// Ruta del elemento, por ejemplo /factura/infoFactura/totalDescuento.
	Ruta string
	// Mensaje describe el incumplimiento.
	Mensaje string
}

func (p Problema) String() string {
	if p.Linea > 0 {
		return fmt.Sprintf("línea %d: %s: %s", p.Linea, p.Ruta, p.Mensaje)
	}
	return p.Ruta + ": " + p.Mensaje
}

// ErrorValidacion es el error que devuelve Validar cuando el documento no
// cumple el XSD. Equivale al rechazo 35 "Documento inválido" del SRI, pero
// detectado antes de enviar.
type ErrorValidacion struct {
	Tipo      Tipo
	Version   string
	Problemas []Problema
}

func (e *ErrorValidacion) Error() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "esquema: %s %s no cumple el XSD (%d problema(s))", e.Tipo, e.Version, len(e.Problemas))
	for i, p := range e.Problemas {
		if i == 5 {
			fmt.Fprintf(&sb, "; … y %d más", len(e.Problemas)-i)
			break
		}
		sb.WriteString("; " + p.String())
	}
	return sb.String()
}

type cargado struct {
	una     sync.Once
	esquema *xsd.Schema
	err     error
}

var (
	mu    sync.Mutex
	cache = map[string]*cargado{} // "archivo|estricto"
)

func cargar(archivo string, estricto bool) (*xsd.Schema, error) {
	k := fmt.Sprintf("%s|%t", archivo, estricto)
	mu.Lock()
	c, ok := cache[k]
	if !ok {
		c = &cargado{}
		cache[k] = c
	}
	mu.Unlock()
	c.una.Do(func() {
		var fsys fs.FS
		sub, err := fs.Sub(archivos, "xsd")
		if err != nil {
			c.err = err
			return
		}
		fsys = sub
		if !estricto {
			principal, err := fs.ReadFile(sub, archivo)
			if err != nil {
				c.err = err
				return
			}
			dsig, err := fs.ReadFile(sub, "xmldsig-core-schema.xsd")
			if err != nil {
				c.err = err
				return
			}
			for _, a := range Ajustes {
				principal = bytes.ReplaceAll(principal, []byte(a.Original), []byte(a.Reemplazo))
			}
			fsys = fstest.MapFS{
				archivo:                   {Data: principal},
				"xmldsig-core-schema.xsd": {Data: dsig},
			}
		}
		c.esquema, c.err = xsd.Load(fsys, archivo)
	})
	return c.esquema, c.err
}

// Identificar devuelve el tipo y la versión que declara un comprobante
// (elemento raíz y atributo version) sin validarlo.
func Identificar(doc []byte) (Tipo, string, error) {
	d, err := xmldom.Analizar(doc)
	if err != nil {
		return "", "", err
	}
	version, _ := d.Raiz.Attr("version")
	return Tipo(d.Raiz.Local), version, nil
}

// Validar comprueba un comprobante, firmado o no, contra el XSD oficial que
// corresponde a su elemento raíz y a su atributo version, con los Ajustes
// documentados. Devuelve nil si es válido, *ErrorValidacion si no cumple el
// esquema, u otro error si el documento no es XML bien formado o su tipo o
// versión no tienen esquema.
func Validar(doc []byte) error { return validar(doc, false) }

// ValidarEstricto es como Validar pero usa los XSD exactamente como los
// publica el SRI, sin Ajustes.
func ValidarEstricto(doc []byte) error { return validar(doc, true) }

func validar(doc []byte, estricto bool) error {
	tipo, version, err := Identificar(doc)
	if err != nil {
		return err
	}
	archivo, ok := catalogo[clave{tipo, version}]
	if !ok {
		conocido := false
		for k := range catalogo {
			if k.tipo == tipo {
				conocido = true
			}
		}
		if !conocido {
			return fmt.Errorf("%w: <%s>", ErrTipoDesconocido, tipo)
		}
		return fmt.Errorf("%w: %s versión %q (disponibles: %s)", ErrVersionDesconocida, tipo, version,
			strings.Join(Versiones(tipo), ", "))
	}
	s, err := cargar(archivo, estricto)
	if err != nil {
		return fmt.Errorf("esquema: no se pudo cargar %s: %w", archivo, err)
	}
	err = s.Validate(doc)
	if err == nil {
		return nil
	}
	var pIssues *xsd.Errors
	if !errors.As(err, &pIssues) {
		return fmt.Errorf("esquema: %w", err)
	}
	issues := *pIssues
	ev := &ErrorValidacion{Tipo: tipo, Version: version}
	for _, i := range issues {
		ev.Problemas = append(ev.Problemas, Problema{Linea: i.Line, Ruta: i.Path, Mensaje: i.Msg})
	}
	return ev
}

// Versiones devuelve, en orden, las versiones con XSD para un tipo.
func Versiones(t Tipo) []string {
	var vs []string
	for k := range catalogo {
		if k.tipo == t {
			vs = append(vs, k.version)
		}
	}
	sort.Strings(vs)
	return vs
}
