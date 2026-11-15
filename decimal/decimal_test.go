package decimal

import (
	"encoding/xml"
	"errors"
	"testing"
)

func TestParseYString(t *testing.T) {
	casos := map[string]string{
		"0": "0", "12": "12", "12.50": "12.50", "-0.5": "-0.5", "+3.10": "3.10",
		"0.000001": "0.000001", ".5": "0.5", "5.": "5", "-0.00": "0.00",
		"347159.40": "347159.40",
	}
	for in, want := range casos {
		d, err := Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if got := d.String(); got != want {
			t.Errorf("Parse(%q).String() = %q, quiero %q", in, got, want)
		}
	}
}

func TestParseInvalido(t *testing.T) {
	for _, in := range []string{"", ".", "-", "1,5", "1e5", "1.2.3", " 1", "abc", "--1", "1_000", "٣"} {
		if _, err := Parse(in); !errors.Is(err, ErrFormato) {
			t.Errorf("Parse(%q): esperaba ErrFormato, obtuve %v", in, err)
		}
	}
}

func TestAritmeticaExacta(t *testing.T) {
	a, b := MustParse("0.1"), MustParse("0.2")
	if got := a.Sumar(b); !got.Igual(MustParse("0.3")) {
		t.Errorf("0.1+0.2 = %s", got)
	}
	if got := MustParse("10.00").Restar(MustParse("0.005")).String(); got != "9.995" {
		t.Errorf("resta = %s", got)
	}
	if got := MustParse("1.5").Multiplicar(MustParse("2.25")).String(); got != "3.375" {
		t.Errorf("producto = %s", got)
	}
	if got := MustParse("100.00").Porcentaje(Entero(15)).Fijo(2); got != "15.00" {
		t.Errorf("15%% de 100 = %s", got)
	}
	var cero Decimal
	if got := cero.Sumar(MustParse("1.10")).String(); got != "1.10" {
		t.Errorf("valor cero + 1.10 = %s", got)
	}
}

func TestRedondear(t *testing.T) {
	casos := []struct {
		in   string
		n    int
		want string
	}{
		{"2.345", 2, "2.35"}, {"2.344", 2, "2.34"}, {"-2.345", 2, "-2.35"},
		{"-2.344", 2, "-2.34"}, {"2.5", 0, "3"}, {"-2.5", 0, "-3"},
		{"1", 2, "1.00"}, {"0.005", 2, "0.01"}, {"0.004999", 2, "0.00"},
		{"37169.4", 2, "37169.40"}, {"1.005", 2, "1.01"}, {"999.995", 2, "1000.00"},
		{"-0.001", 2, "0.00"},
	}
	for _, c := range casos {
		if got := MustParse(c.in).Redondear(c.n).String(); got != c.want {
			t.Errorf("Redondear(%s, %d) = %s, quiero %s", c.in, c.n, got, c.want)
		}
	}
}

func TestCmpYDigitos(t *testing.T) {
	if MustParse("1.0").Cmp(MustParse("1.00")) != 0 {
		t.Error("1.0 debe ser igual a 1.00")
	}
	if MustParse("-1").Cmp(MustParse("0.5")) != -1 || MustParse("2").Cmp(MustParse("1.99")) != 1 {
		t.Error("orden incorrecto")
	}
	e, f := MustParse("123456789012.34").Digitos()
	if e != 12 || f != 2 {
		t.Errorf("Digitos = %d,%d", e, f)
	}
	if e, f := MustParse("0.05").Digitos(); e != 0 || f != 2 {
		t.Errorf("Digitos(0.05) = %d,%d", e, f)
	}
}

func TestXML(t *testing.T) {
	type doc struct {
		XMLName xml.Name `xml:"d"`
		V       Decimal  `xml:"v"`
		O       *Decimal `xml:"o,omitempty"`
		A       Decimal  `xml:"a,attr"`
	}
	in := doc{V: MustParse("12.50"), A: MustParse("3")}
	b, err := xml.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `<d a="3"><v>12.50</v></d>` {
		t.Errorf("xml = %s", b)
	}
	var out doc
	if err := xml.Unmarshal([]byte(`<d a="7.1"><v> 0.10 </v><o>5</o></d>`), &out); err != nil {
		t.Fatal(err)
	}
	if out.V.String() != "0.10" || out.O == nil || out.O.String() != "5" || out.A.String() != "7.1" {
		t.Errorf("unmarshal = %+v", out)
	}
	if err := xml.Unmarshal([]byte(`<d a="1"><v>x</v></d>`), &out); err == nil {
		t.Error("esperaba error con decimal inválido")
	}
}
