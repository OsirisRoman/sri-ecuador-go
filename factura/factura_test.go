package factura_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
	"github.com/OsirisRoman/sri-ecuador-go/esquema"
	"github.com/OsirisRoman/sri-ecuador-go/factura"
)

var hoy = comprobante.NuevaFecha(2026, 11, 14)

func d(s string) decimal.Decimal { return decimal.MustParse(s) }

// base construye una factura completa y válida, con datos ficticios.
func base(t *testing.T) *factura.Factura {
	t.Helper()
	f := &factura.Factura{
		InfoTributaria: comprobante.InfoTributaria{
			Ambiente: comprobante.Pruebas, TipoEmision: comprobante.EmisionNormal,
			RazonSocial: "EMISOR DE PRUEBA S.A.", NombreComercial: "Ñandú & Cía.",
			RUC: "1790000000001", CodDoc: comprobante.CodFactura,
			Estab: "001", PtoEmi: "002", Secuencial: "000000123",
			DirMatriz: "Av. Amazonas N34-10 y Roca",
		},
		InfoFactura: factura.InfoFactura{
			FechaEmision:                hoy,
			DirEstablecimiento:          "Av. Amazonas N34-10 y Roca",
			ObligadoContabilidad:        "SI",
			TipoIdentificacionComprador: comprobante.IDRuc,
			RazonSocialComprador:        "PRUEBAS SERVICIO DE RENTAS INTERNAS",
			IdentificacionComprador:     "1760013210001",
			Pagos:                       []comprobante.Pago{{FormaPago: comprobante.PagoOtrosSistFinanciero}},
		},
		Detalles: []factura.Detalle{
			factura.LineaIVA("SRV-001", "Servicio de desarrollo <web> & soporte", d("2"), d("49.995"), comprobante.IVA15),
			factura.LineaIVA("LIB-002", "Libro técnico", d("3"), d("10.00"), comprobante.IVA0),
		},
		InfoAdicional: []comprobante.CampoAdicional{{Nombre: "Email", Valor: "cliente@example.com"}},
	}
	f.Detalles[0].Descuento = d("1.50")
	f.Calcular()
	if _, err := f.GenerarClaveAcceso("12345678"); err != nil {
		t.Fatal(err)
	}
	return f
}

func validar(f *factura.Factura) comprobante.Hallazgos {
	return f.ValidarCon(factura.OpcionesValidacion{Hoy: hoy})
}

func TestCalcular(t *testing.T) {
	f := base(t)
	// 2 × 49.995 − 1.50 = 98.49; IVA 15 % = 14.7735 → 14.77
	if got := f.Detalles[0].PrecioTotalSinImpuesto.String(); got != "98.49" {
		t.Errorf("línea 0 = %s", got)
	}
	if got := f.Detalles[0].Impuestos[0].Valor.String(); got != "14.77" {
		t.Errorf("IVA línea 0 = %s", got)
	}
	i := f.InfoFactura
	if i.TotalSinImpuestos.String() != "128.49" || i.TotalDescuento.String() != "1.50" || i.ImporteTotal.String() != "143.26" {
		t.Errorf("totales = %s / %s / %s", i.TotalSinImpuestos, i.TotalDescuento, i.ImporteTotal)
	}
	if len(i.TotalConImpuestos) != 2 || i.TotalConImpuestos[0].Valor.String() != "14.77" || i.TotalConImpuestos[1].BaseImponible.String() != "30.00" {
		t.Errorf("totalConImpuestos = %+v", i.TotalConImpuestos)
	}
	if i.Pagos[0].Total.String() != "143.26" || i.Moneda != "DOLAR" || i.Propina.String() != "0.00" {
		t.Errorf("pago/moneda/propina = %s %s %s", i.Pagos[0].Total, i.Moneda, i.Propina)
	}
	// Idempotencia.
	antes, _ := f.XML()
	f.Calcular()
	despues, _ := f.XML()
	if string(antes) != string(despues) {
		t.Error("Calcular no es idempotente")
	}
}

