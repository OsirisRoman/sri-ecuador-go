# Segunda auditoría (candidata a 0.2.0): hallazgos y resolución

Una segunda auditoría adversarial independiente revisó la candidata a `v0.2.0` (commit `4492c75`). No encontró defectos que impidieran publicar, confirmó las correcciones de la primera ([`auditoria-v0.1.0.md`](auditoria-v0.1.0.md)) y añadió nueve hallazgos (N-01 a N-09). Este documento registra qué se hizo con cada uno.

Lo que la auditoría comprobó por medios propios y sigue siendo cierto después de las correcciones: la canonicalización coincide con libxml2 en 4039 de 4040 documentos atípicos (la excepción es `xmlns:xmlns`, que no es XML válido); el validador XSD da el mismo veredicto que `xmllint --schema` en 3500 de 3500 mutantes; `xmlsec1` 1.2.37 y 1.2.41 verifican las firmas SHA-256 y SHA-1 y rechazan la copia alterada; 920 000 ejecuciones de fuzzing sobre lectura, validación, cálculo, XSD, firma y RIDE no producen ningún pánico.

**Estados de resolución:** corregido, documentado (decisión vigente), aceptado (riesgo menor declarado).

## Matriz

| ID | Severidad | Hallazgo | Resolución | Pruebas de regresión |
|---|---|---|---|---|
| N-01 | Alta | Volver a llamar a `Calcular` después de corregir una cantidad, un precio o una tarifa dejaba el ICE, el IRBPNR y el pago único con los importes anteriores; también en una nota de crédito parcial hecha con `DesdeFactura` | **Corregido.** `Calcular` vuelve a deducir lo que dedujo y respeta lo que indicó la aplicación. Cada impuesto recuerda en memoria qué se dedujo; en un comprobante leído de XML se reconoce por coincidencia exacta con la deducción. Afecta a factura, liquidación, nota de crédito, nota de débito y retención (pago único) | `factura/recalculo_test.go` (incluye la propiedad «recalcular equivale a calcular desde cero»), `notacredito/`, `liquidacion/`, `notadebito/` y `retencion/recalculo_test.go` |
| N-02 | Media | `notadebito.Calcular` calculaba el IVA sin incluir el ICE en la base | **Corregido.** Con un único IVA, la base es el total sin impuestos más el ICE | `notadebito/recalculo_test.go` (redondeos, bases propias, varios IVA y la propiedad «lo que calcula `Calcular` lo acepta `Validar` sin hallazgos») |
| N-03 | Media | Con varias autorizaciones para una clave se tomaba como vigente la última de la lista | **Corregido.** El resultado es el de la autorización que contiene el documento enviado (misma `xades.Huella`), en cualquier orden. `Resultado.Autorizaciones` conserva todas | `autorizaciones_multiples_test.go` (cada combinación en los dos órdenes, y la propiedad «el resultado no depende del orden») |
| N-04 | Media | Coste cuadrático al canonicalizar un XML hostil | **Corregido.** Analizar, canonicalizar y verificar cuesta un tiempo proporcional al tamaño. Al corregirlo aparecieron y se cerraron otros dos vectores: texto partido en muchas secciones CDATA o comentarios, y firmas con miles de referencias (`xades.MaxReferencias`, `xades.MaxCertificados`) | `internal/xmldom/coste_test.go`, `internal/xmldom/referencia_test.go` (el algoritmo anterior como oráculo, también en el fuzzing del CI), `xades/recursos_test.go` |
| N-05 | Baja a media | Las compensaciones se restaban del importe total sin validarse | **Corregido.** Reglas `compensacion.codigo`, `compensacion.valor`, `compensacion.total` y advertencia `compensacion.historica` | `TestCompensaciones` |
| N-06 | Baja | Un comprobante con BOM UTF-8 se rechazaba | **Corregido.** Se admite un BOM inicial | `TestBOMUTF8`, `TestParseConBOM` |
| N-07 | Decisión | La regla estricta de IVA por línea (D17) impide representar parte de los precios con IVA incluido | **Documentado**, con la cifra medida y los ejemplos, en [D17](decisiones.md) y en [`pendientes.md`](pendientes.md) (punto 15) | — |
| N-08 | Baja | `EstadoAutorizado` no implicaba `Verificado`: un `AUTORIZADO` sin comprobante se daba por autorizado | **Corregido.** En `Reanudar`, `Emitir` y `ConsultarDocumento` es un estado indeterminado (`ErrSinComprobante`); `EstadoAutorizado` implica `Verificado` | `TestAutorizadoSinComprobante` |
| N-09 | Baja | Observaciones menores | Ver la tabla siguiente | — |

