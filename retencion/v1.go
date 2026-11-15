package retencion

import (
	"encoding/xml"
	"fmt"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
)

// VersionV1 es la versión 1.0.0 del comprobante de retención, anterior al
// formato ATS: una lista plana de impuestos retenidos, cada uno con su
// documento de sustento. Para emisiones nuevas use ComprobanteRetencion
// (versión 2.0.0).
const VersionV1 = "1.0.0"

// ComprobanteV1 es el comprobante <comprobanteRetencion> versión 1.0.0.
type ComprobanteV1 struct {
	XMLName xml.Name `xml:"comprobanteRetencion"`
	ID      string   `xml:"id,attr"`
	Version string   `xml:"version,attr"`

	InfoTributaria    comprobante.InfoTributaria                    `xml:"infoTributaria"`
	InfoCompRetencion InfoV1                                        `xml:"infoCompRetencion"`
	Impuestos         comprobante.Lista[ImpuestoV1]                 `xml:"impuestos"`
	MaquinaFiscal     *comprobante.MaquinaFiscal                    `xml:"maquinaFiscal,omitempty"`
	InfoAdicional     comprobante.Lista[comprobante.CampoAdicional] `xml:"infoAdicional,omitempty"`
}

// InfoV1 es el bloque <infoCompRetencion> de la versión 1.0.0.
type InfoV1 struct {
	FechaEmision                     comprobante.Fecha `xml:"fechaEmision"`
	DirEstablecimiento               string            `xml:"dirEstablecimiento,omitempty"`
	ContribuyenteEspecial            string            `xml:"contribuyenteEspecial,omitempty"`
	ObligadoContabilidad             string            `xml:"obligadoContabilidad,omitempty"`
	TipoIdentificacionSujetoRetenido string            `xml:"tipoIdentificacionSujetoRetenido"`
	RazonSocialSujetoRetenido        string            `xml:"razonSocialSujetoRetenido"`
	IdentificacionSujetoRetenido     string            `xml:"identificacionSujetoRetenido"`
	// PeriodoFiscal con formato mm/aaaa; Calcular lo toma de la fecha de
	// emisión si está vacío.
	PeriodoFiscal string `xml:"periodoFiscal"`
}

// ImpuestoV1 es un <impuesto> de la versión 1.0.0: una retención y el
// documento que la sustenta.
type ImpuestoV1 struct {
	// Codigo es ImpuestoRenta, ImpuestoIVA o ImpuestoISD (Tabla 19).
	Codigo            string          `xml:"codigo"`
	CodigoRetencion   string          `xml:"codigoRetencion"`
	BaseImponible     decimal.Decimal `xml:"baseImponible"`
	PorcentajeRetener decimal.Decimal `xml:"porcentajeRetener"`
	ValorRetenido     decimal.Decimal `xml:"valorRetenido"`
	// Documento de sustento: tipo (Tabla 3), número de 15 dígitos sin
	// guiones y fecha de emisión.
	CodDocSustento          string             `xml:"codDocSustento,omitempty"`
	NumDocSustento          string             `xml:"numDocSustento,omitempty"`
	FechaEmisionDocSustento *comprobante.Fecha `xml:"fechaEmisionDocSustento,omitempty"`
}

// EtiquetaXML implementa comprobante.Etiquetado.
func (ImpuestoV1) EtiquetaXML() string { return "impuesto" }

// Info devuelve el bloque infoTributaria.
func (c *ComprobanteV1) Info() comprobante.InfoTributaria { return c.InfoTributaria }

// XML serializa el comprobante como documento XML UTF-8 listo para firmar.
func (c *ComprobanteV1) XML() ([]byte, error) {
	copia := *c
	copia.XMLName = xml.Name{Local: "comprobanteRetencion"}
	copia.ID = "comprobante"
	copia.Version = VersionV1
	return comprobante.Serializar(&copia)
}

// ParseV1 lee un comprobante de retención versión 1.0.0 desde su XML.
func ParseV1(doc []byte) (*ComprobanteV1, error) {
	var c ComprobanteV1
	if err := xml.Unmarshal(doc, &c); err != nil {
		return nil, fmt.Errorf("retencion: XML ilegible: %w", err)
	}
	return &c, nil
}

