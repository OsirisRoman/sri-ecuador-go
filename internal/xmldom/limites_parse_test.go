package xmldom_test

import (
	"testing"

	"github.com/OsirisRoman/sri-ecuador-go/factura"
)

// AUD-008: Parse rechaza lo que el verificador de firma rechaza; antes
// encoding/xml aceptaba un atributo repetido y tomaba el último valor.
func TestParseRechazaXMLMalFormado(t *testing.T) {
	for _, doc := range []string{
		`<factura id="comprobante" version="1.1.0" version="2.1.0"><infoTributaria/></factura>`,
		`<!DOCTYPE factura [<!ENTITY x "y">]><factura id="comprobante">&x;</factura>`,
	} {
		if _, err := factura.Parse([]byte(doc)); err == nil {
			t.Errorf("se aceptó %q", doc)
		}
	}
}
