# Ronda de certificación de la versión 0.2.0

Resultados de contrastar con el **ambiente de pruebas del SRI** (`celcer.sri.gob.ec`) el código consolidado de la versión 0.2.0 (commit `2e54f8f`), el 15 de noviembre de 2026. Nada se envió a producción. Un emisor persona natural y un certificado del Banco Central del Ecuador. Las claves de acceso, los secuenciales y los datos del emisor no se publican; los XML y las respuestas completas se conservan fuera del repositorio.

Cada prueba se predijo en local antes de enviarla. Las que envían un comprobante que el SDK rechaza (T1 y T5) usan un procedimiento aislado: se valida el XSD, se firma con `xades.Firmar` y se envía con `Cliente.Reanudar`, sin pasar por `Preparar` y sin modificar ninguna validación. El código de la ronda está en [`internal/qa/certificacion`](../internal/qa/certificacion/main.go) y en [`internal/qa`](../internal/qa/main.go); requiere un emisor habilitado y su certificado.

«Verificado» significa que la huella (`xades.Huella`) del comprobante que devolvió el SRI coincide con la del enviado.

## Ciclo completo

| Prueba | Esperado | Respuesta del SRI | Conclusión |
|---|---|---|---|
| Factura 1.1.0, 2.1.0, 1.0.0 y 2.0.0 | AUTORIZADO | RECIBIDA, AUTORIZADO, verificado | Confirmado |
| Nota de crédito 1.1.0 parcial con `DesdeFactura` (pasa las reglas `origen.*`) | AUTORIZADO | AUTORIZADO, verificado | Confirmado |
| Nota de crédito **1.0.0** asociada a su factura y a una nota anterior | AUTORIZADO | AUTORIZADO, verificado | Primera verificación de esta versión |
| Nota de débito 1.0.0; retención 2.0.0 y 1.0.0; guía 1.1.0 y 1.0.0; liquidación 1.1.0 y 1.0.0 | AUTORIZADO | AUTORIZADO, verificados | Confirmado |

## Reglas de IVA

| Prueba | Esperado en el SDK | Respuesta del SRI | Conclusión |
|---|---|---|---|
| T1: base del IVA 1,00 en una línea de 100,00, totales coherentes | Error `iva.base` | NO AUTORIZADO, 52: «La base imponible del impuesto 1.0 no coincide con la calculada 100.0 en FacturaDetalleImpuesto [codigo=2, codigoPorcentaje=4]» | El SRI aplica la misma regla |
| T2: detalle solo con IRBPNR, sin IVA | Advertencia `iva.ausente` | AUTORIZADO | Correcto que sea advertencia |
| T3: diez líneas de 0,10 al 15 %, IVA 0,02 por línea, total 0,20 (suma de líneas; el acumulado daría 0,15) | Aceptado (D5) | AUTORIZADO | Confirmado |
| T4: las mismas líneas con IVA 0,01 (desempate mitad al par), total 0,15 | Aceptado | AUTORIZADO | Confirmado |
| T5: IVA 0,16 sobre una base de 1,00 al 15 % (exacto 0,15) | Error `iva.valor` | AUTORIZADO | **El SDK es más estricto que el SRI por decisión** (D17) |

## Precio final con IVA incluido

Enviadas el 16 de noviembre de 2026 con el modo de precio final (decisión D18), por el flujo normal (`Preparar`).

| Prueba | Esperado en el SDK | Respuesta del SRI | Conclusión |
|---|---|---|---|
| T8: una línea de 12,00 de precio final al 15 %: base 10,43 e IVA 1,57 (la tarifa sobre la base daría 1,56) | Aceptado con el precio declarado; `iva.valor` si se lee del XML sin declararlo | RECIBIDA, AUTORIZADO, verificado | Confirmado |
| T9: diez líneas de 0,50 de precio final: 0,43 + 0,07 cada una; totales 4,30 + 0,70 = 5,00 (la tarifa sobre la base acumulada daría 0,65) | Aceptado; IVA total por suma de líneas | RECIBIDA, AUTORIZADO, verificado | Confirmado |

## Reenvío, estados y conservación

| Prueba | Esperado | Respuesta del SRI | Conclusión |
|---|---|---|---|
| Corrección de T1 con la misma clave y secuencial (ficha 5.10) | RECIBIDA; AUTORIZADO verificado | RECIBIDA (no 43); una sola autorización, la del corregido, desde la primera consulta | Confirmado |
| Reenvío idéntico del comprobante corregido ya autorizado | 43; AUTORIZADO verificado | 43; AUTORIZADO verificado | Sin falsa discrepancia |
| Firma alterada y, después, el original con la misma clave | NO AUTORIZADO (39); luego AUTORIZADO | NO AUTORIZADO (39); luego AUTORIZADO verificado con `Reanudar` | Confirmado |
| Consulta de un comprobante ya autorizado (`ConsultarDocumento`) | AUTORIZADO verificado | AUTORIZADO, `ambiente` `PRUEBAS`, `claveAccesoConsultada` igual a la consultada | Confirmado |
| Consulta de una clave que nunca se envió | Pendiente | `numeroComprobantes` 0, ninguna autorización | Igual que un comprobante en proceso: la guía indica reenviar, no consultar, cuando el envío falló |
| Conservación (todas las pruebas) | Clave, XML firmado y autorizado, RIDE | Huella coincidente en todas las respuestas; RIDE generado de cada autorizado | Confirmado |

## No observado en el SRI

- Estados intermedios (`EN PROCESO`, `PPR`, código 70) y el lapso en que la autorización mostraría la versión anterior de un comprobante corregido: el SRI resolvió todos los comprobantes antes de la primera consulta. El SDK los maneja; están probados con el simulador y con servidores SOAP de prueba.
- `EstadoDiscrepancia` con el SRI real.
- `iva.unico` (dos IVA en un detalle) y `totales.tarifa`.
- Si el SRI contrasta una nota de crédito con su factura o una retención con su sustento.
