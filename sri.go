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
// Lo implementan *factura.Factura, *liquidacion.LiquidacionCompra,
// *notacredito.NotaCredito, *notadebito.NotaDebito,
// *guiaremision.GuiaRemision, *retencion.ComprobanteRetencion y
// *retencion.ComprobanteV1.
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
	// EstadoDiscrepancia: el SRI tiene registrado con esta clave de acceso
	// un comprobante distinto del que se envió o se consulta (por ejemplo,
	// una versión anterior corregida, o un reenvío con contenido cambiado).
	// El estado que informa el SRI (Resultado.Autorizacion) corresponde a
	// ese otro documento, no a Resultado.XMLFirmado: no lo entregue ni lo
	// dé por autorizado. Ver ErrorDiscrepancia.
	EstadoDiscrepancia
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
	case EstadoDiscrepancia:
		return "DISCREPANCIA"
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
	// Autorizaciones son todas las autorizaciones que el SRI devolvió para
	// la clave en la última consulta, en el orden en que llegaron: una por
	// cada envío que procesó. Autorizacion es la que determina el Estado.
	// Sirve para conciliar cuando hay varias versiones de un comprobante
	// con la misma clave.
	Autorizaciones []ws.Autorizacion
	// Advertencias son los hallazgos locales que no impidieron la emisión.
	Advertencias comprobante.Hallazgos
	// Verificado indica que la autorización que determina el Estado
	// contiene, en contenido, Resultado.XMLFirmado (misma xades.Huella): el
	// AUTORIZADO o el NO AUTORIZADO es el de ese documento. Con Reanudar,
	// Emitir y ConsultarDocumento, EstadoAutorizado implica Verificado. Es
	// false si no hay resultado, si el rechazo no trae el comprobante o si
	// el resultado viene de Autorizar, que solo conoce la clave.
	Verificado bool
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
// receptor: el comprobante firmado, tal como lo devolvió el SRI, envuelto
// en el elemento <autorizacion> con el estado, número y fecha que asignó.
// Devuelve nil si el comprobante no está autorizado o si la autorización no
// trae el comprobante.
//
// Si el resultado viene de Autorizar (Verificado es false), el comprobante
// es el que el SRI tiene registrado con esa clave, que no se ha contrastado
// con ningún documento local.
func (r *Resultado) XMLAutorizado() []byte {
	if !r.Autorizado() || r.Autorizacion == nil || strings.TrimSpace(r.Autorizacion.Comprobante) == "" {
		return nil
	}
	a := r.Autorizacion
	doc := a.Comprobante
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
	// ErrDiscrepancia indica que el SRI tiene registrado con la misma clave
	// un comprobante distinto del enviado; ver ErrorDiscrepancia y
	// EstadoDiscrepancia.
	ErrDiscrepancia = errors.New("sri: el SRI tiene otro comprobante registrado con esta clave de acceso")
	// ErrEstadoIndeterminado indica que la respuesta de autorización no
	// permite saber el estado del comprobante: un estado vacío o
	// desconocido. El Resultado queda en EstadoRecibido (pendiente): no
	// genere otro comprobante; vuelva a consultar más tarde.
	ErrEstadoIndeterminado = errors.New("sri: estado de autorización indeterminado")
	// ErrSinComprobante indica que el SRI informa AUTORIZADO para la clave
	// pero la autorización no trae el comprobante, de modo que no se puede
	// comprobar que el documento autorizado sea el que se envió o se
	// consulta. Es un estado indeterminado (errors.Is con
	// ErrEstadoIndeterminado también es true): el Resultado queda en
	// EstadoRecibido, con la autorización del SRI en Resultado.Autorizacion
	// para conciliar (número y fecha de autorización). No entregue el
	// documento como autorizado; vuelva a consultar más tarde.
	ErrSinComprobante = errors.New("sri: el SRI informa AUTORIZADO sin devolver el comprobante; no se puede comprobar que sea este documento")
	// ErrRespuestaIncoherente indica que la respuesta de autorización no
	// corresponde a lo consultado: otra clave de acceso u otro ambiente. Se
	// trata como estado desconocido; el Resultado queda en EstadoRecibido.
	ErrRespuestaIncoherente = errors.New("sri: la respuesta de autorización no corresponde a la consulta")
)

