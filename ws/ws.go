// Package ws es el cliente de los servicios web SOAP del SRI para
// comprobantes electrónicos en el esquema offline: recepción, autorización y
// consulta de validez (ficha técnica v2.34, secciones 7 y 8).
//
// No depende de ninguna pila SOAP: los sobres son XML fijos y las respuestas
// se leen con encoding/xml. Todas las llamadas aceptan un context.Context y
// respetan su cancelación y plazo.
//
// # Reintentos
//
// Las consultas de autorización y de validez son lecturas y se reintentan
// ante fallos de transporte o respuestas 5xx. El envío a recepción también:
// el propio protocolo lo hace seguro, porque reenviar un comprobante ya
// recibido no lo duplica, sino que produce una devolución con el código 43
// (clave de acceso registrada) o 70 (clave en procesamiento), que
// RespuestaRecepcion.YaRecibido reconoce.
package ws

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"github.com/OsirisRoman/sri-ecuador-go/clave"
	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
)

// Endpoints son las direcciones de los servicios de un ambiente.
type Endpoints struct {
	Recepcion           string
	Autorizacion        string
	ConsultaComprobante string
	ConsultaFactura     string
}

const (
	hostPruebas    = "https://celcer.sri.gob.ec"
	hostProduccion = "https://cel.sri.gob.ec"
	rutaBase       = "/comprobantes-electronicos-ws/"
)

// EndpointsDe devuelve las direcciones oficiales del ambiente (secciones
// 7.2 y 8.2). Para cualquier valor distinto de Produccion devuelve las de
// pruebas: apuntar a producción exige pedirlo de forma inequívoca.
func EndpointsDe(a comprobante.Ambiente) Endpoints {
	host := hostPruebas
	if a == comprobante.Produccion {
		host = hostProduccion
	}
	return EndpointsEn(host)
}

// EndpointsEn construye las direcciones sobre otra base, por ejemplo un
// servidor de pruebas local (ver el paquete sritest).
func EndpointsEn(base string) Endpoints {
	b := strings.TrimRight(base, "/") + rutaBase
	return Endpoints{
		Recepcion:           b + "RecepcionComprobantesOffline",
		Autorizacion:        b + "AutorizacionComprobantesOffline",
		ConsultaComprobante: b + "ConsultaComprobante",
		ConsultaFactura:     b + "ConsultaFactura",
	}
}

// TamanoMaximo es el tamaño máximo de un comprobante individual: 320 kB
// (sección 7.5). El SRI devuelve el error 26 si se supera.
const TamanoMaximo = 320 * 1024

// maxRespuesta acota la lectura de una respuesta.
const maxRespuesta = 16 << 20

// Errores del cliente.
var (
	// ErrTamano indica que el comprobante supera TamanoMaximo.
	ErrTamano = errors.New("ws: el comprobante supera el tamaño máximo de 320 kB")
	// ErrRespuesta indica que el servicio respondió algo que no es la
	// respuesta SOAP esperada.
	ErrRespuesta = errors.New("ws: respuesta inesperada del SRI")
)

// ErrorHTTP es una respuesta HTTP no satisfactoria sin falla SOAP legible.
type ErrorHTTP struct {
	Codigo int
	Cuerpo string // primeros bytes, para diagnóstico
}

func (e *ErrorHTTP) Error() string {
	return fmt.Sprintf("ws: el SRI respondió HTTP %d: %s", e.Codigo, e.Cuerpo)
}

// Temporal informa si tiene sentido reintentar.
func (e *ErrorHTTP) Temporal() bool { return e.Codigo >= 500 || e.Codigo == http.StatusTooManyRequests }

// FallaSOAP es un soap:Fault devuelto por el servicio.
type FallaSOAP struct {
	Codigo string
	Texto  string
}

func (e *FallaSOAP) Error() string { return fmt.Sprintf("ws: falla SOAP %s: %s", e.Codigo, e.Texto) }

// Reintentos configura la política de reintentos.
type Reintentos struct {
	// Intentos totales, incluido el primero. 1 desactiva los reintentos.
	Intentos int
	// Espera inicial; se duplica en cada intento, con variación aleatoria,
	// hasta EsperaMaxima.
	Espera       time.Duration
	EsperaMaxima time.Duration
}

