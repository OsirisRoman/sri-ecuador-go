// Package sritest es un simulador en memoria de los servicios web del SRI
// para probar integraciones sin conexión, sin certificado real y sin emitir
// comprobantes.
//
// Reproduce el protocolo (sobres SOAP, estados, códigos de mensaje) y las
// validaciones que se pueden hacer localmente: XML bien formado, esquema XSD
// (código 35), tamaño (26), clave ya registrada (43) o en procesamiento (70)
// y firma electrónica (39). No reproduce las validaciones que dependen de
// las bases del SRI (estado del RUC, autorización de emisión, etc.).
//
// Que un comprobante sea AUTORIZADO por el simulador no garantiza que el
// SRI real lo autorice.
//
//	srv := sritest.Nuevo()
//	defer srv.Cerrar()
//	cliente := ws.Nuevo(comprobante.Pruebas, ws.ConEndpoints(srv.Endpoints()))
package sritest

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/OsirisRoman/sri-ecuador-go/clave"
	"github.com/OsirisRoman/sri-ecuador-go/esquema"
	"github.com/OsirisRoman/sri-ecuador-go/ws"
	"github.com/OsirisRoman/sri-ecuador-go/xades"
)

// Servidor es un SRI simulado.
type Servidor struct {
	// ConsultasEnProceso es el número de consultas de autorización que
	// responden "sin resultado todavía" antes de resolver el comprobante.
	// Simula el procesamiento asíncrono (sección 7.4). Por defecto 0.
	ConsultasEnProceso int
	// VerificarFirma hace que la autorización compruebe la firma XAdES y
	// responda NO AUTORIZADO con el código 39 si es inválida. Por defecto
	// true.
	VerificarFirma bool
	// Ahora permite fijar el reloj; por defecto time.Now.
	Ahora func() time.Time

	http *httptest.Server
	mu   sync.Mutex
	docs map[string]*registro
	// lotes asocia la clave de acceso de cada lote con las de sus
	// comprobantes, en orden.
	lotes map[string][]string
	// fallos pendientes de inyectar: códigos HTTP a devolver en las
	// próximas peticiones.
	fallos []int
	// Peticiones recibidas por servicio, para aserciones.
	recepciones, autorizaciones int
}

type registro struct {
	xml       []byte
	consultas int
	resuelto  bool
	aut       ws.Autorizacion
}

// Nuevo arranca un servidor en un puerto local aleatorio.
func Nuevo() *Servidor {
	s := &Servidor{VerificarFirma: true, Ahora: time.Now, docs: map[string]*registro{}, lotes: map[string][]string{}}
	mux := http.NewServeMux()
	e := ws.EndpointsEn("")
	mux.HandleFunc(e.Recepcion, s.conFallos(s.recepcion))
	mux.HandleFunc(e.Autorizacion, s.conFallos(s.autorizacion))
	mux.HandleFunc(e.ConsultaComprobante, s.conFallos(s.consulta))
	s.http = httptest.NewServer(mux)
	return s
}

// Cerrar detiene el servidor.
func (s *Servidor) Cerrar() { s.http.Close() }

// URL devuelve la dirección base del servidor.
func (s *Servidor) URL() string { return s.http.URL }

// Endpoints devuelve las direcciones de los servicios simulados, listas
// para ws.ConEndpoints.
func (s *Servidor) Endpoints() ws.Endpoints { return ws.EndpointsEn(s.http.URL) }

// FallarProximas hace que las siguientes peticiones respondan con los
// códigos HTTP indicados, en orden, antes de volver a la normalidad. El
// código 0 simula un corte de conexión sin respuesta.
//
// Un fallo se produce después de procesar la petición: el comprobante sí
// queda registrado, que es el caso difícil de un cliente real (el envío
// llegó pero la respuesta se perdió).
func (s *Servidor) FallarProximas(codigos ...int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fallos = append(s.fallos, codigos...)
}

// Peticiones devuelve cuántas peticiones recibió cada servicio.
func (s *Servidor) Peticiones() (recepcion, autorizacion int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recepciones, s.autorizaciones
}

