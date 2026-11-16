package notacredito

import (
	"fmt"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
	"github.com/OsirisRoman/sri-ecuador-go/factura"
)

// origen es la factura que la nota de crédito modifica y las notas de
// crédito ya emitidas sobre ella, cuando la aplicación los da a conocer.
type origen struct {
	factura    *factura.Factura
	anteriores []*NotaCredito
}

// AsociarFactura da a conocer a la nota de crédito la factura que modifica
// y, si las hay, las notas de crédito ya emitidas sobre esa misma factura.
// Desde entonces Validar (y por tanto sri.Cliente.Preparar) aplica también
// las reglas de ValidarContra.
//
// DesdeFactura la asocia automáticamente. Una nota de crédito leída con
// Parse o construida a mano no conoce su factura: la aplicación debe
// asociarla si quiere esta comprobación, porque el XML de la nota de
// crédito no contiene la factura.
func (n *NotaCredito) AsociarFactura(f *factura.Factura, anteriores ...*NotaCredito) {
	if f == nil {
		n.origen = nil
		return
	}
	n.origen = &origen{factura: f, anteriores: anteriores}
}

// ValidarContra contrasta la nota de crédito con la factura que modifica y
// con las notas de crédito anteriores sobre la misma factura. Una nota de
// crédito anula o rebaja, total o parcialmente, la operación de esa factura,
// así que no puede referirse a otro documento ni devolver más de lo
// facturado:
//
//   - origen.documento: codDocModificado, numDocModificado y la fecha del
//     documento modificado deben ser los de la factura, y el emisor y el
//     comprador, los mismos.
//   - origen.valor: valorModificacion, sumado al de las anteriores, no puede
//     superar el importeTotal de la factura.
//   - origen.impuesto y origen.base: cada impuesto de la nota de crédito
//     debe existir en la factura, y su base acumulada no puede superar la
//     de la factura.
//   - origen.precio-final (advertencia): la factura tiene líneas cuyo IVA
//     solo es coherente como precio final (IVA incluido) y la nota de
//     crédito no declara ningún precio final.
//   - origen.cantidad: la cantidad devuelta de cada código, acumulada, no
//     puede superar la facturada. origen.detalle (advertencia): un detalle
//     cuyo código no figura en la factura (descuentos o bonificaciones que
//     no corresponden a una línea).
//
// El SDK no consulta al SRI las notas de crédito ya emitidas: si no se
// pasan en anteriores, el límite se comprueba solo contra esta nota.
func (n *NotaCredito) ValidarContra(f *factura.Factura, anteriores ...*NotaCredito) comprobante.Hallazgos {
	v := &comprobante.Validador{}
	n.validarContra(v, f, anteriores)
	return v.Hallazgos()
}

