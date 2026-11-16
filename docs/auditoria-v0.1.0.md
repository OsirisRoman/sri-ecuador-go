# Auditoría de v0.1.0: hallazgos y resolución

Una auditoría adversarial independiente revisó la versión publicada `v0.1.0` (commit `395b6d0`). Encontró 13 hallazgos (AUD-001 a AUD-013), varias observaciones menores y riesgos que no pudo confirmar. Este documento registra qué se hizo con cada uno en la versión 0.2.0, con la prueba que lo protege y el commit que lo resuelve, y qué sigue abierto.

La auditoría no encontró vulnerabilidades explotables ni defectos de firma: las firmas del SDK las valida `xmlsec1`, y su canonicalización coincidió byte a byte con la de libxml2 en 3000 documentos aleatorios. Los hallazgos de mayor peso fueron de integridad tributaria (validaciones que dejaban pasar importes incoherentes) y de interpretación de las respuestas del SRI.

**Estados de resolución:** corregido, parcialmente corregido, documentado y diferido, no reproducido, descartado con evidencia.

## Matriz

| ID | Severidad | Hallazgo | Resolución | Pruebas de regresión | Commit |
|---|---|---|---|---|---|
| AUD-001 | Alta | La base del IVA de una línea no se contrastaba con la línea; se aceptaban dos IVA por línea y líneas sin IVA | **Corregido.** Reglas `iva.base` (base = total de la línea + ICE) e `iva.unico` como errores en factura, liquidación y nota de crédito; en la nota de débito, bases que no cubren el total. `iva.ausente` es advertencia: no hay norma que permita afirmar que el SRI lo rechaza | `factura/importes_test.go` (incluido un caso leído desde XML), `notacredito/importes_test.go`, `notadebito/importes_test.go`, `liquidacion/importes_test.go` | `794b1e4` |
| AUD-002 | Media | Reenviar con la misma clave un documento distinto terminaba en AUTORIZADO sin aviso | **Corregido.** `Reanudar` y la nueva `ConsultarDocumento` comparan la huella (`xades.Huella`, SHA-256 de la forma canónica sin firma) del comprobante que devuelve el SRI con la del enviado; si difieren, `EstadoDiscrepancia` y `*ErrorDiscrepancia`, con el documento del SRI disponible para diagnosticar Mientras el SRI procesa una corrección reenviada con la misma clave (ficha 5.10), el `NO AUTORIZADO` de la versión anterior se trata como pendiente, no como discrepancia | `autorizacion_test.go` (`TestMismaClaveContenidoDistinto`, `TestMismoComprobanteOtraSerializacion`, `TestCorreccionConLaMismaClave`), `xades/huella_test.go` | `249d593`, `c0d0c80` |
| AUD-003 | Media | La tolerancia de 0,01 por línea se acumulaba en el total | **Corregido.** Cada importe calculado solo admite el redondeo al centavo más cercano (`comprobante.ImporteRedondeado`, ±0,005); el total admite los dos criterios de D5, que ya solo difieren por redondeos legítimos. También en la retención, cuyo `Calcular` redondea ahora la base antes del impuesto | `factura/importes_test.go` (`TestCentavoDeMasPorLineaNoSeAcumula`, `TestRedondeoPorLineaYAgregadoLegitimos`), `retencion/importes_test.go`; propiedad «lo que produce `Calcular` lo acepta `Validar`» en `factura/propiedad_test.go` (3000 facturas aleatorias) y `retencion/calcular_redondeo_test.go` | `794b1e4`, `b2cdfc8` |
| AUD-004 | Media | Retención y nota de crédito sin cota respecto de su documento de origen | **Parcialmente corregido.** Retención 2.0.0: `retencion.base-iva`, `retencion.base-renta` y `sustento.base-iva` como advertencias, o errores con `SustentoEstricto`. Nota de crédito: `ValidarContra` y `AsociarFactura`; `DesdeFactura` la asocia, así que `Preparar` lo comprueba. Queda fuera lo que el SDK no puede saber (ver «Sigue abierto») | `retencion/sustento_test.go`, `notacredito/origen_test.go` | `b4d3d00` |
| AUD-005 | Media | Un estado vacío o desconocido se tomaba como NO AUTORIZADO; la respuesta no se ligaba a la clave ni al ambiente | **Corregido.** Solo NO AUTORIZADO, NAT y RECHAZADO son terminales; el resto es `EstadoRecibido` con `ErrEstadoIndeterminado`. Se comprueban `claveAccesoConsultada`, la clave del comprobante devuelto y el ambiente (`ErrRespuestaIncoherente`). Una recepción sin estado tampoco es una devolución | `autorizacion_test.go` (`TestEstadosDeAutorizacion`, `TestRespuestaIncoherente`, `TestRecepcionSinEstado`) | `249d593` |
| AUD-006 | Baja | `XMLAutorizado` no escapaba los campos | **Corregido** | `TestXMLAutorizadoEscapaLosCampos` | `4e9ba4b` |
| AUD-007 | Baja | Pánico al firmar con un `Certificado` sin clave | **Corregido.** `Firmante` devuelve una interfaz nil, nuevo `Certificado.Utilizable`, `sri.Nuevo` rechaza el certificado | `certificado/robustez_test.go`, `robustez_test.go` | `267cbb6` |
| AUD-008 | Baja | El analizador aceptaba atributos repetidos y no normalizaba los valores de atributo, de modo que el SDK y `xmlsec1` discrepaban | **Corregido.** Atributos repetidos rechazados (también por nombre expandido); normalización según XML 1.0 3.3.3; todas las funciones `Parse` pasan por el mismo analizador | `internal/xmldom/atributos_test.go` (con dos fuzzers), `internal/xmldom/limites_parse_test.go`, `internal/interop` | `9403522` |
| AUD-009 | Baja | Caracteres de control sustituidos en silencio por U+FFFD | **Corregido.** Regla `caracter-invalido` | `TestCaracteresNoRepresentablesEnXML` | `be39ab2` |
| AUD-010 | Baja | No se contrastaban la tarifa del total, la vigencia de las tarifas ni la propina | **Corregido.** `totales.tarifa` (error); `iva.vigencia` (advertencia, solo tarifas generales 12, 14 y 15 %, con fechas de la ley) y `propina.limite` (advertencia, sección 9.16) | `TestTarifaDelTotal`, `TestVigenciaDeTarifasGenerales`, `TestPropinaMayorAlDiezPorCiento` | `794b1e4`, `be39ab2` |
| AUD-011 | Baja | Afirmaciones de la documentación que superaban la evidencia | **Corregido.** Capacidades, README, SECURITY, guía de integración (que además contradecía la sección 5.10 de la ficha sobre reenviar con la misma clave), referencia y decisiones | — | PR #30 |
| AUD-012 | Baja | Dependencia XSD joven; acciones del CI sin fijar; `govulncheck` declarado pero no ejecutado | **Parcialmente corregido.** Acciones por SHA, `go mod verify` y `govulncheck` en el CI (también semanal), diferencial XSD y C14N contra `xmllint` y firma contra `xmlsec1` en el CI, decisión D16. La madurez de la dependencia es un riesgo que solo se mitiga | `internal/interop` | `47aa277`, `f0b4109` |
| AUD-013 | Baja | El RIDE de vista previa era indistinguible de uno válido | **Corregido.** Sin envoltura AUTORIZADO ni fecha de autorización, el RIDE dice «SIN AUTORIZACIÓN DEL SRI» y «VISTA PREVIA: SIN VALIDEZ TRIBUTARIA» | `ride/vistaprevia_test.go` | `293c76f` |

