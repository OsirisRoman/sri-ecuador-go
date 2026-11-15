package liquidacion_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sri "github.com/OsirisRoman/sri-ecuador-go"
	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
	"github.com/OsirisRoman/sri-ecuador-go/esquema"
	"github.com/OsirisRoman/sri-ecuador-go/factura"
	"github.com/OsirisRoman/sri-ecuador-go/internal/certprueba"
	"github.com/OsirisRoman/sri-ecuador-go/internal/ejemplo"
	"github.com/OsirisRoman/sri-ecuador-go/liquidacion"
	"github.com/OsirisRoman/sri-ecuador-go/sritest"
	"github.com/OsirisRoman/sri-ecuador-go/ws"
)

var hoy = comprobante.NuevaFecha(2026, 11, 14)

func d(s string) decimal.Decimal { return decimal.MustParse(s) }

func base(t *testing.T, fecha comprobante.Fecha) *liquidacion.LiquidacionCompra {
	t.Helper()
	l := &liquidacion.LiquidacionCompra{
		InfoTributaria: comprobante.InfoTributaria{
			Ambiente: comprobante.Pruebas, RazonSocial: "EMISOR DE PRUEBA S.A.", RUC: ejemplo.RUC,
			Estab: "001", PtoEmi: "001", Secuencial: "000000009", DirMatriz: "Av. Amazonas N34-10 y Roca",
		},
		InfoLiquidacionCompra: liquidacion.InfoLiquidacionCompra{
			FechaEmision:                fecha,
			ObligadoContabilidad:        "SI",
			TipoIdentificacionProveedor: comprobante.IDCedula,
			RazonSocialProveedor:        "PROVEEDOR DE PRUEBA",
			IdentificacionProveedor:     "1710034065",
			DireccionProveedor:          "Vía a la Costa km 12",
			Pagos:                       []comprobante.Pago{{FormaPago: comprobante.PagoSinSistemaFinanciero}},
		},
		Detalles: []liquidacion.Detalle{
			liquidacion.LineaIVA("CACAO", "Cacao en grano", d("12.345678"), d("3.25"), comprobante.IVA0),
			liquidacion.LineaIVA("FLETE", "Servicio de acarreo", d("1"), d("20.00"), comprobante.IVA15),
		},
	}
	l.Calcular()
	if _, err := l.GenerarClaveAcceso("12345678"); err != nil {
		t.Fatal(err)
	}
	return l
}

func validar(l *liquidacion.LiquidacionCompra) comprobante.Hallazgos {
	return l.ValidarCon(liquidacion.OpcionesValidacion{Hoy: hoy})
}

func TestCalcularValidarYXSD(t *testing.T) {
	l := base(t, hoy)
	i := l.InfoLiquidacionCompra
	// 12.345678 × 3.25 = 40.1234535 → 40.12; + 20.00 = 60.12; IVA 15 % de 20 = 3.00
	if i.TotalSinImpuestos.String() != "60.12" || i.ImporteTotal.String() != "63.12" || i.Pagos[0].Total.String() != "63.12" || len(i.TotalConImpuestos) != 2 {
		t.Errorf("totales = %s %s %+v", i.TotalSinImpuestos, i.ImporteTotal, i.TotalConImpuestos)
	}
	if l.InfoTributaria.ClaveAcceso[8:10] != "03" {
		t.Errorf("clave = %s", l.InfoTributaria.ClaveAcceso)
	}
	if hs := validar(l); len(hs) != 0 {
		t.Fatalf("hallazgos: %v", hs)
	}
	doc, err := l.XML()
	if err != nil {
		t.Fatal(err)
	}
	if err := esquema.ValidarEstricto(doc); err != nil {
		t.Fatal(err)
	}
	otra, err := liquidacion.Parse(doc)
	if err != nil {
		t.Fatal(err)
	}
	if doc2, _ := otra.XML(); string(doc) != string(doc2) {
		t.Error("ida y vuelta no es estable")
	}
}

func TestVersion100(t *testing.T) {
	l := base(t, hoy)
	l.Version = liquidacion.V100
	if hs := validar(l); !hs.Tiene("decimales") {
		t.Errorf("v1.0.0 solo admite 2 decimales: %v", hs)
	}
	l.Detalles[0].Cantidad = d("12.35")
	l.InfoLiquidacionCompra.Pagos[0].Total = decimal.Cero // para que Calcular lo reasigne
	l.Calcular()
	if hs := validar(l); len(hs) != 0 {
		t.Fatalf("hallazgos: %v", hs)
	}
	doc, _ := l.XML()
	if err := esquema.ValidarEstricto(doc); err != nil {
		t.Fatal(err)
	}
}

