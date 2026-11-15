package pdf

import (
	"bytes"
	"image"
	"image/color"
	"strconv"
	"strings"
	"testing"
)

func TestPatronesCode128(t *testing.T) {
	vistos := map[string]bool{}
	for i, p := range patrones128 {
		suma := 0
		for _, c := range p {
			suma += int(c - '0')
		}
		quiero, largo := 11, 6
		if i == c128Parada {
			quiero, largo = 13, 7
		}
		if suma != quiero || len(p) != largo {
			t.Errorf("patrón %d (%s): %d módulos, %d elementos", i, p, suma, len(p))
		}
		if vistos[p] {
			t.Errorf("patrón %d (%s) repetido", i, p)
		}
		vistos[p] = true
	}
}

func TestSimbolosCode128(t *testing.T) {
	v, err := simbolos128("PJJ123C")
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 10 || v[0] != c128InicioB || v[1] != 'P'-32 || v[len(v)-1] != c128Parada {
		t.Errorf("símbolos = %v", v)
	}
	// 49 dígitos: 24 pares en C, cambio a B y un dígito.
	v, _ = simbolos128(strings.Repeat("1", 49))
	if len(v) != 1+24+2+2 || v[0] != c128InicioC || v[25] != c128CambioB || v[26] != '1'-32 {
		t.Errorf("símbolos = %v", v)
	}
	if _, err := simbolos128("ñ"); err == nil {
		t.Error("esperaba error")
	}
}

func TestTextoYPartir(t *testing.T) {
	if a := AnchoTexto("Añ", Normal, 10); a != 6.67+5.56 {
		t.Errorf("ancho = %v", a)
	}
	if string(codificar("ñ€→")) != "\xF1\x80?" {
		t.Errorf("codificar = %q", codificar("ñ€→"))
	}
	ls := Partir("uno dos tres cuatro cinco", Normal, 10, 50)
	for _, l := range ls {
		if AnchoTexto(l, Normal, 10) > 50 {
			t.Errorf("línea %q demasiado ancha", l)
		}
	}
	if strings.Join(ls, " ") != "uno dos tres cuatro cinco" {
		t.Errorf("líneas = %q", ls)
	}
	ls = Partir("0123456789012345678901234567890123456789", Normal, 10, 60)
	if len(ls) < 3 || strings.Join(ls, "") != "0123456789012345678901234567890123456789" {
		t.Errorf("palabra larga = %q", ls)
	}
}

func TestDocumento(t *testing.T) {
	d := Nuevo()
	d.Titulo = "Prueba ñ"
	d.Texto(40, 60, "Hola (mundo) \\ ñ", Negrita, 12)
	d.Rect(40, 80, 100, 20, 0.5)
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	d.Imagen(img, 40, 120, 80, 80)
	if err := d.Code128("12345", 40, 220, 200, 30); err != nil {
		t.Fatal(err)
	}
	d.NuevaPagina()
	d.Texto(40, 60, "Página 2", Normal, 10)
	b := d.Bytes()
	if !bytes.HasPrefix(b, []byte("%PDF-1.4")) || !bytes.HasSuffix(b, []byte("%%EOF\n")) || d.Paginas() != 2 {
		t.Error("estructura inesperada")
	}
	// La tabla de referencias debe apuntar al comienzo de cada objeto.
	i := bytes.LastIndex(b, []byte("startxref\n"))
	xref, err := strconv.Atoi(strings.TrimSuffix(string(b[i+10:]), "\n%%EOF\n"))
	if err != nil || !bytes.HasPrefix(b[xref:], []byte("xref\n")) {
		t.Fatalf("startxref no apunta a xref: %v", err)
	}
	lineas := strings.Split(string(b[xref:i]), "\n")
	for n, l := range lineas[3:] {
		if len(l) < 18 || !strings.HasSuffix(l, " n ") {
			continue
		}
		pos, _ := strconv.Atoi(l[:10])
		if quiero := []byte(strconv.Itoa(n+1) + " 0 obj"); !bytes.HasPrefix(b[pos:], quiero) {
			t.Errorf("objeto %d: la referencia no apunta a %q", n+1, quiero)
		}
	}
}
