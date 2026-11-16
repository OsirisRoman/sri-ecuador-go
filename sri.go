// Package sri es la puerta de entrada del SDK de facturación electrónica del
// SRI de Ecuador: reúne en un Cliente el flujo completo de emisión de un
// comprobante en el esquema offline.
//
//	cert, _ := certificado.CargarArchivo("firma.p12", os.Getenv("SRI_P12_PASSWORD"))
//	cliente, _ := sri.Nuevo(sri.Config{Ambiente: comprobante.Pruebas, Certificado: cert})
//
//	f := &factura.Factura{ … }
//	f.Calcular()
//	f.GenerarClaveAcceso("")
//
//	res, err := cliente.Emitir(ctx, f)
//	if err != nil { … }               // no se pudo completar el flujo
//	if res.Autorizado() { guardar(res.XMLAutorizado()) }
//
// Cada paso está también disponible por separado (Preparar, Enviar,
// Autorizar) y en los paquetes especializados: factura (modelo), esquema
// (XSD), xades (firma), ws (servicios web), clave (clave de acceso).
//
// # Qué significa cada resultado
//
// El SDK distingue cuatro hechos que no deben confundirse:
//
//   - Válido localmente: cumple las reglas tributarias y el XSD (Preparar).
//   - Firmado: lleva una firma XAdES-BES criptográficamente correcta.
//   - Recibido: el SRI lo aceptó para procesarlo (EstadoRecibido). No es
//     todavía un comprobante válido.
//   - Autorizado: el SRI lo autorizó (EstadoAutorizado). Solo entonces
//     existe tributariamente.
package sri

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/OsirisRoman/sri-ecuador-go/certificado"
	"github.com/OsirisRoman/sri-ecuador-go/clave"
	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/esquema"
	"github.com/OsirisRoman/sri-ecuador-go/ws"
	"github.com/OsirisRoman/sri-ecuador-go/xades"
)

// Comprobante es lo que el Cliente necesita de un documento para emitirlo.
// Lo implementan *factura.Factura, *notacredito.NotaCredito,
// *notadebito.NotaDebito y *retencion.ComprobanteRetencion.
type Comprobante interface {
	// XML serializa el comprobante sin firma.
	XML() ([]byte, error)
	// Validar aplica las reglas tributarias locales.
	Validar() comprobante.Hallazgos
	// Info devuelve el bloque infoTributaria.
	Info() comprobante.InfoTributaria
}

// Estado es la situación de un comprobante en el flujo de emisión.
type Estado int

const (
	// EstadoFirmado: validado y firmado localmente; no se ha enviado.
	EstadoFirmado Estado = iota
	// EstadoDevuelto: el servicio de recepción lo rechazó (DEVUELTA). Hay
	// que corregirlo y reenviarlo con la misma clave de acceso.
	EstadoDevuelto
	// EstadoRecibido: el SRI lo recibió pero aún no hay resultado de
	// autorización. Consulte más tarde con Autorizar; no lo reenvíe.
	EstadoRecibido
	// EstadoNoAutorizado: el SRI lo procesó y no lo autorizó.
	EstadoNoAutorizado
	// EstadoAutorizado: el SRI lo autorizó.
	EstadoAutorizado
)

func (e Estado) String() string {
	switch e {
	case EstadoFirmado:
		return "FIRMADO"
	case EstadoDevuelto:
		return "DEVUELTO"
	case EstadoRecibido:
		return "RECIBIDO"
	case EstadoNoAutorizado:
		return "NO AUTORIZADO"
	case EstadoAutorizado:
		return "AUTORIZADO"
	}
	return fmt.Sprintf("Estado(%d)", int(e))
}

// Resultado describe hasta dónde llegó un comprobante y con qué respuesta.
type Resultado struct {
	Estado      Estado
	ClaveAcceso string
	// XMLFirmado es el documento enviado (o listo para enviar).
	XMLFirmado []byte
	// Mensajes son los errores y advertencias devueltos por el SRI en la
	// etapa que determinó el Estado.
	Mensajes []ws.Mensaje
	// Recepcion y Autorizacion son las respuestas completas, cuando las hay.
	Recepcion    *ws.RespuestaRecepcion
	Autorizacion *ws.Autorizacion
	// Advertencias son los hallazgos locales que no impidieron la emisión.
	Advertencias comprobante.Hallazgos
}

// Autorizado informa si el SRI autorizó el comprobante.
func (r *Resultado) Autorizado() bool { return r.Estado == EstadoAutorizado }

// Pendiente informa si el SRI recibió el comprobante pero aún no lo
// resuelve.
func (r *Resultado) Pendiente() bool { return r.Estado == EstadoRecibido }

