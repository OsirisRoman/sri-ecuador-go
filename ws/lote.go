package ws

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/OsirisRoman/sri-ecuador-go/clave"
)

// Límites de un envío por lote (sección 7.5 y Tabla 8).
const (
	// MaxComprobantesLote es el número máximo de comprobantes de un lote.
	MaxComprobantesLote = 50
	// TamanoMaximoLote es el tamaño máximo del documento <lote>: 500 kB.
	TamanoMaximoLote = 500 * 1024
)

// ErrLote indica que un lote no cumple los límites o la forma exigidos.
var ErrLote = errors.New("ws: lote inválido")

// Lote agrupa comprobantes ya firmados para enviarlos en una sola llamada
// (Tabla 8).
type Lote struct {
	// ClaveAcceso identifica al lote. Se compone como la de un comprobante
	// (Tabla 9), pero con una serie y un secuencial que el emisor reserva
	// para sus envíos por lote, no los de un comprobante.
	ClaveAcceso string
	// RUC del emisor de los comprobantes.
	RUC string
	// Comprobantes son los XML firmados, hasta MaxComprobantesLote.
	Comprobantes [][]byte
}

// XML compone el documento <lote>.
func (l Lote) XML() ([]byte, error) {
	if err := clave.Validar(l.ClaveAcceso); err != nil {
		return nil, fmt.Errorf("%w: clave de acceso del lote: %v", ErrLote, err)
	}
	if len(l.RUC) != 13 {
		return nil, fmt.Errorf("%w: el RUC debe tener 13 dígitos", ErrLote)
	}
	if n := len(l.Comprobantes); n == 0 || n > MaxComprobantesLote {
		return nil, fmt.Errorf("%w: tiene %d comprobantes; se admiten de 1 a %d", ErrLote, n, MaxComprobantesLote)
	}
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><lote version="1.0.0"><claveAcceso>` + l.ClaveAcceso + `</claveAcceso><ruc>` + l.RUC + `</ruc><comprobantes>`)
	for i, c := range l.Comprobantes {
		if len(bytes.TrimSpace(c)) == 0 {
			return nil, fmt.Errorf("%w: el comprobante %d está vacío", ErrLote, i)
		}
		b.WriteString("<comprobante><![CDATA[")
		b.WriteString(strings.ReplaceAll(string(c), "]]>", "]]]]><![CDATA[>"))
		b.WriteString("]]></comprobante>")
	}
	b.WriteString(`</comprobantes></lote>`)
	if b.Len() > TamanoMaximoLote {
		return nil, fmt.Errorf("%w: ocupa %d bytes; el máximo es %d", ErrLote, b.Len(), TamanoMaximoLote)
	}
	return b.Bytes(), nil
}

// ValidarLote envía un lote al servicio de recepción, por la misma
// operación que un comprobante individual. Recibida indica que el SRI
// aceptó el lote para procesarlo; el resultado de cada comprobante se
// consulta después con AutorizacionLote.
func (c *Cliente) ValidarLote(ctx context.Context, l Lote) (*RespuestaRecepcion, error) {
	doc, err := l.XML()
	if err != nil {
		return nil, err
	}
	cuerpo := sobre(nsRecepcion, "validarComprobante", "xml", base64.StdEncoding.EncodeToString(doc))
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

// RespuestaLote es la respuesta de autorizacionComprobanteLote.
type RespuestaLote struct {
	ClaveAccesoLoteConsultada string         `xml:"claveAccesoLoteConsultada"`
	NumeroComprobantesLote    string         `xml:"numeroComprobantesLote"`
	Autorizaciones            []Autorizacion `xml:"autorizaciones>autorizacion"`
}

// AutorizacionLote consulta el resultado de cada comprobante de un lote por
// la clave de acceso del lote.
func (c *Cliente) AutorizacionLote(ctx context.Context, claveAccesoLote string) (*RespuestaLote, error) {
	if err := clave.Validar(claveAccesoLote); err != nil {
		return nil, err
	}
	cuerpo := sobre(nsAutorizacion, "autorizacionComprobanteLote", "claveAccesoLote", claveAccesoLote)
	var env struct {
		R *RespuestaLote `xml:"Body>autorizacionComprobanteLoteResponse>RespuestaAutorizacionLote"`
	}
	if err := c.invocar(ctx, c.ep.Autorizacion, cuerpo, &env); err != nil {
		return nil, err
	}
	if env.R == nil {
		return nil, fmt.Errorf("%w: falta RespuestaAutorizacionLote", ErrRespuesta)
	}
	return env.R, nil
}
