package ws

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
)

const claveFicha = "0503201201176001321000110010030009900641234567814"

func servidor(t *testing.T, h http.HandlerFunc) *Cliente {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return Nuevo(comprobante.Pruebas, ConEndpoints(EndpointsEn(srv.URL)), ConReintentos(Reintentos{Intentos: 3, Espera: time.Millisecond}))
}

func TestEndpointsOficiales(t *testing.T) {
	p, prod := EndpointsDe(comprobante.Pruebas), EndpointsDe(comprobante.Produccion)
	if p.Recepcion != "https://celcer.sri.gob.ec/comprobantes-electronicos-ws/RecepcionComprobantesOffline" ||
		prod.Autorizacion != "https://cel.sri.gob.ec/comprobantes-electronicos-ws/AutorizacionComprobantesOffline" ||
		p.ConsultaComprobante != "https://celcer.sri.gob.ec/comprobantes-electronicos-ws/ConsultaComprobante" {
		t.Errorf("endpoints: %+v %+v", p, prod)
	}
	// Un ambiente no reconocido nunca debe resolver a producción.
	if e := EndpointsDe("9"); !strings.Contains(e.Recepcion, "celcer") {
		t.Error("ambiente desconocido apunta a producción")
	}
}

// Respuestas tomadas literalmente de la ficha técnica v2.34, sección 7.2.3.
const (
	recibidaFicha   = `<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><ns2:validarComprobanteResponse xmlns:ns2="http://ec.gob.sri.ws.recepcion"><RespuestaRecepcionComprobante><estado>RECIBIDA</estado><comprobantes/></RespuestaRecepcionComprobante></ns2:validarComprobanteResponse></soap:Body></soap:Envelope>`
	devueltaFicha   = `<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><ns2:validarComprobanteResponse xmlns:ns2="http://ec.gob.sri.ws.recepcion"><RespuestaRecepcionComprobante><estado>DEVUELTA</estado><comprobantes><comprobante><claveAcceso>1702201205176001321000110010030001000011234567816</claveAcceso><mensajes><mensaje><identificador>35</identificador><mensaje>DOCUMENTO INVÁLIDO</mensaje><informacionAdicional>Se encontró el siguiente error en la estructura del comprobante: cvc-complex-type.2.4.a</informacionAdicional><tipo>ERROR</tipo></mensaje></mensajes></comprobante></comprobantes></RespuestaRecepcionComprobante></ns2:validarComprobanteResponse></soap:Body></soap:Envelope>`
	autorizadoFicha = `<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><ns2:autorizacionComprobanteResponse xmlns:ns2="http://ec.gob.sri.ws.autorizacion"><RespuestaAutorizacionComprobante><claveAccesoConsultada>
 0503201201176001321000110010030009900641234567814
 </claveAccesoConsultada><numeroComprobantes>1</numeroComprobantes><autorizaciones><autorizacion><estado>AUTORIZADO</estado><numeroAutorizacion>0503201201176001321000110010030009900641234567814</numeroAutorizacion><fechaAutorizacion>2012-03-05T16:57:34.997-05:00</fechaAutorizacion><ambiente>PRUEBAS</ambiente><comprobante><![CDATA[<?xml version="1.0" encoding="UTF-8"?><factura id="comprobante" version="1.0.0"></factura>]]></comprobante><mensajes><mensaje><identificador>60</identificador><mensaje>ESTE PROCESO FUE REALIZADO EN EL AMBIENTE DE PRUEBAS</mensaje><tipo>ADVERTENCIA</tipo></mensaje></mensajes></autorizacion></autorizaciones></RespuestaAutorizacionComprobante></ns2:autorizacionComprobanteResponse></soap:Body></soap:Envelope>`
	rechazadoFicha = `<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><ns2:autorizacionComprobanteResponse xmlns:ns2="http://ec.gob.sri.ws.autorizacion"><RespuestaAutorizacionComprobante><claveAccesoConsultada>x</claveAccesoConsultada><numeroComprobantes>1</numeroComprobantes><autorizaciones><autorizacion><estado>RECHAZADO</estado><fechaAutorizacion>2012-02-13T16:34:48.997-05:00</fechaAutorizacion><ambiente>PRUEBAS</ambiente><mensajes><mensaje><identificador>46</identificador><mensaje> RUC no existe </mensaje><tipo>ERROR</tipo></mensaje></mensajes></autorizacion></autorizaciones></RespuestaAutorizacionComprobante></ns2:autorizacionComprobanteResponse></soap:Body></soap:Envelope>`
	sinResultado   = `<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><ns2:autorizacionComprobanteResponse xmlns:ns2="http://ec.gob.sri.ws.autorizacion"><RespuestaAutorizacionComprobante><claveAccesoConsultada>x</claveAccesoConsultada><numeroComprobantes>0</numeroComprobantes><autorizaciones/></RespuestaAutorizacionComprobante></ns2:autorizacionComprobanteResponse></soap:Body></soap:Envelope>`
)

