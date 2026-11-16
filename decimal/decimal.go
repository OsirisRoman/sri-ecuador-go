// Package decimal implementa aritmética decimal exacta de punto fijo para los
// importes, cantidades y tarifas de los comprobantes electrónicos del SRI.
//
// Los importes tributarios no deben representarse con float64: 0.1+0.2 no es
// 0.3 en binario y el SRI rechaza comprobantes con diferencias de cálculo
// (error 52). Un Decimal es un entero de precisión arbitraria acompañado de
// una escala (número de dígitos decimales), de modo que toda suma, resta y
// multiplicación es exacta y el redondeo solo ocurre cuando se pide.
//
// El valor cero de Decimal es el número 0 y está listo para usarse. Los
// valores son inmutables: toda operación devuelve un Decimal nuevo.
package decimal

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// MaxEscala es el mayor número de dígitos decimales admitido al analizar
// texto. Los esquemas del SRI usan como máximo 6; el límite evita que una
// entrada hostil reserve memoria sin control.
const MaxEscala = 30

// maxDigitos acota la longitud total de un literal aceptado por Parse.
const maxDigitos = 60

// ErrFormato indica que un texto no es un número decimal válido.
var ErrFormato = errors.New("decimal: formato inválido")

// Decimal es un número decimal exacto: coef × 10^(-escala).
type Decimal struct {
	coef   *big.Int // nil equivale a 0
	escala int
}

var (
	diez = big.NewInt(10)
	// Cero es el decimal 0.
	Cero = Decimal{}
)

// Nuevo construye el decimal coef × 10^(-escala). Nuevo(1250, 2) es 12.50.
func Nuevo(coef int64, escala int) Decimal {
	if escala < 0 {
		return Decimal{coef: new(big.Int).Mul(big.NewInt(coef), pot10(-escala))}
	}
	return Decimal{coef: big.NewInt(coef), escala: escala}
}

// Entero construye un decimal sin parte fraccionaria.
func Entero(n int64) Decimal { return Nuevo(n, 0) }

// Parse analiza un literal decimal como "12", "-0.5" o "100.00". Solo se
// admite la notación del tipo xsd:decimal: dígitos con un punto opcional y
// signo opcional; no se aceptan exponentes, separadores de miles ni comas.
// La escala del resultado conserva los decimales escritos ("1.50" tiene
// escala 2).
func Parse(s string) (Decimal, error) {
	orig := s
	if len(s) == 0 || len(s) > maxDigitos {
		return Decimal{}, fmt.Errorf("%w: %q", ErrFormato, orig)
	}
	neg := false
	switch s[0] {
	case '-':
		neg = true
		s = s[1:]
	case '+':
		s = s[1:]
	}
	entero, frac, hayPunto := strings.Cut(s, ".")
	if entero == "" && frac == "" {
		return Decimal{}, fmt.Errorf("%w: %q", ErrFormato, orig)
	}
	if hayPunto && frac == "" && entero == "" {
		return Decimal{}, fmt.Errorf("%w: %q", ErrFormato, orig)
	}
	if len(frac) > MaxEscala {
		return Decimal{}, fmt.Errorf("%w: %q supera %d decimales", ErrFormato, orig, MaxEscala)
	}
	digitos := entero + frac
	for i := 0; i < len(digitos); i++ {
		if digitos[i] < '0' || digitos[i] > '9' {
			return Decimal{}, fmt.Errorf("%w: %q", ErrFormato, orig)
		}
	}
	coef, ok := new(big.Int).SetString(digitos, 10)
	if !ok {
		return Decimal{}, fmt.Errorf("%w: %q", ErrFormato, orig)
	}
	if neg {
		coef.Neg(coef)
	}
	return Decimal{coef: coef, escala: len(frac)}, nil
}

// MustParse es como Parse pero entra en pánico si el literal es inválido.
// Está pensado para constantes y pruebas, no para datos externos.
func MustParse(s string) Decimal {
	d, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return d
}

func pot10(n int) *big.Int {
	return new(big.Int).Exp(diez, big.NewInt(int64(n)), nil)
}

func (d Decimal) c() *big.Int {
	if d.coef == nil {
		return new(big.Int)
	}
	return d.coef
}

// alinear devuelve los coeficientes de a y b expresados en la mayor escala.
func alinear(a, b Decimal) (*big.Int, *big.Int, int) {
	switch {
	case a.escala == b.escala:
		return a.c(), b.c(), a.escala
	case a.escala > b.escala:
		return a.c(), new(big.Int).Mul(b.c(), pot10(a.escala-b.escala)), a.escala
	default:
		return new(big.Int).Mul(a.c(), pot10(b.escala-a.escala)), b.c(), b.escala
	}
}

// Escala devuelve el número de dígitos decimales que almacena d.
func (d Decimal) Escala() int { return d.escala }

// Sumar devuelve d + o.
func (d Decimal) Sumar(o Decimal) Decimal {
	x, y, e := alinear(d, o)
	return Decimal{coef: new(big.Int).Add(x, y), escala: e}
}

// Restar devuelve d − o.
func (d Decimal) Restar(o Decimal) Decimal {
	x, y, e := alinear(d, o)
	return Decimal{coef: new(big.Int).Sub(x, y), escala: e}
}

