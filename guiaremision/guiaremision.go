// Package guiaremision modela la guía de remisión electrónica del SRI
// (codDoc 06), versiones 1.0.0 y 1.1.0: estructura tipada, serialización y
// lectura de XML, clave de acceso y validación.
//
// Una guía de remisión sustenta el traslado de mercadería: indica quién
// transporta, desde dónde, en qué fechas y, por cada destinatario, qué se
// lleva y con qué comprobante de venta se relaciona. No tiene importes ni
// impuestos.
//
// La guía no tiene un campo de fecha de emisión: la fecha que va en la
// clave de acceso hace de fecha de emisión, y el SRI exige que no sea
// posterior al inicio del transporte (error 82). GenerarClaveAcceso usa la
// fecha de inicio del transporte, como hace el ejemplo de la ficha técnica;
// GenerarClaveAccesoEn permite emitir la guía antes del día del traslado.
//
// Referencia: ficha técnica v2.34, Anexo 1 y Anexo 3 (formato XML guía de
// remisión).
package guiaremision

import (
	"encoding/xml"
	"fmt"
	"regexp"

	"github.com/OsirisRoman/sri-ecuador-go/clave"
	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
)

// Versiones del esquema de guía de remisión.
const (
	// V100 admite cantidades con 2 decimales.
	V100 = "1.0.0"
	// V110 admite cantidades con hasta 6 decimales.
	V110 = "1.1.0"
	// VersionPorDefecto es la que se usa cuando Version está vacío.
	VersionPorDefecto = V110
)

// GuiaRemision es el comprobante <guiaRemision>.
type GuiaRemision struct {
	XMLName xml.Name `xml:"guiaRemision"`
	ID      string   `xml:"id,attr"`
	Version string   `xml:"version,attr"`

	InfoTributaria   comprobante.InfoTributaria                    `xml:"infoTributaria"`
	InfoGuiaRemision InfoGuiaRemision                              `xml:"infoGuiaRemision"`
	Destinatarios    comprobante.Lista[Destinatario]               `xml:"destinatarios"`
	MaquinaFiscal    *comprobante.MaquinaFiscal                    `xml:"maquinaFiscal,omitempty"`
	InfoAdicional    comprobante.Lista[comprobante.CampoAdicional] `xml:"infoAdicional,omitempty"`
}

// InfoGuiaRemision es el bloque <infoGuiaRemision>.
type InfoGuiaRemision struct {
	DirEstablecimiento string `xml:"dirEstablecimiento,omitempty"`
	// DirPartida es la dirección desde donde sale la mercadería.
	DirPartida               string `xml:"dirPartida"`
	RazonSocialTransportista string `xml:"razonSocialTransportista"`
	// TipoIdentificacionTransportista es un código de la Tabla 6 (04 RUC,
	// 05 cédula, 06 pasaporte, 08 identificación del exterior).
	TipoIdentificacionTransportista string `xml:"tipoIdentificacionTransportista"`
	// RucTransportista es la identificación del transportista, del tipo
	// indicado; a pesar del nombre de la etiqueta no tiene que ser un RUC.
	RucTransportista      string            `xml:"rucTransportista"`
	Rise                  string            `xml:"rise,omitempty"`
	ObligadoContabilidad  string            `xml:"obligadoContabilidad,omitempty"`
	ContribuyenteEspecial string            `xml:"contribuyenteEspecial,omitempty"`
	FechaIniTransporte    comprobante.Fecha `xml:"fechaIniTransporte"`
	FechaFinTransporte    comprobante.Fecha `xml:"fechaFinTransporte"`
	Placa                 string            `xml:"placa"`
}

