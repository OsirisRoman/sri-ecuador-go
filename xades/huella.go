package xades

import (
	"crypto/sha256"
	"fmt"

	"github.com/OsirisRoman/sri-ecuador-go/internal/xmldom"
)

// Huella devuelve el SHA-256 de la forma canónica (C14N 1.0) del
// comprobante sin su firma: exactamente el contenido que cubre la
// referencia #comprobante, con la transformación enveloped-signature.
//
// Sirve para saber si dos documentos son el mismo comprobante aunque
// difieran en la serialización (declaración XML, comillas, CDATA,
// espacios fuera del elemento raíz) o en la firma (otra firma del mismo
// contenido). Cualquier cambio de contenido cambia la huella. No verifica
// la firma: para eso está Verificar.
func Huella(doc []byte) ([sha256.Size]byte, error) {
	d, err := xmldom.Analizar(doc)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	var firma *xmldom.Elemento
	for _, h := range d.Raiz.Elementos() {
		if h.Es(NSDS, "Signature") {
			if firma != nil {
				return [sha256.Size]byte{}, fmt.Errorf("%w", ErrVariasFirmas)
			}
			firma = h
		}
	}
	return sha256.Sum256(xmldom.Canonicalizar(d.Raiz, firma)), nil
}
