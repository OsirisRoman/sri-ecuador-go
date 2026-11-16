// Package factura modela la factura electrónica del SRI (codDoc 01) en sus
// versiones 1.0.0, 1.1.0, 2.0.0 y 2.1.0: estructura tipada, serialización y
// lectura de XML, cálculo de totales y reglas de validación tributaria.
//
// Referencias de la ficha técnica v2.34: Anexo 1 (formato 1.0.0), Anexo 3
// (1.1.0), Anexos 4 a 6 (exportación, reembolso, subsidios), Anexos 8 y 9
// (rubros de terceros y sustitutiva de guía de remisión, 2.0.0 y 2.1.0).
//
// Uso típico:
//
//	f := &factura.Factura{ … }
//	f.Calcular()                       // totales a partir de los detalles
//	if err := f.Validar().Err(); err != nil { … }
//	xml, err := f.XML()
package factura

import (
	"encoding/xml"
	"fmt"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
)

// Versiones del esquema de factura.
const (
	V100 = "1.0.0"
	V110 = "1.1.0"
	V200 = "2.0.0"
	V210 = "2.1.0"

	// VersionPorDefecto se usa cuando Factura.Version está vacío. La 1.1.0
	// admite hasta 6 decimales en cantidad y precio unitario (la 1.0.0 solo
	// 2) y es la de uso general. Las 2.x añaden los bloques de rubros de
	// terceros y de sustitución de guía de remisión: la 2.0.0 con 2
	// decimales y la 2.1.0 con 6.
	VersionPorDefecto = V110
)

// Factura es el comprobante <factura>.
//
// Los campos siguen el orden del XSD. Los opcionales se omiten del XML
// cuando están vacíos (cadenas vacías, punteros nil, listas sin elementos).
type Factura struct {
	XMLName xml.Name `xml:"factura"`
	// ID es siempre "comprobante"; XML lo fija si está vacío.
	ID string `xml:"id,attr"`
	// Version del esquema; vacío equivale a VersionPorDefecto.
	Version string `xml:"version,attr"`

	InfoTributaria comprobante.InfoTributaria          `xml:"infoTributaria"`
	InfoFactura    InfoFactura                         `xml:"infoFactura"`
	Detalles       comprobante.Lista[Detalle]          `xml:"detalles"`
	Reembolsos     comprobante.Lista[ReembolsoDetalle] `xml:"reembolsos,omitempty"`
	Retenciones    comprobante.Lista[Retencion]        `xml:"retenciones,omitempty"`
	// InfoSustitutivaGuiaRemision y OtrosRubrosTerceros solo existen en las
	// versiones 2.0.0 y 2.1.0 (Anexos 8 y 9).
	InfoSustitutivaGuiaRemision *InfoSustitutivaGuiaRemision `xml:"infoSustitutivaGuiaRemision,omitempty"`
	OtrosRubrosTerceros         comprobante.Lista[Rubro]     `xml:"otrosRubrosTerceros,omitempty"`
	// TipoNegociable marca la factura comercial negociable (Anexo 11).
	TipoNegociable *TipoNegociable            `xml:"tipoNegociable,omitempty"`
	MaquinaFiscal  *comprobante.MaquinaFiscal `xml:"maquinaFiscal,omitempty"`
	// InfoAdicional admite hasta 15 campos.
	InfoAdicional comprobante.Lista[comprobante.CampoAdicional] `xml:"infoAdicional,omitempty"`
}

