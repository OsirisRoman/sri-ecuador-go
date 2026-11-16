// Package retencion modela el comprobante de retención electrónico del SRI
// (codDoc 07) en su versión 2.0.0, la "versión ATS", que incorpora la
// información del módulo de compras del Anexo Transaccional Simplificado.
//
// Un comprobante de retención agrupa uno o varios documentos de sustento
// (las facturas del proveedor); por cada uno se declaran sus impuestos, las
// retenciones practicadas (renta, IVA o ISD) y las formas de pago.
//
// Referencia: ficha técnica v2.34, Anexo 10 (formato XML de comprobante de
// retención ATS versión 2.0.0) y Tablas 19 a 23. Los códigos de sustento,
// tipo de documento, tipo de pago y país pertenecen al catálogo del ATS, que
// el SRI publica aparte; este paquete valida su forma, no su vigencia.
//
// La versión 1.0.0 del esquema no se modela: su XSD está incrustado en el
// paquete esquema y puede validarse con él.
package retencion

import (
	"encoding/xml"
	"fmt"
	"maps"
	"regexp"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
)

// Version es la versión del esquema que modela este paquete.
const Version = "2.0.0"

// Códigos de impuesto a retener (Tabla 19).
const (
	ImpuestoRenta = "1"
	ImpuestoIVA   = "2"
	ImpuestoISD   = "6"
)

// PorcentajesIVA es la tabla de retención de IVA (Tabla 20 de la ficha
// técnica v2.34): código de retención → porcentaje. Los códigos 7 y 8
// corresponden a "retención en cero" y "no procede retención".
//
// Los porcentajes de retención de renta están en ConceptosRenta, tomados del
// Catálogo ATS; los de ISD no se incluyen.
var PorcentajesIVA = map[string]decimal.Decimal{
	"9":  decimal.Entero(10),
	"10": decimal.Entero(20),
	"1":  decimal.Entero(30),
	"11": decimal.Entero(50),
	"2":  decimal.Entero(70),
	"3":  decimal.Entero(100),
	"7":  decimal.Entero(0),
	"8":  decimal.Entero(0),
}

// porcentajesIVA es la copia que usa la validación; ver comprobante.TarifaIVADe.
var porcentajesIVA = maps.Clone(PorcentajesIVA)

// Tipo de pago (tabla 15 del catálogo ATS).
const (
	PagoLocal    = "01"
	PagoExterior = "02"
)

// ComprobanteRetencion es el comprobante <comprobanteRetencion>.
type ComprobanteRetencion struct {
	XMLName xml.Name `xml:"comprobanteRetencion"`
	ID      string   `xml:"id,attr"`
	Version string   `xml:"version,attr"`

	InfoTributaria    comprobante.InfoTributaria                    `xml:"infoTributaria"`
	InfoCompRetencion InfoCompRetencion                             `xml:"infoCompRetencion"`
	DocsSustento      comprobante.Lista[DocSustento]                `xml:"docsSustento"`
	MaquinaFiscal     *comprobante.MaquinaFiscal                    `xml:"maquinaFiscal,omitempty"`
	InfoAdicional     comprobante.Lista[comprobante.CampoAdicional] `xml:"infoAdicional,omitempty"`
}

// InfoCompRetencion es el bloque <infoCompRetencion>.
type InfoCompRetencion struct {
	FechaEmision          comprobante.Fecha `xml:"fechaEmision"`
	DirEstablecimiento    string            `xml:"dirEstablecimiento,omitempty"`
	ContribuyenteEspecial string            `xml:"contribuyenteEspecial,omitempty"`
	ObligadoContabilidad  string            `xml:"obligadoContabilidad,omitempty"`
	// El sujeto retenido debe estar identificado (Tabla 6, sin consumidor
	// final).
	TipoIdentificacionSujetoRetenido string `xml:"tipoIdentificacionSujetoRetenido"`
	// TipoSujetoRetenido ("01" persona natural, "02" sociedad) es
	// obligatorio cuando la identificación es del exterior.
	TipoSujetoRetenido string `xml:"tipoSujetoRetenido,omitempty"`
	// ParteRel indica si el sujeto retenido es parte relacionada: "SI" o
	// "NO".
	ParteRel                     string `xml:"parteRel"`
	RazonSocialSujetoRetenido    string `xml:"razonSocialSujetoRetenido"`
	IdentificacionSujetoRetenido string `xml:"identificacionSujetoRetenido"`
	// PeriodoFiscal con formato mm/aaaa.
	PeriodoFiscal string `xml:"periodoFiscal"`
}

