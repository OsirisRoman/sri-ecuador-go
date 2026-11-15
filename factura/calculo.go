package factura

import (
	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
)

// impuestosPorLinea devuelve los impuestos de cada detalle.
func (f *Factura) impuestosPorLinea() [][]comprobante.Impuesto {
	out := make([][]comprobante.Impuesto, len(f.Detalles))
	for i, d := range f.Detalles {
		out[i] = d.Impuestos
	}
	return out
}

// totalLinea devuelve cantidad × precioUnitario − descuento con 2 decimales.
func totalLinea(d Detalle) decimal.Decimal {
	return comprobante.TotalLinea(d.Cantidad, d.PrecioUnitario, d.Descuento)
}

// Calcular completa los importes derivados a partir de cantidad, precio
// unitario, descuento y códigos de impuesto de cada detalle:
//
//   - Detalle: precioTotalSinImpuesto = cantidad × precioUnitario − descuento.
//   - Impuestos del detalle: para el IVA, la tarifa según la Tabla 17, la
//     base (el total de la línea más el ICE de la misma línea, que integra
//     la base del IVA) y el valor. Para ICE e IRBPNR se respetan la base y
//     el valor indicados; si faltan, se toma como base el total de la línea
//     y se aplica la tarifa como porcentaje.
//   - infoFactura: totalSinImpuestos, totalDescuento, totalConImpuestos e
//     importeTotal (incluye propina, rubros de exportación y de terceros).
//   - Pagos: si hay un único pago sin total, se le asigna el importe total.
//
// Todos los importes monetarios se redondean a 2 decimales, mitad hacia
// arriba. Calcular es idempotente y no modifica los datos de entrada
// (cantidades, precios, descuentos, códigos).
func (f *Factura) Calcular() {
	totalSin, totalDesc := decimal.Cero, decimal.Cero
	for i := range f.Detalles {
		d := &f.Detalles[i]
		d.Descuento = d.Descuento.Redondear(2)
		d.PrecioTotalSinImpuesto = totalLinea(*d)
		totalSin = totalSin.Sumar(d.PrecioTotalSinImpuesto)
		totalDesc = totalDesc.Sumar(d.Descuento)

		comprobante.CalcularImpuestos(d.PrecioTotalSinImpuesto, d.Impuestos)
	}

	info := &f.InfoFactura
	info.TotalSinImpuestos = totalSin.Redondear(2)
	info.TotalDescuento = totalDesc.Redondear(2)

	// Se conservan los datos que el usuario haya puesto en un total y que
	// no se derivan de los detalles.
	type par struct{ codigo, porcentaje string }
	previos := map[par]comprobante.TotalImpuesto{}
	for _, t := range info.TotalConImpuestos {
		previos[par{t.Codigo, t.CodigoPorcentaje}] = t
	}
	info.TotalConImpuestos = nil
	impuestos := decimal.Cero
	for _, g := range comprobante.AgruparImpuestos(f.impuestosPorLinea()...) {
		tarifa := g.Tarifa.Redondear(2)
		t := comprobante.TotalImpuesto{
			Codigo:           g.Codigo,
			CodigoPorcentaje: g.CodigoPorcentaje,
			BaseImponible:    g.Base.Redondear(2),
			Tarifa:           &tarifa,
			Valor:            g.Valor(),
		}
		if p, ok := previos[par{g.Codigo, g.CodigoPorcentaje}]; ok {
			t.DescuentoAdicional = p.DescuentoAdicional
			t.ValorDevolucionIva = p.ValorDevolucionIva
		}
		info.TotalConImpuestos = append(info.TotalConImpuestos, t)
		impuestos = impuestos.Sumar(t.Valor)
	}

	info.Propina = info.Propina.Redondear(2)
	info.ImporteTotal = f.importeEsperado(impuestos)
	if len(info.Pagos) == 1 && info.Pagos[0].Total.EsCero() {
		info.Pagos[0].Total = info.ImporteTotal
	}
	if info.Moneda == "" {
		info.Moneda = comprobante.MonedaDolar
	}
}

// importeEsperado aplica la fórmula del importe total a partir de la suma
// de impuestos.
func (f *Factura) importeEsperado(impuestos decimal.Decimal) decimal.Decimal {
	info := &f.InfoFactura
	total := info.TotalSinImpuestos.Sumar(impuestos).Sumar(info.Propina)
	for _, extra := range []*decimal.Decimal{info.FleteInternacional, info.SeguroInternacional, info.GastosAduaneros, info.GastosTransporteOtros} {
		if extra != nil {
			total = total.Sumar(*extra)
		}
	}
	for _, r := range f.OtrosRubrosTerceros {
		total = total.Sumar(r.Total)
	}
	for _, c := range info.Compensaciones {
		total = total.Restar(c.Valor)
	}
	return total.Redondear(2)
}

// GenerarClaveAcceso compone la clave de acceso a partir de infoTributaria
// y de la fecha de emisión, y la guarda en InfoTributaria.ClaveAcceso.
//
// codigoNumerico son los 8 dígitos a discreción del emisor; si está vacío
// se genera uno aleatorio. La clave identifica al comprobante durante toda
// su vida: guárdela, porque un comprobante devuelto o no autorizado debe
// reenviarse con la misma clave (ficha técnica 5.10).
func (f *Factura) GenerarClaveAcceso(codigoNumerico string) (string, error) {
	return comprobante.GenerarClaveAcceso(&f.InfoTributaria, f.InfoFactura.FechaEmision, comprobante.CodFactura, codigoNumerico)
}