// FechaAutorizacion devuelve la fecha de autorización, si la hay.
func (r *Resultado) FechaAutorizacion() (time.Time, bool) {
	if r.Autorizacion == nil {
		return time.Time{}, false
	}
	t, err := r.Autorizacion.Fecha()
	return t, err == nil
}

// XMLAutorizado devuelve el documento que debe conservarse y entregarse al
// receptor: el comprobante firmado envuelto en el elemento <autorizacion>
// con el estado, número y fecha que asignó el SRI. Devuelve nil si el
// comprobante no está autorizado.
func (r *Resultado) XMLAutorizado() []byte {
	if !r.Autorizado() || r.Autorizacion == nil {
		return nil
	}
	a := r.Autorizacion
	doc := a.Comprobante
	if strings.TrimSpace(doc) == "" {
		doc = string(r.XMLFirmado)
	}
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n<autorizacion>")
	campo := func(nombre, valor string) {
		sb.WriteString("<" + nombre + ">")
		_ = xml.EscapeText(&sb, []byte(valor))
		sb.WriteString("</" + nombre + ">")
	}
	campo("estado", a.Estado)
	campo("numeroAutorizacion", a.NumeroAutorizacion)
	campo("fechaAutorizacion", a.FechaAutorizacion)
	campo("ambiente", a.Ambiente)
	sb.WriteString("<comprobante><![CDATA[" + strings.ReplaceAll(doc, "]]>", "]]]]><![CDATA[>") + "]]></comprobante>")
	sb.WriteString("</autorizacion>")
	return []byte(sb.String())
}

// Errores del flujo.
var (
	// ErrAmbiente indica que el ambiente del comprobante no es el del
	// Cliente. Evita, sobre todo, enviar a producción por accidente.
	ErrAmbiente = errors.New("sri: el ambiente del comprobante no coincide con el del cliente")
	// ErrSinCertificado indica que la operación requiere firmar y el
	// Cliente no tiene certificado.
	ErrSinCertificado = errors.New("sri: el cliente no tiene certificado de firma")
	// ErrEmisor indica que el RUC del comprobante no es el del emisor
	// configurado.
	ErrEmisor = errors.New("sri: el RUC del comprobante no corresponde al emisor configurado")
)

// ErrorValidacion indica que el comprobante no superó las reglas locales y
// por tanto no se firmó ni se envió.
type ErrorValidacion struct {
	Hallazgos comprobante.Hallazgos
}

func (e *ErrorValidacion) Error() string { return e.Hallazgos.Error() }

// Espera configura la consulta de autorización tras una recepción exitosa.
// El SRI procesa de forma asíncrona (sección 7.4): hay que esperar antes de
// preguntar, y puede hacer falta preguntar más de una vez.
type Espera struct {
	// Inicial es la pausa entre RECIBIDA y la primera consulta.
	Inicial time.Duration
	// Intervalo es la pausa entre consultas; crece un 50 % cada vez hasta
	// IntervaloMaximo.
	Intervalo       time.Duration
	IntervaloMaximo time.Duration
	// Consultas es el número máximo de consultas antes de devolver el
	// comprobante como EstadoRecibido (pendiente).
	Consultas int
}

// EsperaPorDefecto: primera consulta a los 3 s y hasta 6 consultas en
// aproximadamente un minuto.
var EsperaPorDefecto = Espera{Inicial: 3 * time.Second, Intervalo: 4 * time.Second, IntervaloMaximo: 20 * time.Second, Consultas: 6}

// Config configura un Cliente.
type Config struct {
	// Ambiente del SRI. Obligatorio: no hay valor por defecto para que
	// producción nunca se elija por omisión.
	Ambiente comprobante.Ambiente
	// Certificado de firma. Puede ser nil si solo se van a consultar
	// autorizaciones o a enviar documentos ya firmados.
	Certificado *certificado.Certificado
	// Algoritmo de firma; por defecto xades.SHA256.
	Algoritmo xades.Algoritmo
	// RUC del emisor. Si se indica, el Cliente se niega a firmar
	// comprobantes de otro RUC.
	RUC string
	// Servicios permite sustituir el cliente de servicios web, por ejemplo
	// para apuntarlo a sritest o configurar su transporte. Por defecto,
	// ws.Nuevo(Ambiente).
	Servicios *ws.Cliente
	// Espera de autorización; el valor cero usa EsperaPorDefecto.
	Espera Espera
	// OmitirXSD desactiva la validación de esquema previa a la firma. No
	// se recomienda: el SRI devolvería el comprobante con el error 35.
	OmitirXSD bool
}

