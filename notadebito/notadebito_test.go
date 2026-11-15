package notadebito_test

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
	"github.com/OsirisRoman/sri-ecuador-go/notadebito"
	"github.com/OsirisRoman/sri-ecuador-go/sritest"
	"github.com/OsirisRoman/sri-ecuador-go/ws"
)

var hoy = comprobante.NuevaFecha(2026, 11, 14)

func d(s string) decimal.Decimal { return decimal.MustParse(s) }

func base(t *testing.T, fecha comprobante.Fecha) *notadebito.NotaDebito {
	t.Helper()
	n := &notadebito.NotaDebito{
		InfoTributaria: comprobante.InfoTributaria{
			Ambiente: comprobante.Pruebas, RazonSocial: "EMISOR DE PRUEBA S.A.", RUC: ejemplo.RUC,
			Estab: "001", PtoEmi: "001", Secuencial: "000000003", DirMatriz: "Av. Amazonas N34-10 y Roca",
		},
		InfoNotaDebito: notadebito.InfoNotaDebito{
			FechaEmision:                fecha,
			TipoIdentificacionComprador: comprobante.IDRuc,
			RazonSocialComprador:        "PRUEBAS SERVICIO DE RENTAS INTERNAS",
			IdentificacionComprador:     "1760013210001",
			CodDocModificado:            comprobante.CodFactura,
			NumDocModificado:            "001-001-000000045",
			FechaEmisionDocSustento:     comprobante.NuevaFecha(2026, 11, 1),
			Impuestos:                   []notadebito.Impuesto{notadebito.ImpuestoIVA(comprobante.IVA15)},
			Pagos:                       []comprobante.Pago{{FormaPago: comprobante.PagoOtrosSistFinanciero}},
		},
		Motivos: []notadebito.Motivo{{Razon: "Interés por mora", Valor: d("12.345")}, {Razon: "Gastos de cobranza", Valor: d("5.00")}},
	}
	n.Calcular()
	if _, err := n.GenerarClaveAcceso("12345678"); err != nil {
		t.Fatal(err)
	}
	return n
}

func validar(n *notadebito.NotaDebito) comprobante.Hallazgos {
	return n.ValidarCon(notadebito.OpcionesValidacion{Hoy: hoy})
}

func TestCalcularValidarYXSD(t *testing.T) {
	n := base(t, hoy)
	i := n.InfoNotaDebito
	// 12.35 + 5.00 = 17.35; IVA 15 % = 2.6025 → 2.60; total 19.95
	if i.TotalSinImpuestos.String() != "17.35" || i.Impuestos[0].Valor.String() != "2.60" || i.ValorTotal.String() != "19.95" || i.Pagos[0].Total.String() != "19.95" {
		t.Errorf("totales = %s %s %s", i.TotalSinImpuestos, i.Impuestos[0].Valor, i.ValorTotal)
	}
	if n.InfoTributaria.ClaveAcceso[8:10] != "05" {
		t.Errorf("clave = %s", n.InfoTributaria.ClaveAcceso)
	}
	if hs := validar(n); len(hs) != 0 {
		t.Fatalf("hallazgos: %v", hs)
	}
	doc, err := n.XML()
	if err != nil {
		t.Fatal(err)
	}
	if err := esquema.ValidarEstricto(doc); err != nil {
		t.Fatal(err)
	}
	otra, err := notadebito.Parse(doc)
	if err != nil {
		t.Fatal(err)
	}
	if doc2, _ := otra.XML(); string(doc) != string(doc2) {
		t.Error("ida y vuelta no es estable")
	}
}

func TestIdaYVueltaEjemploOficial(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "esquema", "testdata", "NotaDebito_V1.0.0.xml"))
	if err != nil {
		t.Fatal(err)
	}
	doc = []byte(strings.Replace(string(doc), `version="version0"`, `version="1.0.0"`, 1))
	n, err := notadebito.Parse(doc)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := n.XML()
	if err := esquema.ValidarEstricto(out); err != nil {
		t.Errorf("tras ida y vuelta no cumple el XSD: %v", err)
	}
	// El ejemplo repite el contenedor <pagos>; al reescribir, sus pagos
	// quedan en uno solo, así que el contenedor no se cuenta.
	cuenta := func(s string) int {
		return strings.Count(s, "</") + strings.Count(s, "/>") - strings.Count(s, "</pagos>")
	}
	if a, b := cuenta(string(doc)), cuenta(string(out)); a != b {
		t.Errorf("el original tiene %d elementos y el reescrito %d", a, b)
	}
}

func TestReglas(t *testing.T) {
	casos := []struct {
		nombre string
		mutar  func(*notadebito.NotaDebito)
		regla  string
	}{
		{"consumidor final", func(n *notadebito.NotaDebito) {
			n.InfoNotaDebito.TipoIdentificacionComprador = comprobante.IDConsumidorFinal
		}, "consumidor-final.no-permitido"},
		{"sin motivos", func(n *notadebito.NotaDebito) { n.Motivos = nil }, "requerido"},
		{"sin impuestos", func(n *notadebito.NotaDebito) { n.InfoNotaDebito.Impuestos = nil }, "requerido"},
		{"total alterado", func(n *notadebito.NotaDebito) { n.InfoNotaDebito.ValorTotal = d("1.00") }, "totales.importe"},
		{"suma de motivos", func(n *notadebito.NotaDebito) { n.Motivos[0].Valor = d("1.00") }, "totales.sin-impuestos"},
		{"IVA alterado", func(n *notadebito.NotaDebito) { n.InfoNotaDebito.Impuestos[0].Valor = d("9.00") }, "iva.valor"},
		{"base excesiva", func(n *notadebito.NotaDebito) { n.InfoNotaDebito.Impuestos[0].BaseImponible = d("100.00") }, "totales.base"},
		{"sustento posterior", func(n *notadebito.NotaDebito) {
			n.InfoNotaDebito.FechaEmisionDocSustento = comprobante.NuevaFecha(2026, 12, 1)
		}, "fecha.sustento"},
		{"versión", func(n *notadebito.NotaDebito) { n.Version = "1.1.0" }, "version"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			n := base(t, hoy)
			c.mutar(n)
			if hs := validar(n); !hs.Tiene(c.regla) || hs.Err() == nil {
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

func TestDevolucionIVANoAdmitida(t *testing.T) {
	n := base(t, hoy)
	cero := d("0.00")
	n.InfoNotaDebito.Impuestos[0].ValorDevolucionIva = &cero
	if hs := validar(n); len(hs) != 0 {
		t.Fatalf("un valor cero debe admitirse: %v", hs)
	}
	valor := d("1.00")
	n.InfoNotaDebito.Impuestos[0].ValorDevolucionIva = &valor
	if hs := validar(n); !hs.Tiene("devolucion-iva.no-admitida") || hs.Err() == nil {
		t.Fatalf("un valor positivo debe rechazarse: %v", hs)
	}
}
