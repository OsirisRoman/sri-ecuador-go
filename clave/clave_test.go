package clave

import (
	"errors"
	"testing"
	"time"
)

// Ejemplo de la ficha técnica v2.34, pág. 12: cadena 41261533 → dígito 6.
func TestDigitoVerificadorEjemploFicha(t *testing.T) {
	dv, err := DigitoVerificador("41261533")
	if err != nil || dv != 6 {
		t.Fatalf("DigitoVerificador = %d, %v; la ficha indica 6", dv, err)
	}
}

// Claves de acceso que aparecen impresas en la ficha técnica v2.34
// (Anexo 14, sección 7.2.3 y sección 8.2.3). Todas deben verificar.
var clavesFicha = []string{
	"0503201201176001321000110010030009900641234567814", // Anexo 14
	"2111202405176001321000110010010000001241234567810", // 8.2.3
	"2110201101179214673900110020010000000011234567813", // Anexo 1
}

func TestValidarClavesDeLaFicha(t *testing.T) {
	for _, c := range clavesFicha {
		if err := Validar(c); err != nil {
			t.Errorf("Validar(%s): %v", c, err)
		}
	}
}

func TestGenerarReproduceAnexo14(t *testing.T) {
	got, err := Generar(Componentes{
		FechaEmision:    time.Date(2012, 3, 5, 0, 0, 0, 0, time.UTC),
		TipoComprobante: "01",
		RUC:             "1760013210001",
		Ambiente:        "1",
		Establecimiento: "001",
		PuntoEmision:    "003",
		Secuencial:      "000990064",
		CodigoNumerico:  "12345678",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != clavesFicha[0] {
		t.Errorf("Generar = %s\n   quiero %s", got, clavesFicha[0])
	}
	c, err := Analizar(got)
	if err != nil {
		t.Fatal(err)
	}
	if c.RUC != "1760013210001" || c.Secuencial != "000990064" || c.TipoEmision != "1" ||
		c.FechaEmision.Format("2006-01-02") != "2012-03-05" || c.TipoComprobante != "01" {
		t.Errorf("Analizar = %+v", c)
	}
}

func TestDigitoVerificadorCasosLimite(t *testing.T) {
	// Se buscan cadenas cuyo resultado intermedio sea 11 (→0) y 10 (→1).
	vistos := map[int]bool{}
	for i := 0; i < 200; i++ {
		s := "17600132100011001001000000" + string(rune('0'+i/100)) + string(rune('0'+i/10%10)) + string(rune('0'+i%10))
		dv, err := DigitoVerificador(s)
		if err != nil {
			t.Fatal(err)
		}
		if dv < 0 || dv > 9 {
			t.Fatalf("dígito fuera de rango: %d", dv)
		}
		vistos[dv] = true
	}
	for d := 0; d <= 9; d++ {
		if !vistos[d] {
			t.Errorf("el dígito %d nunca se produjo", d)
		}
	}
}

func TestErrores(t *testing.T) {
	if err := Validar("123"); !errors.Is(err, ErrLongitud) {
		t.Errorf("longitud: %v", err)
	}
	mala := clavesFicha[0][:48] + "5"
	if err := Validar(mala); !errors.Is(err, ErrDigitoVerificador) {
		t.Errorf("dígito: %v", err)
	}
	if err := Validar("A" + clavesFicha[0][1:]); !errors.Is(err, ErrNoNumerica) {
		t.Errorf("no numérica: %v", err)
	}
	base := Componentes{
		FechaEmision: time.Now(), TipoComprobante: "01", RUC: "1760013210001", Ambiente: "1",
		Establecimiento: "001", PuntoEmision: "001", Secuencial: "000000001", CodigoNumerico: "12345678",
	}
	for nombre, mut := range map[string]func(*Componentes){
		"secuencial corto": func(c *Componentes) { c.Secuencial = "1" },
		"ruc con letras":   func(c *Componentes) { c.RUC = "17600132100A1" },
		"ambiente 3":       func(c *Componentes) { c.Ambiente = "3" },
		"sin fecha":        func(c *Componentes) { c.FechaEmision = time.Time{} },
		"sin código":       func(c *Componentes) { c.CodigoNumerico = "" },
	} {
		c := base
		mut(&c)
		if _, err := Generar(c); !errors.Is(err, ErrCampo) {
			t.Errorf("%s: esperaba ErrCampo, obtuve %v", nombre, err)
		}
	}
}

func TestCodigoAleatorio(t *testing.T) {
	a, err := CodigoAleatorio()
	if err != nil || len(a) != 8 || !soloDigitos(a) {
		t.Fatalf("CodigoAleatorio = %q, %v", a, err)
	}
}
