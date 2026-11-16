package factura_test

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
	"github.com/OsirisRoman/sri-ecuador-go/esquema"
	"github.com/OsirisRoman/sri-ecuador-go/factura"
)

// conPrecioFinal es una factura con detalles definidos por su precio final
// (IVA incluido), ya calculada.
func conPrecioFinal(t *testing.T, detalles ...factura.Detalle) *factura.Factura {
	t.Helper()
	f := base(t)
	f.Detalles = detalles
	f.Calcular()
	return f
}

func linea(cantidad, precioFinal, codigoIVA string) factura.Detalle {
	return factura.LineaPrecioFinal("ART", "Artículo", d(cantidad), d(precioFinal), codigoIVA)
}

func valida(t *testing.T, f *factura.Factura) {
	t.Helper()
	if hs := validar(f); len(hs) != 0 {
		t.Errorf("hallazgos: %v", hs)
	}
	if err := esquema.Validar(xmlDe(t, f)); err != nil {
		t.Errorf("XSD: %v", err)
	}
}

// Los precios finales que la aritmética sobre el precio sin impuestos no
// puede representar (segunda auditoría, N-07; decisión D17) se facturan
// exactos al declararlos como precio final.
func TestPrecioFinalExacto(t *testing.T) {
	for _, c := range []struct{ precio, codigo, base, iva string }{
		{"0.50", comprobante.IVA15, "0.43", "0.07"},      // 0,4348
		{"12.00", comprobante.IVA15, "10.43", "1.57"},    // 10,4348
		{"35.00", comprobante.IVA15, "30.43", "4.57"},    // 30,4348
		{"350.00", comprobante.IVA15, "304.35", "45.65"}, // 304,3478
		{"0.01", comprobante.IVA15, "0.01", "0.00"},
		{"0.04", comprobante.IVA15, "0.03", "0.01"},
		{"115.00", comprobante.IVA15, "100.00", "15.00"},
		{"99999.99", comprobante.IVA15, "86956.51", "13043.48"},
		{"12.00", comprobante.IVA5, "11.43", "0.57"},  // 11,4286
		{"12.00", comprobante.IVA12, "10.71", "1.29"}, // 10,7143
		{"12.00", comprobante.IVA0, "12.00", "0.00"},
		{"12.00", comprobante.IVAExento, "12.00", "0.00"},
	} {
		f := conPrecioFinal(t, linea("1", c.precio, c.codigo))
		det, info := f.Detalles[0], f.InfoFactura
		imp := det.Impuestos[0]
		if !imp.BaseImponible.Igual(d(c.base)) || !imp.Valor.Igual(d(c.iva)) || !det.PrecioTotalSinImpuesto.Igual(d(c.base)) || !det.PrecioUnitario.Igual(d(c.base)) {
			t.Errorf("%s al código %s: base %s, IVA %s, total %s, unitario %s; se esperaba %s + %s", c.precio, c.codigo, imp.BaseImponible, imp.Valor, det.PrecioTotalSinImpuesto, det.PrecioUnitario, c.base, c.iva)
		}
		if !info.ImporteTotal.Igual(d(c.precio)) || !info.Pagos[0].Total.Igual(d(c.precio)) || !info.TotalConImpuestos[0].Valor.Igual(d(c.iva)) {
			t.Errorf("%s: importeTotal %s, pago %s, IVA total %s", c.precio, info.ImporteTotal, info.Pagos[0].Total, info.TotalConImpuestos[0].Valor)
		}
		if c.codigo != comprobante.IVA12 { // la advertencia de vigencia del 12 % no viene al caso
			valida(t, f)
		}
	}
	// La misma base, definida por su precio sin impuestos, no llega a 12,00.
	f := conPrecioFinal(t, factura.LineaIVA("ART", "Artículo", d("1"), d("10.43"), comprobante.IVA15))
	if !f.InfoFactura.ImporteTotal.Igual(d("11.99")) {
		t.Errorf("con precio sin impuestos de 10,43 el total es %s", f.InfoFactura.ImporteTotal)
	}
}

