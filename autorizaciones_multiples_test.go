package sri_test

// Segunda auditoría (N-03 y N-08): el SRI puede devolver varias
// autorizaciones para una clave (una por cada envío), y nada garantiza su
// orden. El resultado de un documento es el de la autorización que contiene
// ese documento, esté donde esté en la lista.

import (
	"context"
	"errors"
	"strings"
	"testing"

	sri "github.com/OsirisRoman/sri-ecuador-go"
	"github.com/OsirisRoman/sri-ecuador-go/ws"
)

// entrada compone una <autorizacion>. doc nil omite el comprobante.
func entrada(estado, fecha string, doc []byte, mensaje string) string {
	s := "<autorizacion><estado>" + estado + "</estado><fechaAutorizacion>" + fecha + "</fechaAutorizacion><ambiente>PRUEBAS</ambiente>"
	if doc != nil {
		s += "<comprobante><![CDATA[" + string(doc) + "]]></comprobante>"
	}
	if mensaje != "" {
		s += "<mensajes><mensaje><identificador>52</identificador><mensaje>" + mensaje + "</mensaje><tipo>ERROR</tipo></mensaje></mensajes>"
	}
	return s + "</autorizacion>"
}

const (
	antes   = "2026-11-15T10:00:00-05:00"
	despues = "2026-11-15T10:05:00-05:00"
)

// ordenes devuelve la respuesta con las autorizaciones en el orden dado y
// en el inverso.
func ordenes(entradas ...string) map[string]string {
	inverso := make([]string, len(entradas))
	for i, e := range entradas {
		inverso[len(entradas)-1-i] = e
	}
	sobre := func(es []string) string {
		return sobreAutorizacion("<numeroComprobantes>" + string(rune('0'+len(es))) + "</numeroComprobantes><autorizaciones>" + strings.Join(es, "") + "</autorizaciones>")
	}
	return map[string]string{"antigua primero": sobre(entradas), "reciente primero": sobre(inverso)}
}

func mensaje(r *sri.Resultado) string {
	if r == nil || len(r.Mensajes) == 0 {
		return ""
	}
	return r.Mensajes[0].Mensaje
}

// A rechazado y su corrección B, también rechazada: el resultado de B es el
// rechazo de B en cualquier orden. Antes, con la más reciente primero, B
// quedaba pendiente al reanudar y en discrepancia al consultar.
func TestDosRechazosEnCualquierOrden(t *testing.T) {
	a, b := versiones(t)
	ctx := context.Background()
	for orden, resp := range ordenes(entrada("NO AUTORIZADO", antes, a, "error de A"), entrada("NO AUTORIZADO", despues, b, "error de B")) {
		cl := servidorSRI(t, sobreRecepcion("RECIBIDA"), resp)
		res, err := cl.Reanudar(ctx, b, nil)
		if err != nil || res.Estado != sri.EstadoNoAutorizado || !res.Verificado || mensaje(res) != "error de B" {
			t.Errorf("%s: Reanudar(B): estado=%v verificado=%v mensaje=%q err=%v", orden, estado(res), res.Verificado, mensaje(res), err)
		}
		if len(res.Autorizaciones) != 2 {
			t.Errorf("%s: el resultado debe conservar las 2 autorizaciones, tiene %d", orden, len(res.Autorizaciones))
		}
		res, err = cl.ConsultarDocumento(ctx, b)
		if err != nil || res.Estado != sri.EstadoNoAutorizado || !res.Verificado || mensaje(res) != "error de B" {
			t.Errorf("%s: ConsultarDocumento(B): estado=%v mensaje=%q err=%v", orden, estado(res), mensaje(res), err)
		}
		res, err = cl.ConsultarDocumento(ctx, a)
		if err != nil || res.Estado != sri.EstadoNoAutorizado || mensaje(res) != "error de A" {
			t.Errorf("%s: ConsultarDocumento(A): estado=%v mensaje=%q err=%v", orden, estado(res), mensaje(res), err)
		}
		// Por clave no hay documento con el que vincular: se informa el
		// rechazo más reciente por fecha.
		clave := res.ClaveAcceso
		res, err = cl.Autorizar(ctx, clave)
		if err != nil || res.Estado != sri.EstadoNoAutorizado || res.Verificado || mensaje(res) != "error de B" {
			t.Errorf("%s: Autorizar: estado=%v mensaje=%q err=%v", orden, estado(res), mensaje(res), err)
		}
	}
}

