package comprobante

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Severidad distingue los hallazgos que impiden emitir de los que no.
type Severidad int

const (
	// Error es un incumplimiento por el que el SRI devolvería o no
	// autorizaría el comprobante.
	Error Severidad = iota
	// Advertencia es una observación que no impide la autorización (el SRI
	// la devolvería, si acaso, como advertencia) pero que conviene revisar.
	Advertencia
)

func (s Severidad) String() string {
	if s == Advertencia {
		return "advertencia"
	}
	return "error"
}

// Hallazgo es el resultado de una regla de validación.
type Hallazgo struct {
	// Campo es la ruta del dato, por ejemplo "infoFactura.importeTotal" o
	// "detalles[2].impuestos[0].valor".
	Campo string
	// Regla es un identificador estable de la regla, apto para pruebas y
	// para traducir mensajes: "ruc.formato", "totales.importe", …
	Regla string
	// Mensaje explica el problema y, cuando se puede, el valor esperado.
	Mensaje   string
	Severidad Severidad
}

func (h Hallazgo) String() string {
	return fmt.Sprintf("%s [%s] %s: %s", h.Severidad, h.Regla, h.Campo, h.Mensaje)
}

// Hallazgos es el resultado de validar un comprobante. Implementa error;
// use Err para obtener nil cuando no hay errores.
type Hallazgos []Hallazgo

// Error implementa la interfaz error con un resumen de los errores.
func (hs Hallazgos) Error() string {
	errs := hs.Errores()
	if len(errs) == 0 {
		return "comprobante: sin errores de validación"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "comprobante: %d error(es) de validación", len(errs))
	for i, h := range errs {
		if i == 8 {
			fmt.Fprintf(&sb, "; … y %d más", len(errs)-i)
			break
		}
		fmt.Fprintf(&sb, "; %s: %s", h.Campo, h.Mensaje)
	}
	return sb.String()
}

// Errores devuelve solo los hallazgos de severidad Error.
func (hs Hallazgos) Errores() Hallazgos { return hs.filtrar(Error) }

// Advertencias devuelve solo los hallazgos de severidad Advertencia.
func (hs Hallazgos) Advertencias() Hallazgos { return hs.filtrar(Advertencia) }

func (hs Hallazgos) filtrar(s Severidad) Hallazgos {
	var out Hallazgos
	for _, h := range hs {
		if h.Severidad == s {
			out = append(out, h)
		}
	}
	return out
}

// Err devuelve hs como error si contiene al menos un Error, o nil.
func (hs Hallazgos) Err() error {
	if len(hs.Errores()) == 0 {
		return nil
	}
	return hs
}

// Tiene informa si algún hallazgo corresponde a la regla dada.
func (hs Hallazgos) Tiene(regla string) bool {
	for _, h := range hs {
		if h.Regla == regla {
			return true
		}
	}
	return false
}

// Validador acumula hallazgos; lo usan los paquetes de cada comprobante.
type Validador struct{ hs Hallazgos }

// Errorf registra un error.
func (v *Validador) Errorf(campo, regla, formato string, args ...any) {
	v.hs = append(v.hs, Hallazgo{Campo: campo, Regla: regla, Mensaje: fmt.Sprintf(formato, args...), Severidad: Error})
}

// Advertirf registra una advertencia.
func (v *Validador) Advertirf(campo, regla, formato string, args ...any) {
	v.hs = append(v.hs, Hallazgo{Campo: campo, Regla: regla, Mensaje: fmt.Sprintf(formato, args...), Severidad: Advertencia})
}

// Hallazgos devuelve lo acumulado.
func (v *Validador) Hallazgos() Hallazgos { return v.hs }

// Texto valida un campo alfanumérico: obligatoriedad, longitud máxima en
// caracteres y las dos causas de rechazo de esquema que la ficha técnica
// destaca en el Anexo 1: saltos de línea y espacios sobrantes.
func (v *Validador) Texto(campo, valor string, obligatorio bool, max int) {
	if valor == "" {
		if obligatorio {
			v.Errorf(campo, "requerido", "es obligatorio")
		}
		return
	}
	if n := utf8.RuneCountInString(valor); n > max {
		v.Errorf(campo, "longitud", "tiene %d caracteres; el máximo es %d", n, max)
	}
	if strings.ContainsAny(valor, "\n\r") {
		v.Errorf(campo, "salto-de-linea", "no puede contener saltos de línea")
	}
	if strings.TrimSpace(valor) != valor {
		v.Errorf(campo, "espacios", "no puede empezar ni terminar con espacios")
	}
}

// Digitos valida un campo numérico de longitud exacta.
func (v *Validador) Digitos(campo, valor string, largo int) bool {
	if len(valor) != largo || !SoloDigitos(valor) {
		v.Errorf(campo, "formato", "debe tener exactamente %d dígitos, se recibió %q", largo, valor)
		return false
	}
	return true
}