// Destinatario es un <destinatario>: a quién se entrega, por qué motivo y
// qué mercadería.
type Destinatario struct {
	IdentificacionDestinatario string `xml:"identificacionDestinatario"`
	RazonSocialDestinatario    string `xml:"razonSocialDestinatario"`
	DirDestinatario            string `xml:"dirDestinatario"`
	MotivoTraslado             string `xml:"motivoTraslado"`
	// DocAduaneroUnico se indica en importaciones y exportaciones.
	DocAduaneroUnico string `xml:"docAduaneroUnico,omitempty"`
	// CodEstabDestino es el establecimiento del destinatario (3 dígitos),
	// en traslados entre establecimientos.
	CodEstabDestino string `xml:"codEstabDestino,omitempty"`
	Ruta            string `xml:"ruta,omitempty"`
	// Comprobante de venta que sustenta el traslado, si existe. Los cuatro
	// campos van juntos; use Sustento para completarlos desde una factura.
	CodDocSustento          string             `xml:"codDocSustento,omitempty"`
	NumDocSustento          string             `xml:"numDocSustento,omitempty"`
	NumAutDocSustento       string             `xml:"numAutDocSustento,omitempty"`
	FechaEmisionDocSustento *comprobante.Fecha `xml:"fechaEmisionDocSustento,omitempty"`

	Detalles comprobante.Lista[Detalle] `xml:"detalles"`
}

// EtiquetaXML implementa comprobante.Etiquetado.
func (Destinatario) EtiquetaXML() string { return "destinatario" }

// Sustento relaciona el traslado con un comprobante de venta autorizado:
// tipo de documento (Tabla 3), número 001-001-000000001, número de
// autorización (la clave de acceso, en comprobantes electrónicos) y fecha
// de emisión.
func (d *Destinatario) Sustento(codDoc, numero, autorizacion string, fecha comprobante.Fecha) {
	d.CodDocSustento, d.NumDocSustento, d.NumAutDocSustento = codDoc, numero, autorizacion
	d.FechaEmisionDocSustento = &fecha
}

// Detalle es un <detalle>: una mercadería transportada.
type Detalle struct {
	CodigoInterno       string                                      `xml:"codigoInterno,omitempty"`
	CodigoAdicional     string                                      `xml:"codigoAdicional,omitempty"`
	Descripcion         string                                      `xml:"descripcion"`
	Cantidad            decimal.Decimal                             `xml:"cantidad"`
	DetallesAdicionales comprobante.Lista[comprobante.DetAdicional] `xml:"detallesAdicionales,omitempty"`
}

// EtiquetaXML implementa comprobante.Etiquetado.
func (Detalle) EtiquetaXML() string { return "detalle" }

// Info devuelve el bloque infoTributaria.
func (g *GuiaRemision) Info() comprobante.InfoTributaria { return g.InfoTributaria }

func (g *GuiaRemision) version() string {
	if g.Version == "" {
		return VersionPorDefecto
	}
	return g.Version
}

// XML serializa la guía como documento XML UTF-8 listo para firmar.
func (g *GuiaRemision) XML() ([]byte, error) {
	copia := *g
	copia.XMLName = xml.Name{Local: "guiaRemision"}
	copia.ID = "comprobante"
	copia.Version = g.version()
	return comprobante.Serializar(&copia)
}

// Parse lee una guía de remisión desde su XML, firmado o no.
func Parse(doc []byte) (*GuiaRemision, error) {
	var g GuiaRemision
	if err := xml.Unmarshal(doc, &g); err != nil {
		return nil, fmt.Errorf("guiaremision: XML ilegible: %w", err)
	}
	return &g, nil
}

// GenerarClaveAcceso compone la clave de acceso con la fecha de inicio del
// transporte como fecha de emisión y la guarda en
// InfoTributaria.ClaveAcceso. Si codigoNumerico está vacío se genera uno
// aleatorio.
func (g *GuiaRemision) GenerarClaveAcceso(codigoNumerico string) (string, error) {
	return g.GenerarClaveAccesoEn(g.InfoGuiaRemision.FechaIniTransporte, codigoNumerico)
}