// Varias líneas: el importe total es la suma de los importes finales, y el
// IVA total, la suma del IVA de las líneas.
func TestPrecioFinalVariasLineas(t *testing.T) {
	var ls []factura.Detalle
	for i := 0; i < 10; i++ {
		ls = append(ls, linea("1", "0.50", comprobante.IVA15))
	}
	f := conPrecioFinal(t, ls...)
	info := f.InfoFactura
	if !info.TotalSinImpuestos.Igual(d("4.30")) || !info.TotalConImpuestos[0].Valor.Igual(d("0.70")) || !info.ImporteTotal.Igual(d("5.00")) {
		t.Errorf("subtotal %s, IVA %s, total %s; se esperaba 4,30 + 0,70 = 5,00", info.TotalSinImpuestos, info.TotalConImpuestos[0].Valor, info.ImporteTotal)
	}
	valida(t, f)

	// Líneas de los dos modos y de dos tarifas en la misma factura.
	f = conPrecioFinal(t,
		linea("1", "12.00", comprobante.IVA15),                                                 // 10,43 + 1,57
		factura.LineaIVA("NETO", "Precio sin impuestos", d("3"), d("0.33"), comprobante.IVA15), // 0,99 + 0,15
		linea("2", "35.00", comprobante.IVA5),                                                  // 66,67 + 3,33
		factura.LineaIVA("CERO", "Sin IVA", d("1"), d("5.00"), comprobante.IVA0),
	)
	if got := f.InfoFactura.ImporteTotal; !got.Igual(d("88.14")) { // 12,00 + 1,14 + 70,00 + 5,00
		t.Errorf("importeTotal = %s; se esperaba 88,14", got)
	}
	valida(t, f)
}

// Cantidades y descuentos: el importe de la línea es cantidad × precio
// final, y el descuento (sin impuestos) solo informa del precio anterior.
func TestPrecioFinalCantidadesYDescuentos(t *testing.T) {
	for _, c := range []struct{ cantidad, precio, descuento, importe, base, iva, unitario string }{
		{"3", "12.00", "0", "36.00", "31.30", "4.70", "10.433333"},    // 31,3043
		{"2.5", "9.99", "0", "24.98", "21.72", "3.26", "8.688"},       // 24,975 → 24,98; 21,7217
		{"0.333", "10.00", "0", "3.33", "2.90", "0.43", "8.708709"},   // 2,8957; 2,90/0,333
		{"1", "12.00", "5.00", "12.00", "10.43", "1.57", "15.43"},     // precio anterior: 15,43
		{"3", "12.00", "1.50", "36.00", "31.30", "4.70", "10.933333"}, // (31,30 + 1,50)/3
		{"9999", "0.50", "0", "4999.50", "4347.39", "652.11", "0.434782"},
		{"12", "0.50", "0", "6.00", "5.22", "0.78", "0.435"}, // 5,2174
	} {
		det := linea(c.cantidad, c.precio, comprobante.IVA15)
		det.Descuento = d(c.descuento)
		f := conPrecioFinal(t, det)
		got := f.Detalles[0]
		imp := got.Impuestos[0]
		if !imp.BaseImponible.Igual(d(c.base)) || !imp.Valor.Igual(d(c.iva)) || !got.PrecioUnitario.Igual(d(c.unitario)) || !f.InfoFactura.ImporteTotal.Igual(d(c.importe)) {
			t.Errorf("%s × %s − %s: base %s, IVA %s, unitario %s, importe %s; se esperaba %s, %s, %s, %s", c.cantidad, c.precio, c.descuento,
				imp.BaseImponible, imp.Valor, got.PrecioUnitario, f.InfoFactura.ImporteTotal, c.base, c.iva, c.unitario, c.importe)
		}
		valida(t, f)
	}
}