// DocSustento es un <docSustento>: un documento del proveedor y las
// retenciones practicadas sobre él.
type DocSustento struct {
	// CodSustento es el código de sustento tributario (tabla 5 del catálogo
	// ATS), por ejemplo "01" crédito tributario para declaración de IVA.
	CodSustento string `xml:"codSustento"`
	// CodDocSustento es el tipo de comprobante (tabla 4 del catálogo ATS),
	// por ejemplo "01" factura.
	CodDocSustento string `xml:"codDocSustento"`
	// NumDocSustento son 15 dígitos sin guiones: establecimiento, punto de
	// emisión y secuencial.
	NumDocSustento          string             `xml:"numDocSustento"`
	FechaEmisionDocSustento comprobante.Fecha  `xml:"fechaEmisionDocSustento"`
	FechaRegistroContable   *comprobante.Fecha `xml:"fechaRegistroContable,omitempty"`
	// NumAutDocSustento es la autorización del documento (10, 37 o 49
	// dígitos).
	NumAutDocSustento string `xml:"numAutDocSustento,omitempty"`
	// PagoLocExt es PagoLocal o PagoExterior. Los cinco campos siguientes
	// solo aplican a pagos al exterior.
	PagoLocExt         string `xml:"pagoLocExt"`
	TipoRegi           string `xml:"tipoRegi,omitempty"`
	PaisEfecPago       string `xml:"paisEfecPago,omitempty"`
	AplicConvDobTrib   string `xml:"aplicConvDobTrib,omitempty"`
	PagExtSujRetNorLeg string `xml:"pagExtSujRetNorLeg,omitempty"`
	PagoRegFis         string `xml:"pagoRegFis,omitempty"`

	// Totales de reembolso; obligatorios si CodDocSustento es "41".
	TotalComprobantesReembolso  *decimal.Decimal `xml:"totalComprobantesReembolso,omitempty"`
	TotalBaseImponibleReembolso *decimal.Decimal `xml:"totalBaseImponibleReembolso,omitempty"`
	TotalImpuestoReembolso      *decimal.Decimal `xml:"totalImpuestoReembolso,omitempty"`

	TotalSinImpuestos    decimal.Decimal                        `xml:"totalSinImpuestos"`
	ImporteTotal         decimal.Decimal                        `xml:"importeTotal"`
	ImpuestosDocSustento comprobante.Lista[ImpuestoDocSustento] `xml:"impuestosDocSustento"`
	Retenciones          comprobante.Lista[Retencion]           `xml:"retenciones"`
	Reembolsos           comprobante.Lista[ReembolsoDetalle]    `xml:"reembolsos,omitempty"`
	Pagos                comprobante.Lista[Pago]                `xml:"pagos"`
}

// EtiquetaXML implementa comprobante.Etiquetado.
func (DocSustento) EtiquetaXML() string { return "docSustento" }

// ImpuestoDocSustento es un impuesto del documento de sustento.
type ImpuestoDocSustento struct {
	// CodImpuestoDocSustento es un código de la Tabla 16 (2 IVA, 3 ICE,
	// 5 IRBPNR).
	CodImpuestoDocSustento string          `xml:"codImpuestoDocSustento"`
	CodigoPorcentaje       string          `xml:"codigoPorcentaje"`
	BaseImponible          decimal.Decimal `xml:"baseImponible"`
	// Tarifa admite como máximo 3 dígitos en total: escriba 15, no 15.00.
	Tarifa        decimal.Decimal `xml:"tarifa"`
	ValorImpuesto decimal.Decimal `xml:"valorImpuesto"`
}

// EtiquetaXML implementa comprobante.Etiquetado.
func (ImpuestoDocSustento) EtiquetaXML() string { return "impuestoDocSustento" }

