// Package liquidacion modela la liquidación de compra de bienes y
// prestación de servicios electrónica del SRI (codDoc 03), versiones 1.0.0
// y 1.1.0: estructura tipada, serialización y lectura de XML, cálculo de
// totales y validación.
//
// La liquidación de compra la emite el adquirente cuando el proveedor no
// puede entregar un comprobante de venta (por ejemplo, personas naturales
// no obligadas a facturar o proveedores del exterior). Su estructura es la
// de una factura en la que el receptor es el proveedor; los detalles son
// idénticos a los de la factura y se reutilizan sus tipos.
//
// Referencia: ficha técnica v2.34, Anexo 17.
package liquidacion

import (
	"encoding/xml"
	"fmt"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
	"github.com/OsirisRoman/sri-ecuador-go/factura"
)

// Versiones del esquema de liquidación de compra.
const (
	// V100 admite cantidad y precio unitario con 2 decimales.
	V100 = "1.0.0"
	// V110 admite hasta 6 decimales.
	V110 = "1.1.0"
	// VersionPorDefecto es la que se usa cuando Version está vacío.
	VersionPorDefecto = V110
)

// CodDocReembolso es el valor de codDocReembolso cuando la liquidación
// incluye reembolsos de gastos.
const CodDocReembolso = factura.CodDocReembolso

// Detalle es una línea de la liquidación; es el mismo tipo que el de la
// factura, de modo que factura.LineaIVA sirve para construirlo.
type Detalle = factura.Detalle

// ReembolsoDetalle es un comprobante reembolsado. La liquidación no admite
// compensaciones en los reembolsos: deje CompensacionesReembolso vacío.
type ReembolsoDetalle = factura.ReembolsoDetalle

// LineaIVA construye un detalle con un único impuesto IVA; ver
// factura.LineaIVA.
func LineaIVA(codigo, descripcion string, cantidad, precioUnitario decimal.Decimal, codigoIVA string) Detalle {
	return factura.LineaIVA(codigo, descripcion, cantidad, precioUnitario, codigoIVA)
}

// LiquidacionCompra es el comprobante <liquidacionCompra>.
type LiquidacionCompra struct {
	XMLName xml.Name `xml:"liquidacionCompra"`
	ID      string   `xml:"id,attr"`
	Version string   `xml:"version,attr"`

	InfoTributaria        comprobante.InfoTributaria                    `xml:"infoTributaria"`
	InfoLiquidacionCompra InfoLiquidacionCompra                         `xml:"infoLiquidacionCompra"`
	Detalles              comprobante.Lista[Detalle]                    `xml:"detalles"`
	Reembolsos            comprobante.Lista[ReembolsoDetalle]           `xml:"reembolsos,omitempty"`
	TipoNegociable        *factura.TipoNegociable                       `xml:"tipoNegociable,omitempty"`
	MaquinaFiscal         *comprobante.MaquinaFiscal                    `xml:"maquinaFiscal,omitempty"`
	InfoAdicional         comprobante.Lista[comprobante.CampoAdicional] `xml:"infoAdicional,omitempty"`
}

// InfoLiquidacionCompra es el bloque <infoLiquidacionCompra>.
type InfoLiquidacionCompra struct {
	FechaEmision          comprobante.Fecha `xml:"fechaEmision"`
	DirEstablecimiento    string            `xml:"dirEstablecimiento,omitempty"`
	ContribuyenteEspecial string            `xml:"contribuyenteEspecial,omitempty"`
	ObligadoContabilidad  string            `xml:"obligadoContabilidad,omitempty"`

	// El proveedor debe estar identificado: no se admite consumidor final
	// (Tabla 6).
	TipoIdentificacionProveedor string `xml:"tipoIdentificacionProveedor"`
	RazonSocialProveedor        string `xml:"razonSocialProveedor"`
	IdentificacionProveedor     string `xml:"identificacionProveedor"`
	DireccionProveedor          string `xml:"direccionProveedor,omitempty"`

	TotalSinImpuestos decimal.Decimal `xml:"totalSinImpuestos"`
	TotalDescuento    decimal.Decimal `xml:"totalDescuento"`

	// Totales de reembolso; Calcular los completa si hay Reembolsos.
	CodDocReembolso             string           `xml:"codDocReembolso,omitempty"`
	TotalComprobantesReembolso  *decimal.Decimal `xml:"totalComprobantesReembolso,omitempty"`
	TotalBaseImponibleReembolso *decimal.Decimal `xml:"totalBaseImponibleReembolso,omitempty"`
	TotalImpuestoReembolso      *decimal.Decimal `xml:"totalImpuestoReembolso,omitempty"`

	TotalConImpuestos comprobante.Lista[comprobante.TotalImpuesto] `xml:"totalConImpuestos"`
	ImporteTotal      decimal.Decimal                              `xml:"importeTotal"`
	Moneda            string                                       `xml:"moneda,omitempty"`
	Pagos             comprobante.Lista[comprobante.Pago]          `xml:"pagos,omitempty"`
}