// ReintentosPorDefecto: 3 intentos con esperas de ~1 s y ~2 s.
var ReintentosPorDefecto = Reintentos{Intentos: 3, Espera: time.Second, EsperaMaxima: 10 * time.Second}

// Cliente invoca los servicios web del SRI. Es seguro para uso concurrente.
type Cliente struct {
	ep         Endpoints
	http       *http.Client
	reintentos Reintentos
	dormir     func(context.Context, time.Duration) error
}

// Opcion configura un Cliente.
type Opcion func(*Cliente)

// ConHTTP usa un *http.Client propio (proxy, TLS, transporte instrumentado).
// Si no define Timeout, cada llamada queda limitada solo por su contexto.
func ConHTTP(h *http.Client) Opcion { return func(c *Cliente) { c.http = h } }

// ConEndpoints sustituye las direcciones de los servicios.
func ConEndpoints(e Endpoints) Opcion { return func(c *Cliente) { c.ep = e } }

// ConReintentos sustituye la política de reintentos.
func ConReintentos(r Reintentos) Opcion { return func(c *Cliente) { c.reintentos = r } }

// Nuevo crea un cliente para el ambiente indicado.
func Nuevo(a comprobante.Ambiente, ops ...Opcion) *Cliente {
	c := &Cliente{
		ep:         EndpointsDe(a),
		http:       &http.Client{Timeout: 60 * time.Second},
		reintentos: ReintentosPorDefecto,
		dormir:     dormirCtx,
	}
	for _, o := range ops {
		o(c)
	}
	if c.reintentos.Intentos < 1 {
		c.reintentos.Intentos = 1
	}
	return c
}

// Endpoints devuelve las direcciones en uso.
func (c *Cliente) Endpoints() Endpoints { return c.ep }

func dormirCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// ---------------------------------------------------------------------------
// Tipos de respuesta

// Mensaje es un mensaje de error o advertencia del SRI (sección 11).
type Mensaje struct {
	// Identificador es el código de la tabla de errores ("35", "43", …).
	Identificador        string `xml:"identificador"`
	Mensaje              string `xml:"mensaje"`
	InformacionAdicional string `xml:"informacionAdicional"`
	// Tipo es "ERROR" o "ADVERTENCIA".
	Tipo string `xml:"tipo"`
}

// EsError informa si el mensaje es de tipo ERROR.
func (m Mensaje) EsError() bool { return strings.EqualFold(strings.TrimSpace(m.Tipo), "ERROR") }

func (m Mensaje) String() string {
	s := fmt.Sprintf("[%s] %s", m.Identificador, strings.TrimSpace(m.Mensaje))
	if ia := strings.TrimSpace(m.InformacionAdicional); ia != "" {
		s += ": " + ia
	}
	return s
}

// Estados de recepción.
const (
	EstadoRecibida = "RECIBIDA"
	EstadoDevuelta = "DEVUELTA"
)

// Estados de autorización. EnProceso no lo envía el SRI como texto fijo en
// todos los casos: también se deduce de una respuesta sin autorizaciones.
const (
	EstadoAutorizado   = "AUTORIZADO"
	EstadoNoAutorizado = "NO AUTORIZADO"
	EstadoEnProceso    = "EN PROCESO"
	// EstadoRechazado aparece en los ejemplos de la ficha técnica (7.2.3)
	// como estado de un comprobante no autorizado.
	EstadoRechazado = "RECHAZADO"
)

// Códigos de mensaje relevantes para el flujo (sección 11).
const (
	CodClaveRegistrada      = "43"
	CodSecuencialRegistrado = "45"
	CodClaveEnProcesamiento = "70"
)

// ComprobanteRecepcion agrupa los mensajes de un comprobante devuelto.
type ComprobanteRecepcion struct {
	ClaveAcceso string    `xml:"claveAcceso"`
	Mensajes    []Mensaje `xml:"mensajes>mensaje"`
}

