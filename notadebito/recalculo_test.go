package notadebito_test

import (
	"math/rand/v2"
	"testing"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
	"github.com/OsirisRoman/sri-ecuador-go/esquema"
	"github.com/OsirisRoman/sri-ecuador-go/notadebito"
)

func ice(tarifa string) notadebito.Impuesto {
	return notadebito.Impuesto{Codigo: comprobante.ImpuestoICE, CodigoPorcentaje: "3031", Tarifa: d(tarifa)}
}

func conImpuestos(t *testing.T, motivo string, impuestos ...notadebito.Impuesto) *notadebito.NotaDebito {
	n := base(t, hoy)
	n.Motivos = []notadebito.Motivo{{Razon: "Ajuste de precio", Valor: d(motivo)}}
	n.InfoNotaDebito.Impuestos = impuestos
	n.InfoNotaDebito.Pagos = []comprobante.Pago{{FormaPago: comprobante.PagoOtrosSistFinanciero}}
	n.Calcular()
	return n
}

func exigir(t *testing.T, n *notadebito.NotaDebito, quiere ...string) {
	t.Helper()
	info := n.InfoNotaDebito
	var tiene []decimal.Decimal
	for _, imp := range info.Impuestos {
		tiene = append(tiene, imp.BaseImponible, imp.Valor)
	}
	tiene = append(tiene, info.ValorTotal, info.Pagos[0].Total)
	if len(tiene) != len(quiere) {
		t.Fatalf("se esperaban %d importes y hay %d", len(quiere), len(tiene))
	}
	for i := range tiene {
		if !tiene[i].Igual(d(quiere[i])) {
			t.Errorf("importe %d (base y valor de cada impuesto, valorTotal, pago) = %s; se esperaba %s", i, tiene[i], quiere[i])
		}
	}
}

func sinHallazgos(t *testing.T, n *notadebito.NotaDebito) {
	t.Helper()
	if hs := validar(n); len(hs) != 0 {
		t.Errorf("hallazgos: %v", hs)
	}
	doc, err := n.XML()
	if err != nil {
		t.Fatal(err)
	}
	if err := esquema.Validar(doc); err != nil {
		t.Errorf("XSD: %v", err)
	}
}

// Segunda auditoría, N-02: el ICE integra la base imponible del IVA (Tabla
// 18; art. 58 de la LRTI), también en la nota de débito. Calcular tomaba
// como base del IVA el total sin impuestos.
func TestICEIntegraLaBaseDelIVA(t *testing.T) {
	n := conImpuestos(t, "100.00", ice("10"), notadebito.ImpuestoIVA(comprobante.IVA15))
	exigir(t, n, "100.00", "10.00", "110.00", "16.50", "126.50", "126.50")
	sinHallazgos(t, n)

	// El orden de los impuestos no importa.
	n = conImpuestos(t, "100.00", notadebito.ImpuestoIVA(comprobante.IVA15), ice("10"))
	exigir(t, n, "110.00", "16.50", "100.00", "10.00", "126.50", "126.50")
	sinHallazgos(t, n)
}

// Redondeos: el ICE se redondea al centavo antes de integrar la base del
// IVA, y el IVA se calcula sobre esa base ya redondeada.
func TestICEeIVAConDecimales(t *testing.T) {
	for _, c := range []struct{ motivo, tarifa, ice, baseIVA, iva, total string }{
		{"33.33", "10", "3.33", "36.66", "5.50", "42.16"}, // 3,333 → 3,33; 5,499 → 5,50
		{"0.05", "10", "0.01", "0.06", "0.01", "0.07"},    // 0,005 → 0,01; 0,009 → 0,01
		{"0.04", "10", "0.00", "0.04", "0.01", "0.05"},    // 0,004 → 0,00; 0,006 → 0,01
		{"999999.99", "300", "2999999.97", "3999999.96", "599999.99", "4599999.95"},
		{"19.99", "75", "14.99", "34.98", "5.25", "40.23"}, // 14,9925 → 14,99; 5,247 → 5,25
		{"10.10", "0.5", "0.05", "10.15", "1.52", "11.67"}, // 0,0505 → 0,05; 1,5225 → 1,52
	} {
		n := conImpuestos(t, c.motivo, ice(c.tarifa), notadebito.ImpuestoIVA(comprobante.IVA15))
		exigir(t, n, c.motivo, c.ice, c.baseIVA, c.iva, c.total, c.total)
		sinHallazgos(t, n)
	}
}