// El IVA diferenciado (código 8) lleva la tarifa que indique la aplicación.
func TestPrecioFinalConTarifaIndicada(t *testing.T) {
	det := linea("1", "12.00", comprobante.IVADiferenciado)
	det.Impuestos[0].Tarifa = d("8")
	f := conPrecioFinal(t, det)
	if imp := f.Detalles[0].Impuestos[0]; !imp.BaseImponible.Igual(d("11.11")) || !imp.Valor.Igual(d("0.89")) { // 11,1111
		t.Errorf("base %s, IVA %s", imp.BaseImponible, imp.Valor)
	}
	if hs := validar(f); hs.Err() != nil {
		t.Errorf("hallazgos: %v", hs)
	}
}

// El modo no se aplica donde la fórmula es otra: con ICE o IRBPNR el
// detalle se rechaza en vez de calcularse con una descomposición que no le
// corresponde.
func TestPrecioFinalNoAdmiteICEniIRBPNR(t *testing.T) {
	for _, otro := range []comprobante.Impuesto{
		{Codigo: comprobante.ImpuestoICE, CodigoPorcentaje: "3031", Tarifa: d("75")},
		{Codigo: comprobante.ImpuestoIRBPNR, CodigoPorcentaje: "5001", Tarifa: d("0.02"), Valor: d("0.02")},
	} {
		det := linea("1", "12.00", comprobante.IVA15)
		det.Impuestos = append([]comprobante.Impuesto{otro}, det.Impuestos...)
		f := conPrecioFinal(t, det)
		hs := validar(f)
		exigir(t, hs, "precio-final.impuestos", comprobante.Error)
	}
	// Dos IVA, o ninguno, tampoco.
	det := linea("1", "12.00", comprobante.IVA15)
	det.Impuestos = append(det.Impuestos, det.Impuestos[0])
	exigir(t, validar(conPrecioFinal(t, det)), "precio-final.impuestos", comprobante.Error)
}

// En las versiones cuyo precio unitario solo admite 2 decimales, una
// cantidad distinta de 1 puede no tener un precio unitario que reproduzca la
// base: se informa, no se emite un comprobante incoherente.
func TestPrecioFinalConDosDecimales(t *testing.T) {
	f := base(t)
	f.Version = factura.V100
	f.Detalles = []factura.Detalle{linea("1", "12.00", comprobante.IVA15)}
	f.Calcular()
	valida(t, f)

	f.Detalles = []factura.Detalle{linea("3", "12.00", comprobante.IVA15)} // 31,30 / 3 = 10,43 → 31,29
	f.Calcular()
	exigir(t, validar(f), "precio-final.base", comprobante.Error)
}

// Validar contrasta la línea con el precio final declarado: no basta con
// que la diferencia sea pequeña.
func TestPrecioFinalSeContrastaConElPrecioDeclarado(t *testing.T) {
	// El precio declarado no es el de la línea.
	f := conPrecioFinal(t, linea("1", "12.00", comprobante.IVA15))
	f.Detalles[0].PrecioFinal = d("12.01")
	exigir(t, validar(f), "precio-final.importe", comprobante.Error)

	// La base no es la del precio declarado: 10,44 + 1,56 también suma
	// 12,00, pero 12,00 / 1,15 = 10,4348 se redondea a 10,43.
	f = conPrecioFinal(t, linea("1", "12.00", comprobante.IVA15))
	det := &f.Detalles[0]
	det.PrecioUnitario, det.PrecioTotalSinImpuesto = d("10.44"), d("10.44")
	det.Impuestos[0].BaseImponible, det.Impuestos[0].Valor = d("10.44"), d("1.56")
	f.InfoFactura.TotalSinImpuestos = d("10.44")
	f.InfoFactura.TotalConImpuestos[0].BaseImponible, f.InfoFactura.TotalConImpuestos[0].Valor = d("10.44"), d("1.56")
	hs := validar(f)
	exigir(t, hs, "precio-final.base", comprobante.Error)

	// El IVA total debe ser la suma de las líneas: sobre la base acumulada
	// (4,30 × 15 % = 0,65) el importe ya no sería la suma de los precios.
	var ls []factura.Detalle
	for i := 0; i < 10; i++ {
		ls = append(ls, linea("1", "0.50", comprobante.IVA15))
	}
	f = conPrecioFinal(t, ls...)
	f.InfoFactura.TotalConImpuestos[0].Valor = d("0.65")
	f.InfoFactura.ImporteTotal, f.InfoFactura.Pagos[0].Total = d("4.95"), d("4.95")
	exigir(t, validar(f), "precio-final.total", comprobante.Error)
}