// GenerarClaveAccesoEn es GenerarClaveAcceso con una fecha de emisión
// explícita, para guías que se emiten antes del día del traslado. La fecha
// no puede ser posterior al inicio del transporte (error 82 del SRI).
func (g *GuiaRemision) GenerarClaveAccesoEn(emision comprobante.Fecha, codigoNumerico string) (string, error) {
	return comprobante.GenerarClaveAcceso(&g.InfoTributaria, emision, comprobante.CodGuiaRemision, codigoNumerico)
}

// FechaEmision devuelve la fecha de emisión de la guía, que es la que
// figura en su clave de acceso; es la fecha cero si la clave falta o es
// inválida.
func (g *GuiaRemision) FechaEmision() comprobante.Fecha {
	c, err := clave.Analizar(g.InfoTributaria.ClaveAcceso)
	if err != nil {
		return comprobante.Fecha{}
	}
	a, m, d := c.FechaEmision.Date()
	return comprobante.NuevaFecha(a, m, d)
}

var (
	reNumDoc = regexp.MustCompile(`^[0-9]{3}-[0-9]{3}-[0-9]{9}$`)
	reNumAut = regexp.MustCompile(`^[0-9]{10,49}$`)
)

// OpcionesValidacion ajusta ValidarCon.
type OpcionesValidacion struct {
	// Hoy es la fecha de referencia; por defecto, la actual en Ecuador.
	Hoy comprobante.Fecha
}

// Validar aplica las reglas que se pueden comprobar sin consultar al SRI y
// devuelve todos los hallazgos.
func (g *GuiaRemision) Validar() comprobante.Hallazgos { return g.ValidarCon(OpcionesValidacion{}) }

