package sri_test

import (
	"errors"
	"testing"

	sri "github.com/OsirisRoman/sri-ecuador-go"
	"github.com/OsirisRoman/sri-ecuador-go/certificado"
	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/internal/certprueba"
	"github.com/OsirisRoman/sri-ecuador-go/internal/ejemplo"
	"github.com/OsirisRoman/sri-ecuador-go/xades"
)

// AUD-007: firmar con un certificado sin clave devuelve errores, no pánico.
func TestCertificadoSinClaveNoProvocaPanico(t *testing.T) {
	m, err := certprueba.Obtener()
	if err != nil {
		t.Fatal(err)
	}
	soloX509 := &certificado.Certificado{X509: m.Certificado.X509}
	doc, _ := ejemplo.Factura(comprobante.Hoy(), "000000301").XML()
	if _, err := xades.Firmar(doc, soloX509, nil); !errors.Is(err, xades.ErrCertificado) {
		t.Errorf("xades.Firmar: %v", err)
	}
	if _, err := sri.Nuevo(sri.Config{Ambiente: comprobante.Pruebas, Certificado: soloX509}); !errors.Is(err, certificado.ErrSinParDeFirma) {
		t.Errorf("sri.Nuevo: %v", err)
	}
}

// Una espera con varias consultas y sin intervalo no consulta en ráfaga.
func TestEsperaSinIntervalo(t *testing.T) {
	m, err := certprueba.Obtener()
	if err != nil {
		t.Fatal(err)
	}
	c, err := sri.Nuevo(sri.Config{Ambiente: comprobante.Pruebas, Certificado: m.Certificado, Espera: sri.Espera{Consultas: 100}})
	if err != nil || c == nil {
		t.Fatal(err)
	}
	if got := sri.IntervaloDe(c); got != sri.EsperaPorDefecto.Intervalo {
		t.Errorf("intervalo = %v", got)
	}
}
