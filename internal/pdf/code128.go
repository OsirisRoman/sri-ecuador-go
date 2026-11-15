package pdf

import "fmt"

// patrones128 son los anchos de barras y espacios (en módulos) de los 107
// símbolos de Code 128 (ISO/IEC 15417): valores 0 a 102, inicios A, B y C
// (103 a 105) y parada (106).
var patrones128 = [107]string{
	"212222", "222122", "222221", "121223", "121322", "131222", "122213", "122312", "132212", "221213",
	"221312", "231212", "112232", "122132", "122231", "113222", "123122", "123221", "223211", "221132",
	"221231", "213212", "223112", "312131", "311222", "321122", "321221", "312212", "322112", "322211",
	"212123", "212321", "232121", "111323", "131123", "131321", "112313", "132113", "132311", "211313",
	"231113", "231311", "112133", "112331", "132131", "113123", "113321", "133121", "313121", "211331",
	"231131", "213113", "213311", "213131", "311123", "311321", "331121", "312113", "312311", "332111",
	"314111", "221411", "431111", "111224", "111422", "121124", "121421", "141122", "141221", "112214",
	"112412", "122114", "122411", "142112", "142211", "241211", "221114", "413111", "241112", "134111",
	"111242", "121142", "121241", "114212", "124112", "124211", "411212", "421112", "421211", "212141",
	"214121", "412121", "111143", "111341", "131141", "114113", "114311", "411113", "411311", "113141",
	"114131", "311141", "411131", "211412", "211214", "211232", "2331112",
}

const (
	c128CambioB = 100
	c128InicioB = 104
	c128InicioC = 105
	c128Parada  = 106
)

// simbolos128 codifica s: los textos numéricos en el juego C (dos dígitos
// por símbolo; si la longitud es impar, el último dígito va en el juego B)
// y el resto en el juego B (ASCII imprimible).
func simbolos128(s string) ([]int, error) {
	if s == "" {
		return nil, fmt.Errorf("pdf: código de barras vacío")
	}
	numerico := true
	for i := 0; i < len(s); i++ {
		if s[i] < 32 || s[i] > 126 {
			return nil, fmt.Errorf("pdf: carácter %q no representable en Code 128 B", s[i])
		}
		if s[i] < '0' || s[i] > '9' {
			numerico = false
		}
	}
	var v []int
	if numerico && len(s) >= 2 {
		v = append(v, c128InicioC)
		i := 0
		for ; i+1 < len(s); i += 2 {
			v = append(v, int(s[i]-'0')*10+int(s[i+1]-'0'))
		}
		if i < len(s) {
			v = append(v, c128CambioB, int(s[i])-32)
		}
	} else {
		v = append(v, c128InicioB)
		for i := 0; i < len(s); i++ {
			v = append(v, int(s[i])-32)
		}
	}
	suma := v[0]
	for i := 1; i < len(v); i++ {
		suma += i * v[i]
	}
	return append(v, suma%103, c128Parada), nil
}

// Code128 dibuja s como código de barras Code 128 ocupando el ancho dado
// (las zonas de silencio de 10 módulos a cada lado quedan fuera y debe
// respetarlas quien llama).
func (d *Documento) Code128(s string, x, y, ancho, alto float64) error {
	v, err := simbolos128(s)
	if err != nil {
		return err
	}
	modulos := 0
	for _, sim := range v {
		for _, c := range patrones128[sim] {
			modulos += int(c - '0')
		}
	}
	m := ancho / float64(modulos)
	for _, sim := range v {
		for i, c := range patrones128[sim] {
			w := float64(c-'0') * m
			if i%2 == 0 { // las posiciones pares son barras
				d.RectRelleno(x, y, w, alto, 0)
			}
			x += w
		}
	}
	return nil
}