// Una versión autorizada y otra rechazada: quien consulta por la autorizada
// la obtiene verificada; quien consulta por la rechazada obtiene una
// discrepancia con una versión AUTORIZADA, en cualquier orden.
func TestAutorizadaYRechazadaEnCualquierOrden(t *testing.T) {
	a, b := versiones(t)
	ctx := context.Background()
	for orden, resp := range ordenes(entrada("AUTORIZADO", antes, a, ""), entrada("NO AUTORIZADO", despues, b, "error de B")) {
		cl := servidorSRI(t, sobreRecepcion("RECIBIDA"), resp)
		res, err := cl.Reanudar(ctx, a, nil)
		if err != nil || !res.Autorizado() || !res.Verificado || res.XMLAutorizado() == nil {
			t.Errorf("%s: Reanudar(A): estado=%v verificado=%v err=%v", orden, estado(res), res.Verificado, err)
		}
		for nombre, consulta := range map[string]func() (*sri.Resultado, error){
			"Reanudar(B)":           func() (*sri.Resultado, error) { return cl.Reanudar(ctx, b, nil) },
			"ConsultarDocumento(B)": func() (*sri.Resultado, error) { return cl.ConsultarDocumento(ctx, b) },
		} {
			res, err := consulta()
			var ed *sri.ErrorDiscrepancia
			if !errors.As(err, &ed) || res.Estado != sri.EstadoDiscrepancia || ed.EstadoSRI != ws.EstadoAutorizado || res.Verificado || res.XMLAutorizado() != nil {
				t.Errorf("%s: %s: estado=%v err=%v", orden, nombre, estado(res), err)
				continue
			}
			if res.Autorizacion == nil || !res.Autorizacion.Autorizado() || len(res.Autorizaciones) != 2 {
				t.Errorf("%s: %s: debe informar la versión autorizada y conservar la lista", orden, nombre)
			}
		}
		if res, err := cl.Autorizar(ctx, res.ClaveAcceso); err != nil || !res.Autorizado() || res.Verificado {
			t.Errorf("%s: Autorizar: estado=%v verificado=%v err=%v", orden, estado(res), res.Verificado, err)
		}
	}
	// La corrección B autorizada después del rechazo de A.
	for orden, resp := range ordenes(entrada("NO AUTORIZADO", antes, a, "error de A"), entrada("AUTORIZADO", despues, b, "")) {
		cl := servidorSRI(t, sobreRecepcion("RECIBIDA"), resp)
		if res, err := cl.Reanudar(ctx, b, nil); err != nil || !res.Autorizado() || !res.Verificado {
			t.Errorf("%s: Reanudar(B): estado=%v err=%v", orden, estado(res), err)
		}
		if res, err := cl.ConsultarDocumento(ctx, a); !errors.Is(err, sri.ErrDiscrepancia) || res.Estado != sri.EstadoDiscrepancia {
			t.Errorf("%s: ConsultarDocumento(A): estado=%v err=%v", orden, estado(res), err)
		}
	}
}

// El rechazo de la versión anterior junto a un resultado todavía en
// proceso: pendiente, en cualquier orden.
func TestRechazoAnteriorYEnProceso(t *testing.T) {
	a, b := versiones(t)
	ctx := context.Background()
	for orden, resp := range ordenes(entrada("NO AUTORIZADO", antes, a, "error de A"), entrada("EN PROCESO", despues, nil, "")) {
		cl := servidorSRI(t, sobreRecepcion("RECIBIDA"), resp)
		for nombre, consulta := range map[string]func() (*sri.Resultado, error){
			"Reanudar(B)":           func() (*sri.Resultado, error) { return cl.Reanudar(ctx, b, nil) },
			"ConsultarDocumento(B)": func() (*sri.Resultado, error) { return cl.ConsultarDocumento(ctx, b) },
			"Autorizar":             func() (*sri.Resultado, error) { return cl.Autorizar(ctx, claveDe(t, b)) },
		} {
			if res, err := consulta(); err != nil || res.Estado != sri.EstadoRecibido {
				t.Errorf("%s: %s: estado=%v err=%v", orden, nombre, estado(res), err)
			}
		}
	}
}