// Cliente emite comprobantes. Es seguro para uso concurrente.
type Cliente struct {
	cfg    Config
	ws     *ws.Cliente
	dormir func(context.Context, time.Duration) error
	ahora  func() time.Time
}

// Nuevo crea un Cliente.
func Nuevo(cfg Config) (*Cliente, error) {
	if cfg.Ambiente != comprobante.Pruebas && cfg.Ambiente != comprobante.Produccion {
		return nil, fmt.Errorf("sri: Config.Ambiente debe ser comprobante.Pruebas o comprobante.Produccion")
	}
	if cfg.RUC != "" {
		if err := comprobante.ValidarRUC(cfg.RUC); err != nil {
			return nil, fmt.Errorf("sri: Config.RUC: %w", err)
		}
	}
	if cfg.Espera == (Espera{}) {
		cfg.Espera = EsperaPorDefecto
	}
	if cfg.Espera.Consultas < 1 {
		cfg.Espera.Consultas = 1
	}
	c := &Cliente{cfg: cfg, ws: cfg.Servicios, ahora: time.Now}
	if c.ws == nil {
		c.ws = ws.Nuevo(cfg.Ambiente)
	}
	c.dormir = func(ctx context.Context, d time.Duration) error {
		if d <= 0 {
			return ctx.Err()
		}
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			return nil
		}
	}
	return c, nil
}

// Servicios devuelve el cliente de servicios web subyacente.
func (c *Cliente) Servicios() *ws.Cliente { return c.ws }

// Preparar valida el comprobante (reglas tributarias y XSD), lo firma y
// devuelve un Resultado en EstadoFirmado. No se comunica con el SRI.
//
// Si el comprobante tiene errores de validación devuelve *ErrorValidacion
// con todos los hallazgos; si no cumple el XSD, *esquema.ErrorValidacion.
func (c *Cliente) Preparar(doc Comprobante) (*Resultado, error) {
	if c.cfg.Certificado == nil {
		return nil, ErrSinCertificado
	}
	info := doc.Info()
	if info.Ambiente != c.cfg.Ambiente {
		return nil, fmt.Errorf("%w: comprobante en %s, cliente en %s", ErrAmbiente, info.Ambiente, c.cfg.Ambiente)
	}
	if c.cfg.RUC != "" && info.RUC != c.cfg.RUC {
		return nil, fmt.Errorf("%w: %s", ErrEmisor, info.RUC)
	}
	hallazgos := doc.Validar()
	if hallazgos.Err() != nil {
		return nil, &ErrorValidacion{Hallazgos: hallazgos}
	}
	sinFirma, err := doc.XML()
	if err != nil {
		return nil, err
	}
	if !c.cfg.OmitirXSD {
		if err := esquema.Validar(sinFirma); err != nil {
			return nil, err
		}
	}
	firmado, err := xades.Firmar(sinFirma, c.cfg.Certificado, &xades.Opciones{Algoritmo: c.cfg.Algoritmo, Momento: c.ahora()})
	if err != nil {
		return nil, err
	}
	if !c.cfg.OmitirXSD {
		// El documento firmado también debe cumplir el XSD (incluye el
		// esquema de ds:Signature).
		if err := esquema.Validar(firmado); err != nil {
			return nil, fmt.Errorf("sri: el documento firmado no cumple el XSD: %w", err)
		}
	}
	if len(firmado) > ws.TamanoMaximo {
		return nil, fmt.Errorf("%w (%d bytes)", ws.ErrTamano, len(firmado))
	}
	return &Resultado{
		Estado:       EstadoFirmado,
		ClaveAcceso:  info.ClaveAcceso,
		XMLFirmado:   firmado,
		Advertencias: hallazgos.Advertencias(),
	}, nil
}

// Emitir ejecuta el flujo completo: Preparar, Enviar y Autorizar.
//
// Devuelve error solo cuando el flujo no pudo completarse (validación
// local, firma, comunicación). Que el SRI devuelva o no autorice el
// comprobante no es un error de Go: se refleja en Resultado.Estado y
// Resultado.Mensajes.
//
// Si la comunicación falla después de firmar, el Resultado parcial se
// devuelve junto con el error: contiene la clave de acceso y el XML
// firmado, que deben conservarse para reintentar con Reanudar.
func (c *Cliente) Emitir(ctx context.Context, doc Comprobante) (*Resultado, error) {
	res, err := c.Preparar(doc)
	if err != nil {
		return nil, err
	}
	return c.Reanudar(ctx, res.XMLFirmado, res.Advertencias)
}

