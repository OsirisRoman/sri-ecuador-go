// Package clave genera, analiza y valida la clave de acceso de 49 dígitos
// que identifica de forma única a cada comprobante electrónico del SRI y que,
// en el esquema offline, es además su número de autorización.
//
// Referencia normativa: Ficha técnica de comprobantes electrónicos esquema
// offline v2.34, sección 5.2 (Tabla 1) y ejemplo de módulo 11 (pág. 11-12).
//
// Composición (Tabla 1):
//
//	pos  long  campo
//	 0     8   fecha de emisión (ddmmaaaa)
//	 8     2   tipo de comprobante (Tabla 3)
//	10    13   RUC del emisor
//	23     1   tipo de ambiente (Tabla 4)
//	24     6   serie: establecimiento (3) + punto de emisión (3)
//	30     9   número secuencial
//	39     8   código numérico (libre para el emisor)
//	47     1   tipo de emisión (Tabla 2)
//	48     1   dígito verificador (módulo 11)
package clave

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"
)

// Longitud es el número de dígitos de una clave de acceso.
const Longitud = 49

// formatoFecha es ddmmaaaa en la notación de referencia de Go.
const formatoFecha = "02012006"

var (
	// ErrLongitud indica que la clave no tiene 49 caracteres.
	ErrLongitud = errors.New("clave de acceso: debe tener 49 dígitos")
	// ErrNoNumerica indica que la clave contiene caracteres que no son dígitos.
	ErrNoNumerica = errors.New("clave de acceso: solo admite dígitos")
	// ErrDigitoVerificador indica que el dígito verificador no corresponde.
	ErrDigitoVerificador = errors.New("clave de acceso: dígito verificador incorrecto")
	// ErrCampo indica que un componente de la clave es inválido.
	ErrCampo = errors.New("clave de acceso: componente inválido")
)

// Componentes son los campos que conforman una clave de acceso.
type Componentes struct {
	// FechaEmision del comprobante; solo se usan día, mes y año.
	FechaEmision time.Time
	// TipoComprobante es el código de la Tabla 3 ("01" factura, "04" nota
	// de crédito, ...).
	TipoComprobante string
	// RUC del emisor (13 dígitos).
	RUC string
	// Ambiente es "1" (pruebas) o "2" (producción), Tabla 4.
	Ambiente string
	// Establecimiento (3 dígitos) y PuntoEmision (3 dígitos) forman la serie.
	Establecimiento string
	PuntoEmision    string
	// Secuencial del comprobante (9 dígitos, con ceros a la izquierda).
	Secuencial string
	// CodigoNumerico son 8 dígitos a discreción del emisor. Ver
	// CodigoAleatorio.
	CodigoNumerico string
	// TipoEmision es "1" (emisión normal, la única del esquema offline),
	// Tabla 2. Si está vacío se asume "1".
	TipoEmision string
}

// Generar compone la clave de acceso de 49 dígitos, incluido el dígito
// verificador. Todos los campos se validan en longitud y contenido: la ficha
// técnica advierte que una clave mal conformada es motivo de rechazo.
func Generar(c Componentes) (string, error) {
	if c.TipoEmision == "" {
		c.TipoEmision = "1"
	}
	if c.FechaEmision.IsZero() {
		return "", fmt.Errorf("%w: fecha de emisión vacía", ErrCampo)
	}
	campos := []struct {
		nombre, valor string
		largo         int
	}{
		{"tipo de comprobante", c.TipoComprobante, 2},
		{"RUC", c.RUC, 13},
		{"ambiente", c.Ambiente, 1},
		{"establecimiento", c.Establecimiento, 3},
		{"punto de emisión", c.PuntoEmision, 3},
		{"secuencial", c.Secuencial, 9},
		{"código numérico", c.CodigoNumerico, 8},
		{"tipo de emisión", c.TipoEmision, 1},
	}
	for _, f := range campos {
		if len(f.valor) != f.largo || !soloDigitos(f.valor) {
			return "", fmt.Errorf("%w: %s debe tener %d dígitos, se recibió %q", ErrCampo, f.nombre, f.largo, f.valor)
		}
	}
	if c.Ambiente != "1" && c.Ambiente != "2" {
		return "", fmt.Errorf("%w: ambiente debe ser 1 o 2", ErrCampo)
	}
	base := c.FechaEmision.Format(formatoFecha) + c.TipoComprobante + c.RUC + c.Ambiente +
		c.Establecimiento + c.PuntoEmision + c.Secuencial + c.CodigoNumerico + c.TipoEmision
	dv, err := DigitoVerificador(base)
	if err != nil {
		return "", err
	}
	return base + string(rune('0'+dv)), nil
}