func TestReembolsos(t *testing.T) {
	l := base(t, hoy)
	l.Reembolsos = []liquidacion.ReembolsoDetalle{{
		TipoIdentificacionProveedorReembolso: comprobante.IDRuc,
		IdentificacionProveedorReembolso:     "1790011674001",
		CodPaisPagoProveedorReembolso:        "593",
		TipoProveedorReembolso:               "02",
		CodDocReembolso:                      "01",
		EstabDocReembolso:                    "001", PtoEmiDocReembolso: "002", SecuencialDocReembolso: "000000123",
		FechaEmisionDocReembolso:   comprobante.NuevaFecha(2026, 11, 10),
		NumeroAutorizacionDocReemb: "1011202601179001167400110010020000001231234567818",
		DetalleImpuestos: []factura.ImpuestoReembolso{{
			Codigo: comprobante.ImpuestoIVA, CodigoPorcentaje: comprobante.IVA15, Tarifa: d("15"),
			BaseImponibleReembolso: d("50.00"), ImpuestoReembolso: d("7.50"),
		}},
	}}
	if hs := validar(l); !hs.Tiene("reembolso.codigo") {
		t.Errorf("sin totales de reembolso debe fallar: %v", hs)
	}
	l.Calcular()
	i := l.InfoLiquidacionCompra
	if i.CodDocReembolso != "41" || i.TotalComprobantesReembolso.String() != "57.50" || i.TotalImpuestoReembolso.String() != "7.50" {
		t.Errorf("reembolso = %+v", i)
	}
	if hs := validar(l); len(hs) != 0 {
		t.Fatalf("hallazgos: %v", hs)
	}
	doc, _ := l.XML()
	if err := esquema.ValidarEstricto(doc); err != nil {
		t.Fatal(err)
	}
	uno := d("1.00")
	l.InfoLiquidacionCompra.TotalImpuestoReembolso = &uno
	if hs := validar(l); !hs.Tiene("reembolso.totales") {
		t.Errorf("total de reembolso alterado: %v", hs)
	}
}

func TestIdaYVueltaEjemplosOficiales(t *testing.T) {
	for _, ver := range []string{"1.0.0", "1.1.0"} {
		doc, err := os.ReadFile(filepath.Join("..", "esquema", "testdata", "LiquidacionCompra_V"+ver+".xml"))
		if err != nil {
			t.Fatal(err)
		}
		doc = []byte(strings.Replace(string(doc), `version="version0"`, `version="`+ver+`"`, 1))
		l, err := liquidacion.Parse(doc)
		if err != nil {
			t.Fatal(err)
		}
		out, _ := l.XML()
		if err := esquema.ValidarEstricto(out); err != nil {
			t.Errorf("%s: tras ida y vuelta no cumple el XSD: %v", ver, err)
		}
		cuenta := func(s string) int { return strings.Count(s, "</") + strings.Count(s, "/>") }
		if a, b := cuenta(string(doc)), cuenta(string(out)); a != b {
			t.Errorf("%s: el original tiene %d elementos y el reescrito %d", ver, a, b)
		}
	}
}

func TestReglas(t *testing.T) {
	casos := []struct {
		nombre string
		mutar  func(*liquidacion.LiquidacionCompra)
		regla  string
	}{
		{"consumidor final", func(l *liquidacion.LiquidacionCompra) {
			l.InfoLiquidacionCompra.TipoIdentificacionProveedor = comprobante.IDConsumidorFinal
		}, "consumidor-final.no-permitido"},
		{"sin detalles", func(l *liquidacion.LiquidacionCompra) { l.Detalles = nil }, "requerido"},
		{"sin pagos", func(l *liquidacion.LiquidacionCompra) { l.InfoLiquidacionCompra.Pagos = nil }, "requerido"},
		{"sin proveedor", func(l *liquidacion.LiquidacionCompra) { l.InfoLiquidacionCompra.RazonSocialProveedor = "" }, "requerido"},
		{"total alterado", func(l *liquidacion.LiquidacionCompra) { l.InfoLiquidacionCompra.ImporteTotal = d("1.00") }, "totales.importe"},
		{"línea alterada", func(l *liquidacion.LiquidacionCompra) { l.Detalles[0].PrecioTotalSinImpuesto = d("1.00") }, "detalle.total"},
		{"IVA alterado", func(l *liquidacion.LiquidacionCompra) { l.Detalles[1].Impuestos[0].Valor = d("9.00") }, "iva.valor"},
		{"fecha futura", func(l *liquidacion.LiquidacionCompra) {
			l.InfoLiquidacionCompra.FechaEmision = comprobante.NuevaFecha(2026, 12, 1)
		}, "fecha.futura"},
		{"reembolso sin detalle", func(l *liquidacion.LiquidacionCompra) { l.InfoLiquidacionCompra.CodDocReembolso = "41" }, "requerido"},
		{"versión", func(l *liquidacion.LiquidacionCompra) { l.Version = "2.0.0" }, "version"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			l := base(t, hoy)
			c.mutar(l)
			if hs := validar(l); !hs.Tiene(c.regla) || hs.Err() == nil {
				t.Errorf("no se detectó %q: %v", c.regla, hs)
			}
		})
	}
}

func TestEmitirContraElSimulador(t *testing.T) {
	m, err := certprueba.Obtener()
	if err != nil {
		t.Fatal(err)
	}
	srv := sritest.Nuevo()
	defer srv.Cerrar()
	c, err := sri.Nuevo(sri.Config{
		Ambiente: comprobante.Pruebas, Certificado: m.Certificado,
		Servicios: ws.Nuevo(comprobante.Pruebas, ws.ConEndpoints(srv.Endpoints())),
		Espera:    sri.Espera{Inicial: time.Millisecond, Intervalo: time.Millisecond, Consultas: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Emitir(context.Background(), base(t, comprobante.Hoy()))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Autorizado() {
		t.Fatalf("estado = %s %v", res.Estado, res.Mensajes)
	}
}