// InfoFactura es el bloque <infoFactura>.
type InfoFactura struct {
	FechaEmision          comprobante.Fecha `xml:"fechaEmision"`
	DirEstablecimiento    string            `xml:"dirEstablecimiento,omitempty"`
	ContribuyenteEspecial string            `xml:"contribuyenteEspecial,omitempty"`
	// ObligadoContabilidad es "SI", "NO" o vacío.
	ObligadoContabilidad string `xml:"obligadoContabilidad,omitempty"`

	// Campos de exportación (Anexo 4). ComercioExterior vale "EXPORTADOR".
	ComercioExterior string `xml:"comercioExterior,omitempty"`
	IncoTermFactura  string `xml:"incoTermFactura,omitempty"`
	LugarIncoTerm    string `xml:"lugarIncoTerm,omitempty"`
	PaisOrigen       string `xml:"paisOrigen,omitempty"`
	PuertoEmbarque   string `xml:"puertoEmbarque,omitempty"`
	PuertoDestino    string `xml:"puertoDestino,omitempty"`
	PaisDestino      string `xml:"paisDestino,omitempty"`
	PaisAdquisicion  string `xml:"paisAdquisicion,omitempty"`

	// TipoIdentificacionComprador es un código de la Tabla 6
	// (comprobante.IDRuc, IDCedula, …).
	TipoIdentificacionComprador string `xml:"tipoIdentificacionComprador"`
	// GuiaRemision asociada, con formato 001-001-000000001.
	GuiaRemision            string `xml:"guiaRemision,omitempty"`
	RazonSocialComprador    string `xml:"razonSocialComprador"`
	IdentificacionComprador string `xml:"identificacionComprador"`
	DireccionComprador      string `xml:"direccionComprador,omitempty"`

	TotalSinImpuestos         decimal.Decimal  `xml:"totalSinImpuestos"`
	TotalSubsidio             *decimal.Decimal `xml:"totalSubsidio,omitempty"`
	IncoTermTotalSinImpuestos string           `xml:"incoTermTotalSinImpuestos,omitempty"`
	TotalDescuento            decimal.Decimal  `xml:"totalDescuento"`

	// Totales de reembolso (Anexo 5).
	CodDocReembolso             string           `xml:"codDocReembolso,omitempty"`
	TotalComprobantesReembolso  *decimal.Decimal `xml:"totalComprobantesReembolso,omitempty"`
	TotalBaseImponibleReembolso *decimal.Decimal `xml:"totalBaseImponibleReembolso,omitempty"`
	TotalImpuestoReembolso      *decimal.Decimal `xml:"totalImpuestoReembolso,omitempty"`

	TotalConImpuestos comprobante.Lista[comprobante.TotalImpuesto] `xml:"totalConImpuestos"`
	Compensaciones    comprobante.Lista[Compensacion]              `xml:"compensaciones,omitempty"`
	Propina           decimal.Decimal                              `xml:"propina"`

	FleteInternacional    *decimal.Decimal `xml:"fleteInternacional,omitempty"`
	SeguroInternacional   *decimal.Decimal `xml:"seguroInternacional,omitempty"`
	GastosAduaneros       *decimal.Decimal `xml:"gastosAduaneros,omitempty"`
	GastosTransporteOtros *decimal.Decimal `xml:"gastosTransporteOtros,omitempty"`

	ImporteTotal decimal.Decimal `xml:"importeTotal"`
	Moneda       string          `xml:"moneda,omitempty"`
	// Placa del vehículo en ventas de combustibles (Anexos 12 y 16) y de
	// operadoras de transporte comercial (Anexo 25).
	Placa         string                              `xml:"placa,omitempty"`
	Pagos         comprobante.Lista[comprobante.Pago] `xml:"pagos,omitempty"`
	ValorRetIva   *decimal.Decimal                    `xml:"valorRetIva,omitempty"`
	ValorRetRenta *decimal.Decimal                    `xml:"valorRetRenta,omitempty"`
}