// ErrorDiscrepancia describe un comprobante distinto registrado con la
// misma clave. errors.Is(err, ErrDiscrepancia) es true.
//
// Ocurre, por ejemplo, al reenviar con la misma clave un comprobante
// corregido después de un envío de resultado incierto que sí llegó (el SRI
// responde 43 o 70 y devuelve el que ya tenía), o al consultar con un XML
// que no es el que se envió. Qué hacer depende del estado del otro
// documento (Resultado.Autorizacion.Estado): si está autorizado, ese es el
// comprobante válido y lo que corresponde es conservarlo
// (Resultado.Autorizacion.Comprobante) y, si hacía falta corregirlo, emitir
// una nota de crédito o de débito; si no está autorizado, enviar este
// documento (corregido) con Reanudar, que es seguro: si el SRI ya lo tiene
// en proceso, el resultado será pendiente y no se duplica (ficha 5.10).
type ErrorDiscrepancia struct {
	ClaveAcceso string
	// EstadoSRI es el estado que el SRI informa del documento registrado.
	EstadoSRI string
	// Enviado y Registrado son las xades.Huella de los dos documentos. Si
	// el documento del SRI no se pudo leer, Registrado es cero y Causa lo
	// explica.
	Enviado, Registrado [32]byte
	Causa               error
}

func (e *ErrorDiscrepancia) Error() string {
	if e.Causa != nil {
		return fmt.Sprintf("%v (%s): no se pudo leer el comprobante que devolvió el SRI: %v", ErrDiscrepancia, e.ClaveAcceso, e.Causa)
	}
	return fmt.Sprintf("%v (%s, estado del registrado: %s)", ErrDiscrepancia, e.ClaveAcceso, e.EstadoSRI)
}

// Is permite errors.Is(err, ErrDiscrepancia).
func (e *ErrorDiscrepancia) Is(target error) bool { return target == ErrDiscrepancia }