// Retencion es una <retencion> practicada sobre un documento de sustento.
type Retencion struct {
	// Codigo es ImpuestoRenta, ImpuestoIVA o ImpuestoISD (Tabla 19).
	Codigo string `xml:"codigo"`
	// CodigoRetencion es el código del concepto (Tablas 20 a 23): por
	// ejemplo "1" (IVA 30 %), "2" (IVA 70 %), "3" (IVA 100 %) o un código
	// del formulario de renta como "312".
	CodigoRetencion string          `xml:"codigoRetencion"`
	BaseImponible   decimal.Decimal `xml:"baseImponible"`
	// PorcentajeRetener admite como máximo 3 dígitos en total (1.75, 30,
	// 100).
	PorcentajeRetener decimal.Decimal  `xml:"porcentajeRetener"`
	ValorRetenido     decimal.Decimal  `xml:"valorRetenido"`
	Dividendos        *Dividendos      `xml:"dividendos,omitempty"`
	CompraCajBanano   *CompraCajBanano `xml:"compraCajBanano,omitempty"`
}

// EtiquetaXML implementa comprobante.Etiquetado.
func (Retencion) EtiquetaXML() string { return "retencion" }

// Dividendos acompaña a las retenciones por distribución de dividendos.
type Dividendos struct {
	FechaPagoDiv comprobante.Fecha `xml:"fechaPagoDiv"`
	ImRentaSoc   decimal.Decimal   `xml:"imRentaSoc"`
	// EjerFisUtDiv es el ejercicio fiscal (aaaa) de las utilidades.
	EjerFisUtDiv string `xml:"ejerFisUtDiv"`
}

// CompraCajBanano acompaña a las retenciones en compras de banano.
type CompraCajBanano struct {
	NumCajBan  string          `xml:"numCajBan"`
	PrecCajBan decimal.Decimal `xml:"precCajBan"`
}

// Pago es una forma de pago del documento de sustento (sin plazo).
type Pago struct {
	FormaPago string          `xml:"formaPago"`
	Total     decimal.Decimal `xml:"total"`
}

// EtiquetaXML implementa comprobante.Etiquetado.
func (Pago) EtiquetaXML() string { return "pago" }

// ReembolsoDetalle es un comprobante de reembolso asociado a un documento
// de sustento de tipo 41.
type ReembolsoDetalle struct {
	TipoIdentificacionProveedorReembolso string                               `xml:"tipoIdentificacionProveedorReembolso"`
	IdentificacionProveedorReembolso     string                               `xml:"identificacionProveedorReembolso"`
	CodPaisPagoProveedorReembolso        string                               `xml:"codPaisPagoProveedorReembolso,omitempty"`
	TipoProveedorReembolso               string                               `xml:"tipoProveedorReembolso"`
	CodDocReembolso                      string                               `xml:"codDocReembolso"`
	EstabDocReembolso                    string                               `xml:"estabDocReembolso"`
	PtoEmiDocReembolso                   string                               `xml:"ptoEmiDocReembolso"`
	SecuencialDocReembolso               string                               `xml:"secuencialDocReembolso"`
	FechaEmisionDocReembolso             comprobante.Fecha                    `xml:"fechaEmisionDocReembolso"`
	NumeroAutorizacionDocReemb           string                               `xml:"numeroAutorizacionDocReemb"`
	DetalleImpuestos                     comprobante.Lista[ImpuestoReembolso] `xml:"detalleImpuestos"`
}

// EtiquetaXML implementa comprobante.Etiquetado.
func (ReembolsoDetalle) EtiquetaXML() string { return "reembolsoDetalle" }

// ImpuestoReembolso es un <detalleImpuesto> de un reembolso.
type ImpuestoReembolso struct {
	Codigo                 string          `xml:"codigo"`
	CodigoPorcentaje       string          `xml:"codigoPorcentaje"`
	Tarifa                 string          `xml:"tarifa"`
	BaseImponibleReembolso decimal.Decimal `xml:"baseImponibleReembolso"`
	ImpuestoReembolso      decimal.Decimal `xml:"impuestoReembolso"`
}

// EtiquetaXML implementa comprobante.Etiquetado.
func (ImpuestoReembolso) EtiquetaXML() string { return "detalleImpuesto" }

