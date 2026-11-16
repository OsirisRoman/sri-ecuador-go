package factura_test

// Pruebas de regresión de la auditoría de v0.1.0 (docs/auditoria-v0.1.0.md):
// importes internamente coherentes pero tributariamente incorrectos, que
// no salen de Calcular. Cada prueba indica el hallazgo que protege.

import (
	"strings"
	"testing"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
	"github.com/OsirisRoman/sri-ecuador-go/factura"
)

// sencilla es una factura de una línea de 100,00 al 15 %, ya calculada.
func sencilla(t *testing.T) *factura.Factura {
	t.Helper()
	f := base(t)
	f.Detalles = []factura.Detalle{factura.LineaIVA("A", "Servicio", d("1"), d("100.00"), comprobante.IVA15)}
	f.Calcular()
	if _, err := f.GenerarClaveAcceso("12345678"); err != nil {
		t.Fatal(err)
	}
	if hs := validar(f); hs.Err() != nil {
		t.Fatalf("la factura de partida no es válida: %v", hs)
	}
	return f
}

// fijarIVA reescribe el IVA de la única línea y los totales de forma
// coherente entre sí, como lo haría un sistema externo que calcula mal.
func fijarIVA(f *factura.Factura, base, valor string) {
	f.Detalles[0].Impuestos[0].BaseImponible = d(base)
	f.Detalles[0].Impuestos[0].Valor = d(valor)
	f.InfoFactura.TotalConImpuestos[0].BaseImponible = d(base)
	f.InfoFactura.TotalConImpuestos[0].Valor = d(valor)
	f.InfoFactura.ImporteTotal = f.InfoFactura.TotalSinImpuestos.Sumar(d(valor))
	f.InfoFactura.Pagos[0].Total = f.InfoFactura.ImporteTotal
}

func exigir(t *testing.T, hs comprobante.Hallazgos, regla string, sev comprobante.Severidad) {
	t.Helper()
	for _, h := range hs {
		if h.Regla == regla && h.Severidad == sev {
			return
		}
	}
	t.Errorf("falta %s %q; hallazgos: %v", sev, regla, hs)
}

// AUD-001: base del IVA de 1,00 sobre una línea de 100,00.
func TestBaseIVAIncoherenteConLaLinea(t *testing.T) {
	f := sencilla(t)
	fijarIVA(f, "1.00", "0.15")
	exigir(t, validar(f), "iva.base", comprobante.Error)
}