// Una base de ICE distinta del total (precio de venta al público) y un ICE
// específico los indica la aplicación; el IVA se calcula sobre el total más
// el ICE que resulte.
func TestICEConBaseOValorIndicados(t *testing.T) {
	propia := ice("10")
	propia.BaseImponible = d("250.00")
	n := conImpuestos(t, "100.00", propia, notadebito.ImpuestoIVA(comprobante.IVA15))
	exigir(t, n, "250.00", "25.00", "125.00", "18.75", "143.75", "143.75")
	sinHallazgos(t, n)

	especifico := notadebito.Impuesto{Codigo: comprobante.ImpuestoICE, CodigoPorcentaje: "3011", Valor: d("7.22")}
	n = conImpuestos(t, "100.00", especifico, notadebito.ImpuestoIVA(comprobante.IVA15))
	exigir(t, n, "100.00", "7.22", "107.22", "16.08", "123.30", "123.30")
	sinHallazgos(t, n)

	// El IRBPNR no integra la base del IVA.
	botellas := notadebito.Impuesto{Codigo: comprobante.ImpuestoIRBPNR, CodigoPorcentaje: "5001", Tarifa: d("0.02"), Valor: d("0.20")}
	n = conImpuestos(t, "100.00", botellas, notadebito.ImpuestoIVA(comprobante.IVA15))
	exigir(t, n, "100.00", "0.20", "100.00", "15.00", "115.20", "115.20")
}

// Con varios IVA la aplicación reparte las bases; la suma debe incluir el
// ICE.
func TestVariosIVAConICE(t *testing.T) {
	iva15, iva0 := notadebito.ImpuestoIVA(comprobante.IVA15), notadebito.ImpuestoIVA(comprobante.IVA0)
	iva15.BaseImponible, iva0.BaseImponible = d("70.00"), d("40.00")
	n := conImpuestos(t, "100.00", ice("10"), iva15, iva0)
	exigir(t, n, "100.00", "10.00", "70.00", "10.50", "40.00", "0.00", "120.50", "120.50")
	sinHallazgos(t, n)

	n.InfoNotaDebito.Impuestos[1].BaseImponible = d("90.00") // 90 + 40 > 110
	n.Calcular()
	if hs := validar(n); !hs.Tiene("totales.base") {
		t.Errorf("falta totales.base: %v", hs)
	}
}

// Una base de IVA que no incluye el ICE no puede venir de Calcular; si la
// escribe la aplicación, Validar lo advierte y Calcular la corrige.
func TestBaseDeIVASinICE(t *testing.T) {
	n := conImpuestos(t, "100.00", ice("10"), notadebito.ImpuestoIVA(comprobante.IVA15))
	info := &n.InfoNotaDebito
	info.Impuestos[1].BaseImponible, info.Impuestos[1].Valor = d("100.00"), d("15.00")
	info.ValorTotal, info.Pagos[0].Total = d("125.00"), d("125.00")
	if hs := validar(n); !hs.Tiene("iva.base") {
		t.Errorf("falta iva.base: %v", hs)
	}
	n.Calcular()
	exigir(t, n, "100.00", "10.00", "110.00", "16.50", "126.50", "126.50")
}

// Segunda auditoría, N-01 en la nota de débito: cambiar los motivos y
// recalcular actualiza el ICE, el IVA, el valor total y el pago.
func TestRecalcularTrasCambiarLosMotivos(t *testing.T) {
	n := conImpuestos(t, "100.00", ice("10"), notadebito.ImpuestoIVA(comprobante.IVA15))
	n.Motivos[0].Valor = d("10.00")
	n.Calcular()
	exigir(t, n, "10.00", "1.00", "11.00", "1.65", "12.65", "12.65")
	sinHallazgos(t, n)

	// También si el comprobante pasó por XML.
	doc, err := conImpuestos(t, "100.00", ice("10"), notadebito.ImpuestoIVA(comprobante.IVA15)).XML()
	if err != nil {
		t.Fatal(err)
	}
	leida, err := notadebito.Parse(doc)
	if err != nil {
		t.Fatal(err)
	}
	leida.Motivos[0].Valor = d("10.00")
	leida.Calcular()
	exigir(t, leida, "10.00", "1.00", "11.00", "1.65", "12.65", "12.65")

	// Lo indicado por la aplicación se respeta.
	propia := ice("10")
	propia.BaseImponible = d("250.00")
	n = conImpuestos(t, "100.00", propia, notadebito.ImpuestoIVA(comprobante.IVA15))
	n.InfoNotaDebito.Pagos[0].Total = d("50.00")
	n.Motivos[0].Valor = d("10.00")
	n.Calcular()
	exigir(t, n, "250.00", "25.00", "35.00", "5.25", "40.25", "50.00")
}

