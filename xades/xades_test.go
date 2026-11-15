package xades

import (
	"bytes"
	"crypto/x509"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/OsirisRoman/sri-ecuador-go/certificado"
	"github.com/OsirisRoman/sri-ecuador-go/internal/certprueba"
)

const facturaMinima = `<?xml version="1.0" encoding="UTF-8"?>
<factura id="comprobante" version="1.1.0">
  <infoTributaria>
    <razonSocial>Ñandú &amp; Cía. "Ltda" &lt;Quito&gt;</razonSocial>
    <ruc>1790000000001</ruc>
  </infoTributaria>
  <infoAdicional>
    <campoAdicional nombre="Dirección">Av. 10 de Agosto</campoAdicional>
    <vacio/>
  </infoAdicional>
</factura>`

var momento = time.Date(2026, 11, 14, 10, 30, 0, 0, time.UTC)

func mat(t *testing.T) *certprueba.Material {
	t.Helper()
	m, err := certprueba.Obtener()
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func firmar(t *testing.T, alg Algoritmo) []byte {
	t.Helper()
	out, err := Firmar([]byte(facturaMinima), mat(t).Certificado, &Opciones{Algoritmo: alg, Momento: momento})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestFirmarYVerificar(t *testing.T) {
	for _, alg := range []Algoritmo{SHA1, SHA256} {
		t.Run(alg.String(), func(t *testing.T) {
			firmado := firmar(t, alg)
			raices := x509.NewCertPool()
			raices.AddCert(mat(t).CA)
			f, err := Verificar(firmado, &OpcionesVerificacion{Raices: raices})
			if err != nil {
				t.Fatal(err)
			}
			if f.Certificado.Subject.CommonName != "FIRMANTE DE PRUEBA GOSRI" {
				t.Errorf("firmante = %q", f.Certificado.Subject.CommonName)
			}
			if !f.FechaFirma.Equal(momento) {
				t.Errorf("SigningTime = %v", f.FechaFirma)
			}
			if len(f.Referencias) != 3 {
				t.Errorf("referencias = %d, el perfil del SRI usa 3", len(f.Referencias))
			}
			wantAlg := AlgRSASHA1
			if alg == SHA256 {
				wantAlg = AlgRSASHA256
			}
			if f.AlgoritmoFirma != wantAlg {
				t.Errorf("algoritmo = %s", f.AlgoritmoFirma)
			}
		})
	}
}

// La estructura debe corresponder al Anexo 14 de la ficha técnica v2.34.
func TestEstructuraAnexo14(t *testing.T) {
	s := string(firmar(t, SHA1))
	if !strings.HasPrefix(s, `<?xml version="1.0" encoding="UTF-8"?>`) {
		t.Error("falta la declaración XML UTF-8")
	}
	enOrden := []string{
		`<ds:Signature xmlns:ds="http://www.w3.org/2000/09/xmldsig#" xmlns:etsi="http://uri.etsi.org/01903/v1.3.2#" Id="Signature`,
		`<ds:SignedInfo Id="Signature-SignedInfo`,
		`<ds:CanonicalizationMethod Algorithm="http://www.w3.org/TR/2001/REC-xml-c14n-20010315">`,
		`<ds:SignatureMethod Algorithm="http://www.w3.org/2000/09/xmldsig#rsa-sha1">`,
		`Type="http://uri.etsi.org/01903#SignedProperties" URI="#Signature`,
		`<ds:Reference URI="#Certificate`,
		`URI="#comprobante"><ds:Transforms><ds:Transform Algorithm="http://www.w3.org/2000/09/xmldsig#enveloped-signature">`,
		`<ds:SignatureValue Id="SignatureValue`,
		`<ds:KeyInfo Id="Certificate`,
		`<ds:X509Data><ds:X509Certificate>`,
		`<ds:KeyValue><ds:RSAKeyValue><ds:Modulus>`,
		`<ds:Exponent>AQAB</ds:Exponent>`,
		`<etsi:QualifyingProperties Target="#Signature`,
		`<etsi:SignedProperties Id="Signature`,
		`<etsi:SigningTime>2026-11-14T05:30:00-05:00</etsi:SigningTime>`,
		`<etsi:SigningCertificate><etsi:Cert><etsi:CertDigest>`,
		`<ds:X509IssuerName>2.5.4.97=#`,
		`<ds:X509SerialNumber>1312833444</ds:X509SerialNumber>`,
		`<etsi:DataObjectFormat ObjectReference="#Reference-ID-`,
		`<etsi:Description>contenido comprobante</etsi:Description><etsi:MimeType>text/xml</etsi:MimeType>`,
		`</ds:Signature></factura>`,
	}
	pos := 0
	for _, frag := range enOrden {
		i := strings.Index(s[pos:], frag)
		if i < 0 {
			t.Fatalf("no se encontró (en orden) %q", frag)
		}
		pos += i
	}
	// El contenido original se conserva byte a byte.
	if !strings.Contains(s, `<razonSocial>Ñandú &amp; Cía. "Ltda" &lt;Quito&gt;</razonSocial>`) {
		t.Error("el contenido del comprobante fue alterado")
	}
}

func TestAlteraciones(t *testing.T) {
	firmado := string(firmar(t, SHA1))
	casos := []struct {
		nombre string
		mutar  func(string) string
		quiero error
	}{
		{"contenido", func(s string) string { return strings.Replace(s, "1790000000001", "1790000000002", 1) }, ErrDigest},
		{"atributo", func(s string) string { return strings.Replace(s, `version="1.1.0"`, `version="2.1.0"`, 1) }, ErrDigest},
		{"elemento añadido", func(s string) string {
			return strings.Replace(s, "<infoAdicional>", "<propina>1</propina><infoAdicional>", 1)
		}, ErrDigest},
		{"signing time", func(s string) string { return strings.Replace(s, "T05:30:00", "T05:31:00", 1) }, ErrDigest},
		{"valor de firma", func(s string) string {
			i := strings.Index(s, "<ds:SignatureValue")
			j := i + strings.Index(s[i:], ">") + 1
			c := byte('A')
			if s[j] == 'A' {
				c = 'B'
			}
			return s[:j] + string(c) + s[j+1:]
		}, ErrFirmaInvalida},
		{"algoritmo degradado", func(s string) string {
			return strings.Replace(s, AlgRSASHA1, "http://www.w3.org/2001/04/xmldsig-more#rsa-md5", 1)
		}, ErrAlgoritmo},
		{"id duplicado (envoltura)", func(s string) string {
			return strings.Replace(s, "<infoAdicional>", `<infoAdicional id="comprobante">`, 1)
		}, ErrReferencia},
		{"sin firma", func(s string) string {
			i, j := strings.Index(s, "<ds:Signature"), strings.Index(s, "</ds:Signature>")
			return s[:i] + s[j+len("</ds:Signature>"):]
		}, ErrSinFirma},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			mutado := c.mutar(firmado)
			if mutado == firmado {
				t.Fatal("la mutación no cambió nada")
			}
			if _, err := Verificar([]byte(mutado), nil); !errors.Is(err, c.quiero) {
				t.Errorf("esperaba %v, obtuve %v", c.quiero, err)
			}
		})
	}
}

