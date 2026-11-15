package notacredito_test

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
	"github.com/OsirisRoman/sri-ecuador-go/notacredito"
	"github.com/OsirisRoman/sri-ecuador-go/sritest"
	"github.com/OsirisRoman/sri-ecuador-go/ws"
)

var (
	ayer = comprobante.NuevaFecha(2026, 11, 13)
	hoy  = comprobante.NuevaFecha(2026, 11, 14)
)

func d(s string) decimal.Decimal { return decimal.MustParse(s) }

func base(t *testing.T) *notacredito.NotaCredito {
	t.Helper()
	n := notacredito.DesdeFactura(ejemplo.Factura(ayer, "000000045"), "Devolución de mercadería")
	n.InfoNotaCredito.FechaEmision = hoy
	n.InfoTributaria.Secuencial = "000000007"
	if _, err := n.GenerarClaveAcceso("12345678"); err != nil {
		t.Fatal(err)
	}
	return n
}

func validar(n *notacredito.NotaCredito) comprobante.Hallazgos {
	return n.ValidarCon(notacredito.OpcionesValidacion{Hoy: hoy})
}

func TestDesdeFactura(t *testing.T) {
	n := base(t)
	i := n.InfoNotaCredito
	if i.CodDocModificado != "01" || i.NumDocModificado != "001-001-000000045" || i.FechaEmisionDocSustento.String() != "13/11/2026" {
		t.Errorf("documento modificado = %s %s %s", i.CodDocModificado, i.NumDocModificado, i.FechaEmisionDocSustento)
	}
	if i.TotalSinImpuestos.String() != "10.00" || i.ValorModificacion.String() != "11.50" || len(i.TotalConImpuestos) != 1 {
		t.Errorf("totales = %s %s", i.TotalSinImpuestos, i.ValorModificacion)
	}
	if n.InfoTributaria.CodDoc != "04" || n.InfoTributaria.ClaveAcceso[8:10] != "04" {
		t.Errorf("codDoc = %s, clave = %s", n.InfoTributaria.CodDoc, n.InfoTributaria.ClaveAcceso)
	}
	if hs := validar(n); len(hs) != 0 {
		t.Errorf("hallazgos: %v", hs)
	}
}

func TestCumpleXSDEIdaYVuelta(t *testing.T) {
	for _, v := range []string{notacredito.V100, notacredito.V110} {
		n := base(t)
		n.Version = v
		doc, err := n.XML()
		if err != nil {
			t.Fatal(err)
		}
		if err := esquema.ValidarEstricto(doc); err != nil {
			t.Fatalf("%s: %v", v, err)
		}
		otra, err := notacredito.Parse(doc)
		if err != nil {
			t.Fatal(err)
		}
		if doc2, _ := otra.XML(); string(doc) != string(doc2) {
			t.Errorf("%s: ida y vuelta no es estable", v)
		}
	}
}

func TestIdaYVueltaEjemplosOficiales(t *testing.T) {
	for _, v := range []string{notacredito.V100, notacredito.V110} {
		doc, err := os.ReadFile(filepath.Join("..", "esquema", "testdata", "NotaCredito_V"+v+".xml"))
		if err != nil {
			t.Fatal(err)
		}
		doc = []byte(strings.Replace(string(doc), `version="version0"`, `version="`+v+`"`, 1))
		n, err := notacredito.Parse(doc)
		if err != nil {
			t.Fatalf("%s: %v", v, err)
		}
		out, _ := n.XML()
		if err := esquema.ValidarEstricto(out); err != nil {
			t.Errorf("%s: tras ida y vuelta no cumple el XSD: %v", v, err)
		}
		cuenta := func(s string) int { return strings.Count(s, "</") + strings.Count(s, "/>") }
		if a, b := cuenta(string(doc)), cuenta(string(out)); a != b {
			t.Errorf("%s: el original tiene %d elementos y el reescrito %d", v, a, b)
		}
	}
}

func TestReglas(t *testing.T) {
	casos := []struct {
		nombre string
		mutar  func(*notacredito.NotaCredito)
		regla  string
	}{
		{"consumidor final no permitido", func(n *notacredito.NotaCredito) {
			n.InfoNotaCredito.TipoIdentificacionComprador = comprobante.IDConsumidorFinal
			n.InfoNotaCredito.IdentificacionComprador = comprobante.IdentificacionConsumidorFinal
		}, "consumidor-final.no-permitido"},
		{"sin motivo", func(n *notacredito.NotaCredito) { n.InfoNotaCredito.Motivo = "" }, "requerido"},
		{"número de documento", func(n *notacredito.NotaCredito) { n.InfoNotaCredito.NumDocModificado = "1-1-45" }, "formato"},
		{"sustento posterior", func(n *notacredito.NotaCredito) {
			n.InfoNotaCredito.FechaEmisionDocSustento = comprobante.NuevaFecha(2026, 11, 20)
		}, "fecha.sustento"},
		{"valor alterado", func(n *notacredito.NotaCredito) { n.InfoNotaCredito.ValorModificacion = d("99.00") }, "totales.importe"},
		{"total con tarifa", func(n *notacredito.NotaCredito) { x := d("15"); n.InfoNotaCredito.TotalConImpuestos[0].Tarifa = &x }, "esquema"},
		{"clave de factura", func(n *notacredito.NotaCredito) {
			n.InfoTributaria.ClaveAcceso = ejemplo.Factura(hoy, "000000007").InfoTributaria.ClaveAcceso
		}, "clave.coherencia"},
		{"línea alterada", func(n *notacredito.NotaCredito) { n.Detalles[0].PrecioTotalSinImpuesto = d("1.00") }, "detalle.total"},
		{"6 decimales en 1.0.0", func(n *notacredito.NotaCredito) {
			n.Version = notacredito.V100
			n.Detalles[0].PrecioUnitario = d("10.000001")
		}, "decimales"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			n := base(t)
			c.mutar(n)
			if hs := validar(n); !hs.Tiene(c.regla) || hs.Err() == nil {
				t.Errorf("no se detectó %q: %v", c.regla, hs)
			}
		})
	}
}

func TestDevolucionParcial(t *testing.T) {
	n := base(t)
	n.Detalles[0].Cantidad = d("0.5")
	n.Calcular()
	if got := n.InfoNotaCredito.ValorModificacion.String(); got != "5.75" {
		t.Errorf("valor = %s", got)
	}
	if hs := validar(n); len(hs) != 0 {
		t.Errorf("hallazgos: %v", hs)
	}
}

// La nota de crédito recorre el mismo flujo que la factura.
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
	n := notacredito.DesdeFactura(ejemplo.Factura(comprobante.Hoy(), "000000045"), "Anulación")
	n.InfoTributaria.Secuencial = "000000001"
	n.GenerarClaveAcceso("")
	res, err := c.Emitir(context.Background(), n)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Autorizado() {
		t.Fatalf("estado = %s %v", res.Estado, res.Mensajes)
	}
}
