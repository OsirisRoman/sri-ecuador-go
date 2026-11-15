package guiaremision_test

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
	"github.com/OsirisRoman/sri-ecuador-go/guiaremision"
	"github.com/OsirisRoman/sri-ecuador-go/internal/certprueba"
	"github.com/OsirisRoman/sri-ecuador-go/internal/ejemplo"
	"github.com/OsirisRoman/sri-ecuador-go/sritest"
	"github.com/OsirisRoman/sri-ecuador-go/ws"
)

var hoy = comprobante.NuevaFecha(2026, 11, 14)

func d(s string) decimal.Decimal { return decimal.MustParse(s) }

func base(t *testing.T, inicio comprobante.Fecha) *guiaremision.GuiaRemision {
	t.Helper()
	dest := guiaremision.Destinatario{
		IdentificacionDestinatario: "1760013210001",
		RazonSocialDestinatario:    "PRUEBAS SERVICIO DE RENTAS INTERNAS",
		DirDestinatario:            "Av. Amazonas y Unión Nacional de Periodistas",
		MotivoTraslado:             "Venta",
		Ruta:                       "Quito - Cayambe",
		Detalles: []guiaremision.Detalle{
			{CodigoInterno: "P-1", Descripcion: "Caja de repuestos", Cantidad: d("10.254632"),
				DetallesAdicionales: []comprobante.DetAdicional{{Nombre: "Lote", Valor: "L-77"}}},
			{Descripcion: "Pallet", Cantidad: d("2")},
		},
	}
	dest.Sustento(comprobante.CodFactura, "001-001-000000045", "1411202601179214673900110010010000000451234567811", comprobante.NuevaFecha(2026, 11, 13))
	g := &guiaremision.GuiaRemision{
		InfoTributaria: comprobante.InfoTributaria{
			Ambiente: comprobante.Pruebas, RazonSocial: "EMISOR DE PRUEBA S.A.", RUC: ejemplo.RUC,
			Estab: "001", PtoEmi: "001", Secuencial: "000000007", DirMatriz: "Av. Amazonas N34-10 y Roca",
		},
		InfoGuiaRemision: guiaremision.InfoGuiaRemision{
			DirPartida:                      "Av. Eloy Alfaro 34 y Av. Libertad",
			RazonSocialTransportista:        "TRANSPORTES DE PRUEBA S.A.",
			TipoIdentificacionTransportista: comprobante.IDRuc,
			RucTransportista:                "1790011674001",
			ObligadoContabilidad:            "SI",
			FechaIniTransporte:              inicio,
			FechaFinTransporte:              inicio,
			Placa:                           "PBA1234",
		},
		Destinatarios: []guiaremision.Destinatario{dest},
		InfoAdicional: []comprobante.CampoAdicional{{Nombre: "Teléfono", Valor: "022000000"}},
	}
	if _, err := g.GenerarClaveAcceso("12345678"); err != nil {
		t.Fatal(err)
	}
	return g
}

func validar(g *guiaremision.GuiaRemision) comprobante.Hallazgos {
	return g.ValidarCon(guiaremision.OpcionesValidacion{Hoy: hoy})
}

func TestValidarXSDYIdaYVuelta(t *testing.T) {
	g := base(t, hoy)
	if c := g.InfoTributaria.ClaveAcceso; c[:8] != "14112026" || c[8:10] != "06" {
		t.Errorf("clave = %s", c)
	}
	if !g.FechaEmision().Time().Equal(hoy.Time()) {
		t.Errorf("FechaEmision = %s", g.FechaEmision())
	}
	if hs := validar(g); len(hs) != 0 {
		t.Fatalf("hallazgos: %v", hs)
	}
	doc, err := g.XML()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc), `version="1.1.0"`) {
		t.Error("la versión por defecto debe ser 1.1.0")
	}
	if err := esquema.ValidarEstricto(doc); err != nil {
		t.Fatal(err)
	}
	otra, err := guiaremision.Parse(doc)
	if err != nil {
		t.Fatal(err)
	}
	if doc2, _ := otra.XML(); string(doc) != string(doc2) {
		t.Error("ida y vuelta no es estable")
	}
}

func TestVersion100(t *testing.T) {
	g := base(t, hoy)
	g.Version = guiaremision.V100
	if hs := validar(g); !hs.Tiene("decimales") {
		t.Errorf("v1.0.0 solo admite 2 decimales en cantidad: %v", hs)
	}
	g.Destinatarios[0].Detalles[0].Cantidad = d("10.25")
	if hs := validar(g); len(hs) != 0 {
		t.Fatalf("hallazgos: %v", hs)
	}
	doc, _ := g.XML()
	if err := esquema.ValidarEstricto(doc); err != nil {
		t.Fatal(err)
	}
}