// Reanudar envía un documento ya firmado y espera su autorización. Sirve
// para completar una emisión interrumpida: es idempotente respecto del SRI,
// porque un comprobante que ya había sido recibido no se duplica (el SRI
// responde con los códigos 43 o 70 y se pasa a consultar la autorización).
func (c *Cliente) Reanudar(ctx context.Context, xmlFirmado []byte, advertencias comprobante.Hallazgos) (*Resultado, error) {
	res, err := c.Enviar(ctx, xmlFirmado)
	if res != nil {
		res.Advertencias = advertencias
	}
	if err != nil || res.Estado == EstadoDevuelto {
		return res, err
	}
	aut, err := c.esperarAutorizacion(ctx, res.ClaveAcceso)
	if aut != nil {
		aut.XMLFirmado = xmlFirmado
		aut.Recepcion = res.Recepcion
		aut.Advertencias = advertencias
		return aut, err
	}
	return res, err
}

// Enviar entrega un documento firmado al servicio de recepción. El
// Resultado queda en EstadoRecibido o EstadoDevuelto.
func (c *Cliente) Enviar(ctx context.Context, xmlFirmado []byte) (*Resultado, error) {
	claveAcceso, ambiente, err := leerClave(xmlFirmado)
	if err != nil {
		return nil, err
	}
	if ambiente != string(c.cfg.Ambiente) {
		return nil, fmt.Errorf("%w: comprobante en %s, cliente en %s", ErrAmbiente, comprobante.Ambiente(ambiente), c.cfg.Ambiente)
	}
	res := &Resultado{Estado: EstadoFirmado, ClaveAcceso: claveAcceso, XMLFirmado: xmlFirmado}
	rec, err := c.ws.ValidarComprobante(ctx, xmlFirmado)
	if err != nil {
		return res, err
	}
	res.Recepcion = rec
	res.Mensajes = rec.Mensajes()
	switch {
	case rec.Recibida(), rec.YaRecibido():
		res.Estado = EstadoRecibido
	default:
		res.Estado = EstadoDevuelto
	}
	return res, nil
}

// Autorizar consulta una sola vez el resultado de un comprobante por su
// clave de acceso. Útil para resolver más tarde los que quedaron en
// EstadoRecibido.
func (c *Cliente) Autorizar(ctx context.Context, claveAcceso string) (*Resultado, error) {
	c2, err := clave.Analizar(claveAcceso)
	if err != nil {
		return nil, err
	}
	if c2.Ambiente != string(c.cfg.Ambiente) {
		return nil, fmt.Errorf("%w: la clave es de %s", ErrAmbiente, comprobante.Ambiente(c2.Ambiente))
	}
	resp, err := c.ws.AutorizacionComprobante(ctx, claveAcceso)
	if err != nil {
		return nil, err
	}
	res := &Resultado{ClaveAcceso: claveAcceso, Estado: EstadoRecibido}
	if a := resp.Autorizada(); a != nil {
		res.Estado, res.Autorizacion, res.Mensajes = EstadoAutorizado, a, a.Mensajes
		return res, nil
	}
	if resp.EnProceso() {
		return res, nil
	}
	u := resp.Ultima()
	res.Estado, res.Autorizacion, res.Mensajes = EstadoNoAutorizado, u, u.Mensajes
	return res, nil
}

func (c *Cliente) esperarAutorizacion(ctx context.Context, claveAcceso string) (*Resultado, error) {
	e := c.cfg.Espera
	if err := c.dormir(ctx, e.Inicial); err != nil {
		return nil, err
	}
	intervalo := e.Intervalo
	var ultimo *Resultado
	for i := 1; ; i++ {
		res, err := c.Autorizar(ctx, claveAcceso)
		if err != nil {
			return ultimo, err
		}
		ultimo = res
		if !res.Pendiente() || i >= e.Consultas {
			return res, nil
		}
		if err := c.dormir(ctx, intervalo); err != nil {
			return res, err
		}
		if intervalo += intervalo / 2; e.IntervaloMaximo > 0 && intervalo > e.IntervaloMaximo {
			intervalo = e.IntervaloMaximo
		}
	}
}

// leerClave extrae la clave de acceso y el ambiente de un comprobante.
func leerClave(doc []byte) (claveAcceso, ambiente string, err error) {
	var cab struct {
		Ambiente string `xml:"infoTributaria>ambiente"`
		Clave    string `xml:"infoTributaria>claveAcceso"`
	}
	if err := xml.Unmarshal(doc, &cab); err != nil {
		return "", "", fmt.Errorf("sri: comprobante ilegible: %w", err)
	}
	cab.Clave, cab.Ambiente = strings.TrimSpace(cab.Clave), strings.TrimSpace(cab.Ambiente)
	if err := clave.Validar(cab.Clave); err != nil {
		return "", "", err
	}
	return cab.Clave, cab.Ambiente, nil
}
