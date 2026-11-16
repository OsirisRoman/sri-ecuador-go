// Package derivado distingue, al recalcular un comprobante, los importes de
// ICE e IRBPNR que dedujo el propio SDK de los que indicó la aplicación.
//
// El XML del SRI no dice cómo se obtuvo la base ni el valor de un ICE: puede
// ser un porcentaje del total de la línea (ad valorem), un importe por
// unidad (específico) o una combinación, y la base puede ser el precio de
// venta al público y no el total de la línea. Por eso Calcular solo deduce
// lo que la aplicación no indicó. Pero al recalcular un comprobante ya
// calculado, lo deducido la primera vez ya no es cero, y hay que saber si
// sigue siendo «lo que dedujo Calcular» (y debe volver a deducirse con las
// cantidades nuevas) o «lo que indicó la aplicación» (y debe respetarse).
package derivado

import "github.com/OsirisRoman/sri-ecuador-go/decimal"

// Marca recuerda qué dedujo Calcular de un impuesto. Viaja con el impuesto
// al copiarlo, pero no se serializa: un comprobante leído de XML no la
// tiene. Su valor cero significa «todavía no calculado».
type Marca struct {
	// visto indica que Calcular ya procesó este impuesto.
	visto bool
	// base y valor indican que Calcular dedujo ese importe; b y v son los
	// importes deducidos. Si el importe actual ya no coincide, la
	// aplicación lo cambió después y pasa a ser un dato suyo.
	base, valor bool
	b, v        decimal.Decimal
}

// Aplicar completa la base y el valor de un ICE o IRBPNR.
//
// total es el importe sobre el que se deduce la base (el total de la línea,
// o el total sin impuestos de una nota de débito) y anterior es el que tenía
// el comprobante antes de este cálculo (cero si no lo tenía).
//
// La base se deduce (pasa a ser total) si es cero o si es la que dedujo el
// cálculo anterior. El valor se deduce (base × tarifa %) si la tarifa no es
// cero y el valor es cero o es el que dedujo el cálculo anterior. Cualquier
// otro importe lo indicó la aplicación y se respeta, redondeado a 2
// decimales.
//
// Un impuesto sin Marca (leído de XML, o construido por la aplicación con
// sus importes ya calculados) no dice qué se dedujo. En ese caso se
// considera deducido lo que coincide exactamente con la deducción: una base
// igual a anterior, y un valor igual a base × tarifa %. Así, modificar la
// cantidad de un comprobante leído de XML y recalcularlo actualiza un ICE ad
// valorem sobre el total de la línea, y deja intacto cualquier otro.
func (m *Marca) Aplicar(anterior, total, tarifa decimal.Decimal, base, valor *decimal.Decimal) {
	antes := base.Redondear(2)
	var sigueBase, sigueValor bool
	switch {
	case base.EsCero():
		sigueBase = true
	case m.visto:
		sigueBase = m.base && base.Igual(m.b)
	default:
		sigueBase = !anterior.EsCero() && base.Igual(anterior)
	}
	if !tarifa.EsCero() {
		switch {
		case valor.EsCero():
			sigueValor = true
		case m.visto:
			sigueValor = m.valor && valor.Igual(m.v)
		default:
			sigueValor = valor.Igual(antes.Porcentaje(tarifa).Redondear(2))
		}
	}
	if sigueBase {
		*base = total
	}
	*base = base.Redondear(2)
	if sigueValor {
		*valor = base.Porcentaje(tarifa)
	}
	*valor = valor.Redondear(2)
	*m = Marca{visto: true, base: sigueBase, b: *base, valor: sigueValor, v: *valor}
}
