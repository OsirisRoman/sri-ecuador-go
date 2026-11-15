// Package certprueba genera en memoria certificados de firma desechables
// para las pruebas. Nada de lo que produce es un secreto ni se guarda en el
// repositorio: cada ejecución crea claves nuevas.
package certprueba

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"sync"
	"time"

	"github.com/OsirisRoman/sri-ecuador-go/certificado"
)

// Material es una autoridad de prueba y un certificado de firma emitido por
// ella.
type Material struct {
	CA          *x509.Certificate
	Certificado *certificado.Certificado
	Clave       *rsa.PrivateKey
}

var (
	una      sync.Once
	material *Material
	fallo    error
)

// Obtener devuelve un material compartido por todo el proceso de pruebas
// (generar RSA-2048 es costoso). El certificado es válido de 2020 a 2040.
func Obtener() (*Material, error) {
	una.Do(func() {
		material, fallo = Generar(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC))
	})
	return material, fallo
}

// Generar crea una CA y un certificado de entidad final con la vigencia
// indicada. El emisor imita los nombres de las CA ecuatorianas, incluido un
// atributo organizationIdentifier (2.5.4.97) sin palabra clave RFC 2253.
func Generar(desde, hasta time.Time) (*Material, error) {
	claveCA, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	nombreCA := pkix.Name{
		Country: []string{"EC"}, Organization: []string{"ENTIDAD DE PRUEBA S.A., GOSRI"},
		OrganizationalUnit: []string{"ENTIDAD DE CERTIFICACION DE INFORMACION"},
		CommonName:         "AC DE PRUEBA GOSRI",
		ExtraNames: []pkix.AttributeTypeAndValue{
			{Type: asn1.ObjectIdentifier{2, 5, 4, 97}, Value: "VATEC-1790000000001"},
		},
	}
	plantillaCA := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: nombreCA,
		NotBefore: desde, NotAfter: hasta, IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	derCA, err := x509.CreateCertificate(rand.Reader, plantillaCA, plantillaCA, &claveCA.PublicKey, claveCA)
	if err != nil {
		return nil, err
	}
	ca, err := x509.ParseCertificate(derCA)
	if err != nil {
		return nil, err
	}
	clave, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	plantilla := &x509.Certificate{
		SerialNumber: big.NewInt(1312833444),
		Subject: pkix.Name{Country: []string{"EC"}, Organization: []string{"EMISOR DE PRUEBA"},
			CommonName: "FIRMANTE DE PRUEBA GOSRI", SerialNumber: "0000000000"},
		NotBefore: desde, NotAfter: hasta,
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
	}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, ca, &clave.PublicKey, claveCA)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	c, err := certificado.DesdeClave(cert, clave, ca)
	if err != nil {
		return nil, err
	}
	return &Material{CA: ca, Certificado: c, Clave: clave}, nil
}