func TestRecepcionSegunLaFicha(t *testing.T) {
	var peticion string
	respuesta := recibidaFicha
	c := servidor(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		peticion = string(b)
		if r.Header.Get("Content-Type") != "text/xml; charset=utf-8" {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		io.WriteString(w, respuesta)
	})
	r, err := c.ValidarComprobante(context.Background(), []byte("<factura>ñ</factura>"))
	if err != nil || !r.Recibida() || r.YaRecibido() {
		t.Fatalf("%+v %v", r, err)
	}
	// El XML viaja en base64 dentro de <xml>, en el espacio de nombres de recepción.
	if !strings.Contains(peticion, `xmlns:ec="http://ec.gob.sri.ws.recepcion"`) || !strings.Contains(peticion, "<ec:validarComprobante><xml>PGZhY3R1cmE+w7E8L2ZhY3R1cmE+</xml>") {
		t.Errorf("petición = %s", peticion)
	}
	respuesta = devueltaFicha
	r, err = c.ValidarComprobante(context.Background(), []byte("<factura/>"))
	if err != nil || r.Recibida() || r.YaRecibido() {
		t.Fatalf("%+v %v", r, err)
	}
	ms := r.Mensajes()
	if len(ms) != 1 || ms[0].Identificador != "35" || !ms[0].EsError() || !strings.Contains(ms[0].String(), "[35] DOCUMENTO INVÁLIDO: Se encontró") {
		t.Errorf("mensajes = %v", ms)
	}
	if _, err := c.ValidarComprobante(context.Background(), make([]byte, TamanoMaximo+1)); !errors.Is(err, ErrTamano) {
		t.Errorf("tamaño: %v", err)
	}
}

func TestYaRecibido(t *testing.T) {
	r := &RespuestaRecepcion{Estado: "DEVUELTA", Comprobantes: []ComprobanteRecepcion{{Mensajes: []Mensaje{{Identificador: "70", Tipo: "ERROR"}}}}}
	if !r.YaRecibido() {
		t.Error("70 debe contar como ya recibido")
	}
	r.Comprobantes[0].Mensajes = append(r.Comprobantes[0].Mensajes, Mensaje{Identificador: "35", Tipo: "ERROR"})
	if r.YaRecibido() {
		t.Error("con otro error no es un simple reenvío")
	}
}

func TestAutorizacionSegunLaFicha(t *testing.T) {
	respuesta := autorizadoFicha
	c := servidor(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, respuesta) })
	ctx := context.Background()
	r, err := c.AutorizacionComprobante(ctx, claveFicha)
	if err != nil {
		t.Fatal(err)
	}
	a := r.Autorizada()
	if a == nil || r.EnProceso() || a.NumeroAutorizacion != claveFicha || a.Ambiente != "PRUEBAS" || !strings.HasPrefix(a.Comprobante, "<?xml") {
		t.Fatalf("autorización = %+v", a)
	}
	if f, err := a.Fecha(); err != nil || f.UTC().Format(time.RFC3339) != "2012-03-05T21:57:34Z" {
		t.Errorf("fecha = %v %v", f, err)
	}
	if a.Mensajes[0].EsError() {
		t.Error("la advertencia 60 no es un error")
	}
	respuesta = rechazadoFicha
	r, _ = c.AutorizacionComprobante(ctx, claveFicha)
	if r.Autorizada() != nil || r.EnProceso() || r.Ultima().Mensajes[0].Identificador != "46" {
		t.Errorf("rechazado = %+v", r)
	}
	respuesta = sinResultado
	r, _ = c.AutorizacionComprobante(ctx, claveFicha)
	if !r.EnProceso() || r.Ultima() != nil {
		t.Errorf("sin resultado = %+v", r)
	}
	if _, err := c.AutorizacionComprobante(ctx, "123"); err == nil {
		t.Error("una clave inválida no debe consultarse")
	}
}

func TestReintentosYErrores(t *testing.T) {
	n := 0
	c := servidor(t, func(w http.ResponseWriter, r *http.Request) {
		if n++; n < 3 {
			http.Error(w, "mantenimiento", http.StatusServiceUnavailable)
			return
		}
		io.WriteString(w, sinResultado)
	})
	if _, err := c.AutorizacionComprobante(context.Background(), claveFicha); err != nil || n != 3 {
		t.Errorf("tras 2 fallos temporales debía recuperarse: %v (n=%d)", err, n)
	}

	n = 0
	c = servidor(t, func(w http.ResponseWriter, r *http.Request) {
		n++
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><soap:Fault><faultcode>soap:Client</faultcode><faultstring>Unmarshalling Error</faultstring></soap:Fault></soap:Body></soap:Envelope>`)
	})
	_, err := c.AutorizacionComprobante(context.Background(), claveFicha)
	var fs *FallaSOAP
	if !errors.As(err, &fs) || fs.Texto != "Unmarshalling Error" || n != 1 {
		t.Errorf("una falla SOAP no se reintenta: %v (n=%d)", err, n)
	}

	c = servidor(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "<html>portal cautivo</html>") })
	if _, err := c.AutorizacionComprobante(context.Background(), claveFicha); !errors.Is(err, ErrRespuesta) {
		t.Errorf("respuesta ajena: %v", err)
	}

	c = servidor(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", http.StatusForbidden) })
	var eh *ErrorHTTP
	if _, err := c.AutorizacionComprobante(context.Background(), claveFicha); !errors.As(err, &eh) || eh.Temporal() {
		t.Errorf("403: %v", err)
	}

	c = servidor(t, func(w http.ResponseWriter, r *http.Request) { time.Sleep(200 * time.Millisecond) })
	ctx, cancelar := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelar()
	if _, err := c.AutorizacionComprobante(ctx, claveFicha); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("plazo: %v", err)
	}
}