// Info devuelve el bloque infoTributaria.
func (c *ComprobanteRetencion) Info() comprobante.InfoTributaria { return c.InfoTributaria }

// XML serializa el comprobante como documento XML UTF-8 listo para firmar.
func (c *ComprobanteRetencion) XML() ([]byte, error) {
	copia := *c
	copia.XMLName = xml.Name{Local: "comprobanteRetencion"}
	copia.ID = "comprobante"
	if copia.Version == "" {
		copia.Version = Version
	}
	return comprobante.Serializar(&copia)
}

// Parse lee un comprobante de retención v2.0.0 desde su XML, firmado o no.
func Parse(doc []byte) (*ComprobanteRetencion, error) {
	var c ComprobanteRetencion
	if err := xml.Unmarshal(doc, &c); err != nil {
		return nil, fmt.Errorf("retencion: XML ilegible: %w", err)
	}
	return &c, nil
}

// Calcular completa los importes derivados:
//
//   - Impuestos del documento de sustento: para el IVA, la tarifa de la
//     Tabla 17 y el valor (base × tarifa).
//   - importeTotal de cada documento: totalSinImpuestos + impuestos.
//   - Retenciones: valorRetenido = baseImponible × porcentajeRetener / 100.
//   - Pagos: si hay un único pago sin total, se le asigna el importe total.
//   - periodoFiscal: si está vacío, el mes de la fecha de emisión.
func (c *ComprobanteRetencion) Calcular() {
	for i := range c.DocsSustento {
		d := &c.DocsSustento[i]
		total := d.TotalSinImpuestos
		for j := range d.ImpuestosDocSustento {
			imp := &d.ImpuestosDocSustento[j]
			// La base se redondea antes de calcular el impuesto: Validar
			// recalcula sobre la base escrita en el XML.
			imp.BaseImponible = imp.BaseImponible.Redondear(2)
			if imp.CodImpuestoDocSustento == comprobante.ImpuestoIVA {
				if t, ok := comprobante.TarifaIVADe(imp.CodigoPorcentaje); ok && t.Fija {
					imp.Tarifa = t.Tarifa
				}
				imp.ValorImpuesto = imp.BaseImponible.Porcentaje(imp.Tarifa)
			}
			imp.ValorImpuesto = imp.ValorImpuesto.Redondear(2)
			total = total.Sumar(imp.ValorImpuesto)
		}
		d.TotalSinImpuestos = d.TotalSinImpuestos.Redondear(2)
		d.ImporteTotal = total.Redondear(2)
		for j := range d.Retenciones {
			r := &d.Retenciones[j]
			r.BaseImponible = r.BaseImponible.Redondear(2)
			r.ValorRetenido = r.BaseImponible.Porcentaje(r.PorcentajeRetener).Redondear(2)
		}
		if len(d.Pagos) == 1 && d.Pagos[0].Total.EsCero() {
			d.Pagos[0].Total = d.ImporteTotal
		}
	}
	if c.InfoCompRetencion.PeriodoFiscal == "" && !c.InfoCompRetencion.FechaEmision.EsCero() {
		c.InfoCompRetencion.PeriodoFiscal = c.InfoCompRetencion.FechaEmision.String()[3:]
	}
}

// TotalRetenido devuelve la suma de los valores retenidos.
func (c *ComprobanteRetencion) TotalRetenido() decimal.Decimal {
	t := decimal.Cero
	for _, d := range c.DocsSustento {
		for _, r := range d.Retenciones {
			t = t.Sumar(r.ValorRetenido)
		}
	}
	return t.Redondear(2)
}

// GenerarClaveAcceso compone la clave de acceso y la guarda en
// InfoTributaria.ClaveAcceso. Ver factura.Factura.GenerarClaveAcceso.
func (c *ComprobanteRetencion) GenerarClaveAcceso(codigoNumerico string) (string, error) {
	return comprobante.GenerarClaveAcceso(&c.InfoTributaria, c.InfoCompRetencion.FechaEmision, comprobante.CodComprobanteRetencion, codigoNumerico)
}