func (e *ErrorDiscrepancia) Unwrap() error { return e.Causa }

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
	// Espera de autorización; el valor cero usa EsperaPorDefecto. Si se
	// piden varias consultas sin Intervalo, se usa el de EsperaPorDefecto.
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
	if cfg.Certificado != nil && cfg.Certificado.Utilizable() != nil {
		return nil, fmt.Errorf("sri: Config.Certificado: %w", cfg.Certificado.Utilizable())
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
	if cfg.Espera.Consultas > 1 && cfg.Espera.Intervalo <= 0 {
		// Sin intervalo, las consultas irían seguidas, sin dar tiempo al
		// SRI a procesar (sección 7.4).
		cfg.Espera.Intervalo = EsperaPorDefecto.Intervalo
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
	if c.cfg.Certificado.Utilizable() != nil {
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
// para completar una emisión interrumpida: reenviar el mismo documento es
// seguro, porque un comprobante que ya había sido recibido no se duplica (el
// SRI responde con los códigos 43 o 70 y se pasa a consultar la
// autorización).
//
// El resultado es el de la autorización que contiene xmlFirmado (misma
// xades.Huella), esté donde esté entre las que devuelve el SRI; ver
// ConsultarDocumento. EstadoAutorizado implica Verificado.
func (c *Cliente) Reanudar(ctx context.Context, xmlFirmado []byte, advertencias comprobante.Hallazgos) (*Resultado, error) {
	res, err := c.Enviar(ctx, xmlFirmado)
	if res != nil {
		res.Advertencias = advertencias
	}
	if err != nil || res.Estado == EstadoDevuelto {
		return res, err
	}
	doc, err := nuevoDocumento(xmlFirmado)
	if err != nil {
		return res, err
	}
	// Si el SRI acaba de recibir este documento (RECIBIDA, o 70: en
	// procesamiento), el NO AUTORIZADO de otra versión con la misma clave
	// es el resultado anterior a la corrección (ficha 5.10 y 5.11): se
	// sigue esperando el de este documento.
	doc.recienRecibido = recibidoAhora(res.Recepcion)
	aut, err := c.esperarAutorizacion(ctx, res.ClaveAcceso, doc)
	if aut == nil {
		return res, err
	}
	aut.Recepcion = res.Recepcion
	aut.Advertencias = advertencias
	return aut, err
}

// ConsultarDocumento consulta una sola vez la autorización de un
// comprobante ya firmado, sin reenviarlo. Es la forma recomendada de
// resolver más tarde un comprobante que quedó en EstadoRecibido.
//
// El SRI puede devolver varias autorizaciones para una clave, una por cada
// envío que procesó (el original rechazado y su corrección, por ejemplo), y
// su orden no está garantizado. El resultado no depende de él: cada
// autorización se vincula con xmlFirmado comparando la xades.Huella del
// comprobante que contiene.
//
//   - Una autorización AUTORIZADO que contiene este documento:
//     EstadoAutorizado, con Verificado.
//   - Un AUTORIZADO que contiene otro documento: EstadoDiscrepancia y
//     *ErrorDiscrepancia. La clave ya tiene un comprobante válido, que no
//     es este.
//   - Un AUTORIZADO que no trae el comprobante: no se puede vincular. El
//     estado es indeterminado: EstadoRecibido con ErrSinComprobante.
//   - Sin ningún AUTORIZADO, lo que diga el SRI de este documento: NO
//     AUTORIZADO (EstadoNoAutorizado, con Verificado y sus mensajes) o en
//     proceso (EstadoRecibido).
//   - Si solo hay rechazos de otras versiones, este documento no tiene
//     resultado: EstadoDiscrepancia, salvo en Reanudar cuando el SRI acaba
//     de recibirlo, que queda pendiente (EstadoRecibido) a la espera del
//     suyo.
//   - Un rechazo sin comprobante no se puede vincular y se informa como
//     EstadoNoAutorizado sin Verificado: corregir y reenviar con la misma
//     clave es seguro en cualquier caso.
//
// Resultado.Autorizaciones conserva todas las autorizaciones recibidas.
//
// Úsela solo si la recepción está confirmada (RECIBIDA, 43 o 70). El SRI
// responde igual, sin autorizaciones, a una clave que nunca recibió y a un
// comprobante en proceso, y ambos casos dan EstadoRecibido: si el estado es
// desconocido porque falló el envío, reenvíe con Reanudar, que es seguro.
func (c *Cliente) ConsultarDocumento(ctx context.Context, xmlFirmado []byte) (*Resultado, error) {
	claveAcceso, _, err := leerClave(xmlFirmado)
	if err != nil {
		return nil, err
	}
	doc, err := nuevoDocumento(xmlFirmado)
	if err != nil {
		return nil, err
	}
	return c.consultar(ctx, claveAcceso, doc)
}

// documento es el comprobante firmado con el que se vinculan las
// autorizaciones de una respuesta.
type documento struct {
	xml    []byte
	huella [32]byte
	// recienRecibido indica que el SRI aceptó este documento en el envío
	// que precede a la consulta.
	recienRecibido bool
}

func nuevoDocumento(xmlFirmado []byte) (*documento, error) {
	h, err := xades.Huella(xmlFirmado)
	if err != nil {
		return nil, fmt.Errorf("sri: el documento firmado es ilegible: %w", err)
	}
	return &documento{xml: xmlFirmado, huella: h}, nil
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
	case strings.EqualFold(strings.TrimSpace(rec.Estado), ws.EstadoDevuelta):
		res.Estado = EstadoDevuelto
	default:
		// Ni RECIBIDA ni DEVUELTA: no se sabe si el SRI lo registró. Se
		// devuelve como un fallo de comunicación; reenviar el mismo
		// documento es seguro.
		return res, fmt.Errorf("%w: la recepción respondió el estado %q", ErrEstadoIndeterminado, rec.Estado)
	}
	return res, nil
}

// Autorizar consulta una sola vez el estado de una clave de acceso.
//
// Solo conoce la clave: no puede comprobar qué documento autorizó el SRI,
// y Resultado.Verificado es siempre false. Para saber si el comprobante
// autorizado es el que usted envió, use ConsultarDocumento con el XML
// firmado. Con varias autorizaciones para la clave, el Estado es
// EstadoAutorizado si alguna lo está; si no, pendiente si alguna está en
// proceso; si no, EstadoNoAutorizado con la de fecha más reciente.
func (c *Cliente) Autorizar(ctx context.Context, claveAcceso string) (*Resultado, error) {
	return c.consultar(ctx, claveAcceso, nil)
}

// consultar pide al SRI las autorizaciones de una clave y las interpreta;
// con doc, respecto de ese documento.
func (c *Cliente) consultar(ctx context.Context, claveAcceso string, doc *documento) (*Resultado, error) {
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
	if doc != nil {
		res.XMLFirmado = doc.xml
	}
	if err := c.coherente(resp, claveAcceso); err != nil {
		return res, err
	}
	res.Autorizaciones = resp.Autorizaciones
	return res, interpretar(res, doc)
}

// vinculo es la relación entre una autorización y el documento consultado.
type vinculo int

const (
	// sinComprobante: la autorización no trae el comprobante.
	sinComprobante vinculo = iota
	// ilegible: trae un comprobante que no se puede leer.
	ilegible
	// ajena: trae otro documento.
	ajena
	// propia: trae el documento consultado (misma xades.Huella).
	propia
)

// vinculada es una autorización con su vínculo.
type vinculada struct {
	*ws.Autorizacion
	vinculo vinculo
	huella  [32]byte
	causa   error
}

// interpretar fija el Estado de res a partir de res.Autorizaciones. No
// depende del orden de la lista: cada autorización cuenta por su estado y,
// si hay documento, por el comprobante que contiene.
func interpretar(res *Resultado, doc *documento) error {
	todas := make([]vinculada, len(res.Autorizaciones))
	for i := range res.Autorizaciones {
		v := vinculada{Autorizacion: &res.Autorizaciones[i]}
		if x := strings.TrimSpace(v.Comprobante); doc != nil && x != "" {
			if v.huella, v.causa = xades.Huella([]byte(x)); v.causa != nil {
				v.vinculo = ilegible
			} else if v.huella == doc.huella {
				v.vinculo = propia
			} else {
				v.vinculo = ajena
			}
		}
		todas[i] = v
	}
	if doc == nil || len(todas) == 0 {
		return porEstado(res, todas)
	}
	con := func(vs []vinculada, cumple func(vinculada) bool) (out []vinculada) {
		for _, v := range vs {
			if cumple(v) {
				out = append(out, v)
			}
		}
		return out
	}
	es := func(k vinculo) func(vinculada) bool { return func(v vinculada) bool { return v.vinculo == k } }
	discrepancia := func(v vinculada) error {
		res.Estado, res.Autorizacion, res.Mensajes = EstadoDiscrepancia, v.Autorizacion, v.Mensajes
		return &ErrorDiscrepancia{ClaveAcceso: res.ClaveAcceso, EstadoSRI: strings.TrimSpace(v.Estado), Enviado: doc.huella, Registrado: v.huella, Causa: v.causa}
	}

	// 1. Una clave solo puede tener un comprobante autorizado.
	if aut := con(todas, func(v vinculada) bool { return v.Autorizado() }); len(aut) > 0 {
		if p := con(aut, es(propia)); len(p) > 0 {
			res.Estado, res.Autorizacion, res.Mensajes, res.Verificado = EstadoAutorizado, p[0].Autorizacion, p[0].Mensajes, true
			return nil
		}
		for _, k := range []vinculo{ajena, ilegible} {
			if o := con(aut, es(k)); len(o) > 0 {
				return discrepancia(o[0])
			}
		}
		// Autorizado, pero no se sabe qué documento: no se puede dar por
		// autorizado este ni afirmar que es otro.
		res.Autorizacion, res.Mensajes = aut[0].Autorizacion, aut[0].Mensajes
		return fmt.Errorf("%w: %w", ErrEstadoIndeterminado, ErrSinComprobante)
	}
	// 2. Lo que el SRI dice de este documento.
	if p := con(todas, es(propia)); len(p) > 0 {
		err := porEstado(res, p)
		res.Verificado = res.Estado == EstadoNoAutorizado
		return err
	}
	// 3. Ninguna autorización es de este documento.
	if len(con(todas, func(v vinculada) bool { return v.EnProceso() })) > 0 {
		return nil
	}
	if o := con(todas, func(v vinculada) bool { return !v.NoAutorizado() }); len(o) > 0 {
		res.Autorizacion, res.Mensajes = o[0].Autorizacion, o[0].Mensajes
		return fmt.Errorf("%w: %q", ErrEstadoIndeterminado, o[0].Estado)
	}
	// Solo rechazos. Uno sin comprobante puede ser el de este documento.
	if s := con(todas, es(sinComprobante)); len(s) > 0 {
		return porEstado(res, s)
	}
	if i := con(todas, es(ilegible)); len(i) > 0 {
		return discrepancia(masReciente(i))
	}
	if doc.recienRecibido {
		// El SRI todavía muestra solo el rechazo de la versión anterior.
		return nil
	}
	return discrepancia(masReciente(todas))
}

// porEstado fija el Estado de res según los estados de un conjunto de
// autorizaciones, sin atender a su orden: autorizado si alguna lo está;
// pendiente si alguna está en proceso; indeterminado si alguna tiene un
// estado vacío o desconocido; y no autorizado, con la de fecha más
// reciente, si todas son rechazos.
func porEstado(res *Resultado, vs []vinculada) error {
	var rechazos []vinculada
	var rara *vinculada
	pendiente := false
	for i, v := range vs {
		switch {
		case v.Autorizado():
			res.Estado, res.Autorizacion, res.Mensajes = EstadoAutorizado, v.Autorizacion, v.Mensajes
			return nil
		case v.EnProceso():
			pendiente = true
		case v.NoAutorizado():
			rechazos = append(rechazos, v)
		case rara == nil:
			rara = &vs[i]
		}
	}
	switch {
	case pendiente || len(vs) == 0:
		return nil
	case rara != nil:
		// Un estado vacío o desconocido no es un rechazo: tratarlo como
		// definitivo podría llevar a emitir otro comprobante mientras el
		// original sigue en curso.
		res.Autorizacion, res.Mensajes = rara.Autorizacion, rara.Mensajes
		return fmt.Errorf("%w: %q", ErrEstadoIndeterminado, rara.Estado)
	}
	u := masReciente(rechazos)
	res.Estado, res.Autorizacion, res.Mensajes = EstadoNoAutorizado, u.Autorizacion, u.Mensajes
	return nil
}

// masReciente devuelve la autorización de fecha más reciente. Si alguna
// fecha no se puede leer, o hay empate, devuelve la última de la lista.
func masReciente(vs []vinculada) vinculada {
	mejor := vs[len(vs)-1]
	t, err := mejor.Fecha()
	if err != nil {
		return mejor
	}
	for _, v := range vs[:len(vs)-1] {
		f, err := v.Fecha()
		if err != nil {
			return vs[len(vs)-1]
		}
		if f.After(t) {
			mejor, t = v, f
		}
	}
	return mejor
}

// coherente comprueba que la respuesta de autorización corresponda a la
// clave consultada y al ambiente del cliente: claveAccesoConsultada, la
// clave del comprobante devuelto y el ambiente de cada autorización.
func (c *Cliente) coherente(resp *ws.RespuestaAutorizacion, claveAcceso string) error {
	if cc := strings.TrimSpace(resp.ClaveAccesoConsultada); cc != "" && cc != claveAcceso {
		return fmt.Errorf("%w: se consultó %s y respondió por %s", ErrRespuestaIncoherente, claveAcceso, cc)
	}
	for i := range resp.Autorizaciones {
		a := &resp.Autorizaciones[i]
		if amb := strings.TrimSpace(a.Ambiente); amb != "" && !mismoAmbiente(amb, c.cfg.Ambiente) {
			return fmt.Errorf("%w: autorización del ambiente %s y el cliente es de %s", ErrRespuestaIncoherente, amb, c.cfg.Ambiente)
		}
		if doc := strings.TrimSpace(a.Comprobante); doc != "" {
			if clave, _, err := leerClave([]byte(doc)); err == nil && clave != claveAcceso {
				return fmt.Errorf("%w: la autorización contiene el comprobante %s", ErrRespuestaIncoherente, clave)
			}
		}
	}
	return nil
}

// mismoAmbiente compara el texto de ambiente de una autorización
// («PRUEBAS», «PRODUCCIÓN», con o sin tilde) con el del cliente.
func mismoAmbiente(texto string, a comprobante.Ambiente) bool {
	t := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(texto, "Ó", "O"), "ó", "o"))
	switch a {
	case comprobante.Pruebas:
		return t == "PRUEBAS" || t == string(comprobante.Pruebas)
	case comprobante.Produccion:
		return t == "PRODUCCION" || t == string(comprobante.Produccion)
	}
	return false
}

// recibidoAhora informa si la recepción aceptó el documento en este envío
// (RECIBIDA) o lo tiene en procesamiento (código 70), a diferencia del 43,
// que solo dice que la clave ya está registrada.
func recibidoAhora(rec *ws.RespuestaRecepcion) bool {
	if rec == nil {
		return false
	}
	if rec.Recibida() {
		return true
	}
	if !rec.YaRecibido() {
		return false
	}
	for _, m := range rec.Mensajes() {
		if m.EsError() && strings.TrimSpace(m.Identificador) != ws.CodClaveEnProcesamiento {
			return false
		}
	}
	return true
}

// esperarAutorizacion consulta hasta obtener un resultado para doc o
// agotar las consultas configuradas.
func (c *Cliente) esperarAutorizacion(ctx context.Context, claveAcceso string, doc *documento) (*Resultado, error) {
	e := c.cfg.Espera
	if err := c.dormir(ctx, e.Inicial); err != nil {
		return nil, err
	}
	intervalo := e.Intervalo
	var ultimo *Resultado
	for i := 1; ; i++ {
		res, err := c.consultar(ctx, claveAcceso, doc)
		if err != nil {
			if res != nil {
				return res, err
			}
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