// SoloDigitos informa si s no está vacío y solo contiene dígitos ASCII.
func SoloDigitos(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// InfoTributaria valida el bloque común. codDoc es el código de tipo de
// comprobante que debe declarar.
func (v *Validador) InfoTributaria(i InfoTributaria, codDoc string) {
	const p = "infoTributaria."
	if i.Ambiente != Pruebas && i.Ambiente != Produccion {
		v.Errorf(p+"ambiente", "catalogo", "debe ser 1 (pruebas) o 2 (producción), se recibió %q", string(i.Ambiente))
	}
	if i.TipoEmision != EmisionNormal {
		v.Errorf(p+"tipoEmision", "catalogo", "el esquema offline solo admite 1 (emisión normal), se recibió %q", i.TipoEmision)
	}
	v.Texto(p+"razonSocial", i.RazonSocial, true, 300)
	v.Texto(p+"nombreComercial", i.NombreComercial, false, 300)
	v.Texto(p+"dirMatriz", i.DirMatriz, true, 300)
	if err := ValidarRUC(i.RUC); err != nil {
		v.Errorf(p+"ruc", "ruc.formato", "%v", err)
	}
	if i.CodDoc != codDoc {
		v.Errorf(p+"codDoc", "catalogo", "debe ser %s, se recibió %q", codDoc, i.CodDoc)
	}
	v.Digitos(p+"estab", i.Estab, 3)
	v.Digitos(p+"ptoEmi", i.PtoEmi, 3)
	v.Digitos(p+"secuencial", i.Secuencial, 9)
	if i.AgenteRetencion != "" && (!SoloDigitos(i.AgenteRetencion) || len(i.AgenteRetencion) > 8) {
		v.Errorf(p+"agenteRetencion", "formato", "debe ser el número de resolución, hasta 8 dígitos (Anexo 21)")
	}
	switch i.ContribuyenteRimpe {
	case "", RimpeEmprendedor, RimpeNegocioPopular:
	default:
		v.Errorf(p+"contribuyenteRimpe", "catalogo", "debe ser %q o %q (Anexo 22)", RimpeEmprendedor, RimpeNegocioPopular)
	}
}

// Adicionales valida los bloques opcionales <infoAdicional> (hasta 15
// campos, sección 9.11; RUC del proveedor del sistema, Anexo 26) y
// <maquinaFiscal> (Anexo 13), comunes a todos los comprobantes.
func (v *Validador) Adicionales(campos []CampoAdicional, m *MaquinaFiscal) {
	if len(campos) > 15 {
		v.Errorf("infoAdicional", "maximo", "se admiten hasta 15 campos adicionales, hay %d (sección 9.11)", len(campos))
	}
	for i, c := range campos {
		v.Texto(fmt.Sprintf("infoAdicional[%d].nombre", i), c.Nombre, true, 300)
		v.Texto(fmt.Sprintf("infoAdicional[%d].valor", i), c.Valor, true, 300)
		if c.Nombre == NombreRUCProveedor {
			if err := ValidarRUC(c.Valor); err != nil {
				v.Errorf(fmt.Sprintf("infoAdicional[%d].valor", i), "ruc.formato", "el campo %q debe contener el RUC del proveedor (Anexo 26): %v", c.Nombre, err)
			}
		}
	}
	if m != nil {
		v.Texto("maquinaFiscal.marca", m.Marca, true, 30)
		v.Texto("maquinaFiscal.modelo", m.Modelo, true, 30)
		v.Texto("maquinaFiscal.serie", m.Serie, true, 30)
	}
}

// DetallesAdicionales valida los <detAdicional> de un detalle (máximo 3).
// p es la ruta del detalle terminada en punto.
func (v *Validador) DetallesAdicionales(p string, das []DetAdicional) {
	if len(das) > 3 {
		v.Errorf(p+"detallesAdicionales", "maximo", "se admiten hasta 3 detalles adicionales, hay %d", len(das))
	}
	for j, da := range das {
		v.Texto(fmt.Sprintf("%sdetallesAdicionales[%d].nombre", p, j), da.Nombre, true, 300)
		v.Texto(fmt.Sprintf("%sdetallesAdicionales[%d].valor", p, j), da.Valor, true, 300)
	}
}

// Emisor valida los datos opcionales del emisor que se repiten en el bloque
// de información de cada comprobante. p es la ruta del bloque terminada en
// punto.
func (v *Validador) Emisor(p, dirEstablecimiento, contribuyenteEspecial, obligadoContabilidad string) {
	v.Texto(p+"dirEstablecimiento", dirEstablecimiento, false, 300)
	if ce := contribuyenteEspecial; ce != "" {
		ok := len(ce) >= 3 && len(ce) <= 13
		for i := 0; ok && i < len(ce); i++ {
			c := ce[i]
			ok = c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		}
		if !ok {
			v.Errorf(p+"contribuyenteEspecial", "formato", "debe tener de 3 a 13 caracteres alfanuméricos")
		}
	}
	switch obligadoContabilidad {
	case "", "SI", "NO":
	default:
		v.Errorf(p+"obligadoContabilidad", "catalogo", `debe ser "SI" o "NO"`)
	}
}