// Propiedad: lo que produce Calcular lo acepta Validar sin hallazgos, y
// recalcular tras cambiar los motivos equivale a calcular desde cero.
func TestCalcularEsCoherenteConValidar(t *testing.T) {
	r := rand.New(rand.NewPCG(2, 2026))
	codigos := []string{comprobante.IVA0, comprobante.IVA15, comprobante.IVA5, comprobante.IVAExento}
	for caso := 0; caso < 3000; caso++ {
		construir := func(motivos []decimal.Decimal, tarifa decimal.Decimal, codigo string, conICE bool) *notadebito.NotaDebito {
			n := base(t, hoy)
			n.Motivos = nil
			for _, m := range motivos {
				n.Motivos = append(n.Motivos, notadebito.Motivo{Razon: "Motivo", Valor: m})
			}
			n.InfoNotaDebito.Impuestos = []notadebito.Impuesto{notadebito.ImpuestoIVA(codigo)}
			if conICE {
				n.InfoNotaDebito.Impuestos = append(n.InfoNotaDebito.Impuestos, notadebito.Impuesto{Codigo: comprobante.ImpuestoICE, CodigoPorcentaje: "3031", Tarifa: tarifa})
			}
			n.InfoNotaDebito.Pagos = []comprobante.Pago{{FormaPago: comprobante.PagoOtrosSistFinanciero}}
			return n
		}
		motivos := func() []decimal.Decimal {
			var ms []decimal.Decimal
			for j := 0; j < 1+r.IntN(4); j++ {
				ms = append(ms, decimal.Nuevo(int64(1+r.IntN(5_000_000)), 2))
			}
			return ms
		}
		tarifa, codigo, conICE := decimal.Nuevo(int64(1+r.IntN(9999)), 2), codigos[r.IntN(len(codigos))], r.IntN(2) == 0
		n := construir(motivos(), tarifa, codigo, conICE)
		n.Calcular()
		if hs := validar(n); len(hs) != 0 {
			t.Fatalf("caso %d: %v", caso, hs)
		}
		nuevos := motivos()
		n.Motivos = n.Motivos[:0]
		for _, m := range nuevos {
			n.Motivos = append(n.Motivos, notadebito.Motivo{Razon: "Motivo", Valor: m})
		}
		n.Calcular()
		desdeCero := construir(nuevos, tarifa, codigo, conICE)
		desdeCero.Calcular()
		a, _ := n.XML()
		b, _ := desdeCero.XML()
		if string(a) != string(b) {
			t.Fatalf("caso %d: recalcular no equivale a calcular desde cero\n%s\n%s", caso, a, b)
		}
		if hs := validar(n); len(hs) != 0 {
			t.Fatalf("caso %d tras recalcular: %v", caso, hs)
		}
	}
}

// Con varios IVA, la aplicación debe repartir las bases: si las omite, la
// nota no se emite sin IVA, sino que Validar lo señala.
func TestVariosIVASinBases(t *testing.T) {
	n := conImpuestos(t, "100.00", notadebito.ImpuestoIVA(comprobante.IVA15), notadebito.ImpuestoIVA(comprobante.IVA5))
	if hs := validar(n); !hs.Tiene("totales.base") || hs.Err() == nil {
		t.Errorf("falta el error totales.base: %v", hs)
	}
}

// Un impuesto que se calculó como IVA y que la aplicación convierte en ICE
// no conserva los importes del IVA como si los hubiera indicado.
func TestImpuestoQueCambiaDeIVAaICE(t *testing.T) {
	n := conImpuestos(t, "100.00", notadebito.ImpuestoIVA(comprobante.IVA15), notadebito.ImpuestoIVA(comprobante.IVA15))
	imp := &n.InfoNotaDebito.Impuestos[0]
	imp.Codigo, imp.CodigoPorcentaje, imp.Tarifa = comprobante.ImpuestoICE, "3031", d("10")
	n.Calcular()
	exigir(t, n, "100.00", "10.00", "110.00", "16.50", "126.50", "126.50")
}