var (
	rePeriodo      = regexp.MustCompile(`^(0[1-9]|1[012])/20[0-9][0-9]$`)
	reContribEsp   = regexp.MustCompile(`^[A-Za-z0-9]{3,5}$`)
	reNumDoc       = regexp.MustCompile(`^[0-9]{15}$`)
	reNumAut       = regexp.MustCompile(`^[0-9]{10,49}$`)
	reCodDoc       = regexp.MustCompile(`^[0-9]{1,3}$`)
	reDosDigitos   = regexp.MustCompile(`^[0-9]{2}$`)
	rePais         = regexp.MustCompile(`^[0-9]{3,4}$`)
	reFormaPagoXSD = regexp.MustCompile(`^(0[1-9]|1[0-9]|2[01])$`)
)

// OpcionesValidacion ajusta ValidarCon.
type OpcionesValidacion struct {
	// Hoy es la fecha de referencia; por defecto, la actual en Ecuador.
	Hoy comprobante.Fecha
	// CatalogoRentaEstricto convierte en errores las discrepancias con el
	// catálogo de retenciones de renta incluido (código desconocido o
	// porcentaje no admitido), que por defecto son advertencias porque el
	// catálogo puede estar desactualizado. Ver ConceptoRenta.
	CatalogoRentaEstricto bool
}

// Validar aplica las reglas que se pueden comprobar sin consultar al SRI y
// devuelve todos los hallazgos.
func (c *ComprobanteRetencion) Validar() comprobante.Hallazgos {
	return c.ValidarCon(OpcionesValidacion{})
}

func siNo(v *comprobante.Validador, campo, valor string, obligatorio bool) {
	switch valor {
	case "SI", "NO":
	case "":
		if obligatorio {
			v.Errorf(campo, "requerido", `es obligatorio ("SI" o "NO")`)
		}
	default:
		v.Errorf(campo, "catalogo", `debe ser "SI" o "NO"`)
	}
}

