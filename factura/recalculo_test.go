package factura_test

import (
	"bytes"
	"math/rand/v2"
	"testing"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
	"github.com/OsirisRoman/sri-ecuador-go/factura"
)

// lineaICE es un detalle de 100,00 por unidad con ICE ad valorem del 75 %
// sobre el total de la línea e IVA del 15 %; la aplicación solo indica las
// tarifas.
func lineaICE(cantidad string) factura.Detalle {
	return factura.Detalle{CodigoPrincipal: "LIC-1", Descripcion: "Licor", Cantidad: d(cantidad), PrecioUnitario: d("100.00"),
		Impuestos: []comprobante.Impuesto{
			{Codigo: comprobante.ImpuestoICE, CodigoPorcentaje: "3031", Tarifa: d("75")},
			{Codigo: comprobante.ImpuestoIVA, CodigoPorcentaje: comprobante.IVA15},
		}}
}

func conICE(t *testing.T, cantidad string) *factura.Factura {
	f := base(t)
	f.Detalles = []factura.Detalle{lineaICE(cantidad)}
	f.Calcular()
	return f
}

func exigirImportes(t *testing.T, f *factura.Factura, baseICE, ice, baseIVA, iva, importe, pago string) {
	t.Helper()
	imp := f.Detalles[0].Impuestos
	for _, c := range []struct {
		nombre string
		tiene  decimal.Decimal
		quiere string
	}{
		{"base del ICE", imp[0].BaseImponible, baseICE}, {"ICE", imp[0].Valor, ice},
		{"base del IVA", imp[1].BaseImponible, baseIVA}, {"IVA", imp[1].Valor, iva},
		{"importeTotal", f.InfoFactura.ImporteTotal, importe}, {"pago", f.InfoFactura.Pagos[0].Total, pago},
	} {
		if !c.tiene.Igual(d(c.quiere)) {
			t.Errorf("%s = %s; se esperaba %s", c.nombre, c.tiene, c.quiere)
		}
	}
}

// Segunda auditoría, N-01: corregir la cantidad de un comprobante ya
// calculado y volver a llamar a Calcular dejaba el ICE, la base del IVA, el
// importe y el pago con los valores de la cantidad anterior.
func TestRecalcularTrasCorregirLaCantidad(t *testing.T) {
	f := conICE(t, "10")
	exigirImportes(t, f, "1000.00", "750.00", "1750.00", "262.50", "2012.50", "2012.50")
	f.Detalles[0].Cantidad = d("1")
	f.Calcular()
	exigirImportes(t, f, "100.00", "75.00", "175.00", "26.25", "201.25", "201.25")
	if hs := validar(f); len(hs) != 0 {
		t.Errorf("hallazgos tras recalcular: %v", hs)
	}
}

// Lo mismo cuando se corrige el precio, el descuento o la tarifa del ICE.
func TestRecalcularTrasCorregirPrecioDescuentoOTarifa(t *testing.T) {
	f := conICE(t, "10")
	f.Detalles[0].PrecioUnitario = d("50.00")
	f.Calcular()
	exigirImportes(t, f, "500.00", "375.00", "875.00", "131.25", "1006.25", "1006.25")
	f.Detalles[0].Descuento = d("100.00")
	f.Calcular()
	exigirImportes(t, f, "400.00", "300.00", "700.00", "105.00", "805.00", "805.00")
	f.Detalles[0].Impuestos[0].Tarifa = d("50")
	f.Calcular()
	exigirImportes(t, f, "400.00", "200.00", "600.00", "90.00", "690.00", "690.00")
}

// El comprobante puede haber pasado por XML entre un cálculo y otro (la
// aplicación guarda el XML, lo lee y lo corrige): no conserva memoria de lo
// que dedujo Calcular, pero un ICE igual a base × tarifa sobre el total de
// la línea se reconoce como deducido.
func TestRecalcularUnComprobanteLeidoDeXML(t *testing.T) {
	doc, err := conICE(t, "10").XML()
	if err != nil {
		t.Fatal(err)
	}
	f, err := factura.Parse(doc)
	if err != nil {
		t.Fatal(err)
	}
	f.Detalles[0].Cantidad = d("1")
	f.Calcular()
	exigirImportes(t, f, "100.00", "75.00", "175.00", "26.25", "201.25", "201.25")
}

