package retencion_test

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
	"github.com/OsirisRoman/sri-ecuador-go/internal/certprueba"
	"github.com/OsirisRoman/sri-ecuador-go/internal/ejemplo"
	"github.com/OsirisRoman/sri-ecuador-go/retencion"
	"github.com/OsirisRoman/sri-ecuador-go/sritest"
	"github.com/OsirisRoman/sri-ecuador-go/ws"
)

var hoy = comprobante.NuevaFecha(2026, 11, 14)

func d(s string) decimal.Decimal { return decimal.MustParse(s) }

func base(t *testing.T, fecha comprobante.Fecha) *retencion.ComprobanteRetencion {
	t.Helper()
	c := &retencion.ComprobanteRetencion{
		InfoTributaria: comprobante.InfoTributaria{
			Ambiente: comprobante.Pruebas, RazonSocial: "EMISOR DE PRUEBA S.A.", RUC: ejemplo.RUC,
			Estab: "001", PtoEmi: "001", Secuencial: "000000009", DirMatriz: "Av. Amazonas N34-10 y Roca",
		},
		InfoCompRetencion: retencion.InfoCompRetencion{
			FechaEmision:                     fecha,
			ObligadoContabilidad:             "SI",
			TipoIdentificacionSujetoRetenido: comprobante.IDRuc,
			ParteRel:                         "NO",
			RazonSocialSujetoRetenido:        "PROVEEDOR DE PRUEBA S.A.",
			IdentificacionSujetoRetenido:     "1760013210001",
		},
		DocsSustento: []retencion.DocSustento{{
			CodSustento: "01", CodDocSustento: "01", NumDocSustento: "001001000000123",
			FechaEmisionDocSustento: comprobante.NuevaFecha(2026, 11, 10),
			PagoLocExt:              retencion.PagoLocal,
			TotalSinImpuestos:       d("200.00"),
			ImpuestosDocSustento: []retencion.ImpuestoDocSustento{
				{CodImpuestoDocSustento: comprobante.ImpuestoIVA, CodigoPorcentaje: comprobante.IVA15, BaseImponible: d("200.00")},
			},
			Retenciones: []retencion.Retencion{
				{Codigo: retencion.ImpuestoRenta, CodigoRetencion: "312", BaseImponible: d("200.00"), PorcentajeRetener: d("1.75")},
				{Codigo: retencion.ImpuestoIVA, CodigoRetencion: "1", BaseImponible: d("30.00"), PorcentajeRetener: d("30")},
			},
			Pagos: []retencion.Pago{{FormaPago: comprobante.PagoOtrosSistFinanciero}},
		}},
	}
	c.Calcular()
	if _, err := c.GenerarClaveAcceso("12345678"); err != nil {
		t.Fatal(err)
	}
	return c
}

func validar(c *retencion.ComprobanteRetencion) comprobante.Hallazgos {
	return c.ValidarCon(retencion.OpcionesValidacion{Hoy: hoy})
}

func TestCalcularValidarYXSD(t *testing.T) {
	c := base(t, hoy)
	ds := c.DocsSustento[0]
	if ds.ImporteTotal.String() != "230.00" || ds.Retenciones[0].ValorRetenido.String() != "3.50" || ds.Retenciones[1].ValorRetenido.String() != "9.00" {
		t.Errorf("importes = %s %s %s", ds.ImporteTotal, ds.Retenciones[0].ValorRetenido, ds.Retenciones[1].ValorRetenido)
	}
	if c.TotalRetenido().String() != "12.50" || c.InfoCompRetencion.PeriodoFiscal != "11/2026" || ds.Pagos[0].Total.String() != "230.00" {
		t.Errorf("total retenido = %s, período = %s", c.TotalRetenido(), c.InfoCompRetencion.PeriodoFiscal)
	}
	if c.InfoTributaria.ClaveAcceso[8:10] != "07" {
		t.Errorf("clave = %s", c.InfoTributaria.ClaveAcceso)
	}
	if hs := validar(c); len(hs) != 0 {
		t.Fatalf("hallazgos: %v", hs)
	}
	doc, err := c.XML()
	if err != nil {
		t.Fatal(err)
	}
	if err := esquema.ValidarEstricto(doc); err != nil {
		t.Fatal(err)
	}
	// La tarifa admite 3 dígitos en total: debe escribirse 15, no 15.00.
	if !strings.Contains(string(doc), "<tarifa>15</tarifa>") || strings.Contains(string(doc), "fechaRegistroContable") {
		t.Errorf("serialización inesperada")
	}
	otra, err := retencion.Parse(doc)
	if err != nil {
		t.Fatal(err)
	}
	if doc2, _ := otra.XML(); string(doc) != string(doc2) {
		t.Error("ida y vuelta no es estable")
	}
}