// Info devuelve el bloque infoTributaria.
func (l *LiquidacionCompra) Info() comprobante.InfoTributaria { return l.InfoTributaria }

func (l *LiquidacionCompra) version() string {
	if l.Version == "" {
		return VersionPorDefecto
	}
	return l.Version
}

// XML serializa la liquidación como documento XML UTF-8 listo para firmar.
func (l *LiquidacionCompra) XML() ([]byte, error) {
	copia := *l
	copia.XMLName = xml.Name{Local: "liquidacionCompra"}
	copia.ID = "comprobante"
	copia.Version = l.version()
	return comprobante.Serializar(&copia)
}

// Parse lee una liquidación de compra desde su XML, firmado o no.
func Parse(doc []byte) (*LiquidacionCompra, error) {
	var l LiquidacionCompra
	if err := xml.Unmarshal(doc, &l); err != nil {
		return nil, fmt.Errorf("liquidacion: XML ilegible: %w", err)
	}
	return &l, nil
}

func (l *LiquidacionCompra) impuestosPorLinea() [][]comprobante.Impuesto {
	out := make([][]comprobante.Impuesto, len(l.Detalles))
	for i, d := range l.Detalles {
		out[i] = d.Impuestos
	}
	return out
}

// Calcular completa los importes derivados de los detalles (total de cada
// línea, impuestos, totalSinImpuestos, totalDescuento, totalConImpuestos e
// importeTotal), los totales de reembolso si hay reembolsos y el total del
// pago cuando hay uno solo sin importe. Sigue los mismos criterios que
// factura.Factura.Calcular.
func (l *LiquidacionCompra) Calcular() {
	totalSin, totalDesc := decimal.Cero, decimal.Cero
	for i := range l.Detalles {
		d := &l.Detalles[i]
		d.Descuento = d.Descuento.Redondear(2)
		d.PrecioTotalSinImpuesto = comprobante.TotalLinea(d.Cantidad, d.PrecioUnitario, d.Descuento)
		totalSin = totalSin.Sumar(d.PrecioTotalSinImpuesto)
		totalDesc = totalDesc.Sumar(d.Descuento)
		comprobante.CalcularImpuestos(d.PrecioTotalSinImpuesto, d.Impuestos)
	}
	info := &l.InfoLiquidacionCompra
	info.TotalSinImpuestos = totalSin.Redondear(2)
	info.TotalDescuento = totalDesc.Redondear(2)

	type par struct{ codigo, porcentaje string }
	previos := map[par]comprobante.TotalImpuesto{}
	for _, t := range info.TotalConImpuestos {
		previos[par{t.Codigo, t.CodigoPorcentaje}] = t
	}
	info.TotalConImpuestos = nil
	total := info.TotalSinImpuestos
	for _, g := range comprobante.AgruparImpuestos(l.impuestosPorLinea()...) {
		tarifa := g.Tarifa.Redondear(2)
		t := comprobante.TotalImpuesto{
			Codigo: g.Codigo, CodigoPorcentaje: g.CodigoPorcentaje,
			BaseImponible: g.Base.Redondear(2), Tarifa: &tarifa, Valor: g.Valor(),
		}
		if p, ok := previos[par{g.Codigo, g.CodigoPorcentaje}]; ok {
			t.DescuentoAdicional = p.DescuentoAdicional
		}
		info.TotalConImpuestos = append(info.TotalConImpuestos, t)
		total = total.Sumar(t.Valor)
	}
	info.ImporteTotal = total.Redondear(2)

	if len(l.Reembolsos) > 0 {
		base, imp := factura.TotalesReembolso(l.Reembolsos)
		suma := base.Sumar(imp)
		info.CodDocReembolso = CodDocReembolso
		info.TotalBaseImponibleReembolso, info.TotalImpuestoReembolso, info.TotalComprobantesReembolso = &base, &imp, &suma
	}
	if len(info.Pagos) == 1 && info.Pagos[0].Total.EsCero() {
		info.Pagos[0].Total = info.ImporteTotal
	}
	if info.Moneda == "" {
		info.Moneda = comprobante.MonedaDolar
	}
}

// GenerarClaveAcceso compone la clave de acceso y la guarda en
// InfoTributaria.ClaveAcceso. Ver factura.Factura.GenerarClaveAcceso.
func (l *LiquidacionCompra) GenerarClaveAcceso(codigoNumerico string) (string, error) {
	return comprobante.GenerarClaveAcceso(&l.InfoTributaria, l.InfoLiquidacionCompra.FechaEmision, comprobante.CodLiquidacionCompra, codigoNumerico)
}

