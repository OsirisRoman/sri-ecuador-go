// Package comprobante reúne los tipos y catálogos que comparten todos los
// comprobantes electrónicos del SRI: información tributaria del emisor,
// impuestos, pagos, fechas, identificaciones y los hallazgos de validación.
//
// Los nombres de los campos siguen las etiquetas XML del SRI (razonSocial →
// RazonSocial) para que el código se lea junto a la ficha técnica sin tabla
// de traducción.
package comprobante

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/OsirisRoman/sri-ecuador-go/decimal"
)

// Ambiente es el tipo de ambiente (ficha técnica, Tabla 4).
type Ambiente string

// Ambientes del SRI.
const (
	Pruebas    Ambiente = "1"
	Produccion Ambiente = "2"
)

func (a Ambiente) String() string {
	switch a {
	case Pruebas:
		return "PRUEBAS"
	case Produccion:
		return "PRODUCCIÓN"
	}
	return "ambiente(" + string(a) + ")"
}

// EmisionNormal es el único tipo de emisión del esquema offline (Tabla 2).
const EmisionNormal = "1"

// Códigos de tipo de comprobante (Tabla 3).
const (
	CodFactura              = "01"
	CodLiquidacionCompra    = "03"
	CodNotaCredito          = "04"
	CodNotaDebito           = "05"
	CodGuiaRemision         = "06"
	CodComprobanteRetencion = "07"
)

// Tipos de identificación del comprador, sujeto retenido o destinatario
// (Tabla 6).
const (
	IDRuc             = "04"
	IDCedula          = "05"
	IDPasaporte       = "06"
	IDConsumidorFinal = "07"
	IDExterior        = "08"
)

// Datos fijos de la venta a consumidor final (Tabla 6 y sección 9.10).
const (
	IdentificacionConsumidorFinal = "9999999999999"
	RazonSocialConsumidorFinal    = "CONSUMIDOR FINAL"
)

// Códigos de impuesto (Tabla 16).
const (
	ImpuestoIVA    = "2"
	ImpuestoICE    = "3"
	ImpuestoIRBPNR = "5"
)

// Leyendas de régimen RIMPE para <contribuyenteRimpe> (Anexo 22).
const (
	RimpeEmprendedor    = "CONTRIBUYENTE RÉGIMEN RIMPE"
	RimpeNegocioPopular = "CONTRIBUYENTE NEGOCIO POPULAR - RÉGIMEN RIMPE"
)

// Moneda por defecto de los comprobantes.
const MonedaDolar = "DOLAR"

// Fecha es una fecha de calendario que se serializa como dd/mm/aaaa, el
// formato de todas las fechas de los comprobantes.
type Fecha struct{ t time.Time }

const formatoFecha = "02/01/2006"

// NuevaFecha construye una fecha.
func NuevaFecha(anio int, mes time.Month, dia int) Fecha {
	return Fecha{time.Date(anio, mes, dia, 0, 0, 0, 0, time.UTC)}
}

// zonaEcuador es UTC−5: el día tributario es el de Ecuador continental.
var zonaEcuador = time.FixedZone("-05", -5*3600)

// FechaDe toma el día calendario de t en la hora de Ecuador continental
// (UTC−5). Un servidor en UTC que emite a las 20:00 de Quito ya está en el
// día siguiente; usar su fecha local produciría una fecha de emisión futura.
func FechaDe(t time.Time) Fecha {
	y, m, d := t.In(zonaEcuador).Date()
	return NuevaFecha(y, m, d)
}

// Hoy es la fecha actual en Ecuador continental.
func Hoy() Fecha { return FechaDe(time.Now()) }

// ParseFecha analiza una fecha dd/mm/aaaa.
func ParseFecha(s string) (Fecha, error) {
	t, err := time.Parse(formatoFecha, strings.TrimSpace(s))
	if err != nil {
		return Fecha{}, fmt.Errorf("fecha %q inválida: se espera dd/mm/aaaa", s)
	}
	return Fecha{t}, nil
}

// Time devuelve la fecha como medianoche UTC.
func (f Fecha) Time() time.Time { return f.t }

// EsCero informa si la fecha no fue asignada.
func (f Fecha) EsCero() bool { return f.t.IsZero() }