func claveDe(t *testing.T, doc []byte) string {
	t.Helper()
	i := strings.Index(string(doc), "<claveAcceso>")
	if i < 0 {
		t.Fatal("documento sin clave")
	}
	return string(doc[i+13 : i+13+49])
}

// N-08: un AUTORIZADO que no trae el comprobante no se puede vincular con
// el documento enviado: no es un AUTORIZADO de ese documento, sino un
// estado indeterminado, con la autorización a la vista para conciliar.
func TestAutorizadoSinComprobante(t *testing.T) {
	a, b := versiones(t)
	ctx := context.Background()
	indeterminado := func(nombre string, res *sri.Resultado, err error) {
		t.Helper()
		if !errors.Is(err, sri.ErrSinComprobante) || !errors.Is(err, sri.ErrEstadoIndeterminado) {
			t.Errorf("%s: err=%v; se esperaba ErrSinComprobante", nombre, err)
			return
		}
		if res.Estado != sri.EstadoRecibido || res.Autorizado() || res.Verificado || res.XMLAutorizado() != nil {
			t.Errorf("%s: estado=%v verificado=%v", nombre, estado(res), res.Verificado)
		}
		if res.Autorizacion == nil || res.Autorizacion.NumeroAutorizacion != "123" || len(res.XMLFirmado) == 0 {
			t.Errorf("%s: el resultado debe llevar la autorización del SRI y el XML enviado", nombre)
		}
	}
	sin := "<autorizacion><estado>AUTORIZADO</estado><numeroAutorizacion>123</numeroAutorizacion><fechaAutorizacion>" + despues + "</fechaAutorizacion><ambiente>PRUEBAS</ambiente></autorizacion>"
	cl := servidorSRI(t, sobreRecepcion("RECIBIDA"), sobreAutorizacion("<autorizaciones>"+sin+"</autorizaciones>"))
	res, err := cl.Reanudar(ctx, b, nil)
	indeterminado("Reanudar", res, err)
	res, err = cl.ConsultarDocumento(ctx, b)
	indeterminado("ConsultarDocumento", res, err)

	// Autorizar solo conoce la clave: informa el estado de la clave, sin
	// verificar, y no hay documento que entregar.
	res, err = cl.Autorizar(ctx, claveDe(t, b))
	if err != nil || !res.Autorizado() || res.Verificado || res.XMLAutorizado() != nil {
		t.Errorf("Autorizar: estado=%v verificado=%v err=%v", estado(res), res.Verificado, err)
	}

	// Junto al rechazo de otra versión sigue sin poder vincularse.
	for orden, resp := range ordenes(entrada("NO AUTORIZADO", antes, a, "error de A"), sin) {
		cl := servidorSRI(t, sobreRecepcion("RECIBIDA"), resp)
		res, err := cl.Reanudar(ctx, b, nil)
		indeterminado(orden, res, err)
	}
	// Si el comprobante autorizado no se puede leer, es una discrepancia.
	cl = servidorSRI(t, sobreRecepcion("RECIBIDA"), sobreAutorizacion("<autorizaciones>"+entrada("AUTORIZADO", despues, []byte("<factura><infoTributaria>"), "")+"</autorizaciones>"))
	var ed *sri.ErrorDiscrepancia
	if res, err := cl.Reanudar(ctx, b, nil); !errors.As(err, &ed) || ed.Causa == nil || res.Estado != sri.EstadoDiscrepancia {
		t.Errorf("comprobante ilegible: estado=%v err=%v", estado(res), err)
	}
}

