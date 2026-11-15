# Procedencia de los esquemas XSD

Los archivos de este directorio se incrustan en el SDK sin modificación.

| Origen | Archivos |
|---|---|
| Servicio de Rentas Internas, <https://www.sri.gob.ec/facturacion-electronica>, sección "Esquemas XSD y XML" (descargados el 2026-11-14) | todos excepto `xmldsig-core-schema.xsd` |
| W3C, <https://www.w3.org/TR/xmldsig-core/xmldsig-core-schema.xsd> (W3C Software License) | `xmldsig-core-schema.xsd` |

`NOTA_CREDITO-*.xsd` y `NOTA_DEBITO-*.xsd` provienen del paquete "Esquemas XSD
de notas de crédito y débito con el campo Devolución IVA".

Las correcciones que el SDK aplica en memoria (nunca a estos archivos) están
en `esquema.Ajustes`.

## SHA-256

```
79547cecfad14ffe7c3fa5ef7a5ca9d2b9b245fc82873fac8f8b27fcec8ad162  ComprobanteRetencion_V1.0.0.xsd
1e006d6d16c791c8f5b23d1f3e006cd066ccba2bfb797a3cb9098bd09c793cb7  ComprobanteRetencion_V2.0.0.xsd
09532c4c1b459633fc92b8fdb37ebc5da1d0dc66a284825a6e73077fb946663c  GuiaRemision_V1.0.0.xsd
4777fd94b5f7b108da3db0dd3d2c71d1539ced221cba246ebeaa40fb17f3dee1  GuiaRemision_V1.1.0.xsd
88794b18cf556cc127fd69c95df6d5f7a2da02e55b07f4c9680011a003c514e7  LiquidacionCompra_V1.0.0.xsd
58c9cd96edf93d92ef6692cd028e689080257c9ba2941fe04fe1d426ebc4b5b6  LiquidacionCompra_V1.1.0.xsd
12ec9edcf4d2434053cedaad8eab1c27d0aa0cb9e2d7045f0ae0794adaf1cda6  NOTA_CREDITO-1.0.0.xsd
4b4c7099ba2950b043a1e690b47db3f1d4452a035407efa28cf42538590595f1  NOTA_CREDITO-1.1.0.xsd
f9a596a86a4078f18688b16279d19d6fb2548f84b28b889cb958da47b2d7a4a1  NOTA_DEBITO-1.0.0.xsd
8a4c3ec4e0b0dace9316d9cb5825b14f4e60b1d8c8189a0ac994dce2863d9e07  NotaCredito_V1.0.0.xsd
379e9f270d71ab4578f130a736c9abe5eb1d7969375ecdef55d2973583ec5ab0  NotaCredito_V1.1.0.xsd
f40323adef667a2d95de75a611272a78b73a7f82711201bd7a7a52d3bb353fd6  NotaDebito_V1.0.0.xsd
0c2d62f15c9ed03f5e5d3559dc556dd254396396247f3ea919e63473b4299442  factura_V1.0.0.xsd
62db9bf0ecceb00ef2b7ed136e59224815e5e5e33c77efc6a0552001e052eb8b  factura_V1.1.0.xsd
cf1bbd68f43d594f51f9983e8ca2d756e7dfc745ae63ec8ea6301c22d05a1479  factura_V2.0.0.xsd
5f2c37bc1a58bb40e8bbbc366cabe05d5dc199598aeea1561137370f8bd4eace  factura_V2.1.0.xsd
d102ad3df7664c307e0c2c776ba4a90513b1969974d8a940bae1a77f9f21e15d  xmldsig-core-schema.xsd
```
