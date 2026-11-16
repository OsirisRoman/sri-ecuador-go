package comprobante

import (
	"fmt"

	"github.com/OsirisRoman/sri-ecuador-go/decimal"
)

// Precio final con IVA incluido
//
// Un detalle puede definirse de dos maneras:
//
//   - Por su precio sin impuestos (el modo por defecto): la aplicación
//     indica precioUnitario; la base es cantidad × precioUnitario −
//     descuento y el IVA es base × tarifa, redondeado al centavo.
//   - Por su precio final: la aplicación indica lo que paga el comprador
//     por unidad, con el IVA incluido. El importe de la línea es cantidad ×
//     precioFinal; la base es importe / (1 + tarifa), redondeada al
//     centavo, y el IVA es lo que falta hasta el importe.
//
// Los dos modos son la misma operación redondeada desde extremos distintos,
// y dan resultados que pueden diferir en un centavo, porque la base y el IVA
// solo tienen 2 decimales. En el primero es exacta la relación entre la
// base y el IVA; en el segundo, el importe que se cobra. El XML del SRI no
// dice con cuál se calculó una línea: lo declara la aplicación.
//
// # Qué garantiza el modo de precio final
//
// Sea t la tarifa (0,15 para el 15 %), G el importe final de la línea, con 2
// decimales, b = redondear(G / (1 + t)) la base y v = G − b el IVA. Entonces:
//
//   - b + v = G, exactamente: se cobra el precio pactado.
//   - El IVA contenido en el precio es G·t/(1 + t) = G − G/(1 + t), de modo
//     que v − G·t/(1 + t) = G/(1 + t) − b, y |v − G·t/(1 + t)| ≤ 0,005: v es
//     el IVA contenido en el precio, redondeado al centavo.
//   - v − b·t = G − b·(1 + t) = (1 + t)·(G/(1 + t) − b), de modo que
//     |v − b·t| ≤ 0,005·(1 + t): 0,00575 al 15 %. Respecto de la base
//     escrita, el IVA nunca se aparta un centavo.
//   - A la inversa: si |v − b·t| ≤ 0,005·(1 + t), entonces b es el redondeo
//     de (b + v)/(1 + t). Una base y un IVA cumplen esa cota si y solo si
//     son la descomposición de un importe final.
//
// La cota no es una tolerancia que se pueda gastar: para un importe final
// dado hay una sola base y un solo IVA. Por eso Validar no admite un margen
// mayor sin más: exige que la aplicación declare el precio final y comprueba
// la línea contra él.
//
// # A qué se aplica
//
// La deducción anterior vale para un detalle cuyo único impuesto es el IVA,
// porque entonces la base del IVA es el total de la línea. No vale con ICE
// (que integra la base del IVA y puede ser un porcentaje, un importe por
// unidad o ambos) ni con IRBPNR (que se suma al precio sin integrar la
// base): cada caso tiene otra fórmula, y el SDK no las aplica. Un detalle
// con esos impuestos se define por su precio sin impuestos.

// AdmitePrecioFinal informa si un detalle con estos impuestos puede
// definirse por su precio final: debe tener un único impuesto y ser el IVA.
func AdmitePrecioFinal(impuestos []Impuesto) bool {
	return len(impuestos) == 1 && impuestos[0].Codigo == ImpuestoIVA
}

var cien = decimal.Entero(100)

// DescomponerPrecioFinal descompone el importe final de una línea en su
// base imponible y su IVA:
//
//	importe = redondear(cantidad × precioFinal, 2)
//	base    = redondear(importe × 100 / (100 + tarifa), 2)
//	iva     = importe − base
//
// tarifa es el porcentaje (15 para el 15 %). Los redondeos son al centavo
// más cercano, mitad hacia arriba, y las operaciones intermedias, exactas.
func DescomponerPrecioFinal(cantidad, precioFinal, tarifa decimal.Decimal) (importe, base, iva decimal.Decimal) {
	importe = cantidad.Multiplicar(precioFinal).Redondear(2)
	base, err := importe.Multiplicar(cien).Cociente(cien.Sumar(tarifa), 2)
	if err != nil {
		base = importe // tarifa de −100 %: no existe; Validar la rechaza
	}
	return importe, base, importe.Restar(base)
}