// Lo que indica la aplicación se respeta al recalcular: una base de ICE que
// no es el total de la línea (precio de venta al público), un ICE
// específico, un IRBPNR por unidad y un pago que no es el importe total.
func TestRecalcularRespetaLoIndicado(t *testing.T) {
	f := base(t)
	f.Detalles = []factura.Detalle{
		{CodigoPrincipal: "A", Descripcion: "Base de ICE propia", Cantidad: d("10"), PrecioUnitario: d("100.00"),
			Impuestos: []comprobante.Impuesto{
				{Codigo: comprobante.ImpuestoICE, CodigoPorcentaje: "3031", Tarifa: d("75"), BaseImponible: d("800.00")},
				{Codigo: comprobante.ImpuestoIVA, CodigoPorcentaje: comprobante.IVA15}}},
		{CodigoPrincipal: "B", Descripcion: "ICE específico", Cantidad: d("10"), PrecioUnitario: d("2.00"),
			Impuestos: []comprobante.Impuesto{
				{Codigo: comprobante.ImpuestoICE, CodigoPorcentaje: "3011", Tarifa: d("0.16"), BaseImponible: d("200"), Valor: d("32.00")},
				{Codigo: comprobante.ImpuestoIVA, CodigoPorcentaje: comprobante.IVA15}}},
		{CodigoPrincipal: "C", Descripcion: "Botellas", Cantidad: d("10"), PrecioUnitario: d("0.50"),
			Impuestos: []comprobante.Impuesto{
				{Codigo: comprobante.ImpuestoIRBPNR, CodigoPorcentaje: "5001", Tarifa: d("0.02"), Valor: d("0.20")},
				{Codigo: comprobante.ImpuestoIVA, CodigoPorcentaje: comprobante.IVA15}}},
	}
	f.InfoFactura.Pagos = []comprobante.Pago{{FormaPago: comprobante.PagoSinSistemaFinanciero, Total: d("100.00")}}
	for paso := 0; paso < 3; paso++ {
		switch paso {
		case 1: // se corrigen las cantidades
			for i := range f.Detalles {
				f.Detalles[i].Cantidad = d("5")
			}
		case 2: // se corrige un precio, con el mismo ICE específico
			f.Detalles[1].PrecioUnitario = d("3.00")
		}
		f.Calcular()
		a, b, c := f.Detalles[0].Impuestos[0], f.Detalles[1].Impuestos[0], f.Detalles[2].Impuestos[0]
		if !a.BaseImponible.Igual(d("800.00")) || !a.Valor.Igual(d("600.00")) {
			t.Errorf("paso %d: la base de ICE indicada no se respetó: %+v", paso, a)
		}
		if !b.BaseImponible.Igual(d("200")) || !b.Valor.Igual(d("32.00")) {
			t.Errorf("paso %d: el ICE específico no se respetó: %+v", paso, b)
		}
		if !c.Valor.Igual(d("0.20")) {
			t.Errorf("paso %d: el IRBPNR indicado no se respetó: %+v", paso, c)
		}
		if !f.InfoFactura.Pagos[0].Total.Igual(d("100.00")) {
			t.Errorf("paso %d: el pago indicado cambió a %s", paso, f.InfoFactura.Pagos[0].Total)
		}
	}
	// Con una base indicada, el valor deducido sigue a la base y a la
	// tarifa cuando la aplicación las cambia.
	f.Detalles[0].Impuestos[0].BaseImponible = d("400.00")
	f.Calcular()
	if a := f.Detalles[0].Impuestos[0]; !a.Valor.Igual(d("300.00")) {
		t.Errorf("el ICE no siguió a la base indicada: %+v", a)
	}
	// Un valor que la aplicación fija después de calcular pasa a ser suyo.
	f.Detalles[0].Impuestos[0].Valor = d("123.45")
	f.Detalles[0].Cantidad = d("7")
	f.Calcular()
	if a := f.Detalles[0].Impuestos[0]; !a.Valor.Igual(d("123.45")) || !a.BaseImponible.Igual(d("400.00")) {
		t.Errorf("el ICE fijado por la aplicación no se respetó: %+v", a)
	}
	// Poner a cero un importe indicado pide que Calcular lo deduzca.
	f.Detalles[0].Impuestos[0].BaseImponible, f.Detalles[0].Impuestos[0].Valor = decimal.Cero, decimal.Cero
	f.Calcular()
	if a := f.Detalles[0].Impuestos[0]; !a.BaseImponible.Igual(d("700.00")) || !a.Valor.Igual(d("525.00")) {
		t.Errorf("el ICE puesto a cero no se dedujo: %+v", a)
	}
}

