package sritest

import (
	"github.com/OsirisRoman/sri-ecuador-go/certificado"
	"github.com/OsirisRoman/sri-ecuador-go/internal/certprueba"
)

// CertificadoDePrueba devuelve un certificado de firma desechable, generado
// en memoria y válido de 2020 a 2040, con el que firmar comprobantes en
// pruebas automáticas contra el Servidor simulado. Se genera una vez por
// proceso.
//
// No sirve ante el SRI real: lo emite una autoridad ficticia. Para el
// ambiente de pruebas del SRI hace falta un certificado de una entidad de
// certificación autorizada, cargado con certificado.CargarArchivo.
func CertificadoDePrueba() (*certificado.Certificado, error) {
	m, err := certprueba.Obtener()
	if err != nil {
		return nil, err
	}
	return m.Certificado, nil
}