// CalcularPrecioFinal completa un detalle definido por su precio final:
// deduce el precio unitario sin impuestos, el total de la línea y la tarifa,
// la base y el valor de su IVA, modificando impuestos.
//
// El precio unitario es (base + descuento) / cantidad con el número de
// decimales que admite la versión del esquema; el total de la línea es,
// como siempre, cantidad × precioUnitario − descuento. descuento es el
// descuento sin impuestos, que solo informa del precio anterior a la rebaja:
// precioFinal es ya el precio que se cobra.
//
// Devuelve ok = false, sin modificar nada, si el detalle no se puede definir
// por su precio final: precioFinal o cantidad son cero, o los impuestos no
// son un único IVA (AdmitePrecioFinal).
//
// Con 6 decimales en el precio unitario y cantidades menores que 10 000, el
// total de la línea coincide siempre con la base buscada. Con 2 decimales
// (versiones 1.0.0 y 2.0.0 de la factura) solo está garantizado para
// cantidad 1; si no coincide, Validar lo indica con precio-final.base.
func CalcularPrecioFinal(cantidad, precioFinal, descuento decimal.Decimal, decimales int, impuestos []Impuesto) (precioUnitario, total decimal.Decimal, ok bool) {
	if precioFinal.EsCero() || cantidad.EsCero() || !AdmitePrecioFinal(impuestos) {
		return precioUnitario, total, false
	}
	imp := &impuestos[0]
	if t, fija := tarifasIVA[imp.CodigoPorcentaje]; fija && t.Fija {
		imp.Tarifa = t.Tarifa
	}
	importe, base, _ := DescomponerPrecioFinal(cantidad, precioFinal, imp.Tarifa)
	precioUnitario, err := base.Sumar(descuento).Cociente(cantidad, decimales)
	if err != nil {
		return precioUnitario, total, false
	}
	// Sin ceros de relleno: 10.43 y no 10.430000.
	for n := 2; n < decimales; n++ {
		if corto := precioUnitario.Redondear(n); corto.Igual(precioUnitario) {
			precioUnitario = corto
			break
		}
	}
	total = TotalLinea(cantidad, precioUnitario, descuento)
	imp.BaseImponible = total
	imp.Valor = importe.Restar(total)
	imp.derivado.Deducido(imp.BaseImponible, imp.Valor, true)
	return precioUnitario, total, true
}

// PrecioFinalDeclarado devuelve el precio final por unidad que corresponde
// a un detalle ya calculado: (base + IVA) / cantidad, con 6 decimales. Sirve
// para volver a declarar el modo en un comprobante leído de XML, que no lo
// conserva (ver DeclararPreciosFinales en cada comprobante).
//
// ok es false si el detalle no admite el modo, si la cantidad es cero o si
// con 6 decimales no se recupera el importe (cantidades de 10 000 o más).
func PrecioFinalDeclarado(cantidad decimal.Decimal, impuestos []Impuesto) (precioFinal decimal.Decimal, ok bool) {
	if !AdmitePrecioFinal(impuestos) {
		return precioFinal, false
	}
	importe := impuestos[0].BaseImponible.Sumar(impuestos[0].Valor)
	precioFinal, err := importe.Cociente(cantidad, 6)
	if err != nil || !cantidad.Multiplicar(precioFinal).Redondear(2).Igual(importe) {
		return decimal.Cero, false
	}
	return precioFinal, true
}