// Registrado devuelve el XML recibido para una clave de acceso.
func (s *Servidor) Registrado(claveAcceso string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.docs[claveAcceso]
	if !ok {
		return nil, false
	}
	return r.xml, true
}

type respuestaCapturada struct {
	http.ResponseWriter
	cuerpo strings.Builder
}

func (r *respuestaCapturada) Write(b []byte) (int, error) { return r.cuerpo.Write(b) }

func (s *Servidor) conFallos(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "use POST", http.StatusMethodNotAllowed)
			return
		}
		rc := &respuestaCapturada{ResponseWriter: w}
		h(rc, r)
		s.mu.Lock()
		var fallo int
		hayFallo := len(s.fallos) > 0
		if hayFallo {
			fallo, s.fallos = s.fallos[0], s.fallos[1:]
		}
		s.mu.Unlock()
		switch {
		case hayFallo && fallo == 0:
			if hj, ok := w.(http.Hijacker); ok {
				if conn, _, err := hj.Hijack(); err == nil {
					conn.Close()
					return
				}
			}
			panic(http.ErrAbortHandler)
		case hayFallo:
			http.Error(w, "fallo simulado", fallo)
		default:
			w.Header().Set("Content-Type", "text/xml; charset=utf-8")
			io.WriteString(w, rc.cuerpo.String())
		}
	}
}

func parametro(r *http.Request, nombre string) (string, error) {
	_, v, err := parametros(r, nombre)
	return v, err
}

// parametros busca en el sobre SOAP el primer elemento con alguno de los
// nombres dados y devuelve cuál encontró y su contenido.
func parametros(r *http.Request, nombres ...string) (string, string, error) {
	datos, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		return "", "", err
	}
	dec := xml.NewDecoder(strings.NewReader(string(datos)))
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", "", fmt.Errorf("sobre SOAP sin <%s>", strings.Join(nombres, "> ni <"))
		}
		if se, ok := tok.(xml.StartElement); ok && slices.Contains(nombres, se.Name.Local) {
			var v string
			if err := dec.DecodeElement(&v, &se); err != nil {
				return "", "", err
			}
			return se.Name.Local, strings.TrimSpace(v), nil
		}
	}
}

func esc(s string) string {
	var sb strings.Builder
	_ = xml.EscapeText(&sb, []byte(s))
	return sb.String()
}

func mensajesXML(ms []ws.Mensaje) string {
	if len(ms) == 0 {
		return "<mensajes/>"
	}
	var sb strings.Builder
	sb.WriteString("<mensajes>")
	for _, m := range ms {
		fmt.Fprintf(&sb, "<mensaje><identificador>%s</identificador><mensaje>%s</mensaje>", esc(m.Identificador), esc(m.Mensaje))
		if m.InformacionAdicional != "" {
			fmt.Fprintf(&sb, "<informacionAdicional>%s</informacionAdicional>", esc(m.InformacionAdicional))
		}
		fmt.Fprintf(&sb, "<tipo>%s</tipo></mensaje>", esc(m.Tipo))
	}
	sb.WriteString("</mensajes>")
	return sb.String()
}

func envolver(ns, operacion, contenido string) string {
	return `<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body>` +
		`<ns2:` + operacion + `Response xmlns:ns2="` + ns + `">` + contenido +
		`</ns2:` + operacion + `Response></soap:Body></soap:Envelope>`
}

func falla(w http.ResponseWriter, texto string) {
	io.WriteString(w, `<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><soap:Fault>`+
		`<faultcode>soap:Client</faultcode><faultstring>`+esc(texto)+`</faultstring></soap:Fault></soap:Body></soap:Envelope>`)
}