// Detalle es una línea <detalle> de la factura.
type Detalle struct {
	CodigoPrincipal        string                                      `xml:"codigoPrincipal,omitempty"`
	CodigoAuxiliar         string                                      `xml:"codigoAuxiliar,omitempty"`
	Descripcion            string                                      `xml:"descripcion"`
	UnidadMedida           string                                      `xml:"unidadMedida,omitempty"`
	Cantidad               decimal.Decimal                             `xml:"cantidad"`
	PrecioUnitario         decimal.Decimal                             `xml:"precioUnitario"`
	PrecioSinSubsidio      *decimal.Decimal                            `xml:"precioSinSubsidio,omitempty"`
	Descuento              decimal.Decimal                             `xml:"descuento"`
	PrecioTotalSinImpuesto decimal.Decimal                             `xml:"precioTotalSinImpuesto"`
	DetallesAdicionales    comprobante.Lista[comprobante.DetAdicional] `xml:"detallesAdicionales,omitempty"`
	Impuestos              comprobante.Lista[comprobante.Impuesto]     `xml:"impuestos"`
}

// Compensacion es una <compensacion> (figura histórica: descuento solidario
// de IVA de 2016-2017; se conserva porque el XSD aún la define).
type Compensacion = comprobante.Compensacion

// CompensacionReembolso es una <compensacionReembolso> de un reembolso.
type CompensacionReembolso comprobante.Compensacion

// Retencion es una <retencion> informativa dentro de la factura (Sección 10
// de la ficha: comercializadores de derivados del petróleo y retención
// presuntiva de IVA a editores).
type Retencion struct {
	Codigo           string          `xml:"codigo"`
	CodigoPorcentaje string          `xml:"codigoPorcentaje"`
	Tarifa           decimal.Decimal `xml:"tarifa"`
	Valor            decimal.Decimal `xml:"valor"`
}

// TipoNegociable es el bloque <tipoNegociable> de la factura comercial
// negociable (Anexo 11).
type TipoNegociable struct {
	Correo string `xml:"correo"`
}

// Rubro es un <rubro> de <otrosRubrosTerceros> (Anexo 8).
type Rubro struct {
	Concepto string          `xml:"concepto"`
	Total    decimal.Decimal `xml:"total"`
}

// InfoSustitutivaGuiaRemision permite que la factura sustituya a la guía de
// remisión (Anexo 9).
type InfoSustitutivaGuiaRemision struct {
	DirPartida                      string                     `xml:"dirPartida"`
	DirDestinatario                 string                     `xml:"dirDestinatario"`
	FechaIniTransporte              comprobante.Fecha          `xml:"fechaIniTransporte"`
	FechaFinTransporte              comprobante.Fecha          `xml:"fechaFinTransporte"`
	RazonSocialTransportista        string                     `xml:"razonSocialTransportista"`
	TipoIdentificacionTransportista string                     `xml:"tipoIdentificacionTransportista"`
	RucTransportista                string                     `xml:"rucTransportista"`
	Placa                           string                     `xml:"placa"`
	Destinos                        comprobante.Lista[Destino] `xml:"destinos"`
}

// Destino es un <destino> de la información sustitutiva de guía de remisión.
type Destino struct {
	MotivoTraslado   string `xml:"motivoTraslado"`
	DocAduaneroUnico string `xml:"docAduaneroUnico,omitempty"`
	CodEstabDestino  string `xml:"codEstabDestino,omitempty"`
	Ruta             string `xml:"ruta,omitempty"`
}

// ReembolsoDetalle es un <reembolsoDetalle> de la factura de reembolso de
// gastos (Anexo 5).
type ReembolsoDetalle struct {
	TipoIdentificacionProveedorReembolso string                                   `xml:"tipoIdentificacionProveedorReembolso"`
	IdentificacionProveedorReembolso     string                                   `xml:"identificacionProveedorReembolso"`
	CodPaisPagoProveedorReembolso        string                                   `xml:"codPaisPagoProveedorReembolso,omitempty"`
	TipoProveedorReembolso               string                                   `xml:"tipoProveedorReembolso"`
	CodDocReembolso                      string                                   `xml:"codDocReembolso"`
	EstabDocReembolso                    string                                   `xml:"estabDocReembolso"`
	PtoEmiDocReembolso                   string                                   `xml:"ptoEmiDocReembolso"`
	SecuencialDocReembolso               string                                   `xml:"secuencialDocReembolso"`
	FechaEmisionDocReembolso             comprobante.Fecha                        `xml:"fechaEmisionDocReembolso"`
	NumeroAutorizacionDocReemb           string                                   `xml:"numeroautorizacionDocReemb"`
	DetalleImpuestos                     comprobante.Lista[ImpuestoReembolso]     `xml:"detalleImpuestos"`
	CompensacionesReembolso              comprobante.Lista[CompensacionReembolso] `xml:"compensacionesReembolso,omitempty"`
}

