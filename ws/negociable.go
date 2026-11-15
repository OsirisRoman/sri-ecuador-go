package ws

import (
	"context"
	"fmt"
	"strings"

	"github.com/OsirisRoman/sri-ecuador-go/clave"
)

// EstadoNegociable es la respuesta de la consulta de factura comercial
// negociable (sección 8.2).
type EstadoNegociable struct {
	// EstadoConsulta vale "RECHAZADA" cuando la consulta no pudo resolverse
	// (por ejemplo, fecha de emisión fuera del rango permitido); los motivos
	// van en Mensajes.
	EstadoConsulta string    `xml:"estadoConsulta"`
	ClaveAcceso    string    `xml:"claveAcceso"`
	Mensajes       []Mensaje `xml:"mensajes>mensaje"`
	// EstadoConfirmacion vale "SI" cuando la factura fue aceptada como
	// factura comercial negociable.
	EstadoConfirmacion string `xml:"estadoConfirmacion"`
}

// Confirmada informa si la factura fue aceptada como comercial negociable.
func (e *EstadoNegociable) Confirmada() bool {
	return strings.EqualFold(strings.TrimSpace(e.EstadoConfirmacion), "SI")
}

// ConsultarNegociable consulta si una factura fue aceptada como factura
// comercial negociable.
func (c *Cliente) ConsultarNegociable(ctx context.Context, claveAcceso string) (*EstadoNegociable, error) {
	if err := clave.Validar(claveAcceso); err != nil {
		return nil, err
	}
	cuerpo := sobre(nsConsultas, "consultarEstadoConfirmacionFacturaComercialNegociable", "claveAcceso", claveAcceso)
	var env struct {
		R *EstadoNegociable `xml:"Body>consultarEstadoConfirmacionFacturaComercialNegociableResponse>EstadoConfirmacionFacturaComercialNegociable"`
	}
	if err := c.invocar(ctx, c.ep.ConsultaFactura, cuerpo, &env); err != nil {
		return nil, err
	}
	if env.R == nil {
		return nil, fmt.Errorf("%w: falta EstadoConfirmacionFacturaComercialNegociable", ErrRespuesta)
	}
	return env.R, nil
}
