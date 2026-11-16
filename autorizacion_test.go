package sri_test

// Pruebas de regresión de la auditoría de v0.1.0 (AUD-002, AUD-005 y
// AUD-006): documentos distintos con la misma clave y respuestas de
// autorización anómalas.

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sri "github.com/OsirisRoman/sri-ecuador-go"
	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
	"github.com/OsirisRoman/sri-ecuador-go/internal/certprueba"
	"github.com/OsirisRoman/sri-ecuador-go/internal/ejemplo"
	"github.com/OsirisRoman/sri-ecuador-go/ws"
	"github.com/OsirisRoman/sri-ecuador-go/xades"
)

// AUD-002: reenviar con la misma clave un comprobante con otro contenido
// después de que el primero se autorizó.
func TestMismaClaveContenidoDistinto(t *testing.T) {
	c, _ := entorno(t, xades.SHA256)
	ctx := context.Background()
	a := ejemplo.Factura(comprobante.Hoy(), "000000201")
	resA, err := c.Emitir(ctx, a)
	if err != nil || !resA.Autorizado() || !resA.Verificado {
		t.Fatalf("A: estado=%v verificado=%v err=%v", resA.Estado, resA.Verificado, err)
	}

	b := ejemplo.Factura(comprobante.Hoy(), "000000201")
	b.Detalles[0].PrecioUnitario = decimal.MustParse("999.00")
	b.Calcular()
	if _, err := b.GenerarClaveAcceso("12345678"); err != nil || b.InfoTributaria.ClaveAcceso != a.InfoTributaria.ClaveAcceso {
		t.Fatalf("la prueba necesita la misma clave: %v", err)
	}
	resB, err := c.Emitir(ctx, b)
	if !errors.Is(err, sri.ErrDiscrepancia) {
		t.Fatalf("se esperaba ErrDiscrepancia, err=%v", err)
	}
	var ed *sri.ErrorDiscrepancia
	if !errors.As(err, &ed) || ed.EstadoSRI != ws.EstadoAutorizado || ed.Enviado == ed.Registrado {
		t.Errorf("detalle de la discrepancia: %+v", ed)
	}
	if resB.Estado != sri.EstadoDiscrepancia || resB.Autorizado() || resB.XMLAutorizado() != nil || resB.Verificado {
		t.Errorf("B no debe quedar autorizado: estado=%v verificado=%v", resB.Estado, resB.Verificado)
	}
	// El documento que sí está autorizado se puede recuperar para
	// diagnosticar, sin generar otro comprobante.
	if resB.Autorizacion == nil || !strings.Contains(resB.Autorizacion.Comprobante, "<importeTotal>11.50</importeTotal>") {
		t.Errorf("falta el comprobante registrado en el SRI")
	}

	// ConsultarDocumento resuelve el estado real de cada documento.
	if r, err := c.ConsultarDocumento(ctx, resA.XMLFirmado); err != nil || !r.Autorizado() || !r.Verificado {
		t.Errorf("A consultado: estado=%v err=%v", r.Estado, err)
	}
	if r, err := c.ConsultarDocumento(ctx, resB.XMLFirmado); !errors.Is(err, sri.ErrDiscrepancia) || r.Estado != sri.EstadoDiscrepancia {
		t.Errorf("B consultado: estado=%v err=%v", r.Estado, err)
	}
}

// La comparación es por contenido canónico: el mismo comprobante con otra
// serialización no es una discrepancia.
func TestMismoComprobanteOtraSerializacion(t *testing.T) {
	c, _ := entorno(t, xades.SHA256)
	ctx := context.Background()
	res, err := c.Emitir(ctx, ejemplo.Factura(comprobante.Hoy(), "000000202"))
	if err != nil || !res.Autorizado() {
		t.Fatal(err)
	}
	otra := bytes.Replace(res.XMLFirmado, []byte(`<factura id="comprobante" version="1.1.0">`), []byte("<factura version='1.1.0'  id='comprobante'>"), 1)
	if bytes.Equal(otra, res.XMLFirmado) {
		t.Fatal("la prueba no cambió la serialización")
	}
	r, err := c.Reanudar(ctx, otra, nil)
	if err != nil || !r.Autorizado() || !r.Verificado {
		t.Errorf("estado=%v verificado=%v err=%v", r.Estado, r.Verificado, err)
	}
}