// ImpuestoReembolso es un <detalleImpuesto> de un reembolso.
type ImpuestoReembolso struct {
	Codigo                 string          `xml:"codigo"`
	CodigoPorcentaje       string          `xml:"codigoPorcentaje"`
	Tarifa                 decimal.Decimal `xml:"tarifa"`
	BaseImponibleReembolso decimal.Decimal `xml:"baseImponibleReembolso"`
	ImpuestoReembolso      decimal.Decimal `xml:"impuestoReembolso"`
}

// version devuelve la versión efectiva.
func (f *Factura) version() string {
	if f.Version == "" {
		return VersionPorDefecto
	}
	return f.Version
}

// XML serializa la factura como documento XML UTF-8 listo para firmar. Fija
// id="comprobante" y la versión por defecto si están vacíos. No valida: use
// Validar (reglas tributarias) y esquema.Validar (XSD) antes de firmar.
func (f *Factura) XML() ([]byte, error) {
	copia := *f
	copia.XMLName = xml.Name{Local: "factura"}
	copia.ID = "comprobante"
	copia.Version = f.version()
	return comprobante.Serializar(&copia)
}

// Info devuelve el bloque infoTributaria.
func (f *Factura) Info() comprobante.InfoTributaria { return f.InfoTributaria }

// Parse lee una factura desde su XML, firmado o no (la firma se ignora; para
// comprobarla use xades.Verificar).
func Parse(doc []byte) (*Factura, error) {
	var f Factura
	if err := comprobante.Deserializar(doc, &f); err != nil {
		return nil, fmt.Errorf("factura: XML ilegible: %w", err)
	}
	return &f, nil
}

// LineaIVA construye un detalle con un único impuesto IVA. codigoIVA es un
// código de la Tabla 17 (comprobante.IVA15, comprobante.IVA0, …). Los
// importes derivados (total de la línea, base y valor del impuesto) los
// completa Factura.Calcular.
func LineaIVA(codigo, descripcion string, cantidad, precioUnitario decimal.Decimal, codigoIVA string) Detalle {
	return Detalle{
		CodigoPrincipal: codigo,
		Descripcion:     descripcion,
		Cantidad:        cantidad,
		PrecioUnitario:  precioUnitario,
		Impuestos: []comprobante.Impuesto{{
			Codigo:           comprobante.ImpuestoIVA,
			CodigoPorcentaje: codigoIVA,
		}},
	}
}

// EtiquetaXML implementa comprobante.Etiquetado.
func (Detalle) EtiquetaXML() string { return "detalle" }

// EtiquetaXML implementa comprobante.Etiquetado.
func (CompensacionReembolso) EtiquetaXML() string { return "compensacionReembolso" }

// EtiquetaXML implementa comprobante.Etiquetado.
func (Retencion) EtiquetaXML() string { return "retencion" }

// EtiquetaXML implementa comprobante.Etiquetado.
func (Rubro) EtiquetaXML() string { return "rubro" }

// EtiquetaXML implementa comprobante.Etiquetado.
func (Destino) EtiquetaXML() string { return "destino" }

// EtiquetaXML implementa comprobante.Etiquetado.
func (ReembolsoDetalle) EtiquetaXML() string { return "reembolsoDetalle" }

// EtiquetaXML implementa comprobante.Etiquetado.
func (ImpuestoReembolso) EtiquetaXML() string { return "detalleImpuesto" }