func (s *Servidor) recepcion(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.recepciones++
	s.mu.Unlock()
	b64, err := parametro(r, "xml")
	if err != nil {
		falla(w, err.Error())
		return
	}
	devolver := func(claveAcceso, id, msg, info string) {
		io.WriteString(w, envolver("http://ec.gob.sri.ws.recepcion", "validarComprobante",
			`<RespuestaRecepcionComprobante><estado>DEVUELTA</estado><comprobantes><comprobante><claveAcceso>`+esc(claveAcceso)+
				`</claveAcceso>`+mensajesXML([]ws.Mensaje{{Identificador: id, Mensaje: msg, InformacionAdicional: info, Tipo: "ERROR"}})+
				`</comprobante></comprobantes></RespuestaRecepcionComprobante>`))
	}
	doc, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(doc) == 0 {
		devolver("N/A", "49", "ARGUMENTOS QUE ENVIAN AL WS NULOS", "")
		return
	}
	if raizDe(doc) == "lote" {
		s.recepcionLote(w, doc, devolver)
		return
	}
	if len(doc) > ws.TamanoMaximo {
		devolver("N/A", "26", "TAMAÑO MAXIMO SUPERADO", "")
		return
	}
	var cab struct {
		Clave string `xml:"infoTributaria>claveAcceso"`
	}
	if err := xml.Unmarshal(doc, &cab); err != nil {
		devolver("N/A", "35", "ARCHIVO NO CUMPLE ESTRUCTURA XML", err.Error())
		return
	}
	if err := esquema.Validar(doc); err != nil {
		devolver(cab.Clave, "35", "ARCHIVO NO CUMPLE ESTRUCTURA XML", err.Error())
		return
	}
	if err := clave.Validar(cab.Clave); err != nil {
		devolver(cab.Clave, "35", "ARCHIVO NO CUMPLE ESTRUCTURA XML", err.Error())
		return
	}
	s.mu.Lock()
	previo, existe := s.docs[cab.Clave]
	switch {
	case existe && !previo.resuelto:
		s.mu.Unlock()
		devolver(cab.Clave, ws.CodClaveEnProcesamiento, "CLAVE DE ACCESO EN PROCESAMIENTO", "")
		return
	case existe && previo.aut.Autorizado():
		s.mu.Unlock()
		devolver(cab.Clave, ws.CodClaveRegistrada, "CLAVE ACCESO REGISTRADA", "")
		return
	}
	// Nuevo, o reenvío de un comprobante no autorizado (sección 5.10).
	s.docs[cab.Clave] = &registro{xml: doc}
	s.mu.Unlock()
	io.WriteString(w, envolver("http://ec.gob.sri.ws.recepcion", "validarComprobante",
		`<RespuestaRecepcionComprobante><estado>RECIBIDA</estado><comprobantes/></RespuestaRecepcionComprobante>`))
}

// resolver decide el resultado de un comprobante recibido.
// raizDe devuelve el nombre del elemento raíz de doc, o "".
func raizDe(doc []byte) string {
	dec := xml.NewDecoder(strings.NewReader(string(doc)))
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		if se, ok := tok.(xml.StartElement); ok {
			return se.Name.Local
		}
	}
}