// ValidarCon es Validar con opciones.
func (g *GuiaRemision) ValidarCon(op OpcionesValidacion) comprobante.Hallazgos {
	v := &comprobante.Validador{}
	hoy := op.Hoy
	if hoy.EsCero() {
		hoy = comprobante.Hoy()
	}
	decCantidad := 6
	switch g.version() {
	case V100:
		decCantidad = 2
	case V110:
	default:
		v.Errorf("version", "version", "versión %q desconocida; existen %s y %s", g.Version, V100, V110)
	}
	v.InfoTributaria(g.InfoTributaria, comprobante.CodGuiaRemision)
	info := &g.InfoGuiaRemision
	const p = "infoGuiaRemision."

	// --- Fechas y clave de acceso --------------------------------------------
	switch {
	case info.FechaIniTransporte.EsCero():
		v.Errorf(p+"fechaIniTransporte", "requerido", "es obligatoria")
	case info.FechaFinTransporte.EsCero():
		v.Errorf(p+"fechaFinTransporte", "requerido", "es obligatoria")
	case info.FechaFinTransporte.Time().Before(info.FechaIniTransporte.Time()):
		v.Errorf(p+"fechaFinTransporte", "fecha.transporte", "el transporte termina (%s) antes de empezar (%s)",
			info.FechaFinTransporte, info.FechaIniTransporte)
	}
	v.ClaveAcceso(g.InfoTributaria, comprobante.Fecha{})
	if em := g.FechaEmision(); !em.EsCero() {
		if em.Time().After(hoy.Time()) {
			v.Errorf("infoTributaria.claveAcceso", "fecha.futura", "la fecha de emisión de la clave (%s) es posterior a la fecha actual en Ecuador (%s); use GenerarClaveAccesoEn para emitir hoy una guía de un traslado futuro", em, hoy)
		}
		if !info.FechaIniTransporte.EsCero() && info.FechaIniTransporte.Time().Before(em.Time()) {
			v.Errorf(p+"fechaIniTransporte", "fecha.inicio-transporte", "el transporte empieza (%s) antes de la fecha de emisión de la clave de acceso (%s); el SRI lo rechaza con el error 82",
				info.FechaIniTransporte, em)
		}
	}

	// --- Emisor y transportista ----------------------------------------------
	v.Emisor(p, info.DirEstablecimiento, info.ContribuyenteEspecial, info.ObligadoContabilidad)
	v.Texto(p+"rise", info.Rise, false, 40)
	v.Texto(p+"dirPartida", info.DirPartida, true, 300)
	v.Texto(p+"razonSocialTransportista", info.RazonSocialTransportista, true, 300)
	v.Identificacion(p+"tipoIdentificacionTransportista", p+"rucTransportista",
		info.TipoIdentificacionTransportista, info.RucTransportista, false)
	v.Texto(p+"placa", info.Placa, true, 20)

	// --- Destinatarios --------------------------------------------------------
	if len(g.Destinatarios) == 0 {
		v.Errorf("destinatarios", "requerido", "la guía debe tener al menos un destinatario")
	}
	for i := range g.Destinatarios {
		d := &g.Destinatarios[i]
		q := fmt.Sprintf("destinatarios[%d].", i)
		v.Texto(q+"identificacionDestinatario", d.IdentificacionDestinatario, true, 20)
		v.Texto(q+"razonSocialDestinatario", d.RazonSocialDestinatario, true, 300)
		v.Texto(q+"dirDestinatario", d.DirDestinatario, true, 300)
		v.Texto(q+"motivoTraslado", d.MotivoTraslado, true, 300)
		v.Texto(q+"docAduaneroUnico", d.DocAduaneroUnico, false, 20)
		v.Texto(q+"ruta", d.Ruta, false, 300)
		if d.CodEstabDestino != "" {
			v.Digitos(q+"codEstabDestino", d.CodEstabDestino, 3)
		}
		if d.CodDocSustento != "" && (len(d.CodDocSustento) != 2 || !comprobante.SoloDigitos(d.CodDocSustento)) {
			v.Errorf(q+"codDocSustento", "formato", "debe ser un código de 2 dígitos (01 para factura)")
		}
		if d.NumDocSustento != "" && !reNumDoc.MatchString(d.NumDocSustento) {
			v.Errorf(q+"numDocSustento", "formato", "debe tener el formato 001-001-000000001")
		}
		if d.NumAutDocSustento != "" && !reNumAut.MatchString(d.NumAutDocSustento) {
			v.Errorf(q+"numAutDocSustento", "formato", "debe tener de 10 a 49 dígitos (la clave de acceso, en comprobantes electrónicos)")
		}
		if f := d.FechaEmisionDocSustento; f != nil && !f.EsCero() && f.Time().After(hoy.Time()) {
			v.Errorf(q+"fechaEmisionDocSustento", "fecha.futura", "%s es posterior a la fecha actual en Ecuador (%s)", f, hoy)
		}
		n := 0
		for _, presente := range []bool{d.CodDocSustento != "", d.NumDocSustento != "", d.NumAutDocSustento != "", d.FechaEmisionDocSustento != nil} {
			if presente {
				n++
			}
		}
		if n != 0 && n != 4 {
			v.Advertirf(q+"codDocSustento", "sustento.incompleto", "el comprobante de sustento está incompleto: indique tipo, número, autorización y fecha de emisión")
		}
		if len(d.Detalles) == 0 {
			v.Errorf(q+"detalles", "requerido", "el destinatario debe tener al menos un detalle")
		}
		for j, det := range d.Detalles {
			r := fmt.Sprintf("%sdetalles[%d].", q, j)
			v.Texto(r+"codigoInterno", det.CodigoInterno, false, 25)
			v.Texto(r+"codigoAdicional", det.CodigoAdicional, false, 25)
			v.Texto(r+"descripcion", det.Descripcion, true, 300)
			v.Numero(r+"cantidad", det.Cantidad, 18, decCantidad)
			if det.Cantidad.EsCero() {
				v.Advertirf(r+"cantidad", "cantidad.cero", "la cantidad transportada es 0")
			}
			v.DetallesAdicionales(r, det.DetallesAdicionales)
		}
	}
	v.Adicionales(g.InfoAdicional, g.MaquinaFiscal)
	return v.Hallazgos()
}