// Auditoría de v0.1.0, AUD-003: un centavo de más por línea no pasa tampoco
// declarando un precio final. 1,00 + 0,16 no es la descomposición de 1,16
// (que es 1,01 + 0,15), y cien líneas así dan cien errores.
func TestPrecioFinalNoReintroduceElCentavoPorLinea(t *testing.T) {
	f := varias(t, 100, "1.00")
	for i := range f.Detalles {
		f.Detalles[i].Impuestos[0].Valor = d("0.16")
	}
	fijarTotalIVA(f, d("16.00"))
	if n := len(validar(f).Errores()); n != 100 {
		t.Fatalf("sin declarar precio final: %d errores; se esperaban 100 (iva.valor)", n)
	}
	for i := range f.Detalles {
		f.Detalles[i].PrecioFinal = d("1.16")
	}
	hs := validar(f)
	n := 0
	for _, h := range hs.Errores() {
		if h.Regla == "precio-final.base" {
			n++
		}
	}
	if n != 100 {
		t.Errorf("declarando 1,16 como precio final: %d errores precio-final.base; se esperaban 100: %v", n, hs)
	}
	// Lo que sí es 1,16 de precio final.
	g := conPrecioFinal(t, linea("1", "1.16", comprobante.IVA15))
	if imp := g.Detalles[0].Impuestos[0]; !imp.BaseImponible.Igual(d("1.01")) || !imp.Valor.Igual(d("0.15")) {
		t.Errorf("1,16 se descompone en %s + %s", imp.BaseImponible, imp.Valor)
	}
}