func TestEmisionAnteriorAlTraslado(t *testing.T) {
	// Guía emitida hoy para un traslado de mañana.
	manana := comprobante.NuevaFecha(2026, 11, 15)
	g := base(t, manana)
	if hs := validar(g); !hs.Tiene("fecha.futura") {
		t.Errorf("la clave con fecha futura debe rechazarse: %v", hs)
	}
	if _, err := g.GenerarClaveAccesoEn(hoy, "12345678"); err != nil {
		t.Fatal(err)
	}
	if hs := validar(g); len(hs) != 0 {
		t.Fatalf("hallazgos: %v", hs)
	}
}

func TestIdaYVueltaEjemplosOficiales(t *testing.T) {
	for _, ver := range []string{"1.0.0", "1.1.0"} {
		doc, err := os.ReadFile(filepath.Join("..", "esquema", "testdata", "GuiaRemision_V"+ver+".xml"))
		if err != nil {
			t.Fatal(err)
		}
		doc = []byte(strings.Replace(string(doc), `version="version0"`, `version="`+ver+`"`, 1))
		g, err := guiaremision.Parse(doc)
		if err != nil {
			t.Fatal(err)
		}
		if len(g.Destinatarios) != 2 || len(g.Destinatarios[1].Detalles) != 2 {
			t.Errorf("%s: destinatarios = %+v", ver, g.Destinatarios)
		}
		out, _ := g.XML()
		if err := esquema.ValidarEstricto(out); err != nil {
			t.Errorf("%s: tras ida y vuelta no cumple el XSD: %v", ver, err)
		}
		// El ejemplo trae <detallesAdicionales> vacíos, que al reescribir se
		// omiten.
		cuenta := func(s string) int {
			return strings.Count(s, "</") + strings.Count(s, "/>") - strings.Count(s, "</detallesAdicionales>")
		}
		if a, b := cuenta(string(doc)), cuenta(string(out)); a != b {
			t.Errorf("%s: el original tiene %d elementos y el reescrito %d", ver, a, b)
		}
	}
}

func TestReglas(t *testing.T) {
	casos := []struct {
		nombre string
		mutar  func(*guiaremision.GuiaRemision)
		regla  string
	}{
		{"sin destinatarios", func(g *guiaremision.GuiaRemision) { g.Destinatarios = nil }, "requerido"},
		{"sin detalles", func(g *guiaremision.GuiaRemision) { g.Destinatarios[0].Detalles = nil }, "requerido"},
		{"sin placa", func(g *guiaremision.GuiaRemision) { g.InfoGuiaRemision.Placa = "" }, "requerido"},
		{"sin partida", func(g *guiaremision.GuiaRemision) { g.InfoGuiaRemision.DirPartida = "" }, "requerido"},
		{"fin antes del inicio", func(g *guiaremision.GuiaRemision) {
			g.InfoGuiaRemision.FechaFinTransporte = comprobante.NuevaFecha(2026, 11, 13)
		}, "fecha.transporte"},
		{"inicio antes de la emisión (error 82)", func(g *guiaremision.GuiaRemision) {
			g.InfoGuiaRemision.FechaIniTransporte = comprobante.NuevaFecha(2026, 11, 13)
		}, "fecha.inicio-transporte"},
		{"transportista consumidor final", func(g *guiaremision.GuiaRemision) {
			g.InfoGuiaRemision.TipoIdentificacionTransportista = comprobante.IDConsumidorFinal
		}, "consumidor-final.no-permitido"},
		{"RUC del transportista", func(g *guiaremision.GuiaRemision) { g.InfoGuiaRemision.RucTransportista = "123" }, "ruc.formato"},
		{"número de sustento", func(g *guiaremision.GuiaRemision) { g.Destinatarios[0].NumDocSustento = "1-1-45" }, "formato"},
		{"autorización de sustento", func(g *guiaremision.GuiaRemision) { g.Destinatarios[0].NumAutDocSustento = "ABC" }, "formato"},
		{"establecimiento destino", func(g *guiaremision.GuiaRemision) { g.Destinatarios[0].CodEstabDestino = "1" }, "formato"},
		{"cantidad negativa", func(g *guiaremision.GuiaRemision) { g.Destinatarios[0].Detalles[0].Cantidad = d("-1") }, "negativo"},
		{"clave de otro secuencial", func(g *guiaremision.GuiaRemision) { g.InfoTributaria.Secuencial = "000000008" }, "clave.coherencia"},
		{"versión", func(g *guiaremision.GuiaRemision) { g.Version = "2.0.0" }, "version"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			g := base(t, hoy)
			c.mutar(g)
			if hs := validar(g); !hs.Tiene(c.regla) || hs.Err() == nil {
				t.Errorf("no se detectó %q: %v", c.regla, hs)
			}
		})
	}
	g := base(t, hoy)
	g.Destinatarios[0].NumAutDocSustento = ""
	if hs := validar(g); !hs.Tiene("sustento.incompleto") || hs.Err() != nil {
		t.Errorf("el sustento incompleto debe ser solo advertencia: %v", hs)
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