// DigitoVerificador calcula el dígito de control "módulo 11" de una cadena
// numérica: los dígitos se ponderan de derecha a izquierda con los factores
// 2,3,4,5,6,7 de forma cíclica; el dígito es 11 − (suma mod 11), con las
// sustituciones 11 → 0 y 10 → 1 que fija la ficha técnica.
func DigitoVerificador(digitos string) (int, error) {
	if digitos == "" {
		return 0, fmt.Errorf("%w: cadena vacía", ErrCampo)
	}
	suma, factor := 0, 2
	for i := len(digitos) - 1; i >= 0; i-- {
		ch := digitos[i]
		if ch < '0' || ch > '9' {
			return 0, ErrNoNumerica
		}
		suma += int(ch-'0') * factor
		if factor++; factor > 7 {
			factor = 2
		}
	}
	switch dv := 11 - suma%11; dv {
	case 11:
		return 0, nil
	case 10:
		return 1, nil
	default:
		return dv, nil
	}
}

// Validar comprueba que una clave tenga 49 dígitos, un dígito verificador
// correcto y componentes plausibles (fecha existente, ambiente 1 o 2).
func Validar(clave string) error {
	_, err := Analizar(clave)
	return err
}

// Analizar descompone una clave de acceso en sus campos tras validarla.
func Analizar(clave string) (Componentes, error) {
	if len(clave) != Longitud {
		return Componentes{}, fmt.Errorf("%w (tiene %d)", ErrLongitud, len(clave))
	}
	if !soloDigitos(clave) {
		return Componentes{}, ErrNoNumerica
	}
	dv, _ := DigitoVerificador(clave[:48])
	if int(clave[48]-'0') != dv {
		return Componentes{}, fmt.Errorf("%w: se esperaba %d", ErrDigitoVerificador, dv)
	}
	fecha, err := time.Parse(formatoFecha, clave[0:8])
	if err != nil {
		return Componentes{}, fmt.Errorf("%w: fecha %q no existe", ErrCampo, clave[0:8])
	}
	c := Componentes{
		FechaEmision:    fecha,
		TipoComprobante: clave[8:10],
		RUC:             clave[10:23],
		Ambiente:        clave[23:24],
		Establecimiento: clave[24:27],
		PuntoEmision:    clave[27:30],
		Secuencial:      clave[30:39],
		CodigoNumerico:  clave[39:47],
		TipoEmision:     clave[47:48],
	}
	if c.Ambiente != "1" && c.Ambiente != "2" {
		return Componentes{}, fmt.Errorf("%w: ambiente %q", ErrCampo, c.Ambiente)
	}
	return c, nil
}

// CodigoAleatorio devuelve un código numérico de 8 dígitos generado con una
// fuente criptográficamente segura. El código es potestad del emisor
// (sección 5.2); usar uno impredecible dificulta que terceros adivinen las
// claves de acceso de sus comprobantes.
//
// La clave resultante debe guardarse junto al comprobante: si este es
// devuelto o no autorizado, la ficha técnica (5.10) exige reenviarlo con la
// misma clave de acceso.
func CodigoAleatorio() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(100_000_000))
	if err != nil {
		return "", fmt.Errorf("clave de acceso: sin entropía: %w", err)
	}
	return fmt.Sprintf("%08d", n.Int64()), nil
}

func soloDigitos(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