// RespuestaRecepcion es la respuesta de validarComprobante.
type RespuestaRecepcion struct {
	// Estado es EstadoRecibida o EstadoDevuelta.
	Estado       string                 `xml:"estado"`
	Comprobantes []ComprobanteRecepcion `xml:"comprobantes>comprobante"`
}

// Recibida informa si el SRI aceptó el comprobante para su procesamiento.
// RECIBIDA no significa AUTORIZADO: la autorización se consulta después.
func (r *RespuestaRecepcion) Recibida() bool {
	return strings.EqualFold(strings.TrimSpace(r.Estado), EstadoRecibida)
}

// Mensajes devuelve todos los mensajes de la respuesta.
func (r *RespuestaRecepcion) Mensajes() []Mensaje {
	var ms []Mensaje
	for _, c := range r.Comprobantes {
		ms = append(ms, c.Mensajes...)
	}
	return ms
}

// YaRecibido informa si la devolución se debe únicamente a que el SRI ya
// tenía el comprobante (códigos 43 o 70): ocurre al reenviar tras un fallo
// de red en el que el primer envío sí llegó. En ese caso corresponde
// consultar la autorización, no corregir ni volver a enviar.
func (r *RespuestaRecepcion) YaRecibido() bool {
	errores := 0
	for _, m := range r.Mensajes() {
		if !m.EsError() {
			continue
		}
		errores++
		if id := strings.TrimSpace(m.Identificador); id != CodClaveRegistrada && id != CodClaveEnProcesamiento {
			return false
		}
	}
	return !r.Recibida() && errores > 0
}

// Autorizacion es el resultado del procesamiento de un comprobante.
type Autorizacion struct {
	Estado             string `xml:"estado"`
	NumeroAutorizacion string `xml:"numeroAutorizacion"`
	// FechaAutorizacion tal como la envía el SRI; ver Fecha.
	FechaAutorizacion string `xml:"fechaAutorizacion"`
	// Ambiente es "PRUEBAS" o "PRODUCCIÓN".
	Ambiente string `xml:"ambiente"`
	// Comprobante es el XML firmado tal como lo almacenó el SRI.
	Comprobante string    `xml:"comprobante"`
	Mensajes    []Mensaje `xml:"mensajes>mensaje"`
}

// Autorizado informa si el estado es AUTORIZADO (o su sigla AUT, Tabla 6).
func (a *Autorizacion) Autorizado() bool {
	e := strings.TrimSpace(a.Estado)
	return strings.EqualFold(e, EstadoAutorizado) || strings.EqualFold(e, "AUT")
}

// NoAutorizado informa si el estado es un rechazo definitivo: NO AUTORIZADO,
// su sigla NAT (Tabla 6) o RECHAZADO (ejemplo de la sección 7.2.3). Un estado
// vacío o desconocido no es un rechazo.
func (a *Autorizacion) NoAutorizado() bool {
	switch strings.ToUpper(strings.TrimSpace(a.Estado)) {
	case EstadoNoAutorizado, "NAT", EstadoRechazado:
		return true
	}
	return false
}

// Fecha analiza FechaAutorizacion. El SRI ha usado más de un formato a lo
// largo del tiempo; se aceptan RFC 3339 y dd/mm/aaaa hh:mm:ss.
func (a *Autorizacion) Fecha() (time.Time, error) {
	s := strings.TrimSpace(a.FechaAutorizacion)
	for _, f := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999", "02/01/2006 15:04:05"} {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("ws: fechaAutorizacion %q ilegible", s)
}

// RespuestaAutorizacion es la respuesta de autorizacionComprobante.
type RespuestaAutorizacion struct {
	ClaveAccesoConsultada string         `xml:"claveAccesoConsultada"`
	NumeroComprobantes    string         `xml:"numeroComprobantes"`
	Autorizaciones        []Autorizacion `xml:"autorizaciones>autorizacion"`
}

// Autorizada devuelve la autorización en estado AUTORIZADO, si existe. Un
// comprobante reenviado varias veces puede traer varios intentos.
func (r *RespuestaAutorizacion) Autorizada() *Autorizacion {
	for i := range r.Autorizaciones {
		if r.Autorizaciones[i].Autorizado() {
			return &r.Autorizaciones[i]
		}
	}
	return nil
}

