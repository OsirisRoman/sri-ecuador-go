package factura_test

import (
	"fmt"

	"github.com/OsirisRoman/sri-ecuador-go/comprobante"
	"github.com/OsirisRoman/sri-ecuador-go/decimal"
	"github.com/OsirisRoman/sri-ecuador-go/factura"
)

// Un arriendo pactado en 350,00 con el IVA incluido y un recargo de 12,00,
// también final. LineaPrecioFinal recibe lo que paga el comprador; Calcular
// deduce la base y el IVA, y el total es exactamente la suma de los precios.
func ExampleLineaPrecioFinal() {
	f := &factura.Factura{
		Detalles: []factura.Detalle{
			factura.LineaPrecioFinal("ARR-01", "Arriendo de noviembre", decimal.Entero(1), decimal.MustParse("350.00"), comprobante.IVA15),
			factura.LineaPrecioFinal("REC-01", "Recargo por mora", decimal.Entero(1), decimal.MustParse("12.00"), comprobante.IVA15),
		},
	}
	f.Calcular()
	for _, d := range f.Detalles {
		iva := d.Impuestos[0]
		fmt.Printf("%s: base %s + IVA %s = %s\n", d.Descripcion, iva.BaseImponible, iva.Valor, iva.BaseImponible.Sumar(iva.Valor))
	}
	fmt.Println("importe total:", f.InfoFactura.ImporteTotal)

	// El mismo recargo definido por su precio sin impuestos no llega a 12,00.
	g := &factura.Factura{Detalles: []factura.Detalle{
		factura.LineaIVA("REC-01", "Recargo por mora", decimal.Entero(1), decimal.MustParse("10.43"), comprobante.IVA15),
	}}
	g.Calcular()
	fmt.Println("con precio sin impuestos de 10.43:", g.InfoFactura.ImporteTotal)
	// Output:
	// Arriendo de noviembre: base 304.35 + IVA 45.65 = 350.00
	// Recargo por mora: base 10.43 + IVA 1.57 = 12.00
	// importe total: 362.00
	// con precio sin impuestos de 10.43: 11.99
}
