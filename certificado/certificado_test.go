package certificado_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"software.sslmate.com/src/go-pkcs12"

	"github.com/OsirisRoman/sri-ecuador-go/certificado"
	"github.com/OsirisRoman/sri-ecuador-go/internal/certprueba"
)

func TestCargarPKCS12(t *testing.T) {
	m, err := certprueba.Obtener()
	if err != nil {
		t.Fatal(err)
	}
	// Se prueban los cifrados que se encuentran en la práctica: el moderno
	// (AES/PBES2) y los heredados (3DES, RC2) de los .p12 más antiguos.
	for nombre, enc := range map[string]*pkcs12.Encoder{"moderno": pkcs12.Modern, "3DES": pkcs12.LegacyDES, "RC2": pkcs12.LegacyRC2} {
		t.Run(nombre, func(t *testing.T) {
			p12, err := enc.Encode(m.Clave, m.Certificado.X509, m.Certificado.Cadena, "clave-de-prueba")
			if err != nil {
				t.Fatal(err)
			}
			c, err := certificado.Cargar(p12, "clave-de-prueba")
			if err != nil {
				t.Fatal(err)
			}
			if c.X509.Subject.CommonName != "FIRMANTE DE PRUEBA GOSRI" || len(c.Cadena) != 1 {
				t.Errorf("certificado = %v, cadena = %d", c, len(c.Cadena))
			}
			if err := c.Vigente(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
				t.Error(err)
			}
			if _, err := certificado.Cargar(p12, "incorrecta"); !errors.Is(err, certificado.ErrContrasena) {
				t.Errorf("contraseña incorrecta: %v", err)
			}
		})
	}
	if _, err := certificado.Cargar([]byte("no es un p12"), "x"); !errors.Is(err, certificado.ErrFormato) {
		t.Errorf("formato: %v", err)
	}
}

func TestVigenciaYNoExponeLaClave(t *testing.T) {
	m, _ := certprueba.Obtener()
	c := m.Certificado
	if err := c.Vigente(time.Date(2041, 1, 1, 0, 0, 0, 0, time.UTC)); !errors.Is(err, certificado.ErrVencido) {
		t.Errorf("vencido: %v", err)
	}
	if err := c.Vigente(time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)); !errors.Is(err, certificado.ErrAunNoVigente) {
		t.Errorf("no vigente: %v", err)
	}
	for _, formato := range []string{"%v", "%+v", "%#v", "%s"} {
		s := fmt.Sprintf(formato, c)
		if strings.Contains(s, m.Clave.D.String()) || strings.Contains(s, m.Clave.Primes[0].String()) || len(s) > 300 {
			t.Errorf("fmt %s expone material privado o es excesivo: %.80s", formato, s)
		}
	}
	otro, _ := certprueba.Generar(time.Now(), time.Now().Add(time.Hour))
	if _, err := certificado.DesdeClave(c.X509, otro.Clave); !errors.Is(err, certificado.ErrSinParDeFirma) {
		t.Errorf("par que no corresponde: %v", err)
	}
}