// Ultima devuelve la autorización más reciente de la respuesta (la última
// de la lista), o nil si el SRI aún no tiene resultado.
func (r *RespuestaAutorizacion) Ultima() *Autorizacion {
	if len(r.Autorizaciones) == 0 {
		return nil
	}
	return &r.Autorizaciones[len(r.Autorizaciones)-1]
}

// EnProceso informa si el SRI todavía no ha resuelto el comprobante: no hay
// autorizaciones o la última sigue en procesamiento.
func (r *RespuestaAutorizacion) EnProceso() bool {
	if r.Autorizada() != nil {
		return false
	}
	u := r.Ultima()
	if u == nil {
		return true
	}
	e := strings.ToUpper(strings.TrimSpace(u.Estado))
	return e == EstadoEnProceso || e == "EN PROCESAMIENTO" || e == "PPR"
}

// EstadoComprobante es la respuesta del servicio de consulta de validez
// (sección 8.2.3).
type EstadoComprobante struct {
	// EstadoConsulta vale "RECHAZADA" cuando la consulta no pudo
	// resolverse (clave inexistente o fuera del rango de fechas).
	EstadoConsulta string    `xml:"estadoConsulta"`
	ClaveAcceso    string    `xml:"claveAcceso"`
	Mensajes       []Mensaje `xml:"mensajes>mensaje"`
	// EstadoAutorizacion es AUTORIZADO, NO AUTORIZADO, PENDIENTE DE ANULAR
	// o ANULADO.
	EstadoAutorizacion string `xml:"estadoAutorizacion"`
	TipoComprobante    string `xml:"tipoComprobante"`
	RucEmisor          string `xml:"rucEmisor"`
	FechaAutorizacion  string `xml:"fechaAutorizacion"`
}

// ---------------------------------------------------------------------------
// Operaciones

const (
	nsRecepcion    = "http://ec.gob.sri.ws.recepcion"
	nsAutorizacion = "http://ec.gob.sri.ws.autorizacion"
	nsConsultas    = "http://ec.gob.sri.ws.consultas"
)

