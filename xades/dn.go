package xades

import (
	"encoding/asn1"
	"encoding/hex"
	"errors"
	"strings"
	"unicode/utf16"
)

// Palabras clave de RFC 2253 sección 2.3. Son deliberadamente solo estas:
// es el conjunto que emite X500Principal.getName(RFC2253) de Java, la
// plataforma con la que el SRI valida las firmas. Cualquier otro tipo de
// atributo (por ejemplo serialNumber 2.5.4.5 u organizationIdentifier
// 2.5.4.97, frecuentes en certificados ecuatorianos) se escribe como OID
// con su valor DER en hexadecimal, que es la forma canónica de RFC 2253.
var clavesRFC2253 = map[string]string{
	"2.5.4.3":                    "CN",
	"2.5.4.7":                    "L",
	"2.5.4.8":                    "ST",
	"2.5.4.10":                   "O",
	"2.5.4.11":                   "OU",
	"2.5.4.6":                    "C",
	"2.5.4.9":                    "STREET",
	"0.9.2342.19200300.100.1.25": "DC",
	"0.9.2342.19200300.100.1.1":  "UID",
}

type avaCrudo struct {
	Tipo  asn1.ObjectIdentifier
	Valor asn1.RawValue
}

// NombreRFC2253 convierte un nombre distinguido X.501 codificado en DER
// (por ejemplo Certificate.RawIssuer) a su representación de texto RFC 2253,
// que es el contenido de ds:X509IssuerName.
//
// Se trabaja sobre el DER original, y no sobre pkix.Name, para no perder ni
// reordenar atributos.
func NombreRFC2253(der []byte) (string, error) {
	var rdns []asn1.RawValue
	resto, err := asn1.Unmarshal(der, &rdns)
	if err != nil {
		return "", err
	}
	if len(resto) != 0 {
		return "", errors.New("datos sobrantes tras el nombre")
	}
	partes := make([]string, 0, len(rdns))
	// RFC 2253: se empieza por el último RDN de la secuencia.
	for i := len(rdns) - 1; i >= 0; i-- {
		var avas []avaCrudo
		if _, err := asn1.UnmarshalWithParams(rdns[i].FullBytes, &avas, "set"); err != nil {
			return "", err
		}
		var sb strings.Builder
		for j, a := range avas {
			if j > 0 {
				sb.WriteByte('+')
			}
			oid := a.Tipo.String()
			clave, conocida := clavesRFC2253[oid]
			texto, esTexto := textoDirectorio(a.Valor)
			if conocida && esTexto {
				sb.WriteString(clave + "=" + escaparRFC2253(texto))
			} else {
				sb.WriteString(oid + "=#" + hex.EncodeToString(a.Valor.FullBytes))
			}
		}
		partes = append(partes, sb.String())
	}
	return strings.Join(partes, ","), nil
}

// textoDirectorio decodifica los tipos de cadena de DirectoryString.
func textoDirectorio(v asn1.RawValue) (string, bool) {
	if v.Class != asn1.ClassUniversal {
		return "", false
	}
	switch v.Tag {
	case asn1.TagUTF8String, asn1.TagPrintableString, asn1.TagIA5String, asn1.TagT61String:
		return string(v.Bytes), true
	case asn1.TagBMPString:
		if len(v.Bytes)%2 != 0 {
			return "", false
		}
		u := make([]uint16, len(v.Bytes)/2)
		for i := range u {
			u[i] = uint16(v.Bytes[2*i])<<8 | uint16(v.Bytes[2*i+1])
		}
		return string(utf16.Decode(u)), true
	}
	return "", false
}

func escaparRFC2253(s string) string {
	var sb strings.Builder
	for i, r := range s {
		switch {
		case strings.ContainsRune(`,+"\<>;`, r),
			i == 0 && (r == ' ' || r == '#'),
			i == len(s)-1 && r == ' ':
			sb.WriteByte('\\')
		}
		sb.WriteRune(r)
	}
	return sb.String()
}