// Una base de ICE indicada por la aplicación que coincide con el total de
// la línea no pasa a seguirlo: sigue siendo un dato de la aplicación.
func TestBaseIndicadaIgualAlTotalNoLoSigue(t *testing.T) {
	f := base(t)
	f.Detalles = []factura.Detalle{lineaICE("10")}
	f.Detalles[0].Impuestos[0].BaseImponible = d("1000.00") // precio de venta al público
	f.Calcular()
	f.Detalles[0].Descuento = d("100.00")
	f.Calcular()
	if a := f.Detalles[0].Impuestos[0]; !a.BaseImponible.Igual(d("1000.00")) || !a.Valor.Igual(d("750.00")) {
		t.Errorf("la base indicada no se respetó: %+v", a)
	}
}

// aleatoria construye una factura con ICE, IRBPNR e importes indicados por
// la aplicación en algunas líneas.
func aleatoria(t *testing.T, r *rand.Rand) *factura.Factura {
	codigos := []string{comprobante.IVA0, comprobante.IVA15, comprobante.IVA5, comprobante.IVANoObjeto, comprobante.IVAExento}
	f := base(t)
	f.Detalles = nil
	for j := 0; j < 1+r.IntN(8); j++ {
		det := factura.LineaIVA("C", "Artículo", decimal.Nuevo(int64(1+r.IntN(50_000)), r.IntN(4)), decimal.Nuevo(int64(1+r.IntN(500_000)), r.IntN(5)), codigos[r.IntN(len(codigos))])
		switch r.IntN(6) {
		case 0, 1: // ICE ad valorem sobre el total de la línea
			det.Impuestos = append([]comprobante.Impuesto{{Codigo: comprobante.ImpuestoICE, CodigoPorcentaje: "3031", Tarifa: decimal.Entero(int64(5 + r.IntN(200)))}}, det.Impuestos...)
		case 2: // ICE con base propia
			det.Impuestos = append([]comprobante.Impuesto{{Codigo: comprobante.ImpuestoICE, CodigoPorcentaje: "3031", Tarifa: decimal.Entero(int64(5 + r.IntN(90))), BaseImponible: decimal.Nuevo(int64(1+r.IntN(100_000)), 2)}}, det.Impuestos...)
		case 3: // ICE específico e IRBPNR
			det.Impuestos = append([]comprobante.Impuesto{
				{Codigo: comprobante.ImpuestoICE, CodigoPorcentaje: "3011", Valor: decimal.Nuevo(int64(1+r.IntN(10_000)), 2)},
				{Codigo: comprobante.ImpuestoIRBPNR, CodigoPorcentaje: "5001", Tarifa: d("0.02"), Valor: decimal.Nuevo(int64(1+r.IntN(500)), 2)},
			}, det.Impuestos...)
		}
		f.Detalles = append(f.Detalles, det)
	}
	return f
}

// entradas copia de f solo lo que indica la aplicación: es la factura que
// se construiría desde cero con los datos actuales. indicados tiene, por
// línea e impuesto, los importes de ICE e IRBPNR que indicó la aplicación.
func entradas(t *testing.T, f *factura.Factura, indicados [][]comprobante.Impuesto) *factura.Factura {
	g := base(t)
	g.Detalles = nil
	for i, det := range f.Detalles {
		n := factura.Detalle{CodigoPrincipal: det.CodigoPrincipal, Descripcion: det.Descripcion, Cantidad: det.Cantidad, PrecioUnitario: det.PrecioUnitario, Descuento: det.Descuento}
		for j, imp := range det.Impuestos {
			n.Impuestos = append(n.Impuestos, comprobante.Impuesto{Codigo: imp.Codigo, CodigoPorcentaje: imp.CodigoPorcentaje, Tarifa: indicados[i][j].Tarifa,
				BaseImponible: indicados[i][j].BaseImponible, Valor: indicados[i][j].Valor})
		}
		g.Detalles = append(g.Detalles, n)
	}
	return g
}