// Multiplicar devuelve d × o de forma exacta (las escalas se suman).
func (d Decimal) Multiplicar(o Decimal) Decimal {
	return Decimal{coef: new(big.Int).Mul(d.c(), o.c()), escala: d.escala + o.escala}
}

// Porcentaje devuelve d × tasa / 100 de forma exacta. Para el IVA del 15 %,
// base.Porcentaje(decimal.Entero(15)).
func (d Decimal) Porcentaje(tasa Decimal) Decimal {
	r := d.Multiplicar(tasa)
	r.escala += 2
	return r
}

// ErrDivisionPorCero indica una división por cero en Cociente.
var ErrDivisionPorCero = errors.New("decimal: división por cero")

// Cociente devuelve d / o redondeado a n decimales, mitad hacia arriba en
// magnitud como Redondear. La división se hace con enteros exactos: el
// resultado es el decimal de n cifras más cercano al cociente verdadero.
func (d Decimal) Cociente(o Decimal, n int) (Decimal, error) {
	if o.EsCero() {
		return Decimal{}, ErrDivisionPorCero
	}
	if n < 0 {
		n = 0
	}
	// d / o = (a × 10^eo) / (b × 10^ed); se busca ese valor × 10^n.
	num := new(big.Int).Mul(d.c(), pot10(o.escala+n))
	den := new(big.Int).Mul(o.c(), pot10(d.escala))
	negativo := num.Sign()*den.Sign() < 0
	num.Abs(num)
	den.Abs(den)
	q, r := new(big.Int).QuoRem(num, den, new(big.Int))
	if r.Lsh(r, 1).Cmp(den) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if negativo {
		q.Neg(q)
	}
	return Decimal{coef: q, escala: n}, nil
}

// Negar devuelve −d.
func (d Decimal) Negar() Decimal {
	return Decimal{coef: new(big.Int).Neg(d.c()), escala: d.escala}
}

// Redondear ajusta d a n decimales con redondeo aritmético "mitad hacia
// arriba" en magnitud (2.345 → 2.35 y −2.345 → −2.35), que es el criterio
// comercial habitual en la facturación ecuatoriana. Si d tiene menos de n
// decimales se completa con ceros, por lo que el resultado siempre tiene
// escala n.
func (d Decimal) Redondear(n int) Decimal {
	if n < 0 {
		n = 0
	}
	if d.escala <= n {
		return Decimal{coef: new(big.Int).Mul(d.c(), pot10(n-d.escala)), escala: n}
	}
	div := pot10(d.escala - n)
	q, r := new(big.Int).QuoRem(d.c(), div, new(big.Int))
	// r tiene el signo de d; se compara 2|r| con el divisor.
	r.Abs(r).Lsh(r, 1)
	if r.Cmp(div) >= 0 {
		if d.c().Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	return Decimal{coef: q, escala: n}
}

// Cmp compara d con o: −1 si d < o, 0 si son numéricamente iguales (1.0 y
// 1.00 lo son) y +1 si d > o.
func (d Decimal) Cmp(o Decimal) int {
	x, y, _ := alinear(d, o)
	return x.Cmp(y)
}

// Igual informa si d y o representan el mismo número.
func (d Decimal) Igual(o Decimal) bool { return d.Cmp(o) == 0 }

// Signo devuelve −1, 0 o +1 según el signo de d.
func (d Decimal) Signo() int { return d.c().Sign() }

// EsCero informa si d es 0.
func (d Decimal) EsCero() bool { return d.c().Sign() == 0 }

// Abs devuelve el valor absoluto de d.
func (d Decimal) Abs() Decimal {
	return Decimal{coef: new(big.Int).Abs(d.c()), escala: d.escala}
}

// Digitos devuelve el número de dígitos significativos enteros y
// fraccionarios que ocupa d tal como se escribe, que es lo que limitan las
// facetas totalDigits y fractionDigits de los XSD del SRI.
func (d Decimal) Digitos() (enteros, fraccion int) {
	s := new(big.Int).Abs(d.c()).String()
	if d.escala >= len(s) {
		return 0, d.escala
	}
	return len(s) - d.escala, d.escala
}

// String escribe d con exactamente Escala() decimales, sin exponente:
// es la forma que se serializa en el XML.
func (d Decimal) String() string {
	c := d.c()
	s := new(big.Int).Abs(c).String()
	if d.escala > 0 {
		if len(s) <= d.escala {
			s = strings.Repeat("0", d.escala-len(s)+1) + s
		}
		s = s[:len(s)-d.escala] + "." + s[len(s)-d.escala:]
	}
	if c.Sign() < 0 {
		return "-" + s
	}
	return s
}

// Fijo equivale a d.Redondear(n).String().
func (d Decimal) Fijo(n int) string { return d.Redondear(n).String() }

// MarshalText implementa encoding.TextMarshaler; encoding/xml y
// encoding/json lo usan para serializar el valor.
func (d Decimal) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// UnmarshalText implementa encoding.TextUnmarshaler. Se toleran espacios en
// blanco alrededor del número, como hace xsd:decimal.
func (d *Decimal) UnmarshalText(b []byte) error {
	v, err := Parse(strings.TrimSpace(string(b)))
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// Suma devuelve la suma de todos los valores.
func Suma(vs ...Decimal) Decimal {
	t := Cero
	for _, v := range vs {
		t = t.Sumar(v)
	}
	return t
}