// Calcular completa el valor retenido de cada impuesto (base × porcentaje)
// y el período fiscal si está vacío.
func (c *ComprobanteV1) Calcular() {
	for i := range c.Impuestos {
		r := &c.Impuestos[i]
		r.BaseImponible = r.BaseImponible.Redondear(2)
		r.ValorRetenido = r.BaseImponible.Porcentaje(r.PorcentajeRetener).Redondear(2)
	}
	if c.InfoCompRetencion.PeriodoFiscal == "" && !c.InfoCompRetencion.FechaEmision.EsCero() {
		c.InfoCompRetencion.PeriodoFiscal = c.InfoCompRetencion.FechaEmision.String()[3:]
	}
}

// TotalRetenido suma los valores retenidos.
func (c *ComprobanteV1) TotalRetenido() decimal.Decimal {
	t := decimal.Cero
	for _, r := range c.Impuestos {
		t = t.Sumar(r.ValorRetenido)
	}
	return t.Redondear(2)
}

// GenerarClaveAcceso compone la clave de acceso y la guarda en
// InfoTributaria.ClaveAcceso.
func (c *ComprobanteV1) GenerarClaveAcceso(codigoNumerico string) (string, error) {
	return comprobante.GenerarClaveAcceso(&c.InfoTributaria, c.InfoCompRetencion.FechaEmision, comprobante.CodComprobanteRetencion, codigoNumerico)
}

// Validar aplica las reglas que se pueden comprobar sin consultar al SRI.
func (c *ComprobanteV1) Validar() comprobante.Hallazgos { return c.ValidarCon(OpcionesValidacion{}) }

// ValidarCon es Validar con opciones.
func (c *ComprobanteV1) ValidarCon(op OpcionesValidacion) comprobante.Hallazgos {
	v := &comprobante.Validador{}
	hoy := op.Hoy
	if hoy.EsCero() {
		hoy = comprobante.Hoy()
	}
	if c.Version != "" && c.Version != VersionV1 {
		v.Errorf("version", "version", "ComprobanteV1 es la versión %s; para la %s use ComprobanteRetencion", VersionV1, Version)
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
	v.Emisor(p, info.DirEstablecimiento, info.ContribuyenteEspecial, info.ObligadoContabilidad)
	v.Identificacion(p+"tipoIdentificacionSujetoRetenido", p+"identificacionSujetoRetenido",
		info.TipoIdentificacionSujetoRetenido, info.IdentificacionSujetoRetenido, false)
	v.Texto(p+"razonSocialSujetoRetenido", info.RazonSocialSujetoRetenido, true, 300)
	if !rePeriodo.MatchString(info.PeriodoFiscal) {
		v.Errorf(p+"periodoFiscal", "formato", "debe tener el formato mm/aaaa")
	} else if !info.FechaEmision.EsCero() && info.PeriodoFiscal != info.FechaEmision.String()[3:] {
		v.Advertirf(p+"periodoFiscal", "periodo", "el período fiscal (%s) no es el mes de la fecha de emisión (%s)", info.PeriodoFiscal, info.FechaEmision)
	}
	if len(c.Impuestos) == 0 {
		v.Errorf("impuestos", "requerido", "debe haber al menos una retención")
	}
	for i, r := range c.Impuestos {
		q := fmt.Sprintf("impuestos[%d].", i)
		validarRetencion(v, q, r.Codigo, r.CodigoRetencion, r.BaseImponible, r.PorcentajeRetener, r.ValorRetenido, info.FechaEmision, op)
		if r.CodDocSustento != "" && !reDosDigitos.MatchString(r.CodDocSustento) {
			v.Errorf(q+"codDocSustento", "formato", "debe ser un código de 2 dígitos (01 para factura)")
		}
		if r.NumDocSustento != "" && !reNumDoc.MatchString(r.NumDocSustento) {
			v.Errorf(q+"numDocSustento", "formato", "debe tener 15 dígitos, sin guiones")
		}
		if f := r.FechaEmisionDocSustento; f != nil && !info.FechaEmision.EsCero() && f.Time().After(info.FechaEmision.Time()) {
			v.Errorf(q+"fechaEmisionDocSustento", "fecha.sustento", "el documento de sustento (%s) no puede ser posterior a la retención (%s)", f, info.FechaEmision)
		}
	}
	v.Adicionales(c.InfoAdicional, c.MaquinaFiscal)
	return v.Hallazgos()
}