func (n *NotaCredito) validarContra(v *comprobante.Validador, f *factura.Factura, anteriores []*NotaCredito) {
	if f == nil {
		return
	}
	const p = "infoNotaCredito."
	info, fi := &n.InfoNotaCredito, &f.InfoFactura
	dif := func(campo, nombre, enNC, enFactura string) {
		if enNC != enFactura {
			v.Errorf(campo, "origen.documento", "%s es %q pero en la factura es %q", nombre, enNC, enFactura)
		}
	}
	dif(p+"codDocModificado", "el tipo de documento modificado", info.CodDocModificado, comprobante.CodFactura)
	dif(p+"numDocModificado", "el número del documento modificado", info.NumDocModificado, f.InfoTributaria.NumeroComprobante())
	dif(p+"fechaEmisionDocSustento", "la fecha del documento modificado", info.FechaEmisionDocSustento.String(), fi.FechaEmision.String())
	dif("infoTributaria.ruc", "el RUC del emisor", n.InfoTributaria.RUC, f.InfoTributaria.RUC)
	dif(p+"identificacionComprador", "la identificación del comprador", info.IdentificacionComprador, fi.IdentificacionComprador)

	todas := append([]*NotaCredito{n}, anteriores...)
	valor := decimal.Cero
	for _, nc := range todas {
		valor = valor.Sumar(nc.InfoNotaCredito.ValorModificacion)
	}
	if valor.Cmp(fi.ImporteTotal) > 0 {
		v.Errorf(p+"valorModificacion", "origen.valor", "las notas de crédito suman %s (esta y %d anteriores), más que el importe total de la factura (%s)",
			valor, len(anteriores), fi.ImporteTotal)
	}

	type par struct{ codigo, porcentaje string }
	basesFactura := map[par]decimal.Decimal{}
	for _, t := range fi.TotalConImpuestos {
		k := par{t.Codigo, t.CodigoPorcentaje}
		basesFactura[k] = basesFactura[k].Sumar(t.BaseImponible)
	}
	basesNC := map[par]decimal.Decimal{}
	var orden []par
	for _, nc := range todas {
		for _, t := range nc.InfoNotaCredito.TotalConImpuestos {
			k := par{t.Codigo, t.CodigoPorcentaje}
			if _, visto := basesNC[k]; !visto {
				orden = append(orden, k)
			}
			basesNC[k] = basesNC[k].Sumar(t.BaseImponible)
		}
	}
	for _, k := range orden {
		campo := fmt.Sprintf("%stotalConImpuestos(%s/%s)", p, k.codigo, k.porcentaje)
		bf, ok := basesFactura[k]
		switch {
		case !ok:
			v.Errorf(campo, "origen.impuesto", "la factura no tiene el impuesto %s/%s", k.codigo, k.porcentaje)
		case basesNC[k].Cmp(bf) > 0:
			v.Errorf(campo+".baseImponible", "origen.base", "las notas de crédito suman una base de %s, más que la de la factura (%s)", basesNC[k], bf)
		}
	}

	// Una factura emitida con precios finales (IVA incluido) cuya nota de
	// crédito se calcula con precios sin impuestos puede diferir de ella en
	// un centavo por línea, y ya no la revierte por completo.
	if len(n.impuestosFinales()) == 0 {
		for i, d := range f.Detalles {
			if !comprobante.AdmitePrecioFinal(d.Impuestos) {
				continue
			}
			imp := d.Impuestos[0]
			if _, final := comprobante.PrecioFinalDeclarado(d.Cantidad, d.Impuestos); final && !comprobante.ImporteRedondeado(imp.Valor, imp.BaseImponible.Porcentaje(imp.Tarifa)) {
				v.Advertirf(fmt.Sprintf("factura.detalles[%d]", i), "origen.precio-final", "la factura se emitió con precios finales (IVA incluido) y la nota de crédito no los declara: sus importes pueden diferir en un centavo por línea; llame a DeclararPreciosFinales en la factura antes de DesdeFactura, o indique PrecioFinal en los detalles")
				break
			}
		}
	}

	facturado := map[string]decimal.Decimal{}
	for _, d := range f.Detalles {
		if d.CodigoPrincipal != "" {
			facturado[d.CodigoPrincipal] = facturado[d.CodigoPrincipal].Sumar(d.Cantidad)
		}
	}
	devuelto := map[string]decimal.Decimal{}
	for _, nc := range todas {
		for _, d := range nc.Detalles {
			if d.CodigoInterno != "" {
				devuelto[d.CodigoInterno] = devuelto[d.CodigoInterno].Sumar(d.Cantidad)
			}
		}
	}
	for i, d := range n.Detalles {
		if d.CodigoInterno == "" {
			continue
		}
		q := fmt.Sprintf("detalles[%d].", i)
		cant, ok := facturado[d.CodigoInterno]
		switch {
		case !ok:
			v.Advertirf(q+"codigoInterno", "origen.detalle", "el código %q no figura en la factura", d.CodigoInterno)
		case devuelto[d.CodigoInterno].Cmp(cant) > 0:
			v.Errorf(q+"cantidad", "origen.cantidad", "se devuelven %s unidades del código %q (con las notas anteriores) y se facturaron %s",
				devuelto[d.CodigoInterno], d.CodigoInterno, cant)
			devuelto[d.CodigoInterno] = cant // un hallazgo por código
		}
	}
}