// ValidarCon es Validar con opciones.
func (c *ComprobanteRetencion) ValidarCon(op OpcionesValidacion) comprobante.Hallazgos {
	v := &comprobante.Validador{}
	hoy := op.Hoy
	if hoy.EsCero() {
		hoy = comprobante.Hoy()
	}
	if c.Version != "" && c.Version != Version {
		v.Errorf("version", "version", "este paquete modela la versión %s; se recibió %q", Version, c.Version)
	}
	v.InfoTributaria(c.InfoTributaria, comprobante.CodComprobanteRetencion)
	info := &c.InfoCompRetencion
	const p = "infoCompRetencion."

	if info.FechaEmision.EsCero() {
		v.Errorf(p+"fechaEmision", "requerido", "es obligatoria")
	} else if info.FechaEmision.Time().After(hoy.Time()) {
		v.Errorf(p+"fechaEmision", "fecha.futura", "%s es posterior a la fecha actual en Ecuador (%s)", info.FechaEmision, hoy)
	}
	v.ClaveAcceso(c.InfoTributaria, info.FechaEmision)
	v.Texto(p+"dirEstablecimiento", info.DirEstablecimiento, false, 300)
	if info.ContribuyenteEspecial != "" && !reContribEsp.MatchString(info.ContribuyenteEspecial) {
		v.Errorf(p+"contribuyenteEspecial", "formato", "debe tener de 3 a 5 caracteres alfanuméricos en la versión 2.0.0")
	}
	siNo(v, p+"obligadoContabilidad", info.ObligadoContabilidad, false)
	v.Identificacion(p+"tipoIdentificacionSujetoRetenido", p+"identificacionSujetoRetenido",
		info.TipoIdentificacionSujetoRetenido, info.IdentificacionSujetoRetenido, false)
	v.Texto(p+"razonSocialSujetoRetenido", info.RazonSocialSujetoRetenido, true, 300)
	switch info.TipoSujetoRetenido {
	case "01", "02":
	case "":
		if info.TipoIdentificacionSujetoRetenido == comprobante.IDExterior {
			v.Errorf(p+"tipoSujetoRetenido", "requerido", "es obligatorio cuando la identificación es del exterior (01 persona natural, 02 sociedad)")
		}
	default:
		v.Errorf(p+"tipoSujetoRetenido", "catalogo", "debe ser 01 (persona natural) o 02 (sociedad)")
	}
	siNo(v, p+"parteRel", info.ParteRel, true)
	if !rePeriodo.MatchString(info.PeriodoFiscal) {
		v.Errorf(p+"periodoFiscal", "formato", "debe tener el formato mm/aaaa")
	} else if !info.FechaEmision.EsCero() && info.PeriodoFiscal != info.FechaEmision.String()[3:] {
		v.Advertirf(p+"periodoFiscal", "periodo", "el período fiscal (%s) no es el mes de la fecha de emisión (%s)", info.PeriodoFiscal, info.FechaEmision)
	}

	if len(c.DocsSustento) == 0 {
		v.Errorf("docsSustento", "requerido", "debe haber al menos un documento de sustento")
	}
	for i, d := range c.DocsSustento {
		q := fmt.Sprintf("docsSustento[%d].", i)
		if !reDosDigitos.MatchString(d.CodSustento) {
			v.Errorf(q+"codSustento", "formato", "debe ser un código de 2 dígitos (tabla 5 del catálogo ATS)")
		}
		if !reCodDoc.MatchString(d.CodDocSustento) {
			v.Errorf(q+"codDocSustento", "formato", "debe ser un código de 1 a 3 dígitos (tabla 4 del catálogo ATS)")
		}
		if !reNumDoc.MatchString(d.NumDocSustento) {
			v.Errorf(q+"numDocSustento", "formato", "deben ser 15 dígitos sin guiones (establecimiento, punto de emisión y secuencial)")
		}
		switch {
		case d.FechaEmisionDocSustento.EsCero():
			v.Errorf(q+"fechaEmisionDocSustento", "requerido", "es obligatoria")
		case !info.FechaEmision.EsCero() && d.FechaEmisionDocSustento.Time().After(info.FechaEmision.Time()):
			v.Errorf(q+"fechaEmisionDocSustento", "fecha.sustento", "el documento de sustento (%s) no puede ser posterior al comprobante de retención (%s)",
				d.FechaEmisionDocSustento, info.FechaEmision)
		}
		if d.NumAutDocSustento != "" && !reNumAut.MatchString(d.NumAutDocSustento) {
			v.Errorf(q+"numAutDocSustento", "formato", "debe tener de 10 a 49 dígitos")
		}
		switch d.PagoLocExt {
		case PagoLocal:
		case PagoExterior:
			if !reDosDigitos.MatchString(d.TipoRegi) {
				v.Errorf(q+"tipoRegi", "requerido", "es obligatorio en pagos al exterior (tabla 19 del catálogo ATS)")
			}
			if !rePais.MatchString(d.PaisEfecPago) {
				v.Errorf(q+"paisEfecPago", "requerido", "es obligatorio en pagos al exterior (código de país de 3 o 4 dígitos)")
			}
			siNo(v, q+"aplicConvDobTrib", d.AplicConvDobTrib, true)
			siNo(v, q+"pagExtSujRetNorLeg", d.PagExtSujRetNorLeg, d.AplicConvDobTrib == "NO")
			siNo(v, q+"pagoRegFis", d.PagoRegFis, true)
		default:
			v.Errorf(q+"pagoLocExt", "catalogo", "debe ser 01 (pago local) o 02 (pago al exterior)")
		}
		if d.CodDocSustento == "41" && (d.TotalComprobantesReembolso == nil || d.TotalBaseImponibleReembolso == nil || d.TotalImpuestoReembolso == nil || len(d.Reembolsos) == 0) {
			v.Errorf(q+"reembolsos", "requerido", "un documento de sustento de tipo 41 exige los totales y el detalle de reembolsos")
		}

		v.Numero(q+"totalSinImpuestos", d.TotalSinImpuestos, 14, 2)
		v.Numero(q+"importeTotal", d.ImporteTotal, 14, 2)
		if len(d.ImpuestosDocSustento) == 0 {
			v.Errorf(q+"impuestosDocSustento", "requerido", "debe declarar al menos un impuesto del documento de sustento")
		}
		esperado := d.TotalSinImpuestos
		for j, imp := range d.ImpuestosDocSustento {
			r := fmt.Sprintf("%simpuestosDocSustento[%d].", q, j)
			switch imp.CodImpuestoDocSustento {
			case comprobante.ImpuestoIVA:
				if t, ok := comprobante.TarifaIVADe(imp.CodigoPorcentaje); !ok {
					v.Errorf(r+"codigoPorcentaje", "catalogo", "código de IVA %q desconocido (Tabla 17)", imp.CodigoPorcentaje)
				} else if t.Fija && !imp.Tarifa.Igual(t.Tarifa) {
					v.Errorf(r+"tarifa", "iva.tarifa", "el código %s corresponde a %s; la tarifa declarada es %s", t.Codigo, t.Descripcion, imp.Tarifa)
				}
				if exacto := imp.BaseImponible.Porcentaje(imp.Tarifa); !comprobante.ImporteRedondeado(imp.ValorImpuesto, exacto) {
					v.Errorf(r+"valorImpuesto", "iva.valor", "es %s pero base × tarifa = %s", imp.ValorImpuesto, exacto.Redondear(2))
				}
			case comprobante.ImpuestoICE, comprobante.ImpuestoIRBPNR:
			default:
				v.Errorf(r+"codImpuestoDocSustento", "catalogo", "código de impuesto %q desconocido (Tabla 16)", imp.CodImpuestoDocSustento)
			}
			v.Numero(r+"baseImponible", imp.BaseImponible, 14, 2)
			v.Numero(r+"tarifa", imp.Tarifa, 3, 2)
			v.Numero(r+"valorImpuesto", imp.ValorImpuesto, 14, 2)
			esperado = esperado.Sumar(imp.ValorImpuesto)
		}
		if esperado = esperado.Redondear(2); !d.ImporteTotal.Igual(esperado) {
			v.Errorf(q+"importeTotal", "totales.importe", "es %s pero totalSinImpuestos + impuestos = %s", d.ImporteTotal, esperado)
		}

		if len(d.Retenciones) == 0 {
			v.Errorf(q+"retenciones", "requerido", "debe haber al menos una retención por documento de sustento")
		}
		for j, ret := range d.Retenciones {
			r := fmt.Sprintf("%sretenciones[%d].", q, j)
			validarRetencion(v, r, ret.Codigo, ret.CodigoRetencion, ret.BaseImponible, ret.PorcentajeRetener, ret.ValorRetenido, info.FechaEmision, op)
			if dv := ret.Dividendos; dv != nil {
				if dv.FechaPagoDiv.EsCero() {
					v.Errorf(r+"dividendos.fechaPagoDiv", "requerido", "es obligatoria")
				}
				if len(dv.EjerFisUtDiv) != 4 || !comprobante.SoloDigitos(dv.EjerFisUtDiv) {
					v.Errorf(r+"dividendos.ejerFisUtDiv", "formato", "debe ser un año de 4 dígitos")
				}
			}
		}

		if len(d.Pagos) == 0 {
			v.Errorf(q+"pagos", "requerido", "debe indicar al menos una forma de pago (Tabla 24)")
		}
		suma := decimal.Cero
		for j, pg := range d.Pagos {
			r := fmt.Sprintf("%spagos[%d].", q, j)
			switch _, enTabla := comprobante.FormaPagoDe(pg.FormaPago); {
			case !reFormaPagoXSD.MatchString(pg.FormaPago):
				v.Errorf(r+"formaPago", "catalogo", "código %q inválido (Tabla 24)", pg.FormaPago)
			case !enTabla:
				v.Advertirf(r+"formaPago", "catalogo", "el código %q no figura entre las formas de pago vigentes de la Tabla 24", pg.FormaPago)
			}
			v.Numero(r+"total", pg.Total, 14, 2)
			suma = suma.Sumar(pg.Total)
		}
		if len(d.Pagos) > 0 && !suma.Igual(d.ImporteTotal) {
			v.Advertirf(q+"pagos", "pagos.suma", "los pagos suman %s y el importe total es %s", suma.Redondear(2), d.ImporteTotal)
		}
	}

	if len(c.InfoAdicional) > 15 {
		v.Errorf("infoAdicional", "maximo", "se admiten hasta 15 campos adicionales, hay %d (sección 9.11)", len(c.InfoAdicional))
	}
	for i, ca := range c.InfoAdicional {
		v.Texto(fmt.Sprintf("infoAdicional[%d].nombre", i), ca.Nombre, true, 300)
		v.Texto(fmt.Sprintf("infoAdicional[%d].valor", i), ca.Valor, true, 300)
		if ca.Nombre == comprobante.NombreRUCProveedor {
			if err := comprobante.ValidarRUC(ca.Valor); err != nil {
				v.Errorf(fmt.Sprintf("infoAdicional[%d].valor", i), "ruc.formato", "el campo %q debe contener el RUC del proveedor (Anexo 26): %v", ca.Nombre, err)
			}
		}
	}
	if m := c.MaquinaFiscal; m != nil {
		v.Texto("maquinaFiscal.marca", m.Marca, true, 30)
		v.Texto("maquinaFiscal.modelo", m.Modelo, true, 30)
		v.Texto("maquinaFiscal.serie", m.Serie, true, 30)
	}
	return v.Hallazgos()
}