### Observaciones y riesgos no confirmados

| Observación | Resolución | Prueba o commit |
|---|---|---|
| Catálogos exportados y mutables (`TarifasIVA`, `FormasPago`, `Paises`, `PorcentajesIVA`) | **Corregido.** El SDK valida contra una copia privada; nuevas `TarifaIVADe`, `FormaPagoDe`, `PaisDe` | `TestCatalogoExportadoNoAlteraLaValidacion`, `ede85dc` |
| `sri.Espera` con varias consultas y sin intervalo consultaba en ráfaga | **Corregido** | `TestEsperaSinIntervalo`, `267cbb6` |
| `ws.Lote.XML` insertaba el RUC sin validarlo | **Corregido** | `ws/lote_test.go`, `267cbb6` |
| `Decimal.Digitos` cuenta los ceros finales, más estricto que el XSD | **Descartado con evidencia.** Es intencional y conservador: el SDK nunca escribe un valor que el SRI pueda rechazar por dígitos (lo documenta `retencion` en sus pruebas); solo afecta a la lectura de XML de terceros con ceros finales sobrantes | — |
| Un RUC de forma válida, quizá real, en las pruebas | **Corregido.** Sustituido por uno ficticio | `91b37fd` |
| El logotipo del RIDE no limitaba sus dimensiones antes de decodificarse | **Corregido.** Límite de 25 megapíxeles | `ride/logo_limite_test.go`, `a9f6bd8` |
| Liquidación de compra heredaba AUD-001 por compartir el código de los detalles | **Corregido** con AUD-001 y probado expresamente | `liquidacion/importes_test.go` |
| Nota de débito, revisada solo en parte | **Corregido** lo que aplica (bases de IVA) | `notadebito/importes_test.go` |

## Antes y después

Con la suite reproducible de la auditoría, conservada fuera del repositorio, contra `v0.1.0` y contra 0.2.0:

| Caso de la auditoría | v0.1.0 | 0.2.0 |
|---|---|---|
| Línea de 100,00 con IVA de 0,15 | 0 errores | 1 error (`iva.base`) |
| Dos IVA en una línea | 0 errores | 1 error (`iva.unico`) |
| 100 líneas con un centavo de más de IVA | 0 errores | 100 errores (`iva.valor`) |
| `totalImpuesto` del 15 % con tarifa 12 | 0 errores | 1 error (`totales.tarifa`) |
| IVA 15 % en 2023; IVA 12 % en 2026; propina del 90 % | sin hallazgos | advertencias |
| Retención de 19 500 sobre un sustento de 230 | sin hallazgos | 2 advertencias (errores con `SustentoEstricto`) |
| Nota de crédito por 50 veces la factura (`DesdeFactura`) | 0 errores | 3 errores (`origen.*`) |
| Misma clave, contenido distinto | AUTORIZADO, con el XML de otro documento | `EstadoDiscrepancia` y `ErrDiscrepancia` |
| Estado de autorización vacío o desconocido | NO AUTORIZADO (definitivo) | RECIBIDO con `ErrEstadoIndeterminado` |
| Autorización de otra clave y otro ambiente | AUTORIZADO | RECIBIDO con `ErrRespuestaIncoherente` |
| Recepción sin estado | DEVUELTO | FIRMADO con `ErrEstadoIndeterminado` |
| Número de autorización con `&` y `<` | XML mal formado | XML correcto |
| Firmar con un certificado sin clave | pánico | `ErrCertificado` / `ErrSinParDeFirma` |
| Atributo repetido; salto de línea literal en un atributo | aceptados (el segundo, con una firma que `xmlsec1` rechaza) | atributo repetido rechazado; normalización igual a libxml2 |
| Caracteres de control en un texto | 0 errores, sustituidos por U+FFFD | 1 error (`caracter-invalido`) |

## Contraste con el SRI

Una ronda en el ambiente de pruebas con el código de 0.2.0 confirmó con el SRI real: que rechaza (52) la base de IVA incoherente de AUD-001; que el reenvío corregido con la misma clave tras un `NO AUTORIZADO` se recibe y se autoriza, y que el SDK lo da por autorizado y verificado (AUD-002); y que el SRI responde a una clave desconocida igual que a un comprobante en proceso (AUD-005), lo que llevó a precisar en la guía que un estado desconocido se resuelve reenviando, no consultando. También mostró que el SRI **tolera un centavo** en el IVA de una línea. La regla de AUD-003 se mantiene más estricta que el SRI por decisión del propietario (D17): evita la acumulación que motivó el hallazgo y conserva los redondeos legítimos que el SRI autoriza. Detalle en [`pruebas-sri.md`](pruebas-sri.md).

## Comprobantes ya autorizados por el SRI

Las reglas nuevas se aplicaron, en local y sin enviar nada, a todos los comprobantes que el ambiente de pruebas del SRI autorizó a este proyecto (los seis tipos, en todas sus versiones; 48 respuestas de autorización contando las de la ronda de certificación de 0.2.0), con `SustentoEstricto` en la retención. En todos la firma verifica, el RIDE se genera y, cuando se conserva el XML enviado, su huella coincide con la del comprobante que devolvió el SRI. En cuanto a las reglas:

- Los 24 comprobantes autorizados **antes** de la ronda no dan ningún error ni advertencia nuevos.
- De los de la ronda, el único con un error es el **T5**: un IVA de línea con un centavo de más que se envió a propósito para contrastar `iva.valor`, que el SRI autorizó y el SDK rechaza por decisión (D17). El T2, un detalle sin IVA, da la advertencia `iva.ausente`, como se esperaba.

Es decir, salvo esa diferencia deliberada, las reglas nuevas no rechazan nada que el SRI haya autorizado, y la comparación de `Reanudar` funciona con respuestas reales.

## Sigue abierto

Lo que no se puede resolver con código, o solo en parte. Está también en [`pendientes.md`](pendientes.md) (punto 15).

- **Tolerancia por línea (AUD-003).** El SRI autoriza un centavo de diferencia en el IVA de una línea; el SDK no, por decisión (D17). Un modo de compatibilidad optativo queda como posible mejora, sin implementar.
- **Reglas sin contrastar con el SRI:** `iva.unico`, `totales.tarifa` y si el SRI contrasta notas de crédito y retenciones con su origen.
- **Estados intermedios reales:** el SRI resolvió todo antes de la primera consulta; el lapso en que mostraría el resultado de la versión anterior de una corrección solo se ha simulado.
- **Notas de crédito anteriores.** `ValidarContra` acumula las que la aplicación le pasa; el SDK no las consulta al SRI. Una nota leída con `Parse` no conoce su factura hasta que se le asocia.
- **Retención 1.0.0.** No declara los importes del sustento, así que no hay con qué contrastar sus bases.
- **Vigencia de tarifas.** Solo las tarifas generales, con fechas tomadas de la ley (la ficha no las publica); son advertencias y caducan con cada cambio de tarifa.
- **Dependencia XSD.** Mitigada (D16), no eliminada.
- **Producción y otras entidades de certificación**, como antes de la auditoría.