// OpcionesValidacion ajusta ValidarCon.
type OpcionesValidacion struct {
	// Hoy es la fecha de referencia; por defecto, la actual en Ecuador.
	Hoy comprobante.Fecha
}

// Validar aplica las reglas que se pueden comprobar sin consultar al SRI y
// devuelve todos los hallazgos.
func (l *LiquidacionCompra) Validar() comprobante.Hallazgos {
	return l.ValidarCon(OpcionesValidacion{})
}

// ValidarCon es Validar con opciones.
func (l *LiquidacionCompra) ValidarCon(op OpcionesValidacion) comprobante.Hallazgos {
	v := &comprobante.Validador{}
	hoy := op.Hoy
	if hoy.EsCero() {
		hoy = comprobante.Hoy()
	}
	decimales := 6
	switch l.version() {
	case V100:
		decimales = 2
	case V110:
	default:
		v.Errorf("version", "version", "versión %q desconocida; existen %s y %s", l.Version, V100, V110)
	}
	v.InfoTributaria(l.InfoTributaria, comprobante.CodLiquidacionCompra)
	info := &l.InfoLiquidacionCompra
	const p = "infoLiquidacionCompra."

	if info.FechaEmision.EsCero() {
		v.Errorf(p+"fechaEmision", "requerido", "es obligatoria")
	} else if info.FechaEmision.Time().After(hoy.Time()) {
		v.Errorf(p+"fechaEmision", "fecha.futura", "%s es posterior a la fecha actual en Ecuador (%s)", info.FechaEmision, hoy)
	}
	v.ClaveAcceso(l.InfoTributaria, info.FechaEmision)
	v.Emisor(p, info.DirEstablecimiento, info.ContribuyenteEspecial, info.ObligadoContabilidad)

	// --- Proveedor ------------------------------------------------------------
	v.Identificacion(p+"tipoIdentificacionProveedor", p+"identificacionProveedor",
		info.TipoIdentificacionProveedor, info.IdentificacionProveedor, false)
	v.Texto(p+"razonSocialProveedor", info.RazonSocialProveedor, true, 300)
	v.Texto(p+"direccionProveedor", info.DireccionProveedor, false, 300)
	v.Texto(p+"moneda", info.Moneda, false, 15)

	// --- Detalles y totales ---------------------------------------------------
	if len(l.Detalles) == 0 {
		v.Errorf("detalles", "requerido", "la liquidación debe tener al menos un detalle")
	}
	sumaLineas, sumaDesc := factura.ValidarDetalles(v, l.Detalles, decimales)
	factura.VigenciaDetalles(v, l.Detalles, info.FechaEmision)
	v.Numero(p+"totalSinImpuestos", info.TotalSinImpuestos, 14, 2)
	v.Numero(p+"totalDescuento", info.TotalDescuento, 14, 2)
	v.Numero(p+"importeTotal", info.ImporteTotal, 14, 2)
	if len(l.Detalles) > 0 {
		if !info.TotalSinImpuestos.Igual(sumaLineas) {
			v.Errorf(p+"totalSinImpuestos", "totales.sin-impuestos", "es %s pero los detalles suman %s", info.TotalSinImpuestos, sumaLineas.Redondear(2))
		}
		if !info.TotalDescuento.Igual(sumaDesc) {
			v.Errorf(p+"totalDescuento", "totales.descuento", "es %s pero los descuentos de los detalles suman %s", info.TotalDescuento, sumaDesc.Redondear(2))
		}
	}
	impuestos := v.TotalesImpuestos(p+"totalConImpuestos", info.TotalConImpuestos, l.impuestosPorLinea()...)
	if esperado := info.TotalSinImpuestos.Sumar(impuestos).Redondear(2); !info.ImporteTotal.Igual(esperado) {
		v.Errorf(p+"importeTotal", "totales.importe", "es %s pero totalSinImpuestos + impuestos = %s", info.ImporteTotal, esperado)
	}

	// --- Reembolsos -----------------------------------------------------------
	factura.ValidarReembolsos(v, p, factura.DatosReembolso{CodDocReembolso: info.CodDocReembolso,
		TotalComprobantesReembolso: info.TotalComprobantesReembolso, TotalBaseImponibleReembolso: info.TotalBaseImponibleReembolso,
		TotalImpuestoReembolso: info.TotalImpuestoReembolso}, l.Reembolsos, false)

	v.Pagos(p+"pagos", info.Pagos, info.ImporteTotal, true)
	if l.TipoNegociable != nil {
		v.Texto("tipoNegociable.correo", l.TipoNegociable.Correo, true, 100)
	}
	v.Adicionales(l.InfoAdicional, l.MaquinaFiscal)
	return v.Hallazgos()
}