func TestIdaYVueltaEjemploOficial(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "esquema", "testdata", "ComprobanteRetencion_V2.0.0.xml"))
	if err != nil {
		t.Fatal(err)
	}
	doc = []byte(strings.Replace(string(doc), `version="version0"`, `version="2.0.0"`, 1))
	c, err := retencion.Parse(doc)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := c.XML()
	if err := esquema.ValidarEstricto(out); err != nil {
		t.Errorf("tras ida y vuelta no cumple el XSD: %v", err)
	}
	cuenta := func(s string) int { return strings.Count(s, "</") + strings.Count(s, "/>") }
	if a, b := cuenta(string(doc)), cuenta(string(out)); a != b {
		t.Errorf("el original tiene %d elementos y el reescrito %d", a, b)
	}
}

func TestReglas(t *testing.T) {
	casos := []struct {
		nombre string
		mutar  func(*retencion.ComprobanteRetencion)
		regla  string
		aviso  bool
	}{
		{"consumidor final", func(c *retencion.ComprobanteRetencion) {
			c.InfoCompRetencion.TipoIdentificacionSujetoRetenido = comprobante.IDConsumidorFinal
		}, "consumidor-final.no-permitido", false},
		{"sin parteRel", func(c *retencion.ComprobanteRetencion) { c.InfoCompRetencion.ParteRel = "" }, "requerido", false},
		{"período", func(c *retencion.ComprobanteRetencion) { c.InfoCompRetencion.PeriodoFiscal = "2026-11" }, "formato", false},
		{"período de otro mes", func(c *retencion.ComprobanteRetencion) { c.InfoCompRetencion.PeriodoFiscal = "10/2026" }, "periodo", true},
		{"exterior sin tipo de sujeto", func(c *retencion.ComprobanteRetencion) {
			c.InfoCompRetencion.TipoIdentificacionSujetoRetenido = comprobante.IDExterior
		}, "requerido", false},
		{"número con guiones", func(c *retencion.ComprobanteRetencion) { c.DocsSustento[0].NumDocSustento = "001-001-000000123" }, "formato", false},
		{"valor retenido", func(c *retencion.ComprobanteRetencion) { c.DocsSustento[0].Retenciones[0].ValorRetenido = d("9.99") }, "retencion.valor", false},
		{"porcentaje con 4 dígitos", func(c *retencion.ComprobanteRetencion) {
			c.DocsSustento[0].Retenciones[0].PorcentajeRetener = d("1.750")
		}, "digitos", false},
		{"tarifa 15.00", func(c *retencion.ComprobanteRetencion) { c.DocsSustento[0].ImpuestosDocSustento[0].Tarifa = d("15.00") }, "digitos", false},
		{"porcentaje de IVA", func(c *retencion.ComprobanteRetencion) {
			c.DocsSustento[0].Retenciones[1].PorcentajeRetener = d("70")
			c.Calcular()
		}, "retencion.porcentaje", false},
		{"importe", func(c *retencion.ComprobanteRetencion) { c.DocsSustento[0].ImporteTotal = d("1.00") }, "totales.importe", false},
		{"pago exterior incompleto", func(c *retencion.ComprobanteRetencion) { c.DocsSustento[0].PagoLocExt = retencion.PagoExterior }, "requerido", false},
		{"sin retenciones", func(c *retencion.ComprobanteRetencion) { c.DocsSustento[0].Retenciones = nil }, "requerido", false},
		{"sustento posterior", func(c *retencion.ComprobanteRetencion) {
			c.DocsSustento[0].FechaEmisionDocSustento = comprobante.NuevaFecha(2026, 12, 1)
		}, "fecha.sustento", false},
		{"reembolso sin detalle", func(c *retencion.ComprobanteRetencion) { c.DocsSustento[0].CodDocSustento = "41" }, "requerido", false},
	}
	for _, k := range casos {
		t.Run(k.nombre, func(t *testing.T) {
			c := base(t, hoy)
			k.mutar(c)
			hs := validar(c)
			if !hs.Tiene(k.regla) {
				t.Fatalf("no se detectó %q: %v", k.regla, hs)
			}
			if k.aviso != (hs.Err() == nil) {
				t.Errorf("severidad inesperada: %v", hs)
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
	cli, err := sri.Nuevo(sri.Config{
		Ambiente: comprobante.Pruebas, Certificado: m.Certificado,
		Servicios: ws.Nuevo(comprobante.Pruebas, ws.ConEndpoints(srv.Endpoints())),
		Espera:    sri.Espera{Inicial: time.Millisecond, Intervalo: time.Millisecond, Consultas: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := cli.Emitir(context.Background(), base(t, comprobante.Hoy()))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Autorizado() {
		t.Fatalf("estado = %s %v", res.Estado, res.Mensajes)
	}
}
