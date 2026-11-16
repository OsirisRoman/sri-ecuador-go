package xmldom

import (
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Normalización de valores de atributo (XML 1.0, sección 3.3.3).
//
// Un procesador XML conforme sustituye por un espacio cada tabulador, salto
// de línea o retorno de carro que aparezca *literalmente* en el valor de un
// atributo, pero conserva los que se escriben como referencias de carácter
// (&#x9;, &#xA;, &#xD;). encoding/xml entrega el valor ya decodificado y no
// hace esa sustitución, así que no se puede distinguir un caso del otro sin
// mirar el texto original de la etiqueta. Sin esta corrección, un documento
// con un salto de línea literal en un atributo tiene para este paquete una
// forma canónica distinta de la que calcula cualquier otro verificador
// (libxml2, Java), y una firma válida para uno es inválida para el otro.

var errEtiqueta = errors.New("etiqueta ilegible")

// valoresCrudos devuelve, en orden, el texto sin decodificar de los valores
// de atributo de una etiqueta de apertura (<a x="1" y='2'> o <a/>).
func valoresCrudos(etiqueta []byte) ([]string, error) {
	s := string(etiqueta)
	if len(s) < 2 || s[0] != '<' {
		return nil, errEtiqueta
	}
	i := 1
	// Nombre del elemento.
	for i < len(s) && !esEspacio(s[i]) && s[i] != '>' && s[i] != '/' {
		i++
	}
	var vals []string
	for {
		for i < len(s) && esEspacio(s[i]) {
			i++
		}
		if i >= len(s) {
			return nil, errEtiqueta
		}
		if s[i] == '>' || s[i] == '/' {
			return vals, nil
		}
		// Nombre del atributo, espacios opcionales, '=', espacios opcionales.
		for i < len(s) && s[i] != '=' && !esEspacio(s[i]) {
			i++
		}
		for i < len(s) && esEspacio(s[i]) {
			i++
		}
		if i >= len(s) || s[i] != '=' {
			return nil, errEtiqueta
		}
		i++
		for i < len(s) && esEspacio(s[i]) {
			i++
		}
		if i >= len(s) || (s[i] != '"' && s[i] != '\'') {
			return nil, errEtiqueta
		}
		comilla := s[i]
		i++
		fin := strings.IndexByte(s[i:], comilla)
		if fin < 0 {
			return nil, errEtiqueta
		}
		vals = append(vals, s[i:i+fin])
		i += fin + 1
	}
}

func esEspacio(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

// normalizarValor aplica al texto crudo de un valor de atributo el
// tratamiento de fin de línea (sección 2.11) y la normalización de la
// sección 3.3.3, y decodifica las referencias. El documento ya fue aceptado
// por encoding/xml en modo estricto, así que solo aparecen las cinco
// entidades predefinidas y referencias de carácter.
func normalizarValor(crudo string) (string, error) {
	crudo = strings.ReplaceAll(crudo, "\r\n", "\n")
	var b strings.Builder
	for i := 0; i < len(crudo); {
		c := crudo[i]
		switch {
		case c == '\r' || c == '\n' || c == '\t':
			b.WriteByte(' ')
			i++
		case c == '&':
			fin := strings.IndexByte(crudo[i:], ';')
			if fin < 0 {
				return "", errEtiqueta
			}
			ref := crudo[i+1 : i+fin]
			i += fin + 1
			switch ref {
			case "lt":
				b.WriteByte('<')
			case "gt":
				b.WriteByte('>')
			case "amp":
				b.WriteByte('&')
			case "quot":
				b.WriteByte('"')
			case "apos":
				b.WriteByte('\'')
			default:
				if !strings.HasPrefix(ref, "#") {
					return "", errEtiqueta
				}
				base, num := 10, ref[1:]
				if strings.HasPrefix(num, "x") {
					base, num = 16, num[1:]
				}
				n, err := strconv.ParseUint(num, base, 32)
				if err != nil || !utf8.ValidRune(rune(n)) {
					return "", errEtiqueta
				}
				b.WriteRune(rune(n))
			}
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String(), nil
}
