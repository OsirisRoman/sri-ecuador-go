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
	"github.com/OsirisRoman/sri-ecuador-go/esquema"
	"github.com/OsirisRoman/sri-ecuador-go/internal/certprueba"
	"github.com/OsirisRoman/sri-ecuador-go/internal/ejemplo"
	"github.com/OsirisRoman/sri-ecuador-go/retencion"
	"github.com/OsirisRoman/sri-ecuador-go/sritest"
	"github.com/OsirisRoman/sri-ecuador-go/ws"
)

func baseV1(t *testing.T, fecha comprobante.Fecha) *retencion.ComprobanteV1 {
	t.Helper()
	sustento := comprobante.NuevaFecha(2026, 11, 10)
	c := &retencion.ComprobanteV1{
		InfoTributaria: comprobante.InfoTributaria{
			Ambiente: comprobante.Pruebas, RazonSocial: "EMISOR DE PRUEBA S.A.", RUC: ejemplo.RUC,
			Estab: "001", PtoEmi: "001", Secuencial: "000000011", DirMatriz: "Av. Amazonas N34-10 y Roca",
		},
		InfoCompRetencion: retencion.InfoV1{
			FechaEmision:                     fecha,
			ObligadoContabilidad:             "SI",
			TipoIdentificacionSujetoRetenido: comprobante.IDRuc,
			RazonSocialSujetoRetenido:        "PROVEEDOR DE PRUEBA S.A.",
			IdentificacionSujetoRetenido:     "1760013210001",
		},
		Impuestos: []retencion.ImpuestoV1{
			{Codigo: retencion.ImpuestoRenta, CodigoRetencion: "312", BaseImponible: d("200.00"), PorcentajeRetener: d("2"),
				CodDocSustento: "01", NumDocSustento: "001001000000123", FechaEmisionDocSustento: &sustento},
			{Codigo: retencion.ImpuestoIVA, CodigoRetencion: "1", BaseImponible: d("30.00"), PorcentajeRetener: d("30"),
				CodDocSustento: "01", NumDocSustento: "001001000000123", FechaEmisionDocSustento: &sustento},
		},
	}
	c.Calcular()
	if _, err := c.GenerarClaveAcceso("12345678"); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestV1CalcularValidarYXSD(t *testing.T) {
	c := baseV1(t, hoy)
	if c.Impuestos[0].ValorRetenido.String() != "4.00" || c.Impuestos[1].ValorRetenido.String() != "9.00" ||
		c.TotalRetenido().String() != "13.00" || c.InfoCompRetencion.PeriodoFiscal != "11/2026" {
		t.Errorf("cálculo = %+v", c.Impuestos)
	}
	if hs := c.ValidarCon(retencion.OpcionesValidacion{Hoy: hoy}); len(hs) != 0 {
		t.Fatalf("hallazgos: %v", hs)
	}
	doc, err := c.XML()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc), `version="1.0.0"`) {
		t.Error("debe declarar la versión 1.0.0")
	}
	if err := esquema.ValidarEstricto(doc); err != nil {
		t.Fatal(err)
	}
	otro, err := retencion.ParseV1(doc)
	if err != nil {
		t.Fatal(err)
	}
	if doc2, _ := otro.XML(); string(doc) != string(doc2) {
		t.Error("ida y vuelta no es estable")
	}
}

func TestV1IdaYVueltaEjemploOficial(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "esquema", "testdata", "ComprobanteRetencion_V1.0.0.xml"))
	if err != nil {
		t.Fatal(err)
	}
	doc = []byte(strings.Replace(string(doc), `version="version0"`, `version="1.0.0"`, 1))
	c, err := retencion.ParseV1(doc)
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

func TestV1Reglas(t *testing.T) {
	casos := []struct {
		nombre string
		mutar  func(*retencion.ComprobanteV1)
		regla  string
		aviso  bool
	}{
		{"sin impuestos", func(c *retencion.ComprobanteV1) { c.Impuestos = nil }, "requerido", false},
		{"consumidor final", func(c *retencion.ComprobanteV1) {
			c.InfoCompRetencion.TipoIdentificacionSujetoRetenido = comprobante.IDConsumidorFinal
		}, "consumidor-final.no-permitido", false},
		{"valor alterado", func(c *retencion.ComprobanteV1) { c.Impuestos[0].ValorRetenido = d("9.99") }, "retencion.valor", false},
		{"porcentaje de IVA", func(c *retencion.ComprobanteV1) { c.Impuestos[1].PorcentajeRetener = d("70") }, "retencion.porcentaje", false},
		{"porcentaje de renta", func(c *retencion.ComprobanteV1) {
			c.Impuestos[0].PorcentajeRetener = d("1.75")
			c.Calcular()
		}, "retencion.porcentaje-renta", true},
		{"número de sustento", func(c *retencion.ComprobanteV1) { c.Impuestos[0].NumDocSustento = "001-001-000000123" }, "formato", false},
		{"sustento posterior", func(c *retencion.ComprobanteV1) {
			f := comprobante.NuevaFecha(2026, 12, 1)
			c.Impuestos[0].FechaEmisionDocSustento = &f
		}, "fecha.sustento", false},
		{"período", func(c *retencion.ComprobanteV1) { c.InfoCompRetencion.PeriodoFiscal = "2026-11" }, "formato", false},
		{"versión", func(c *retencion.ComprobanteV1) { c.Version = "2.0.0" }, "version", false},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			c := baseV1(t, hoy)
			caso.mutar(c)
			hs := c.ValidarCon(retencion.OpcionesValidacion{Hoy: hoy})
			if !hs.Tiene(caso.regla) || (hs.Err() == nil) != caso.aviso {
				t.Errorf("no se detectó %q como se esperaba: %v", caso.regla, hs)
			}
		})
	}
}

func TestV1EmitirContraElSimulador(t *testing.T) {
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
	res, err := cli.Emitir(context.Background(), baseV1(t, comprobante.Hoy()))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Autorizado() {
		t.Fatalf("estado = %s %v", res.Estado, res.Mensajes)
	}
}