// Ejemplo del Anexo 14 de la ficha técnica: 1 × 100.00 al 12 % = 112.00.
func TestCalcularEjemploFicha(t *testing.T) {
	f := &factura.Factura{Detalles: []factura.Detalle{factura.LineaIVA("001", "SILLA DE MADERA", d("1.00"), d("100.00"), comprobante.IVA12)}}
	f.Calcular()
	if f.InfoFactura.ImporteTotal.String() != "112.00" || f.InfoFactura.TotalConImpuestos[0].Valor.String() != "12.00" {
		t.Errorf("importe = %s", f.InfoFactura.ImporteTotal)
	}
}

func TestICEIntegraLaBaseDelIVA(t *testing.T) {
	det := factura.LineaIVA("C-1", "Bebida", d("10"), d("2.00"), comprobante.IVA15)
	det.Impuestos = append([]comprobante.Impuesto{{Codigo: comprobante.ImpuestoICE, CodigoPorcentaje: "3101", Tarifa: d("10")}}, det.Impuestos...)
	f := &factura.Factura{Detalles: []factura.Detalle{det}}
	f.Calcular()
	// base 20.00; ICE 10 % = 2.00; IVA sobre 22.00 = 3.30; total 25.30
	if got := f.InfoFactura.ImporteTotal.String(); got != "25.30" {
		t.Errorf("importe = %s", got)
	}
}

func TestValidaYCumpleXSDEnTodasLasVersiones(t *testing.T) {
	for _, v := range []string{factura.V100, factura.V110, factura.V200, factura.V210} {
		t.Run(v, func(t *testing.T) {
			f := base(t)
			f.Version = v
			if v == factura.V100 || v == factura.V200 {
				// 1.0.0 y 2.0.0 solo admiten 2 decimales en el precio unitario.
				f.Detalles[0].PrecioUnitario = d("50.00")
				f.InfoFactura.Pagos[0].Total = decimal.Cero
				f.Calcular()
			}
			if hs := validar(f); len(hs) != 0 {
				t.Fatalf("hallazgos inesperados: %v", hs)
			}
			doc, err := f.XML()
			if err != nil {
				t.Fatal(err)
			}
			if err := esquema.ValidarEstricto(doc); err != nil {
				t.Fatalf("XSD: %v", err)
			}
			otra, err := factura.Parse(doc)
			if err != nil {
				t.Fatal(err)
			}
			doc2, _ := otra.XML()
			if string(doc) != string(doc2) {
				t.Error("ida y vuelta no es estable")
			}
		})
	}
}

func TestXMLEscapaYNoIndenta(t *testing.T) {
	doc, _ := base(t).XML()
	s := string(doc)
	if !strings.HasPrefix(s, `<?xml version="1.0" encoding="UTF-8"?>`+"\n"+`<factura id="comprobante" version="1.1.0"><infoTributaria><ambiente>1</ambiente>`) {
		t.Errorf("cabecera = %.150s", s)
	}
	if !strings.Contains(s, "<descripcion>Servicio de desarrollo &lt;web&gt; &amp; soporte</descripcion>") {
		t.Error("escape incorrecto")
	}
	if strings.Contains(s, "> <") || strings.Contains(s, ">\n<f") && strings.Count(s, "\n") > 1 {
		t.Error("el XML no debe llevar sangrado")
	}
}

// Los ejemplos que distribuye el SRI deben sobrevivir a leer → escribir sin
// perder ningún elemento: lo comprueba el XSD sobre el resultado.
func TestIdaYVueltaEjemplosOficiales(t *testing.T) {
	for _, v := range []string{factura.V100, factura.V110, factura.V210} {
		doc, err := os.ReadFile(filepath.Join("..", "esquema", "testdata", "factura_V"+v+".xml"))
		if err != nil {
			t.Fatal(err)
		}
		doc = []byte(strings.Replace(string(doc), `version="version0"`, `version="`+v+`"`, 1))
		f, err := factura.Parse(doc)
		if err != nil {
			t.Fatalf("%s: %v", v, err)
		}
		out, _ := f.XML()
		if err := esquema.ValidarEstricto(out); err != nil {
			t.Errorf("%s: tras ida y vuelta no cumple el XSD: %v", v, err)
		}
		// Mismo número de elementos que el original (sin contar la raíz).
		if a, b := strings.Count(string(doc), "</")+strings.Count(string(doc), "/>"), strings.Count(string(out), "</")+strings.Count(string(out), "/>"); a != b {
			t.Errorf("%s: el original tiene %d elementos y el reescrito %d", v, a, b)
		}
	}
}