### Observaciones menores (N-09)

| Observación | Resolución |
|---|---|
| `liquidacion.Validar` aceptaba ICE e IRBPNR, que el XSD de liquidación no admite | **Corregido.** Regla `liquidacion.impuesto` (`TestSoloAdmiteIVA`) |
| El comentario de `sri.Comprobante` nombraba cuatro de sus siete implementaciones | **Corregido** |
| Una tarifa de ICE escrita `100.00` se rechaza por dígitos, y `100` no | **Documentado.** Es la misma decisión conservadora de la primera auditoría: los dígitos se cuentan como están escritos (`TestTarifaDeICEDeTresDigitos`) |
| `descuentoAdicional`, `valorRetRenta` y `totalSubsidio` no se contrastan con el resto del comprobante | **Aceptado.** No intervienen en ningún importe que calcule el SDK; el XSD comprueba su forma |
| El analizador acepta algunas formas que libxml2 rechaza (`&#xD800;`, atributos sin espacio entre sí, `us-ascii` con bytes no ASCII) | **Aceptado.** No afecta a la firma ni a la canonicalización de lo que sí es XML válido; el SRI devolvería esos documentos |
| Una devolución con código 43 y `tipo` vacío se trata como devuelta | **Aceptado.** El SRI siempre envía el tipo; reenviar con la misma clave es seguro |
| `pkcs12.ToPEM` está obsoleta | **Aceptado.** El PEM se usa solo en memoria para leer la clave y la cadena |
| Variables exportadas que cambian el comportamiento (`factura.LimiteConsumidorFinal`, `sri.EsperaPorDefecto`, `ws.ReintentosPorDefecto`, `esquema.Ajustes`) | **Aceptado.** Son puntos de configuración documentados; no alteran las reglas de coherencia de importes |

## Antes y después

Con la suite reproducible de la auditoría, conservada fuera del repositorio, contra el commit auditado y contra la versión corregida:

| Caso | Commit auditado | Corregido |
|---|---|---|
| 10 unidades de 100,00 con ICE del 75 %, corregidas a 1 y recalculadas | ICE 750,00; importe 977,50; pago 2012,50; `Preparar` firma | ICE 75,00; importe y pago 201,25 |
| Nota de crédito parcial (1 de 10) con `DesdeFactura` | 977,50 | 201,25 |
| Nota de débito de 100,00 con ICE del 10 % e IVA del 15 % | 125,00, sin errores | 126,50 |
| Dos rechazos (A y su corrección B), el más reciente primero | `Reanudar(B)`: pendiente; `ConsultarDocumento(B)`: discrepancia | Rechazo de B, verificado |
| `AUTORIZADO` sin comprobante | `EstadoAutorizado` sin verificar | `EstadoRecibido` con `ErrSinComprobante` |
| XML hostil de 467 kB (5000 `xmlns` × 100 000 hijos) | 19,5 s de CPU | 0,09 s |
| Firma con 3000 referencias al documento entero (2 MB) | 91 s | Rechazo inmediato |
| Compensación de 114,99 sobre un total de 115,00 | Importe 0,01; `Preparar` firma | Error `compensacion.valor` |
| Comprobante firmado con BOM UTF-8 | Rechazado | Aceptado |
| Suite adversarial de la auditoría | 9 pruebas fallan | Falla 1 (la tarifa `100.00`, documentada) |

## Lo que sigue abierto

- **Precios con IVA incluido (N-07, D17).** Con el IVA del 15 %, `Validar` rechaza la descomposición de 8696 de cada 100 000 precios finales entre 0,01 y 1000,00 (8,70 %), y `Calcular` no llega exactamente a 13 044 (13,04 %): 12,00, 35,00 o 0,50, por ejemplo. Es la aritmética de la ficha (el facturador del SRI calcula igual), pero el SRI autoriza la diferencia. Ver D17.
- **ICE contra el SRI.** N-01 y N-02 afectan a comprobantes con ICE, que no se han podido enviar al SRI: el emisor de las pruebas no tiene actividad gravada. La regla «el ICE integra la base del IVA» está confirmada en la factura; en la nota de débito, solo en local.
- **Varias autorizaciones por clave.** El SRI devolvió siempre una sola en las pruebas. El resultado ya no depende del orden, de modo que no hace falta observarlo; sigue sin conocerse cuántas devuelve.
- **Límites del recálculo.** En un comprobante leído de XML, cambiar la tarifa de un ICE (o una base propia) no actualiza su valor, que se toma por dato de la aplicación; hay que ponerlo a cero. Un ICE específico o un IRBPNR por unidad los actualiza siempre la aplicación.