// Sustituir el certificado por otro (aunque sea de la misma CA) debe fallar:
// lo impiden SigningCertificate y la referencia a KeyInfo (ficha 6.5).
func TestSustitucionDeCertificado(t *testing.T) {
	otro, err := certprueba.Generar(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	a := string(firmar(t, SHA1))
	bFirmado, err := Firmar([]byte(facturaMinima), otro.Certificado, &Opciones{Momento: momento})
	if err != nil {
		t.Fatal(err)
	}
	extraer := func(s string) string {
		i := strings.Index(s, "<ds:X509Certificate>")
		j := strings.Index(s, "</ds:X509Certificate>")
		return s[i:j]
	}
	mutado := strings.Replace(a, extraer(a), extraer(string(bFirmado)), 1)
	if _, err := Verificar([]byte(mutado), nil); err == nil {
		t.Fatal("se aceptó una firma con el certificado sustituido")
	}
}

func TestErroresDeFirma(t *testing.T) {
	m := mat(t)
	firmado := firmar(t, SHA1)
	if _, err := Firmar(firmado, m.Certificado, &Opciones{Momento: momento}); !errors.Is(err, ErrYaFirmado) {
		t.Errorf("doble firma: %v", err)
	}
	if _, err := Firmar([]byte(facturaMinima), nil, nil); !errors.Is(err, ErrCertificado) {
		t.Errorf("sin certificado: %v", err)
	}
	fuera := time.Date(2050, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := Firmar([]byte(facturaMinima), m.Certificado, &Opciones{Momento: fuera}); !errors.Is(err, certificado.ErrVencido) {
		t.Errorf("vencido: %v", err)
	}
	if _, err := Firmar([]byte(`<!DOCTYPE a [<!ENTITY x "y">]><a>&x;</a>`), m.Certificado, &Opciones{Momento: momento}); err == nil {
		t.Error("se aceptó un documento con DOCTYPE")
	}
}

func TestRaizDesconocidaYVigencia(t *testing.T) {
	firmado := firmar(t, SHA1)
	if _, err := Verificar(firmado, &OpcionesVerificacion{Raices: x509.NewCertPool()}); !errors.Is(err, ErrCertificadoFirma) {
		t.Errorf("raíz desconocida: %v", err)
	}
	fuera := time.Date(2050, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := Verificar(firmado, &OpcionesVerificacion{Momento: fuera}); !errors.Is(err, ErrCertificadoFirma) {
		t.Errorf("fuera de vigencia: %v", err)
	}
}

// Añade id="comprobante" si el documento no lo trae.
func TestAgregaIDRaiz(t *testing.T) {
	out, err := Firmar([]byte(`<factura version="1.0.0"><a>1</a></factura>`), mat(t).Certificado, &Opciones{Momento: momento})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte(`<factura id="comprobante" version="1.0.0">`)) {
		t.Errorf("raíz = %s", out[:80])
	}
}

func TestNombreRFC2253(t *testing.T) {
	got, err := NombreRFC2253(mat(t).CA.RawSubject)
	if err != nil {
		t.Fatal(err)
	}
	// Orden inverso al DER, coma escapada, OID desconocido en hexadecimal.
	if !strings.HasPrefix(got, "2.5.4.97=#0c13564154") && !strings.HasPrefix(got, "2.5.4.97=#1313564154") {
		t.Errorf("OID sin palabra clave mal codificado: %s", got)
	}
	if !strings.HasSuffix(got, `CN=AC DE PRUEBA GOSRI,OU=ENTIDAD DE CERTIFICACION DE INFORMACION,O=ENTIDAD DE PRUEBA S.A.\, GOSRI,C=EC`) {
		t.Errorf("nombre = %s", got)
	}
}