func TestReglas(t *testing.T) {
	casos := []struct {
		nombre string
		mutar  func(*factura.Factura)
		regla  string
		aviso  bool
	}{
		{"clave de otro secuencial", func(f *factura.Factura) { f.InfoTributaria.Secuencial = "000000124" }, "clave.coherencia", false},
		{"clave de otra fecha", func(f *factura.Factura) { f.InfoFactura.FechaEmision = comprobante.NuevaFecha(2026, 11, 13) }, "clave.coherencia", false},
		{"clave alterada", func(f *factura.Factura) { f.InfoTributaria.ClaveAcceso = f.InfoTributaria.ClaveAcceso[:48] + "x" }, "clave.formato", false},
		{"sin clave", func(f *factura.Factura) { f.InfoTributaria.ClaveAcceso = "" }, "requerido", false},
		{"fecha futura", func(f *factura.Factura) {
			f.InfoFactura.FechaEmision = comprobante.NuevaFecha(2026, 11, 15)
			f.GenerarClaveAcceso("12345678")
		}, "fecha.futura", false},
		{"RUC emisor", func(f *factura.Factura) { f.InfoTributaria.RUC = "9990000000001" }, "ruc.formato", false},
		{"importe alterado", func(f *factura.Factura) { f.InfoFactura.ImporteTotal = d("143.27") }, "totales.importe", false},
		{"total sin impuestos", func(f *factura.Factura) { f.InfoFactura.TotalSinImpuestos = d("1.00") }, "totales.sin-impuestos", false},
		{"línea alterada", func(f *factura.Factura) { f.Detalles[1].PrecioTotalSinImpuesto = d("31.00") }, "detalle.total", false},
		{"IVA de línea", func(f *factura.Factura) { f.Detalles[0].Impuestos[0].Valor = d("15.00") }, "iva.valor", false},
		{"tarifa no corresponde", func(f *factura.Factura) { f.Detalles[0].Impuestos[0].Tarifa = d("12") }, "iva.tarifa", false},
		{"código IVA inexistente", func(f *factura.Factura) { f.Detalles[0].Impuestos[0].CodigoPorcentaje = "99" }, "catalogo", false},
		{"total de impuesto", func(f *factura.Factura) { f.InfoFactura.TotalConImpuestos[0].Valor = d("20.00") }, "totales.impuesto", false},
		{"impuesto faltante en totales", func(f *factura.Factura) { f.InfoFactura.TotalConImpuestos = f.InfoFactura.TotalConImpuestos[:1] }, "totales.impuesto-faltante", false},
		{"sin detalles", func(f *factura.Factura) { f.Detalles = nil }, "requerido", false},
		{"sin pagos", func(f *factura.Factura) { f.InfoFactura.Pagos = nil }, "requerido", false},
		{"pagos no cuadran", func(f *factura.Factura) { f.InfoFactura.Pagos[0].Total = d("1.00") }, "pagos.suma", true},
		{"forma de pago inválida", func(f *factura.Factura) { f.InfoFactura.Pagos[0].FormaPago = "99" }, "catalogo", false},
		{"salto de línea", func(f *factura.Factura) { f.InfoTributaria.DirMatriz = "Calle A\nCalle B" }, "salto-de-linea", false},
		{"espacio final", func(f *factura.Factura) { f.Detalles[0].Descripcion = "Servicio " }, "espacios", false},
		{"descripción larga", func(f *factura.Factura) { f.Detalles[0].Descripcion = strings.Repeat("ñ", 301) }, "longitud", false},
		{"3 decimales en dinero", func(f *factura.Factura) { f.InfoFactura.Propina = d("1.005") }, "decimales", false},
		{"cantidad negativa", func(f *factura.Factura) { f.Detalles[0].Cantidad = d("-1") }, "negativo", false},
		{"consumidor final sobre el límite", func(f *factura.Factura) {
			f.InfoFactura.TipoIdentificacionComprador = comprobante.IDConsumidorFinal
			f.InfoFactura.IdentificacionComprador = comprobante.IdentificacionConsumidorFinal
		}, "consumidor-final.monto", false},
		{"consumidor final mal identificado", func(f *factura.Factura) {
			f.InfoFactura.TipoIdentificacionComprador = comprobante.IDConsumidorFinal
		}, "consumidor-final.identificacion", false},
		{"cédula con dígito erróneo", func(f *factura.Factura) {
			f.InfoFactura.TipoIdentificacionComprador = comprobante.IDCedula
			f.InfoFactura.IdentificacionComprador = "1710034066"
		}, "cedula.verificador", true},
		{"16 campos adicionales", func(f *factura.Factura) {
			for i := 0; i < 15; i++ {
				f.InfoAdicional = append(f.InfoAdicional, comprobante.CampoAdicional{Nombre: "n", Valor: "v"})
			}
		}, "maximo", false},
		{"RUC proveedor inválido (Anexo 26)", func(f *factura.Factura) {
			f.InfoAdicional = append(f.InfoAdicional, comprobante.CampoAdicional{Nombre: comprobante.NombreRUCProveedor, Valor: "123"})
		}, "ruc.formato", false},
		{"rubros de terceros en 1.1.0", func(f *factura.Factura) {
			f.OtrosRubrosTerceros = []factura.Rubro{{Concepto: "x", Total: d("0.00")}}
		}, "version", false},
		{"RIMPE mal escrito", func(f *factura.Factura) { f.InfoTributaria.ContribuyenteRimpe = "RIMPE" }, "catalogo", false},
		{"6 decimales en 1.0.0", func(f *factura.Factura) { f.Version = factura.V100 }, "decimales", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			f := base(t)
			c.mutar(f)
			hs := validar(f)
			if !hs.Tiene(c.regla) {
				t.Fatalf("no se detectó la regla %q; hallazgos: %v", c.regla, hs)
			}
			if c.aviso && hs.Err() != nil {
				t.Errorf("debía ser solo advertencia: %v", hs.Err())
			}
			if !c.aviso && hs.Err() == nil {
				t.Error("debía ser un error")
			}
		})
	}
}