// ImpuestosDetallePrecioFinal valida los impuestos de un detalle que la
// aplicación declara definido por su precio final con IVA incluido. Aplica
// las reglas de ImpuestosDetalle, pero contrasta el IVA con el precio final
// declarado en lugar de con la base:
//
//   - precio-final.impuestos: el detalle debe tener un único impuesto, el
//     IVA. Con ICE o IRBPNR la descomposición es otra y el SDK no la
//     aplica: defina el detalle por su precio sin impuestos.
//   - precio-final.importe: la base más el IVA deben sumar cantidad ×
//     precioFinal, redondeado al centavo.
//   - precio-final.base: la base debe ser ese importe entre (1 + tarifa),
//     redondeado al centavo. Con el importe y la base fijados, el IVA queda
//     determinado: no hay margen para un centavo de más.
//
// Si el detalle no admite el modo se aplican además las reglas de
// ImpuestosDetalle, incluida iva.valor.
func (v *Validador) ImpuestosDetallePrecioFinal(p string, totalLinea, cantidad, precioFinal decimal.Decimal, impuestos []Impuesto) {
	v.Numero(p+"precioFinal", precioFinal, 18, 6)
	if !AdmitePrecioFinal(impuestos) {
		v.Errorf(p+"precioFinal", "precio-final.impuestos", "un detalle definido por su precio final debe tener un único impuesto, el IVA; con ICE o IRBPNR indique el precio sin impuestos")
		v.ImpuestosDetalle(p, totalLinea, impuestos)
		return
	}
	v.impuestosDetalle(p, totalLinea, impuestos, false)
	imp := impuestos[0]
	q := p + "impuestos[0]."
	importe := imp.BaseImponible.Sumar(imp.Valor)
	if exacto := cantidad.Multiplicar(precioFinal); !ImporteRedondeado(importe, exacto) {
		v.Errorf(p+"precioFinal", "precio-final.importe", "la base más el IVA suman %s pero cantidad × precioFinal = %s", importe, exacto.Redondear(2))
		return
	}
	// base = redondear(importe / (1 + t))  ⟺  |base × (100 + tarifa) − importe × 100| ≤ 0,005 × (100 + tarifa)
	factor := cien.Sumar(imp.Tarifa)
	if dif := imp.BaseImponible.Multiplicar(factor).Restar(importe.Multiplicar(cien)).Abs(); dif.Cmp(medioCentavo.Multiplicar(factor)) > 0 {
		_, base, iva := DescomponerPrecioFinal(decimal.Entero(1), importe, imp.Tarifa)
		v.Errorf(q+"baseImponible", "precio-final.base", "es %s (con un IVA de %s), pero un importe final de %s al %s %% se descompone en una base de %s y un IVA de %s",
			imp.BaseImponible, imp.Valor, importe, imp.Tarifa, base, iva)
	}
}

// TotalesPrecioFinal comprueba que, en los impuestos que gravan algún
// detalle definido por su precio final, el valor total sea la suma de los
// valores de las líneas (regla precio-final.total). Así el importe total del
// comprobante es la suma de los importes finales pactados, y no se le suma
// ni se le resta un redondeo hecho sobre la base acumulada.
//
// finales son los impuestos de los detalles definidos por su precio final;
// porLinea, los de todos los detalles.
func (v *Validador) TotalesPrecioFinal(campo string, declarados []TotalImpuesto, finales [][]Impuesto, porLinea ...[]Impuesto) {
	type par struct{ codigo, porcentaje string }
	afectados := map[par]bool{}
	for _, imps := range finales {
		for _, imp := range imps {
			afectados[par{imp.Codigo, imp.CodigoPorcentaje}] = true
		}
	}
	if len(afectados) == 0 {
		return
	}
	suma := map[par]decimal.Decimal{}
	for _, imps := range porLinea {
		for _, imp := range imps {
			k := par{imp.Codigo, imp.CodigoPorcentaje}
			suma[k] = suma[k].Sumar(imp.Valor)
		}
	}
	declarado := map[par]decimal.Decimal{}
	visto := map[par]bool{}
	for _, t := range declarados {
		k := par{t.Codigo, t.CodigoPorcentaje}
		declarado[k] = declarado[k].Sumar(t.Valor)
		visto[k] = true
	}
	for i, t := range declarados {
		k := par{t.Codigo, t.CodigoPorcentaje}
		if !afectados[k] || !visto[k] {
			continue
		}
		visto[k] = false // un hallazgo por impuesto
		if !declarado[k].Igual(suma[k]) {
			v.Errorf(fmt.Sprintf("%s[%d].valor", campo, i), "precio-final.total",
				"es %s pero los detalles definidos por su precio final exigen la suma de las líneas, %s, para que el importe total sea la suma de los importes finales", declarado[k], suma[k].Redondear(2))
		}
	}
}

// ValorCon devuelve el impuesto total del grupo teniendo en cuenta los
// detalles definidos por su precio final (finales son sus impuestos): si
// alguno lleva este impuesto, el total es la suma de los valores de las
// líneas, para que el importe total del comprobante sea la suma de los
// importes finales; si no, es Valor.
func (g GrupoImpuesto) ValorCon(finales [][]Impuesto) decimal.Decimal {
	for _, imps := range finales {
		for _, imp := range imps {
			if imp.Codigo == g.Codigo && imp.CodigoPorcentaje == g.CodigoPorcentaje {
				return g.SumaLineas.Redondear(2)
			}
		}
	}
	return g.Valor()
}