func sobre(ns, operacion, parametro, valor string) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:ec="` + ns + `">`)
	b.WriteString(`<soapenv:Header/><soapenv:Body><ec:` + operacion + `><` + parametro + `>`)
	_ = xml.EscapeText(&b, []byte(valor))
	b.WriteString(`</` + parametro + `></ec:` + operacion + `></soapenv:Body></soapenv:Envelope>`)
	return b.Bytes()
}

// ValidarComprobante envía un comprobante firmado al servicio de recepción.
//
// Un resultado sin error significa que el SRI respondió; hay que examinar
// RespuestaRecepcion.Recibida. Una devolución (DEVUELTA) no es un error de
// Go: trae los motivos en Mensajes.
func (c *Cliente) ValidarComprobante(ctx context.Context, xmlFirmado []byte) (*RespuestaRecepcion, error) {
	if len(xmlFirmado) > TamanoMaximo {
		return nil, fmt.Errorf("%w (%d bytes)", ErrTamano, len(xmlFirmado))
	}
	cuerpo := sobre(nsRecepcion, "validarComprobante", "xml", base64.StdEncoding.EncodeToString(xmlFirmado))
	var env struct {
		R *RespuestaRecepcion `xml:"Body>validarComprobanteResponse>RespuestaRecepcionComprobante"`
	}
	if err := c.invocar(ctx, c.ep.Recepcion, cuerpo, &env); err != nil {
		return nil, err
	}
	if env.R == nil {
		return nil, fmt.Errorf("%w: falta RespuestaRecepcionComprobante", ErrRespuesta)
	}
	return env.R, nil
}

// AutorizacionComprobante consulta el resultado del procesamiento de un
// comprobante por su clave de acceso.
func (c *Cliente) AutorizacionComprobante(ctx context.Context, claveAcceso string) (*RespuestaAutorizacion, error) {
	if err := clave.Validar(claveAcceso); err != nil {
		return nil, err
	}
	cuerpo := sobre(nsAutorizacion, "autorizacionComprobante", "claveAccesoComprobante", claveAcceso)
	var env struct {
		R *RespuestaAutorizacion `xml:"Body>autorizacionComprobanteResponse>RespuestaAutorizacionComprobante"`
	}
	if err := c.invocar(ctx, c.ep.Autorizacion, cuerpo, &env); err != nil {
		return nil, err
	}
	if env.R == nil {
		return nil, fmt.Errorf("%w: falta RespuestaAutorizacionComprobante", ErrRespuesta)
	}
	return env.R, nil
}

// ConsultarEstado usa el servicio de consulta de validez para conocer el
// estado actual de un comprobante, incluidas las anulaciones.
func (c *Cliente) ConsultarEstado(ctx context.Context, claveAcceso string) (*EstadoComprobante, error) {
	if err := clave.Validar(claveAcceso); err != nil {
		return nil, err
	}
	cuerpo := sobre(nsConsultas, "consultarEstadoAutorizacionComprobante", "claveAcceso", claveAcceso)
	var env struct {
		R *EstadoComprobante `xml:"Body>consultarEstadoAutorizacionComprobanteResponse>EstadoAutorizacionComprobante"`
	}
	if err := c.invocar(ctx, c.ep.ConsultaComprobante, cuerpo, &env); err != nil {
		return nil, err
	}
	if env.R == nil {
		return nil, fmt.Errorf("%w: falta EstadoAutorizacionComprobante", ErrRespuesta)
	}
	return env.R, nil
}

// invocar hace el POST con reintentos y decodifica el sobre de respuesta.
func (c *Cliente) invocar(ctx context.Context, url string, cuerpo []byte, destino any) error {
	espera := c.reintentos.Espera
	var ultimo error
	for intento := 1; ; intento++ {
		resp, err := c.post(ctx, url, cuerpo)
		if err == nil {
			if err := xml.Unmarshal(resp, destino); err != nil {
				return fmt.Errorf("%w: %v", ErrRespuesta, err)
			}
			return nil
		}
		ultimo = err
		if !reintentable(err) || intento >= c.reintentos.Intentos || ctx.Err() != nil {
			break
		}
		// Variación de ±25 % para no sincronizar reintentos de muchos
		// clientes.
		d := espera + time.Duration((rand.Float64()-0.5)*0.5*float64(espera))
		if err := c.dormir(ctx, d); err != nil {
			break
		}
		if espera *= 2; c.reintentos.EsperaMaxima > 0 && espera > c.reintentos.EsperaMaxima {
			espera = c.reintentos.EsperaMaxima
		}
	}
	return ultimo
}

func reintentable(err error) bool {
	var eh *ErrorHTTP
	if errors.As(err, &eh) {
		return eh.Temporal()
	}
	var fs *FallaSOAP
	if errors.As(err, &fs) || errors.Is(err, ErrRespuesta) {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	// Errores de transporte: conexión rechazada, reinicio, tiempo agotado.
	return true
}

func (c *Cliente) post(ctx context.Context, url string, cuerpo []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(cuerpo))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", `""`)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ws: no se pudo contactar al SRI: %w", err)
	}
	defer resp.Body.Close()
	datos, err := io.ReadAll(io.LimitReader(resp.Body, maxRespuesta))
	if err != nil {
		return nil, fmt.Errorf("ws: respuesta interrumpida: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		var env struct {
			F *struct {
				Codigo string `xml:"faultcode"`
				Texto  string `xml:"faultstring"`
			} `xml:"Body>Fault"`
		}
		if xml.Unmarshal(datos, &env) == nil && env.F != nil {
			return nil, &FallaSOAP{Codigo: env.F.Codigo, Texto: env.F.Texto}
		}
		extracto := datos
		if len(extracto) > 300 {
			extracto = extracto[:300]
		}
		return nil, &ErrorHTTP{Codigo: resp.StatusCode, Cuerpo: string(extracto)}
	}
	return datos, nil
}