// servidorSRI responde a recepción y autorización con los sobres dados.
func servidorSRI(t *testing.T, recepcion, autorizacion string) *sri.Cliente {
	t.Helper()
	mux := http.NewServeMux()
	e := ws.EndpointsEn("")
	mux.HandleFunc(e.Recepcion, func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		io.WriteString(w, recepcion)
	})
	mux.HandleFunc(e.Autorizacion, func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		io.WriteString(w, autorizacion)
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	m, err := certprueba.Obtener()
	if err != nil {
		t.Fatal(err)
	}
	c, err := sri.Nuevo(sri.Config{Ambiente: comprobante.Pruebas, Certificado: m.Certificado,
		Servicios: ws.Nuevo(comprobante.Pruebas, ws.ConEndpoints(ws.EndpointsEn(s.URL)), ws.ConReintentos(ws.Reintentos{Intentos: 1})),
		Espera:    sri.Espera{Inicial: time.Millisecond, Intervalo: time.Millisecond, Consultas: 2}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func sobreRecepcion(estado string) string {
	return `<S:Envelope xmlns:S="http://schemas.xmlsoap.org/soap/envelope/"><S:Body><ns2:validarComprobanteResponse xmlns:ns2="http://ec.gob.sri.ws.recepcion"><RespuestaRecepcionComprobante><estado>` +
		estado + `</estado><comprobantes/></RespuestaRecepcionComprobante></ns2:validarComprobanteResponse></S:Body></S:Envelope>`
}

func sobreAutorizacion(contenido string) string {
	return `<S:Envelope xmlns:S="http://schemas.xmlsoap.org/soap/envelope/"><S:Body><ns2:autorizacionComprobanteResponse xmlns:ns2="http://ec.gob.sri.ws.autorizacion"><RespuestaAutorizacionComprobante>` +
		contenido + `</RespuestaAutorizacionComprobante></ns2:autorizacionComprobanteResponse></S:Body></S:Envelope>`
}

func unaAutorizacion(estado, extra string) string {
	return `<autorizaciones><autorizacion><estado>` + estado + `</estado>` + extra + `</autorizacion></autorizaciones>`
}

// AUD-005: estados vacíos o desconocidos no son un rechazo definitivo.
func TestEstadosDeAutorizacion(t *testing.T) {
	for _, c := range []struct {
		estado string
		quedar sri.Estado
		err    error
	}{
		{"", sri.EstadoRecibido, sri.ErrEstadoIndeterminado},
		{"PENDIENTE", sri.EstadoRecibido, sri.ErrEstadoIndeterminado},
		{"EN PROCESO", sri.EstadoRecibido, nil},
		{"PPR", sri.EstadoRecibido, nil},
		{"NO AUTORIZADO", sri.EstadoNoAutorizado, nil},
		{"NAT", sri.EstadoNoAutorizado, nil},
		{"RECHAZADO", sri.EstadoNoAutorizado, nil},
		{"AUT", sri.EstadoAutorizado, nil},
	} {
		cl := servidorSRI(t, sobreRecepcion("RECIBIDA"), sobreAutorizacion(unaAutorizacion(c.estado, "")))
		res, err := cl.Emitir(context.Background(), ejemplo.Factura(comprobante.Hoy(), "000000203"))
		if res == nil || res.Estado != c.quedar || !errors.Is(err, c.err) {
			t.Errorf("estado %q: estado=%v err=%v; se esperaba %v y %v", c.estado, estado(res), err, c.quedar, c.err)
			continue
		}
		if res.ClaveAcceso == "" || len(res.XMLFirmado) == 0 {
			t.Errorf("estado %q: el resultado debe conservar la clave y el XML firmado para reintentar", c.estado)
		}
	}
}

// AUD-005: respuestas que no corresponden a la clave o al ambiente.
func TestRespuestaIncoherente(t *testing.T) {
	otra := ejemplo.Factura(comprobante.Hoy(), "000000999").InfoTributaria.ClaveAcceso
	for nombre, cont := range map[string]string{
		"otra clave consultada":  `<claveAccesoConsultada>` + otra + `</claveAccesoConsultada>` + unaAutorizacion("AUTORIZADO", ""),
		"ambiente de producción": unaAutorizacion("AUTORIZADO", "<ambiente>PRODUCCIÓN</ambiente>"),
		"comprobante de otra clave": unaAutorizacion("AUTORIZADO", "<comprobante><![CDATA[<factura><infoTributaria><ambiente>1</ambiente><claveAcceso>"+
			otra+"</claveAcceso></infoTributaria></factura>]]></comprobante>"),
	} {
		cl := servidorSRI(t, sobreRecepcion("RECIBIDA"), sobreAutorizacion(cont))
		res, err := cl.Emitir(context.Background(), ejemplo.Factura(comprobante.Hoy(), "000000204"))
		if !errors.Is(err, sri.ErrRespuestaIncoherente) || res == nil || res.Estado != sri.EstadoRecibido {
			t.Errorf("%s: estado=%v err=%v", nombre, estado(res), err)
		}
	}
	// El ambiente de pruebas, con o sin tilde de por medio, es coherente.
	cl := servidorSRI(t, sobreRecepcion("RECIBIDA"), sobreAutorizacion(unaAutorizacion("AUTORIZADO", "<ambiente>PRUEBAS</ambiente>")))
	if res, err := cl.Emitir(context.Background(), ejemplo.Factura(comprobante.Hoy(), "000000205")); err != nil || !res.Autorizado() {
		t.Errorf("ambiente de pruebas: %v %v", estado(res), err)
	}
}

// AUD-005: una recepción sin estado no se toma como devolución.
func TestRecepcionSinEstado(t *testing.T) {
	cl := servidorSRI(t, sobreRecepcion(""), sobreAutorizacion(""))
	res, err := cl.Emitir(context.Background(), ejemplo.Factura(comprobante.Hoy(), "000000206"))
	if !errors.Is(err, sri.ErrEstadoIndeterminado) || res == nil || res.Estado != sri.EstadoFirmado || len(res.XMLFirmado) == 0 {
		t.Errorf("estado=%v err=%v", estado(res), err)
	}
}

// AUD-006: los campos de la autorización se escapan.
func TestXMLAutorizadoEscapaLosCampos(t *testing.T) {
	cl := servidorSRI(t, sobreRecepcion("RECIBIDA"),
		sobreAutorizacion(unaAutorizacion("AUTORIZADO", "<numeroAutorizacion>1&amp;2&lt;/x&gt;</numeroAutorizacion><fechaAutorizacion>a&lt;b</fechaAutorizacion>")))
	res, err := cl.Emitir(context.Background(), ejemplo.Factura(comprobante.Hoy(), "000000207"))
	if err != nil || !res.Autorizado() {
		t.Fatal(estado(res), err)
	}
	var v struct {
		Numero string `xml:"numeroAutorizacion"`
		Fecha  string `xml:"fechaAutorizacion"`
	}
	if err := xml.Unmarshal(res.XMLAutorizado(), &v); err != nil || v.Numero != "1&2</x>" || v.Fecha != "a<b" {
		t.Errorf("XMLAutorizado mal formado o alterado: %v %+v", err, v)
	}
}

func estado(r *sri.Resultado) string {
	if r == nil {
		return "<nil>"
	}
	return r.Estado.String()
}
