package certificado_test

import (
	"errors"
	"testing"
	"time"

	"github.com/OsirisRoman/sri-ecuador-go/certificado"
	"github.com/OsirisRoman/sri-ecuador-go/internal/certprueba"
)

// AUD-007: un Certificado sin clave (construido a mano) no provoca pánicos:
// se informa con errores tipados.
func TestCertificadoSinClave(t *testing.T) {
	m, err := certprueba.Obtener()
	if err != nil {
		t.Fatal(err)
	}
	for nombre, c := range map[string]*certificado.Certificado{
		"nil":       nil,
		"vacío":     {},
		"solo X509": {X509: m.Certificado.X509},
	} {
		if c.Firmante() != nil {
			t.Errorf("%s: Firmante debe ser una interfaz nil", nombre)
		}
		if c.ClavePublica() != nil {
			t.Errorf("%s: ClavePublica debe ser nil", nombre)
		}
		if err := c.Utilizable(); !errors.Is(err, certificado.ErrSinParDeFirma) {
			t.Errorf("%s: Utilizable = %v", nombre, err)
		}
		if err := c.Vigente(time.Now()); !errors.Is(err, certificado.ErrSinParDeFirma) {
			t.Errorf("%s: Vigente = %v", nombre, err)
		}
		_ = c.String()
		_ = c.Resumen()
	}
	if err := m.Certificado.Utilizable(); err != nil {
		t.Errorf("un certificado cargado debe ser utilizable: %v", err)
	}
}