func TestCedulaValidaYConsumidorFinal(t *testing.T) {
	f := base(t)
	f.InfoFactura.TipoIdentificacionComprador = comprobante.IDCedula
	f.InfoFactura.IdentificacionComprador = "1710034065"
	if hs := validar(f); len(hs) != 0 {
		t.Errorf("cédula válida rechazada: %v", hs)
	}
	g := &factura.Factura{InfoTributaria: f.InfoTributaria, InfoFactura: factura.InfoFactura{
		FechaEmision: hoy, TipoIdentificacionComprador: comprobante.IDConsumidorFinal,
		RazonSocialComprador:    comprobante.RazonSocialConsumidorFinal,
		IdentificacionComprador: comprobante.IdentificacionConsumidorFinal,
		Pagos:                   []comprobante.Pago{{FormaPago: comprobante.PagoSinSistemaFinanciero}},
	}, Detalles: []factura.Detalle{factura.LineaIVA("A", "Café", d("1"), d("43.47"), comprobante.IVA15)}}
	g.Calcular() // 43.47 + 6.52 = 49.99
	g.GenerarClaveAcceso("00000001")
	if hs := validar(g); len(hs) != 0 {
		t.Errorf("consumidor final bajo el límite rechazado: %v", hs)
	}
}

// El XSD publicado rechaza la leyenda de negocio popular que exige el Anexo
// 22; Validar la acepta gracias al ajuste documentado.
func TestRimpeNegocioPopular(t *testing.T) {
	f := base(t)
	f.InfoTributaria.ContribuyenteRimpe = comprobante.RimpeNegocioPopular
	if hs := validar(f); len(hs) != 0 {
		t.Fatalf("hallazgos: %v", hs)
	}
	doc, _ := f.XML()
	if err := esquema.Validar(doc); err != nil {
		t.Errorf("Validar con ajustes: %v", err)
	}
	if err := esquema.ValidarEstricto(doc); err == nil {
		t.Error("el XSD publicado dejó de rechazar la leyenda: revise si el ajuste sigue siendo necesario")
	}
}