// AUD-001 sin pasar por el modelo: el mismo caso leído desde XML.
func TestBaseIVAIncoherenteDesdeXML(t *testing.T) {
	f := sencilla(t)
	doc, err := f.XML()
	if err != nil {
		t.Fatal(err)
	}
	s := strings.NewReplacer(
		"<baseImponible>100.00</baseImponible>", "<baseImponible>1.00</baseImponible>",
		"<valor>15.00</valor>", "<valor>0.15</valor>",
		"<importeTotal>115.00</importeTotal>", "<importeTotal>100.15</importeTotal>",
		"<total>115.00</total>", "<total>100.15</total>",
	).Replace(string(doc))
	g, err := factura.Parse([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	hs := validar(g)
	exigir(t, hs, "iva.base", comprobante.Error)
	if hs.Tiene("totales.importe") {
		t.Errorf("el caso debe ser coherente en los totales para probar la regla de la línea: %v", hs)
	}
}

// AUD-001: el ICE integra la base del IVA (Tabla 18; art. 58 LRTI).
func TestBaseIVAConICE(t *testing.T) {
	f := base(t)
	det := factura.LineaIVA("C-1", "Bebida", d("10"), d("2.00"), comprobante.IVA15)
	det.Impuestos = append([]comprobante.Impuesto{{Codigo: comprobante.ImpuestoICE, CodigoPorcentaje: "3101", Tarifa: d("10")}}, det.Impuestos...)
	f.Detalles = []factura.Detalle{det}
	f.Calcular()
	f.GenerarClaveAcceso("12345678")
	if hs := validar(f); hs.Err() != nil {
		t.Fatalf("base 20 + ICE 2 = 22 debe ser válida: %v", hs)
	}
	// IVA calculado sobre la línea sin el ICE: incorrecto.
	iva := &f.Detalles[0].Impuestos[1]
	iva.BaseImponible, iva.Valor = d("20.00"), d("3.00")
	exigir(t, validar(f), "iva.base", comprobante.Error)
}

// AUD-001: dos IVA en el mismo detalle.
func TestUnSoloIVAPorDetalle(t *testing.T) {
	f := base(t)
	f.Detalles = []factura.Detalle{factura.LineaIVA("A", "Servicio", d("1"), d("100.00"), comprobante.IVA15)}
	f.Detalles[0].Impuestos = append(f.Detalles[0].Impuestos, comprobante.Impuesto{Codigo: comprobante.ImpuestoIVA, CodigoPorcentaje: comprobante.IVA12})
	f.Calcular()
	f.GenerarClaveAcceso("12345678")
	exigir(t, validar(f), "iva.unico", comprobante.Error)
}

// AUD-001: un detalle sin IVA se advierte, no se prohíbe.
func TestDetalleSinIVA(t *testing.T) {
	f := base(t)
	f.Detalles = []factura.Detalle{{CodigoPrincipal: "A", Descripcion: "Bebida", Cantidad: d("1"), PrecioUnitario: d("10"),
		Impuestos: []comprobante.Impuesto{{Codigo: comprobante.ImpuestoICE, CodigoPorcentaje: "3101", Tarifa: d("10")}}}}
	f.Calcular()
	f.GenerarClaveAcceso("12345678")
	hs := validar(f)
	exigir(t, hs, "iva.ausente", comprobante.Advertencia)
	if hs.Err() != nil {
		t.Errorf("no debe ser un error: %v", hs.Errores())
	}
}

// varias construye n líneas de precio p al 15 %, calculadas.
func varias(t *testing.T, n int, p string) *factura.Factura {
	t.Helper()
	f := base(t)
	f.Detalles = nil
	for i := 0; i < n; i++ {
		f.Detalles = append(f.Detalles, factura.LineaIVA("A", "Artículo", d("1"), d(p), comprobante.IVA15))
	}
	f.Calcular()
	f.GenerarClaveAcceso("12345678")
	return f
}

// fijarTotalIVA reescribe el total de IVA y el importe de forma coherente.
func fijarTotalIVA(f *factura.Factura, total decimal.Decimal) {
	f.InfoFactura.TotalConImpuestos[0].Valor = total
	f.InfoFactura.ImporteTotal = f.InfoFactura.TotalSinImpuestos.Sumar(total)
	f.InfoFactura.Pagos[0].Total = f.InfoFactura.ImporteTotal
}

// AUD-003: un centavo de más en cada una de 100 líneas, con totales que lo
// acumulan.
func TestCentavoDeMasPorLineaNoSeAcumula(t *testing.T) {
	f := varias(t, 100, "1.00")
	suma := decimal.Cero
	for i := range f.Detalles {
		f.Detalles[i].Impuestos[0].Valor = d("0.16") // correcto: 0.15
		suma = suma.Sumar(d("0.16"))
	}
	fijarTotalIVA(f, suma)
	hs := validar(f)
	exigir(t, hs, "iva.valor", comprobante.Error)
	if n := len(hs.Errores()); n < 100 {
		t.Errorf("se esperaba un error por línea, hay %d", n)
	}
}

// AUD-003: el total no admite un centavo que ninguno de los dos criterios
// produce.
func TestTotalConCentavoDeMas(t *testing.T) {
	f := varias(t, 3, "100.00")
	fijarTotalIVA(f, d("45.01"))
	exigir(t, validar(f), "totales.impuesto", comprobante.Error)
}

// AUD-003: diferencias legítimas entre el redondeo por línea y el de la
// base acumulada. 100 líneas de 0,10 al 15 %: cada línea vale 0,015 exacto,
// que se redondea a 0,02 (mitad hacia arriba) o a 0,01 (mitad al par); el
// total puede ser la suma de las líneas o el 15 % de 10,00 = 1,50.
func TestRedondeoPorLineaYAgregadoLegitimos(t *testing.T) {
	for _, c := range []struct {
		nombre string
		linea  string
		total  string
	}{
		{"mitad hacia arriba, suma de líneas", "0.02", "2.00"},
		{"mitad hacia arriba, base acumulada", "0.02", "1.50"},
		{"mitad al par, suma de líneas", "0.01", "1.00"},
	} {
		f := varias(t, 100, "0.10")
		for i := range f.Detalles {
			f.Detalles[i].Impuestos[0].Valor = d(c.linea)
		}
		fijarTotalIVA(f, d(c.total))
		if hs := validar(f); hs.Err() != nil {
			t.Errorf("%s: %v", c.nombre, hs.Errores())
		}
	}
	// Ningún criterio produce 1,75.
	f := varias(t, 100, "0.10")
	fijarTotalIVA(f, d("1.75"))
	exigir(t, validar(f), "totales.impuesto", comprobante.Error)
}

// AUD-010: tarifa del totalImpuesto incoherente con el código de los detalles.
func TestTarifaDelTotal(t *testing.T) {
	f := sencilla(t)
	doce := d("12")
	f.InfoFactura.TotalConImpuestos[0].Tarifa = &doce
	exigir(t, validar(f), "totales.tarifa", comprobante.Error)
}

// AUD-010: propina mayor que el 10 % del subtotal (sección 9.16).
func TestPropinaMayorAlDiezPorCiento(t *testing.T) {
	f := sencilla(t)
	f.InfoFactura.Propina = d("10.00")
	f.Calcular()
	if hs := validar(f); hs.Tiene("propina.limite") {
		t.Errorf("10 %% exacto no debe advertirse: %v", hs)
	}
	f.InfoFactura.Propina = d("10.01")
	f.Calcular()
	hs := validar(f)
	exigir(t, hs, "propina.limite", comprobante.Advertencia)
	if hs.Err() != nil {
		t.Errorf("la propina alta es advertencia, no error: %v", hs.Errores())
	}
}

// AUD-010: vigencia de las tarifas generales del IVA.
func TestVigenciaDeTarifasGenerales(t *testing.T) {
	for _, c := range []struct {
		codigo   string
		fecha    comprobante.Fecha
		advierte bool
	}{
		{comprobante.IVA15, comprobante.NuevaFecha(2023, 1, 10), true},
		{comprobante.IVA15, comprobante.NuevaFecha(2024, 4, 1), false},
		{comprobante.IVA12, comprobante.NuevaFecha(2024, 3, 31), false},
		{comprobante.IVA12, comprobante.NuevaFecha(2026, 11, 10), true},
		{comprobante.IVA14, comprobante.NuevaFecha(2016, 7, 1), false},
		{comprobante.IVA12, comprobante.NuevaFecha(2016, 7, 1), true},
		{comprobante.IVA0, comprobante.NuevaFecha(2026, 11, 10), false},
	} {
		f := base(t)
		f.InfoFactura.FechaEmision = c.fecha
		f.Detalles = []factura.Detalle{factura.LineaIVA("A", "Servicio", d("1"), d("100.00"), c.codigo)}
		f.Calcular()
		f.GenerarClaveAcceso("12345678")
		hs := validar(f)
		if got := hs.Tiene("iva.vigencia"); got != c.advierte {
			t.Errorf("código %s el %s: advertencia=%v, se esperaba %v", c.codigo, c.fecha, got, c.advierte)
		}
		if hs.Err() != nil {
			t.Errorf("código %s el %s: la vigencia nunca es un error: %v", c.codigo, c.fecha, hs.Errores())
		}
	}
}

// AUD-009: caracteres que XML 1.0 no admite y UTF-8 mal formado.
func TestCaracteresNoRepresentablesEnXML(t *testing.T) {
	for _, s := range []string{"ACME\x01CORP", "ACME\x00", "A\x1bB", "ACME\xffCORP", "ACME￾"} {
		f := sencilla(t)
		f.InfoFactura.RazonSocialComprador = s
		exigir(t, validar(f), "caracter-invalido", comprobante.Error)
	}
	f := sencilla(t)
	f.InfoFactura.RazonSocialComprador = "ÑANDÚ € 𝄞 S.A."
	if hs := validar(f); hs.Tiene("caracter-invalido") {
		t.Errorf("caracteres válidos rechazados: %v", hs)
	}
}

// Configuraciones globales: modificar el catálogo exportado no altera la
// validación.
func TestCatalogoExportadoNoAlteraLaValidacion(t *testing.T) {
	orig := comprobante.TarifasIVA[comprobante.IVA15]
	defer func() { comprobante.TarifasIVA[comprobante.IVA15] = orig }()
	mod := orig
	mod.Tarifa = decimal.Entero(1)
	comprobante.TarifasIVA[comprobante.IVA15] = mod
	f := sencilla(t)
	if hs := validar(f); hs.Err() != nil {
		t.Errorf("la validación usó el catálogo modificado: %v", hs.Errores())
	}
	if got, _ := comprobante.TarifaIVADe(comprobante.IVA15); !got.Tarifa.Igual(decimal.Entero(15)) {
		t.Errorf("TarifaIVADe = %s", got.Tarifa)
	}
}