// El XML no dice con qué modo se calculó. Una factura de precios finales
// leída con Parse se valida por defecto como de precios sin impuestos; la
// aplicación vuelve a dar el contexto con su propio precio o con
// DeclararPreciosFinales.
func TestPrecioFinalTrasParse(t *testing.T) {
	original := conPrecioFinal(t, linea("1", "12.00", comprobante.IVA15), linea("3", "0.50", comprobante.IVA15))
	doc := xmlDe(t, original)
	leer := func() *factura.Factura {
		f, err := factura.Parse(doc)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	// Sin contexto: la regla por defecto (D17).
	exigir(t, validar(leer()), "iva.valor", comprobante.Error)

	// Con el precio que consta en los registros de la aplicación.
	f := leer()
	f.Detalles[0].PrecioFinal, f.Detalles[1].PrecioFinal = d("12.00"), d("0.50")
	valida(t, f)
	f.Detalles[0].PrecioFinal = d("11.99") // no es el precio de esta factura
	exigir(t, validar(f), "precio-final.importe", comprobante.Error)

	// Sin el precio original: el que figura en la propia factura.
	f = leer()
	f.DeclararPreciosFinales()
	if !f.Detalles[0].PrecioFinal.Igual(d("12.00")) || !f.Detalles[1].PrecioFinal.Igual(d("0.50")) {
		t.Errorf("precios declarados: %s y %s", f.Detalles[0].PrecioFinal, f.Detalles[1].PrecioFinal)
	}
	valida(t, f)
	f.Calcular()
	if !bytes.Equal(xmlDe(t, f), doc) {
		t.Errorf("recalcular la factura leída y declarada cambia el XML")
	}

	// DeclararPreciosFinales no convierte en válido lo que no es la
	// descomposición de ningún precio final.
	g := sencilla(t)
	fijarIVA(g, "100.00", "15.01")
	g.DeclararPreciosFinales()
	if hs := validar(g); hs.Err() == nil {
		t.Errorf("100,00 + 15,01 se aceptó como precio final")
	}
}

// Recalcular (segunda auditoría, N-01) en el modo de precio final: cambiar
// la cantidad o el precio deja la factura como construida desde cero.
func TestPrecioFinalRecalcular(t *testing.T) {
	f := conPrecioFinal(t, linea("10", "12.00", comprobante.IVA15))
	if !f.InfoFactura.ImporteTotal.Igual(d("120.00")) {
		t.Fatalf("importe %s", f.InfoFactura.ImporteTotal)
	}
	f.Detalles[0].Cantidad = d("1")
	f.Calcular()
	if !bytes.Equal(xmlDe(t, f), xmlDe(t, conPrecioFinal(t, linea("1", "12.00", comprobante.IVA15)))) {
		t.Errorf("recalcular tras cambiar la cantidad no equivale a calcular desde cero")
	}
	f.Detalles[0].PrecioFinal = d("35.00")
	f.Calcular()
	if !bytes.Equal(xmlDe(t, f), xmlDe(t, conPrecioFinal(t, linea("1", "35.00", comprobante.IVA15)))) {
		t.Errorf("recalcular tras cambiar el precio no equivale a calcular desde cero")
	}
	// Volver al precio sin impuestos: se quita el precio final.
	f.Detalles[0].PrecioFinal = decimal.Cero
	f.Detalles[0].PrecioUnitario = d("30.43")
	f.Calcular()
	if !f.InfoFactura.ImporteTotal.Igual(d("34.99")) {
		t.Errorf("con precio sin impuestos de 30,43 el total es %s", f.InfoFactura.ImporteTotal)
	}
	valida(t, f)
}

// Propiedades de la descomposición, comprobadas de forma exhaustiva para
// todos los importes finales de 0,01 a 500,00 y cada tarifa:
//
//  1. base + IVA = importe final.
//  2. |IVA − base × t| ≤ 0,005 × (1 + t): el IVA nunca se aparta un centavo
//     del que corresponde a la base.
//  3. El IVA difiere como mucho un centavo del que daría el modo por
//     defecto sobre esa misma base.
//  4. Validar acepta la descomposición contra el precio declarado.
//  5. Es única: mover un centavo de la base al IVA, o al revés, se rechaza.
func TestPrecioFinalPropiedadesExhaustivas(t *testing.T) {
	medio := d("0.005")
	for _, tarifa := range []string{"15", "5", "12", "14", "13", "8", "0"} {
		tf := d(tarifa)
		cota := medio.Sumar(medio.Porcentaje(tf))
		for c := int64(1); c <= 50000; c++ {
			importe := decimal.Nuevo(c, 2)
			g, b, v := comprobante.DescomponerPrecioFinal(decimal.Entero(1), importe, tf)
			if !g.Igual(importe) || !b.Sumar(v).Igual(importe) || v.Signo() < 0 {
				t.Fatalf("%s al %s %%: %s + %s", importe, tarifa, b, v)
			}
			exacto := b.Porcentaje(tf)
			if v.Restar(exacto).Abs().Cmp(cota) > 0 {
				t.Fatalf("%s al %s %%: IVA %s, base × tarifa %s, fuera de la cota %s", importe, tarifa, v, exacto, cota)
			}
			if v.Restar(exacto.Redondear(2)).Abs().Cmp(d("0.01")) > 0 {
				t.Fatalf("%s al %s %%: IVA %s a más de un centavo de %s", importe, tarifa, v, exacto.Redondear(2))
			}
			aceptada := func(base, iva decimal.Decimal) bool {
				val := &comprobante.Validador{}
				val.ImpuestosDetallePrecioFinal("d.", base, decimal.Entero(1), importe,
					[]comprobante.Impuesto{{Codigo: comprobante.ImpuestoIVA, CodigoPorcentaje: comprobante.IVADiferenciado, Tarifa: tf, BaseImponible: base, Valor: iva}})
				return val.Hallazgos().Err() == nil
			}
			if !aceptada(b, v) {
				t.Fatalf("%s al %s %%: Validar rechaza %s + %s", importe, tarifa, b, v)
			}
			centavo := d("0.01")
			if tarifa != "0" && (aceptada(b.Sumar(centavo), v.Restar(centavo)) && !empate(importe, tf)) {
				t.Fatalf("%s al %s %%: también se acepta %s + %s", importe, tarifa, b.Sumar(centavo), v.Restar(centavo))
			}
			if b.Cmp(centavo) >= 0 && aceptada(b.Restar(centavo), v.Sumar(centavo)) && !empate(importe, tf) {
				t.Fatalf("%s al %s %%: también se acepta %s + %s", importe, tarifa, b.Restar(centavo), v.Sumar(centavo))
			}
		}
	}
}

// empate informa si importe / (1 + t) cae justo en medio centavo, único caso
// en que dos bases son el redondeo del mismo importe.
func empate(importe, tarifa decimal.Decimal) bool {
	// importe × 100 / (100 + t) = k + 0,005  ⟺  importe × 100 × 1000 ≡ 5 × (100 + t) (mód 10 × (100 + t))
	q, _ := importe.Multiplicar(decimal.Entero(100)).Cociente(decimal.Entero(100).Sumar(tarifa), 3)
	return q.Multiplicar(decimal.Entero(100).Sumar(tarifa)).Igual(importe.Multiplicar(decimal.Entero(100))) && q.Redondear(2).Restar(q).Abs().Igual(d("0.005"))
}

// Propiedad sobre facturas aleatorias que mezclan los dos modos, con
// cantidades, descuentos y tarifas: lo que calcula Calcular lo aceptan
// Validar y el XSD; el importe total es la suma de los importes de las
// líneas; recalcular no cambia nada; y la factura leída de su XML y
// declarada como de precios finales se valida y se recalcula igual.
func TestPrecioFinalFacturasAleatorias(t *testing.T) {
	r := rand.New(rand.NewPCG(18, 2026))
	codigos := []string{comprobante.IVA15, comprobante.IVA15, comprobante.IVA5, comprobante.IVA0, comprobante.IVAExento}
	for caso := 0; caso < 1500; caso++ {
		f := base(t)
		f.Detalles = nil
		esperado := decimal.Cero
		todasFinales := true
		for j := 0; j < 1+r.IntN(10); j++ {
			cantidad := decimal.Nuevo(int64(1+r.IntN(9_999_999)), 3) // de 0,001 a 9999,999
			if r.IntN(3) == 0 {
				cantidad = decimal.Entero(int64(1 + r.IntN(20)))
			}
			codigo := codigos[r.IntN(len(codigos))]
			if r.IntN(4) == 0 {
				f.Detalles = append(f.Detalles, factura.LineaIVA("N", "Neto", cantidad, decimal.Nuevo(int64(1+r.IntN(500_000)), 2+r.IntN(3)), codigo))
				todasFinales = false
				continue
			}
			precio := decimal.Nuevo(int64(1+r.IntN(500_000)), 2)
			det := factura.LineaPrecioFinal("F", "Final", cantidad, precio, codigo)
			if r.IntN(4) == 0 {
				det.Descuento = decimal.Nuevo(int64(r.IntN(10_000)), 2)
			}
			f.Detalles = append(f.Detalles, det)
			esperado = esperado.Sumar(cantidad.Multiplicar(precio).Redondear(2))
		}
		f.Calcular()
		for _, h := range validar(f).Errores() {
			if h.Regla != "digitos" {
				t.Fatalf("caso %d: %v\n%s", caso, h, resumen(f))
			}
		}
		suma := decimal.Cero
		for _, det := range f.Detalles {
			suma = suma.Sumar(det.PrecioTotalSinImpuesto).Sumar(det.Impuestos[0].Valor)
		}
		if todasFinales && !f.InfoFactura.ImporteTotal.Igual(esperado) {
			t.Fatalf("caso %d: importeTotal %s; la suma de cantidad × precio final es %s\n%s", caso, f.InfoFactura.ImporteTotal, esperado, resumen(f))
		}
		if finales := factura.ImpuestosFinales(f.Detalles); len(finales) == len(f.Detalles) && !f.InfoFactura.ImporteTotal.Igual(suma) {
			t.Fatalf("caso %d: importeTotal %s; las líneas suman %s", caso, f.InfoFactura.ImporteTotal, suma)
		}
		doc := xmlDe(t, f)
		if caso%25 == 0 {
			if err := esquema.Validar(doc); err != nil && validar(f).Err() == nil {
				t.Fatalf("caso %d: XSD: %v", caso, err)
			}
		}
		f.Calcular()
		if !bytes.Equal(xmlDe(t, f), doc) {
			t.Fatalf("caso %d: recalcular sin cambios alteró la factura", caso)
		}
		leida, err := factura.Parse(doc)
		if err != nil {
			t.Fatal(err)
		}
		for i, det := range f.Detalles {
			if !det.PrecioFinal.EsCero() {
				leida.Detalles[i].PrecioFinal = det.PrecioFinal // el precio de los registros de la aplicación
			}
		}
		for _, h := range validar(leida).Errores() {
			if h.Regla != "digitos" {
				t.Fatalf("caso %d: la factura leída y con sus precios finales no valida: %v", caso, h)
			}
		}
		leida.Calcular()
		if !bytes.Equal(xmlDe(t, leida), doc) {
			t.Fatalf("caso %d: recalcular la factura leída cambia el XML\n%s", caso, fmt.Sprint(resumen(f)))
		}
	}
}

// Límite del esquema: el precio unitario tiene 6 decimales, de modo que con
// cantidades de 10 000 o más puede no existir ninguno cuyo producto se
// redondee a la base buscada. Por debajo de 10 000 existe siempre; por
// encima, si no existe, Validar lo dice (no se emite una línea incoherente).
func TestPrecioFinalCantidadesGrandes(t *testing.T) {
	r := rand.New(rand.NewPCG(19, 2026))
	fallos := 0
	for i := 0; i < 3000; i++ {
		cantidad := decimal.Entero(int64(10_000 + r.IntN(990_000)))
		f := conPrecioFinal(t, factura.LineaPrecioFinal("F", "Final", cantidad, decimal.Nuevo(int64(1+r.IntN(5_000)), 2), comprobante.IVA15))
		hs := validar(f)
		det := f.Detalles[0]
		_, base, _ := comprobante.DescomponerPrecioFinal(det.Cantidad, det.PrecioFinal, d("15"))
		if det.PrecioTotalSinImpuesto.Igual(base) {
			if hs.Err() != nil {
				t.Fatalf("%s × %s: %v", det.Cantidad, det.PrecioFinal, hs)
			}
			continue
		}
		fallos++
		if !hs.Tiene("precio-final.base") {
			t.Fatalf("%s × %s: la base es %s y no %s, y Validar no lo indica: %v", det.Cantidad, det.PrecioFinal, det.PrecioTotalSinImpuesto, base, hs)
		}
	}
	if fallos == 0 {
		t.Error("la prueba no encontró ningún caso sin precio unitario exacto")
	}
	t.Logf("%d de 3000 líneas con cantidad ≥ 10 000 no tienen precio unitario exacto", fallos)
}