// Casos sin ambigüedad que no deben cambiar: el mismo documento rechazado
// dos veces, un rechazo sin comprobante y un estado desconocido.
func TestOtrasCombinaciones(t *testing.T) {
	a, b := versiones(t)
	ctx := context.Background()
	for orden, resp := range ordenes(entrada("NO AUTORIZADO", antes, b, "primer rechazo"), entrada("NO AUTORIZADO", despues, b, "segundo rechazo")) {
		cl := servidorSRI(t, sobreRecepcion("RECIBIDA"), resp)
		if res, err := cl.Reanudar(ctx, b, nil); err != nil || res.Estado != sri.EstadoNoAutorizado || !res.Verificado || mensaje(res) != "segundo rechazo" {
			t.Errorf("%s: dos rechazos de B: estado=%v mensaje=%q err=%v", orden, estado(res), mensaje(res), err)
		}
	}
	// Un rechazo sin comprobante no se puede vincular: se informa como
	// rechazo sin verificar (corregir y reenviar con la misma clave es
	// seguro en cualquier caso).
	cl := servidorSRI(t, sobreRecepcion("RECIBIDA"), sobreAutorizacion("<autorizaciones>"+entrada("NO AUTORIZADO", antes, nil, "rechazo")+"</autorizaciones>"))
	if res, err := cl.Reanudar(ctx, b, nil); err != nil || res.Estado != sri.EstadoNoAutorizado || res.Verificado {
		t.Errorf("rechazo sin comprobante: estado=%v verificado=%v err=%v", estado(res), res.Verificado, err)
	}
	// Un estado desconocido junto al rechazo de otra versión: indeterminado.
	for orden, resp := range ordenes(entrada("NO AUTORIZADO", antes, a, "error de A"), entrada("EN REVISION", despues, nil, "")) {
		cl := servidorSRI(t, sobreRecepcion("RECIBIDA"), resp)
		if res, err := cl.Reanudar(ctx, b, nil); !errors.Is(err, sri.ErrEstadoIndeterminado) || res.Estado != sri.EstadoRecibido {
			t.Errorf("%s: estado desconocido: estado=%v err=%v", orden, estado(res), err)
		}
	}
	// El rechazo propio prevalece sobre un estado desconocido ajeno.
	for orden, resp := range ordenes(entrada("EN REVISION", antes, nil, ""), entrada("NO AUTORIZADO", despues, b, "error de B")) {
		cl := servidorSRI(t, sobreRecepcion("RECIBIDA"), resp)
		if res, err := cl.Reanudar(ctx, b, nil); err != nil || res.Estado != sri.EstadoNoAutorizado || !res.Verificado {
			t.Errorf("%s: rechazo propio: estado=%v err=%v", orden, estado(res), err)
		}
	}
}

// Sin ninguna autorización el comprobante está pendiente, también cuando la
// recepción respondió 43 (clave ya registrada) y al consultar el documento.
func TestSinAutorizaciones(t *testing.T) {
	_, b := versiones(t)
	ctx := context.Background()
	registrada := `<S:Envelope xmlns:S="http://schemas.xmlsoap.org/soap/envelope/"><S:Body><ns2:validarComprobanteResponse xmlns:ns2="http://ec.gob.sri.ws.recepcion"><RespuestaRecepcionComprobante><estado>DEVUELTA</estado><comprobantes><comprobante><claveAcceso>x</claveAcceso><mensajes><mensaje><identificador>43</identificador><mensaje>CLAVE ACCESO REGISTRADA</mensaje><tipo>ERROR</tipo></mensaje></mensajes></comprobante></comprobantes></RespuestaRecepcionComprobante></ns2:validarComprobanteResponse></S:Body></S:Envelope>`
	for nombre, recepcion := range map[string]string{"RECIBIDA": sobreRecepcion("RECIBIDA"), "43": registrada} {
		for _, vacia := range []string{"<numeroComprobantes>0</numeroComprobantes><autorizaciones/>", "<numeroComprobantes>0</numeroComprobantes>", ""} {
			cl := servidorSRI(t, recepcion, sobreAutorizacion(vacia))
			if res, err := cl.Reanudar(ctx, b, nil); err != nil || res.Estado != sri.EstadoRecibido || res.Autorizacion != nil {
				t.Errorf("%s, %q: Reanudar: estado=%v err=%v", nombre, vacia, estado(res), err)
			}
			if res, err := cl.ConsultarDocumento(ctx, b); err != nil || res.Estado != sri.EstadoRecibido {
				t.Errorf("%s, %q: ConsultarDocumento: estado=%v err=%v", nombre, vacia, estado(res), err)
			}
		}
	}
}
