package ws_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/ws"
)

// Respuestas de la ficha técnica v2.34, sección 8.2.
func TestConsultarNegociableSegunLaFicha(t *testing.T) {
	const claveFicha = "1111202401099338176200110020010000003961234567815"
	respuesta := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(cuerpo), "<ec:consultarEstadoConfirmacionFacturaComercialNegociable><claveAcceso>"+claveFicha) ||
			!strings.HasSuffix(r.URL.Path, "/ConsultaFactura") {
			t.Errorf("petición inesperada a %s: %s", r.URL.Path, cuerpo)
		}
		io.WriteString(w, `<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body>`+
			`<ns2:consultarEstadoConfirmacionFacturaComercialNegociableResponse xmlns:ns2="http://ec.gob.sri.ws.consultas">`+
			`<EstadoConfirmacionFacturaComercialNegociable>`+respuesta+`</EstadoConfirmacionFacturaComercialNegociable>`+
			`</ns2:consultarEstadoConfirmacionFacturaComercialNegociableResponse></soap:Body></soap:Envelope>`)
	}))
	defer srv.Close()
	c := ws.Nuevo(comprobante.Pruebas, ws.ConEndpoints(ws.EndpointsEn(srv.URL)))

	respuesta = `<claveAcceso>` + claveFicha + `</claveAcceso><mensajes/><estadoConfirmacion>SI</estadoConfirmacion>`
	e, err := c.ConsultarNegociable(context.Background(), claveFicha)
	if err != nil || !e.Confirmada() || e.ClaveAcceso != claveFicha {
		t.Fatalf("respuesta = %+v, %v", e, err)
	}

	respuesta = `<estadoConsulta>RECHAZADA</estadoConsulta><claveAcceso>` + claveFicha + `</claveAcceso><mensajes><mensaje>` +
		`<identificador>99</identificador><mensaje>ERROR AL CONSULTAR DATOS DEL SERVICIO WEB</mensaje>` +
		`<informacionAdicional>No es posible validar la clave de acceso ya que la fecha de emisión está fuera del rango permitido.</informacionAdicional>` +
		`<tipo>ERROR</tipo></mensaje></mensajes>`
	e, err = c.ConsultarNegociable(context.Background(), claveFicha)
	if err != nil || e.Confirmada() || e.EstadoConsulta != "RECHAZADA" || len(e.Mensajes) != 1 || e.Mensajes[0].Identificador != "99" {
		t.Fatalf("respuesta = %+v, %v", e, err)
	}
	if _, err := c.ConsultarNegociable(context.Background(), "123"); err == nil {
		t.Error("esperaba error con una clave inválida")
	}
}
