package sri_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	sri "github.com/OsirisRoman/sri-ecuador-go"
	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/esquema"
	"github.com/OsirisRoman/sri-ecuador-go/internal/certprueba"
	"github.com/OsirisRoman/sri-ecuador-go/internal/ejemplo"
	"github.com/OsirisRoman/sri-ecuador-go/sritest"
	"github.com/OsirisRoman/sri-ecuador-go/ws"
	"github.com/OsirisRoman/sri-ecuador-go/xades"
)

var sinEspera = sri.Espera{Inicial: time.Millisecond, Intervalo: time.Millisecond, Consultas: 5}

func entorno(t *testing.T, alg xades.Algoritmo) (*sri.Cliente, *sritest.Servidor) {
	t.Helper()
	m, err := certprueba.Obtener()
	if err != nil {
		t.Fatal(err)
	}
	srv := sritest.Nuevo()
	t.Cleanup(srv.Cerrar)
	servicios := ws.Nuevo(comprobante.Pruebas, ws.ConEndpoints(srv.Endpoints()),
		ws.ConReintentos(ws.Reintentos{Intentos: 3, Espera: time.Millisecond}))
	c, err := sri.Nuevo(sri.Config{
		Ambiente: comprobante.Pruebas, Certificado: m.Certificado, Algoritmo: alg,
		RUC: ejemplo.RUC, Servicios: servicios, Espera: sinEspera,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c, srv
}

func TestEmitirFlujoCompleto(t *testing.T) {
	for _, alg := range []xades.Algoritmo{xades.SHA1, xades.SHA256} {
		t.Run(alg.String(), func(t *testing.T) {
			c, srv := entorno(t, alg)
			srv.ConsultasEnProceso = 2 // las dos primeras consultas no traen resultado
			f := ejemplo.Factura(comprobante.Hoy(), "000000001")
			res, err := c.Emitir(context.Background(), f)
			if err != nil {
				t.Fatal(err)
			}
			if !res.Autorizado() {
				t.Fatalf("estado = %s, mensajes = %v", res.Estado, res.Mensajes)
			}
			if res.ClaveAcceso != f.InfoTributaria.ClaveAcceso || res.Autorizacion.NumeroAutorizacion != res.ClaveAcceso {
				t.Error("la clave de acceso debe ser el número de autorización (ficha 5.9)")
			}
			if _, ok := res.FechaAutorizacion(); !ok {
				t.Error("sin fecha de autorización")
			}
			if len(res.Mensajes) != 1 || res.Mensajes[0].Identificador != "60" || res.Mensajes[0].EsError() {
				t.Errorf("se esperaba la advertencia 60 de ambiente de pruebas: %v", res.Mensajes)
			}
			if _, aut := srv.Peticiones(); aut != 3 {
				t.Errorf("consultas de autorización = %d, quiero 3", aut)
			}
			// El documento firmado cumple el XSD y su firma verifica.
			if err := esquema.Validar(res.XMLFirmado); err != nil {
				t.Errorf("XSD del firmado: %v", err)
			}
			if _, err := xades.Verificar(res.XMLFirmado, nil); err != nil {
				t.Errorf("firma: %v", err)
			}
			aut := string(res.XMLAutorizado())
			if !strings.Contains(aut, "<estado>AUTORIZADO</estado>") || !strings.Contains(aut, "<![CDATA[<?xml") {
				t.Errorf("XMLAutorizado = %.200s", aut)
			}
		})
	}
}

func TestValidacionLocalImpideEnviar(t *testing.T) {
	c, srv := entorno(t, xades.SHA1)
	f := ejemplo.Factura(comprobante.Hoy(), "000000002")
	f.InfoFactura.ImporteTotal = f.InfoFactura.ImporteTotal.Sumar(f.InfoFactura.ImporteTotal)
	_, err := c.Emitir(context.Background(), f)
	var ev *sri.ErrorValidacion
	if !errors.As(err, &ev) || !ev.Hallazgos.Tiene("totales.importe") {
		t.Fatalf("esperaba ErrorValidacion por totales, obtuve %v", err)
	}
	if rec, _ := srv.Peticiones(); rec != 0 {
		t.Error("un comprobante inválido no debe enviarse")
	}
}

func TestProteccionDeAmbienteYEmisor(t *testing.T) {
	c, _ := entorno(t, xades.SHA1)
	f := ejemplo.Factura(comprobante.Hoy(), "000000003")
	f.InfoTributaria.Ambiente = comprobante.Produccion
	f.GenerarClaveAcceso("12345678")
	if _, err := c.Emitir(context.Background(), f); !errors.Is(err, sri.ErrAmbiente) {
		t.Errorf("ambiente: %v", err)
	}
	g := ejemplo.Factura(comprobante.Hoy(), "000000003")
	g.InfoTributaria.RUC = "1791111111001"
	g.GenerarClaveAcceso("12345678")
	if _, err := c.Emitir(context.Background(), g); !errors.Is(err, sri.ErrEmisor) {
		t.Errorf("emisor: %v", err)
	}
	if _, err := sri.Nuevo(sri.Config{}); err == nil {
		t.Error("Config sin ambiente debe rechazarse")
	}
}

// La respuesta de recepción se pierde (corte de conexión) aunque el SRI sí
// registró el comprobante: el reintento recibe "clave en procesamiento" y el
// flujo continúa hasta la autorización sin duplicar nada.
func TestReintentoTrasRespuestaPerdida(t *testing.T) {
	c, srv := entorno(t, xades.SHA1)
	srv.FallarProximas(0)
	res, err := c.Emitir(context.Background(), ejemplo.Factura(comprobante.Hoy(), "000000004"))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Autorizado() {
		t.Fatalf("estado = %s %v", res.Estado, res.Mensajes)
	}
	if rec, _ := srv.Peticiones(); rec != 2 {
		t.Errorf("recepciones = %d, quiero 2", rec)
	}
}

func TestReenvioDeAutorizadoEsIdempotente(t *testing.T) {
	c, _ := entorno(t, xades.SHA1)
	f := ejemplo.Factura(comprobante.Hoy(), "000000005")
	primero, err := c.Emitir(context.Background(), f)
	if err != nil || !primero.Autorizado() {
		t.Fatal(err, primero)
	}
	segundo, err := c.Reanudar(context.Background(), primero.XMLFirmado, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !segundo.Autorizado() || segundo.Recepcion.Recibida() || !segundo.Recepcion.YaRecibido() {
		t.Errorf("reenvío: estado=%s recepción=%+v", segundo.Estado, segundo.Recepcion)
	}
}

func TestPendienteYConsultaPosterior(t *testing.T) {
	c, srv := entorno(t, xades.SHA1)
	srv.ConsultasEnProceso = 7 // más que las 5 consultas configuradas
	res, err := c.Emitir(context.Background(), ejemplo.Factura(comprobante.Hoy(), "000000006"))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Pendiente() || res.XMLAutorizado() != nil {
		t.Fatalf("estado = %s", res.Estado)
	}
	for i := 0; i < 3 && !res.Autorizado(); i++ {
		if res, err = c.Autorizar(context.Background(), res.ClaveAcceso); err != nil {
			t.Fatal(err)
		}
	}
	if !res.Autorizado() {
		t.Errorf("tras consultar de nuevo: %s", res.Estado)
	}
}

func TestDevueltaPorEsquemaYNoAutorizadaPorFirma(t *testing.T) {
	c, _ := entorno(t, xades.SHA1)
	ctx := context.Background()
	prep, err := c.Preparar(ejemplo.Factura(comprobante.Hoy(), "000000007"))
	if err != nil {
		t.Fatal(err)
	}
	// Se rompe el esquema: el servicio de recepción lo devuelve (35).
	roto := strings.Replace(string(prep.XMLFirmado), "<razonSocial>", "<razonSocial></razonSocial><razonSocial>", 1)
	res, err := c.Reanudar(ctx, []byte(roto), nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Estado != sri.EstadoDevuelto || len(res.Mensajes) == 0 || res.Mensajes[0].Identificador != "35" {
		t.Errorf("esperaba DEVUELTO con código 35: %s %v", res.Estado, res.Mensajes)
	}
	// Se altera el contenido tras firmar: se recibe, pero no se autoriza (39).
	alterado := strings.Replace(string(prep.XMLFirmado), "EMISOR DE PRUEBA S.A.", "EMISOR ALTERADO S.A.", 1)
	res, err = c.Reanudar(ctx, []byte(alterado), nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Estado != sri.EstadoNoAutorizado || res.Mensajes[0].Identificador != "39" {
		t.Errorf("esperaba NO AUTORIZADO con código 39: %s %v", res.Estado, res.Mensajes)
	}
	// Corregido, se reenvía con la misma clave (ficha 5.10) y se autoriza.
	res, err = c.Reanudar(ctx, prep.XMLFirmado, nil)
	if err != nil || !res.Autorizado() {
		t.Errorf("reenvío corregido: %v %v", res, err)
	}
}

func TestServicioCaidoYCancelacion(t *testing.T) {
	c, srv := entorno(t, xades.SHA1)
	srv.FallarProximas(503, 503, 503)
	res, err := c.Emitir(context.Background(), ejemplo.Factura(comprobante.Hoy(), "000000008"))
	var eh *ws.ErrorHTTP
	if !errors.As(err, &eh) || eh.Codigo != 503 {
		t.Fatalf("esperaba ErrorHTTP 503, obtuve %v", err)
	}
	// El resultado parcial conserva lo necesario para reanudar.
	if res == nil || res.Estado != sri.EstadoFirmado || len(res.XMLFirmado) == 0 || res.ClaveAcceso == "" {
		t.Fatalf("resultado parcial = %+v", res)
	}
	res, err = c.Reanudar(context.Background(), res.XMLFirmado, nil)
	if err != nil || !res.Autorizado() {
		t.Errorf("reanudar: %v %v", res, err)
	}
	ctx, cancelar := context.WithCancel(context.Background())
	cancelar()
	if _, err := c.Emitir(ctx, ejemplo.Factura(comprobante.Hoy(), "000000009")); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelación: %v", err)
	}
}

func TestSinCertificado(t *testing.T) {
	c, err := sri.Nuevo(sri.Config{Ambiente: comprobante.Pruebas})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Preparar(ejemplo.Factura(comprobante.Hoy(), "000000010")); !errors.Is(err, sri.ErrSinCertificado) {
		t.Errorf("%v", err)
	}
}