// validarRetencion aplica las reglas de una línea de retención, comunes a
// las versiones 1.0.0 y 2.0.0: tipo de impuesto (Tabla 19), código y
// porcentaje (Tabla 20 para el IVA; Catálogo ATS para la renta) y valor
// retenido. r es la ruta de la línea terminada en punto.
func validarRetencion(v *comprobante.Validador, r, codigo, codigoRetencion string, base, porcentaje, valor decimal.Decimal, emision comprobante.Fecha, op OpcionesValidacion) {
	switch codigo {
	case ImpuestoRenta, ImpuestoIVA, ImpuestoISD:
	default:
		v.Errorf(r+"codigo", "catalogo", "debe ser 1 (renta), 2 (IVA) o 6 (ISD) (Tabla 19)")
	}
	v.Texto(r+"codigoRetencion", codigoRetencion, true, 5)
	if codigo == ImpuestoRenta && !emision.EsCero() && emision.Time().Format("2006-01-02") >= CatalogoRentaVigencia {
		reportar := v.Advertirf
		if op.CatalogoRentaEstricto {
			reportar = v.Errorf
		}
		const nota = "según el Catálogo ATS vigente desde %s (descargado el %s; puede estar desactualizado, ver docs/actualizar-catalogos.md)"
		if cr, ok := BuscarConceptoRenta(codigoRetencion); !ok {
			reportar(r+"codigoRetencion", "retencion.codigo-renta", "el código %q no figura entre los conceptos de retención de renta "+nota,
				codigoRetencion, CatalogoRentaVigencia, CatalogoRentaDescarga)
		} else if !cr.Admite(porcentaje) {
			reportar(r+"porcentajeRetener", "retencion.porcentaje-renta", "el código %s (%s) admite %s %% y se declaró %s, "+nota+"; el SRI rechaza con el error 52 un porcentaje distinto del parametrizado",
				cr.Codigo, cr.Concepto, cr.Texto, porcentaje, CatalogoRentaVigencia, CatalogoRentaDescarga)
		}
	}
	if codigo == ImpuestoIVA {
		if pct, ok := porcentajesIVA[codigoRetencion]; !ok {
			v.Errorf(r+"codigoRetencion", "catalogo", "código de retención de IVA %q desconocido (Tabla 20)", codigoRetencion)
		} else if !porcentaje.Igual(pct) {
			v.Errorf(r+"porcentajeRetener", "retencion.porcentaje", "el código %s de retención de IVA corresponde al %s %%; se declaró %s", codigoRetencion, pct, porcentaje)
		}
	}
	v.Numero(r+"baseImponible", base, 14, 2)
	v.Numero(r+"porcentajeRetener", porcentaje, 3, 2)
	v.Numero(r+"valorRetenido", valor, 14, 2)
	if exacto := base.Porcentaje(porcentaje); !comprobante.ImporteRedondeado(valor, exacto) {
		v.Errorf(r+"valorRetenido", "retencion.valor", "es %s pero base × porcentaje = %s", valor, exacto.Redondear(2))
	}
}