func (f Fecha) String() string {
	if f.t.IsZero() {
		return ""
	}
	return f.t.Format(formatoFecha)
}

// MarshalText implementa encoding.TextMarshaler.
func (f Fecha) MarshalText() ([]byte, error) { return []byte(f.String()), nil }

// UnmarshalText implementa encoding.TextUnmarshaler.
func (f *Fecha) UnmarshalText(b []byte) error {
	v, err := ParseFecha(string(b))
	if err != nil {
		return err
	}
	*f = v
	return nil
}

// InfoTributaria es el bloque <infoTributaria>, común a todos los
// comprobantes.
type InfoTributaria struct {
	Ambiente        Ambiente `xml:"ambiente"`
	TipoEmision     string   `xml:"tipoEmision"`
	RazonSocial     string   `xml:"razonSocial"`
	NombreComercial string   `xml:"nombreComercial,omitempty"`
	RUC             string   `xml:"ruc"`
	ClaveAcceso     string   `xml:"claveAcceso"`
	CodDoc          string   `xml:"codDoc"`
	Estab           string   `xml:"estab"`
	PtoEmi          string   `xml:"ptoEmi"`
	Secuencial      string   `xml:"secuencial"`
	DirMatriz       string   `xml:"dirMatriz"`
	// AgenteRetencion es el número de la resolución que designa al emisor
	// como agente de retención, sin ceros a la izquierda (Anexo 21).
	AgenteRetencion string `xml:"agenteRetencion,omitempty"`
	// ContribuyenteRimpe es RimpeEmprendedor o RimpeNegocioPopular
	// (Anexo 22).
	ContribuyenteRimpe string `xml:"contribuyenteRimpe,omitempty"`
}

// NumeroComprobante devuelve estab-ptoEmi-secuencial (001-001-000000001).
func (i InfoTributaria) NumeroComprobante() string {
	return i.Estab + "-" + i.PtoEmi + "-" + i.Secuencial
}

// Impuesto es un <impuesto> de un detalle.
type Impuesto struct {
	Codigo           string          `xml:"codigo"`
	CodigoPorcentaje string          `xml:"codigoPorcentaje"`
	Tarifa           decimal.Decimal `xml:"tarifa"`
	BaseImponible    decimal.Decimal `xml:"baseImponible"`
	Valor            decimal.Decimal `xml:"valor"`
}

// TotalImpuesto es un <totalImpuesto> de <totalConImpuestos>.
type TotalImpuesto struct {
	Codigo             string           `xml:"codigo"`
	CodigoPorcentaje   string           `xml:"codigoPorcentaje"`
	DescuentoAdicional *decimal.Decimal `xml:"descuentoAdicional,omitempty"`
	BaseImponible      decimal.Decimal  `xml:"baseImponible"`
	Tarifa             *decimal.Decimal `xml:"tarifa,omitempty"`
	Valor              decimal.Decimal  `xml:"valor"`
	// ValorDevolucionIva solo puede omitirse o ser cero: los validadores
	// rechazan un valor positivo porque el SDK no implementa la devolución
	// automática del IVA (ver Validador.DevolucionIVA).
	ValorDevolucionIva *decimal.Decimal `xml:"valorDevolucionIva,omitempty"`
}

// Pago es un <pago> de <pagos>.
type Pago struct {
	// FormaPago es un código de la Tabla 24; ver las constantes Pago*.
	FormaPago string          `xml:"formaPago"`
	Total     decimal.Decimal `xml:"total"`
	// Plazo y UnidadTiempo describen el crédito ("30", "dias").
	Plazo        *decimal.Decimal `xml:"plazo,omitempty"`
	UnidadTiempo string           `xml:"unidadTiempo,omitempty"`
}

// Formas de pago (Tabla 24).
const (
	PagoSinSistemaFinanciero = "01"
	PagoCompensacionDeudas   = "15"
	PagoTarjetaDebito        = "16"
	PagoDineroElectronico    = "17"
	PagoTarjetaPrepago       = "18"
	PagoTarjetaCredito       = "19"
	PagoOtrosSistFinanciero  = "20"
	PagoEndosoTitulos        = "21"
)

