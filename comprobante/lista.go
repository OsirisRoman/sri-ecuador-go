package comprobante

import "encoding/xml"

// Etiquetado lo implementan los tipos que pueden ir en una Lista: indica el
// nombre del elemento XML de cada ítem.
type Etiquetado interface {
	EtiquetaXML() string
}

// Lista es una secuencia de elementos XML dentro de un contenedor, como
// <pagos><pago/>…</pagos>. Es un slice corriente (admite append, len, range
// y literales []T{…}); solo cambia su serialización: si está vacía, el
// contenedor no se escribe.
//
// Existe porque encoding/xml, con una etiqueta "pagos>pago,omitempty",
// escribe <pagos></pagos> para un slice vacío, y los XSD del SRI exigen al
// menos un ítem dentro de cada contenedor presente.
type Lista[T Etiquetado] []T

// MarshalXML implementa xml.Marshaler.
func (l Lista[T]) MarshalXML(e *xml.Encoder, inicio xml.StartElement) error {
	if len(l) == 0 {
		return nil
	}
	if err := e.EncodeToken(inicio); err != nil {
		return err
	}
	for _, item := range l {
		if err := e.EncodeElement(item, xml.StartElement{Name: xml.Name{Local: item.EtiquetaXML()}}); err != nil {
			return err
		}
	}
	return e.EncodeToken(inicio.End())
}

// UnmarshalXML implementa xml.Unmarshaler.
func (l *Lista[T]) UnmarshalXML(d *xml.Decoder, inicio xml.StartElement) error {
	*l = nil
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			var item T
			if err := d.DecodeElement(&item, &t); err != nil {
				return err
			}
			*l = append(*l, item)
		case xml.EndElement:
			return nil
		}
	}
}

// EtiquetaXML implementa Etiquetado.
func (Impuesto) EtiquetaXML() string { return "impuesto" }

// EtiquetaXML implementa Etiquetado.
func (TotalImpuesto) EtiquetaXML() string { return "totalImpuesto" }

// EtiquetaXML implementa Etiquetado.
func (Pago) EtiquetaXML() string { return "pago" }

// EtiquetaXML implementa Etiquetado.
func (CampoAdicional) EtiquetaXML() string { return "campoAdicional" }

// EtiquetaXML implementa Etiquetado.
func (DetAdicional) EtiquetaXML() string { return "detAdicional" }