func xmlDe(t *testing.T, f *factura.Factura) []byte {
	t.Helper()
	doc, err := f.XML()
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// Propiedades de Calcular (segunda auditoría, N-01):
//
//  1. Recalcular sin cambiar nada no altera el comprobante.
//  2. Cambiar cantidades, precios, descuentos o tarifas y recalcular da
//     exactamente el comprobante que se obtendría construyéndolo desde
//     cero con los datos nuevos: no queda ningún importe del cálculo
//     anterior, y lo que indicó la aplicación se conserva.
//  3. El resultado supera Validar.
func TestRecalcularEquivaleACalcularDesdeCero(t *testing.T) {
	r := rand.New(rand.NewPCG(2026, 1116))
	for caso := 0; caso < 1500; caso++ {
		f := aleatoria(t, r)
		indicados := make([][]comprobante.Impuesto, len(f.Detalles))
		for i, det := range f.Detalles {
			indicados[i] = append([]comprobante.Impuesto(nil), det.Impuestos...)
		}
		f.Calcular()
		for paso := 0; paso < 4; paso++ {
			antes := xmlDe(t, f)
			f.Calcular()
			if despues := xmlDe(t, f); !bytes.Equal(antes, despues) {
				t.Fatalf("caso %d, paso %d: recalcular sin cambios alteró el comprobante\n%s\n%s", caso, paso, antes, despues)
			}
			cambio := r.IntN(4)
			if cambio != 3 && r.IntN(3) == 0 {
				// El comprobante pasa por XML entre un cálculo y otro y
				// pierde la memoria de lo deducido. (Un cambio de tarifa
				// en un comprobante leído de XML no se puede reconocer:
				// ver TestLimitesDelRecalculoDesdeXML.)
				leida, err := factura.Parse(antes)
				if err != nil {
					t.Fatal(err)
				}
				f = leida
			}
			det := &f.Detalles[r.IntN(len(f.Detalles))]
			switch cambio {
			case 0:
				det.Cantidad = decimal.Nuevo(int64(1+r.IntN(50_000)), r.IntN(4))
			case 1:
				det.PrecioUnitario = decimal.Nuevo(int64(1+r.IntN(500_000)), r.IntN(5))
			case 2:
				det.Descuento = det.Cantidad.Multiplicar(det.PrecioUnitario).Porcentaje(decimal.Entero(int64(r.IntN(40)))).Redondear(2)
			case 3:
				i := r.IntN(len(f.Detalles))
				if imp := &f.Detalles[i].Impuestos[0]; imp.Codigo == comprobante.ImpuestoICE && !imp.Tarifa.EsCero() {
					imp.Tarifa = decimal.Entero(int64(5 + r.IntN(90)))
					indicados[i][0].Tarifa = imp.Tarifa
				}
			}
			if det.Descuento.Cmp(det.Cantidad.Multiplicar(det.PrecioUnitario)) > 0 {
				det.Descuento = decimal.Cero
			}
			f.Calcular()
			desdeCero := entradas(t, f, indicados)
			desdeCero.Calcular()
			if a, b := xmlDe(t, f), xmlDe(t, desdeCero); !bytes.Equal(a, b) {
				t.Fatalf("caso %d, paso %d: recalcular no equivale a calcular desde cero\nrecalculado: %s\ndesde cero:  %s", caso, paso, a, b)
			}
			for _, h := range validar(f).Errores() {
				if h.Regla != "digitos" {
					t.Fatalf("caso %d, paso %d: %v", caso, paso, h)
				}
			}
		}
	}
}

// Límite conocido: un comprobante leído de XML no dice qué importes dedujo
// Calcular. Se reconoce como deducido el ICE que coincide con base × tarifa,
// de modo que al cambiar la tarifa (o una base propia) el valor anterior ya
// no coincide y se toma por un dato de la aplicación. Poner el valor a cero
// pide que se deduzca.
func TestLimitesDelRecalculoDesdeXML(t *testing.T) {
	f, err := factura.Parse(xmlDe(t, conICE(t, "10")))
	if err != nil {
		t.Fatal(err)
	}
	f.Detalles[0].Impuestos[0].Tarifa = d("50")
	f.Calcular()
	if ice := f.Detalles[0].Impuestos[0]; !ice.Valor.Igual(d("750.00")) {
		t.Errorf("se esperaba el valor leído (750.00), no %s", ice.Valor)
	}
	f.Detalles[0].Impuestos[0].Valor = decimal.Cero
	f.Calcular()
	exigirImportes(t, f, "1000.00", "500.00", "1500.00", "225.00", "1725.00", "1725.00")
}