// recepcionLote recibe un <lote> (Tabla 8). Como el SRI real, acepta el
// lote si su forma es correcta y deja para la autorización el resultado de
// cada comprobante, incluidos los que no cumplen el XSD.
func (s *Servidor) recepcionLote(w http.ResponseWriter, doc []byte, devolver func(claveAcceso, id, msg, info string)) {
	var l struct {
		Clave        string   `xml:"claveAcceso"`
		RUC          string   `xml:"ruc"`
		Comprobantes []string `xml:"comprobantes>comprobante"`
	}
	if len(doc) > ws.TamanoMaximoLote {
		devolver("N/A", "26", "TAMAÑO MAXIMO SUPERADO", "")
		return
	}
	if err := xml.Unmarshal(doc, &l); err != nil || clave.Validar(l.Clave) != nil || len(l.Comprobantes) == 0 || len(l.Comprobantes) > ws.MaxComprobantesLote {
		devolver(l.Clave, "35", "ARCHIVO NO CUMPLE ESTRUCTURA XML", "lote mal formado")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, existe := s.lotes[l.Clave]; existe {
		devolver(l.Clave, ws.CodClaveRegistrada, "CLAVE ACCESO REGISTRADA", "")
		return
	}
	var claves []string
	for i, c := range l.Comprobantes {
		xmlDoc := []byte(strings.TrimSpace(c))
		var cab struct {
			Clave string `xml:"infoTributaria>claveAcceso"`
		}
		xml.Unmarshal(xmlDoc, &cab)
		id := cab.Clave
		if clave.Validar(id) != nil {
			id = fmt.Sprintf("%s#%d", l.Clave, i) // sin clave utilizable: identificador interno
		}
		reg := &registro{xml: xmlDoc}
		if err := esquema.Validar(xmlDoc); err != nil {
			reg.resuelto = true
			reg.aut = ws.Autorizacion{Estado: ws.EstadoNoAutorizado, FechaAutorizacion: s.Ahora().Format("2006-01-02T15:04:05-07:00"),
				Ambiente: "PRUEBAS", Comprobante: string(xmlDoc),
				Mensajes: []ws.Mensaje{{Identificador: "35", Mensaje: "ARCHIVO NO CUMPLE ESTRUCTURA XML", InformacionAdicional: err.Error(), Tipo: "ERROR"}}}
		}
		if previo, ok := s.docs[id]; !ok || !previo.aut.Autorizado() {
			s.docs[id] = reg
		}
		claves = append(claves, id)
	}
	s.lotes[l.Clave] = claves
	io.WriteString(w, envolver("http://ec.gob.sri.ws.recepcion", "validarComprobante",
		`<RespuestaRecepcionComprobante><estado>RECIBIDA</estado><comprobantes/></RespuestaRecepcionComprobante>`))
}

// autorizacionXML escribe el elemento <autorizacion> de un comprobante
// resuelto.
func autorizacionXML(a ws.Autorizacion) string {
	var sb strings.Builder
	sb.WriteString(`<autorizacion><estado>` + a.Estado + `</estado>`)
	if a.NumeroAutorizacion != "" {
		sb.WriteString(`<numeroAutorizacion>` + a.NumeroAutorizacion + `</numeroAutorizacion>`)
	}
	sb.WriteString(`<fechaAutorizacion>` + a.FechaAutorizacion + `</fechaAutorizacion><ambiente>` + a.Ambiente + `</ambiente>`)
	sb.WriteString(`<comprobante><![CDATA[` + strings.ReplaceAll(a.Comprobante, "]]>", "]]]]><![CDATA[>") + `]]></comprobante>`)
	sb.WriteString(mensajesXML(a.Mensajes) + `</autorizacion>`)
	return sb.String()
}

// autorizacionLote responde a autorizacionComprobanteLote.
func (s *Servidor) autorizacionLote(w http.ResponseWriter, claveLote string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.autorizaciones++
	var sb strings.Builder
	sb.WriteString(`<RespuestaAutorizacionLote><claveAccesoLoteConsultada>` + esc(claveLote) + `</claveAccesoLoteConsultada>`)
	claves := s.lotes[claveLote]
	var auts strings.Builder
	n := 0
	for _, c := range claves {
		reg := s.docs[c]
		if !reg.resuelto {
			reg.consultas++
			if reg.consultas > s.ConsultasEnProceso {
				s.resolver(c, reg)
			}
		}
		if reg.resuelto {
			auts.WriteString(autorizacionXML(reg.aut))
			n++
		}
	}
	fmt.Fprintf(&sb, `<numeroComprobantesLote>%d</numeroComprobantesLote><autorizaciones>%s</autorizaciones></RespuestaAutorizacionLote>`, n, auts.String())
	io.WriteString(w, envolver("http://ec.gob.sri.ws.autorizacion", "autorizacionComprobanteLote", sb.String()))
}

func (s *Servidor) resolver(claveAcceso string, r *registro) {
	r.resuelto = true
	ambiente := "PRUEBAS"
	if c, err := clave.Analizar(claveAcceso); err == nil && c.Ambiente == "2" {
		ambiente = "PRODUCCIÓN"
	}
	r.aut = ws.Autorizacion{
		FechaAutorizacion: s.Ahora().Format("2006-01-02T15:04:05-07:00"),
		Ambiente:          ambiente,
		Comprobante:       string(r.xml),
	}
	if s.VerificarFirma {
		if _, err := xades.Verificar(r.xml, nil); err != nil {
			r.aut.Estado = ws.EstadoNoAutorizado
			r.aut.Mensajes = []ws.Mensaje{{Identificador: "39", Mensaje: "FIRMA INVALIDA", InformacionAdicional: err.Error(), Tipo: "ERROR"}}
			return
		}
	}
	r.aut.Estado = ws.EstadoAutorizado
	r.aut.NumeroAutorizacion = claveAcceso
	if ambiente == "PRUEBAS" {
		r.aut.Mensajes = []ws.Mensaje{{Identificador: "60", Mensaje: "ESTE PROCESO FUE REALIZADO EN EL AMBIENTE DE PRUEBAS", Tipo: "ADVERTENCIA"}}
	}
}

func (s *Servidor) autorizacion(w http.ResponseWriter, r *http.Request) {
	nombre, claveAcceso, err := parametros(r, "claveAccesoComprobante", "claveAccesoLote")
	if err != nil {
		falla(w, err.Error())
		return
	}
	if nombre == "claveAccesoLote" {
		s.autorizacionLote(w, claveAcceso)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.autorizaciones++
	var sb strings.Builder
	sb.WriteString(`<RespuestaAutorizacionComprobante><claveAccesoConsultada>` + esc(claveAcceso) + `</claveAccesoConsultada>`)
	reg, ok := s.docs[claveAcceso]
	if ok && !reg.resuelto {
		reg.consultas++
		if reg.consultas > s.ConsultasEnProceso {
			s.resolver(claveAcceso, reg)
		}
	}
	if !ok || !reg.resuelto {
		sb.WriteString(`<numeroComprobantes>0</numeroComprobantes><autorizaciones/>`)
	} else {
		sb.WriteString(`<numeroComprobantes>1</numeroComprobantes><autorizaciones>` + autorizacionXML(reg.aut) + `</autorizaciones>`)
	}
	sb.WriteString(`</RespuestaAutorizacionComprobante>`)
	io.WriteString(w, envolver("http://ec.gob.sri.ws.autorizacion", "autorizacionComprobante", sb.String()))
}

func (s *Servidor) consulta(w http.ResponseWriter, r *http.Request) {
	claveAcceso, err := parametro(r, "claveAcceso")
	if err != nil {
		falla(w, err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	reg, ok := s.docs[claveAcceso]
	if len(claveAcceso) != clave.Longitud {
		ok = false
	}
	var cont string
	if !ok || !reg.resuelto {
		cont = `<EstadoAutorizacionComprobante><estadoConsulta>RECHAZADA</estadoConsulta><claveAcceso>` + esc(claveAcceso) + `</claveAcceso>` +
			mensajesXML([]ws.Mensaje{{Identificador: "99", Mensaje: "ERROR AL CONSULTAR DATOS DEL SERVICIO WEB",
				InformacionAdicional: "No existen datos para los parámetros ingresados", Tipo: "ERROR"}}) + `</EstadoAutorizacionComprobante>`
	} else {
		cont = `<EstadoAutorizacionComprobante><claveAcceso>` + esc(claveAcceso) + `</claveAcceso><mensajes/><estadoAutorizacion>` +
			reg.aut.Estado + `</estadoAutorizacion><tipoComprobante>Factura</tipoComprobante><rucEmisor>` + claveAcceso[10:23] +
			`</rucEmisor><fechaAutorizacion>` + reg.aut.FechaAutorizacion + `</fechaAutorizacion></EstadoAutorizacionComprobante>`
	}
	io.WriteString(w, envolver("http://ec.gob.sri.ws.consultas", "consultarEstadoAutorizacionComprobante", cont))
}