// FormasPago describe las formas de pago vigentes de la Tabla 24. Es una
// copia de consulta: modificarla no altera las validaciones del SDK, que
// usan FormaPagoDe.
var FormasPago = map[string]string{
	PagoSinSistemaFinanciero: "SIN UTILIZACION DEL SISTEMA FINANCIERO",
	PagoCompensacionDeudas:   "COMPENSACIÓN DE DEUDAS",
	PagoTarjetaDebito:        "TARJETA DE DÉBITO",
	PagoDineroElectronico:    "DINERO ELECTRÓNICO",
	PagoTarjetaPrepago:       "TARJETA PREPAGO",
	PagoTarjetaCredito:       "TARJETA DE CRÉDITO",
	PagoOtrosSistFinanciero:  "OTROS CON UTILIZACIÓN DEL SISTEMA FINANCIERO",
	PagoEndosoTitulos:        "ENDOSO DE TÍTULOS",
}

// CampoAdicional es un <campoAdicional nombre="…">valor</campoAdicional>
// de <infoAdicional> (máximo 15 por comprobante, sección 9.11).
type CampoAdicional struct {
	Nombre string `xml:"nombre,attr"`
	Valor  string `xml:",chardata"`
}

// NombreRUCProveedor es el nombre del campo adicional que debe llevar el
// RUC del proveedor del sistema de facturación cuando el emisor usa un
// sistema de terceros (Anexo 26, obligatorio desde la versión 2.34).
const NombreRUCProveedor = "RUC Proveedor"

// DetAdicional es un <detAdicional nombre="…" valor="…"/> de un detalle
// (máximo 3).
type DetAdicional struct {
	Nombre string `xml:"nombre,attr"`
	Valor  string `xml:"valor,attr"`
}

// MaquinaFiscal es el bloque <maquinaFiscal> (Anexo 13).
type MaquinaFiscal struct {
	Marca  string `xml:"marca"`
	Modelo string `xml:"modelo"`
	Serie  string `xml:"serie"`
}

// TarifaIVA describe una fila de la Tabla 17.
type TarifaIVA struct {
	// Codigo es el valor de <codigoPorcentaje>.
	Codigo string
	// Tarifa es el porcentaje; Fija es false cuando el código no determina
	// la tarifa (IVA diferenciado, código 8).
	Tarifa      decimal.Decimal
	Fija        bool
	Descripcion string
}

// Códigos de porcentaje de IVA (Tabla 17).
const (
	IVA0            = "0"
	IVA12           = "2"
	IVA14           = "3"
	IVA15           = "4"
	IVA5            = "5"
	IVANoObjeto     = "6"
	IVAExento       = "7"
	IVADiferenciado = "8"
	IVA13           = "10"
)

// TarifasIVA es la Tabla 17 de la ficha técnica v2.34. Es una copia de
// consulta: modificarla no altera los cálculos ni las validaciones del SDK,
// que usan TarifaIVADe.
var TarifasIVA = map[string]TarifaIVA{
	IVA0:            {IVA0, decimal.Entero(0), true, "0%"},
	IVA12:           {IVA12, decimal.Entero(12), true, "12%"},
	IVA14:           {IVA14, decimal.Entero(14), true, "14%"},
	IVA15:           {IVA15, decimal.Entero(15), true, "15%"},
	IVA5:            {IVA5, decimal.Entero(5), true, "5%"},
	IVANoObjeto:     {IVANoObjeto, decimal.Entero(0), true, "No objeto de impuesto"},
	IVAExento:       {IVAExento, decimal.Entero(0), true, "Exento de IVA"},
	IVADiferenciado: {IVADiferenciado, decimal.Cero, false, "IVA diferenciado"},
	IVA13:           {IVA13, decimal.Entero(13), true, "13%"},
}

// Serializar escribe un comprobante como documento XML UTF-8 con su
// declaración, sin sangrado: la ficha técnica (Anexo 1) advierte que los
// espacios introducidos en los campos son motivo de rechazo, así que no se
// añade ningún espacio que el modelo no contenga.
func Serializar(v any) ([]byte, error) {
	cuerpo, err := xml.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("comprobante: no se pudo serializar: %w", err)
	}
	return append([]byte(xml.Header), cuerpo...), nil
}
