# Pendientes y oportunidades de contribución

Lo que este SDK **no** hace todavía, o hace solo en parte, explicado con el detalle suficiente para que otra persona pueda continuarlo sin repetir la investigación. Lo que ya funciona está en [`capacidades.md`](capacidades.md).

Cada entrada dice qué debería hacer, por qué no está, qué se sabe, qué falta y cuándo se consideraría terminada. Si quiere tomar una, lea [`CONTRIBUTING.md`](../CONTRIBUTING.md) y abra un *issue* para coordinar. Nada de esto está prometido para una fecha ni asignado a nadie.

La dificultad es orientativa. **Requiere** indica lo que no se puede suplir solo con código.

| # | Pendiente | Dificultad | Requiere |
|---|---|---|---|
| 1 | [Devolución automática de IVA (DIG)](#1-devolución-automática-de-iva-dig) | Alta | Habilitación del SRI y credenciales |
| 2 | [`valorDevolucionIva` en los totales](#2-valordevolucioniva-en-los-totales) | Media | Depende del 1 |
| 3 | [Certificados de otras entidades](#3-certificados-de-otras-entidades-de-certificación) | Baja | Un certificado de cada entidad |
| 4 | [ICE e IRBPNR contra el SRI](#4-ice-e-irbpnr-contra-el-sri) | Media | Emisor habilitado en pruebas |
| 5 | [Casos de retención sin probar](#5-casos-del-comprobante-de-retención-sin-probar) | Media | Emisor habilitado en pruebas |
| 6 | [Casos de factura sin probar](#6-casos-de-factura-sin-probar) | Baja a media | Emisor habilitado; algunos, un régimen concreto |
| 7 | [Nota de crédito 1.0.0 y otros casos menores](#7-nota-de-crédito-100-guía-y-liquidación-casos-menores) | Baja | Emisor habilitado en pruebas |
| 8 | [Lotes en el flujo de alto nivel](#8-lotes-en-el-flujo-de-alto-nivel) | Media | Emisor habilitado para completar las pruebas |
| 9 | [Revocación de certificados](#9-comprobación-de-revocación-ocsp-y-crl) | Media | — |
| 10 | [Firma con token o HSM](#10-firma-con-dispositivos-criptográficos-pkcs11) | Media a alta | Un dispositivo |
| 11 | [Catálogos históricos y de ISD](#11-catálogos-históricos-y-de-isd) | Baja | — |
| 12 | [Requisitos por actividad no reconocibles](#12-requisitos-por-actividad-que-el-sdk-no-puede-reconocer) | Baja | Decisión de diseño |
| 13 | [RIDE: fuentes incrustadas y PDF/A](#13-ride-fuentes-incrustadas-y-pdfa) | Media | — |
| 14 | [Producción](#14-verificación-en-producción) | — | Un emisor que lo use en producción |
| 15 | [Reglas de la auditoría de v0.1.0 frente al SRI](#15-reglas-de-la-auditoría-de-v010-frente-al-sri) | Baja | Emisor habilitado en pruebas |

---

## 1. Devolución automática de IVA (DIG)

Servicios REST con los que un comercio aplica en caja la devolución de IVA a personas adultas mayores o con discapacidad. **No implementado.** Depende de una habilitación del SRI cuyo procedimiento no está documentado y de detalles del protocolo que la ficha no define (formato del token y de varias tramas).

Todo lo investigado (referencias, direcciones, servicios, códigos de respuesta, incógnitas y pasos para retomarlo) está en [`devolucion-iva-dig.md`](devolucion-iva-dig.md).

**Terminado cuando:** exista un paquete opcional que obtenga el token, consulte y anule devoluciones, con simulador y pruebas, y se haya verificado contra el ambiente de certificación del SRI con un emisor habilitado.

## 2. `valorDevolucionIva` en los totales

**Qué debería hacer.** Cuando una factura o una nota de débito declara `valorDevolucionIva` (Anexo 20), el importe total debe restar ese valor, y el valor debe ser el que autorizó el servicio DIG.

**Estado.** El campo existe en los modelos, pero `Calcular` y `Validar` no aplican el descuento. Para no emitir importes incorrectos, **el SDK rechaza cualquier valor mayor que cero** (regla `devolucion-iva.no-admitida`); cero o ausente se admiten.

**Por qué no se completó.** Sin el servicio DIG no hay forma de obtener un monto válido ni de probar el comprobante: el SRI lo rechaza con el código 92 si no coincide con lo autorizado.

**Qué se sabe.** Las reglas del Anexo 20 (comprador con cédula; valor entre cero y el valor del impuesto; se resta del total). Además, el SRI publica dos familias de XSD de nota de crédito y de débito incompatibles: `NotaCredito_V*` y `NotaDebito_V*` admiten `agenteRetencion` y `contribuyenteRimpe`; `NOTA_CREDITO-*` y `NOTA_DEBITO-*` admiten `valorDevolucionIva`. El SDK valida con la primera familia en la nota de crédito (por eso su modelo no tiene el campo); `NotaDebito_V1.0.0` sí admite ambos. Las dos familias están incrustadas en `esquema/xsd/`.

**Qué falta.** El pendiente 1; decidir con pruebas reales qué XSD aplica el SRI a una nota de crédito con devolución; implementar el descuento en `Calcular` y `Validar` de factura y nota de débito; retirar la regla de rechazo.

**Terminado cuando:** una factura con devolución de IVA calculada por el SDK sea autorizada por el SRI, y haya pruebas del cálculo y del código 92.

## 3. Certificados de otras entidades de certificación

**Qué debería hacer.** Firmar con certificados de cualquier entidad autorizada en Ecuador (Security Data, ANF, Uanataca, Consejo de la Judicatura y otras).

**Estado.** Probado con el SRI solo con un certificado del Banco Central del Ecuador. Las pruebas locales usan un emisor ficticio con atributos poco comunes (`organizationIdentifier`).

**Riesgo conocido.** El punto sensible es `X509IssuerName`: cada entidad compone el nombre de su emisor con atributos distintos, y el texto debe ser el que produce Java (ver [`referencia/firma-xades.md`](referencia/firma-xades.md)). La regla implementada es general, pero no está contrastada con otras entidades. También puede variar el contenido del `.p12` (varias claves, cadenas incompletas, cifrados antiguos).

**Cómo contribuir sin exponer nada.** No hace falta compartir el certificado. Basta con ejecutar `go run ./cmd/sri certificado`, que muestra el emisor y el algoritmo sin datos secretos, firmar una factura de prueba en el ambiente de pruebas e informar del resultado con la plantilla de interoperabilidad. Si la firma falla, lo más útil es el campo `X509IssuerName` que generó el SDK y el nombre del emisor tal como lo muestra otra herramienta; ninguno de los dos es secreto.

**Terminado cuando:** `pruebas-sri.md` recoja una autorización con cada entidad, o el fallo esté corregido y cubierto por una prueba con un emisor ficticio de la misma forma.

## 4. ICE e IRBPNR contra el SRI

**Estado.** El modelo y el cálculo admiten ICE e IRBPNR (el ICE integra la base del IVA). Solo hay pruebas locales.

**Por qué no se probó.** El emisor de las pruebas no tiene actividad gravada con ICE, y las tarifas específicas (por unidad o volumen, Tabla 18) dependen del producto.

**Qué falta.** Emitir en pruebas facturas con ICE *ad valorem* y con tarifa específica, y con IRBPNR; comprobar si el SRI contrasta la tarifa con su parametrización, como hace con las retenciones; añadir el catálogo de la Tabla 18 si resulta necesario. El Anexo 18 (fundas plásticas) es un caso concreto.

**Terminado cuando:** haya al menos una factura con ICE y otra con IRBPNR autorizadas, y el cálculo esté cubierto por vectores.

## 5. Casos del comprobante de retención sin probar

**Estado.** La versión 2.0.0 modela ISD, pagos al exterior, convenios de doble tributación, dividendos, compra de banano y reembolsos. Autorizado por el SRI solo el caso común: retención de renta y de IVA sobre una factura con pago local.

**Qué falta.** Emitir en pruebas cada caso. Para los pagos al exterior hay que confirmar los códigos de país y de régimen fiscal que acepta el SRI (`paisEfecPago` admite 3 o 4 dígitos).

**Terminado cuando:** cada caso tenga una autorización registrada, o esté documentado el motivo por el que no puede probarse.

## 6. Casos de factura sin probar

Tienen modelo y, en su caso, reglas locales, pero no una autorización del SRI:

| Caso | Referencia | Qué falta |
|---|---|---|
| Retenciones informativas en la factura | Sección 10 | Un emisor comercializador de derivados del petróleo o editor |
| Factura comercial negociable | Anexo 11 | Emitirla, confirmarla en el portal y consultar con `ws.ConsultarNegociable` la respuesta afirmativa |
| Sustitutiva de guía de remisión | Anexo 9 | Emitir una factura 2.x con `infoSustitutivaGuiaRemision`; no hay reglas locales |
| Combustibles y transporte comercial | Anexos 12, 16 y 25 | Emitir con placa; hoy son advertencias |
| Subsidios | Anexo 6 | Reglas para `totalSubsidio` y `precioSinSubsidio`; no se muestran en el RIDE |
| Negocio popular RIMPE | Anexo 22 | Un emisor de ese régimen: el XSD publicado no admite la leyenda y el SDK la acepta por un ajuste |
| Máquina fiscal | Anexo 13 | Emitir con el bloque `maquinaFiscal` |

**Terminado cuando:** cada caso tenga su autorización registrada y, donde aplique, reglas con pruebas.

## 7. Nota de crédito 1.0.0, guía y liquidación: casos menores

Sin probar contra el SRI: la nota de crédito versión 1.0.0; guías de remisión con varios destinatarios, con documento aduanero o emitidas antes del día del traslado (`GenerarClaveAccesoEn`); liquidaciones de compra con reembolsos, con proveedor del exterior o negociables. Todos tienen pruebas locales y cumplen el XSD.

**Terminado cuando:** cada uno tenga una autorización registrada.

## 8. Lotes en el flujo de alto nivel

**Estado.** `ws.Lote`, `ValidarLote` y `AutorizacionLote` funcionan y se probaron con el SRI. El cliente de alto nivel (`sri.Cliente`) sigue trabajando comprobante a comprobante.

**Qué debería hacer.** Un método que prepare varios comprobantes, los envíe en un lote y devuelva un resultado por comprobante, reanudable tras un corte.

**Qué falta decidir.** Cómo asignar y guardar la clave del lote (necesita su propio secuencial, que es responsabilidad de la aplicación) y cómo relacionar cada autorización con su comprobante cuando alguno no tiene una clave legible.

**Sin probar con el SRI:** lotes con otros tipos de comprobante o mezclados, el máximo de 50, la reutilización de una clave de lote y un comprobante que incumple el XSD dentro de un lote (el simulador responde con el código 35 por comprobante: es una suposición).

**Terminado cuando:** exista la API con pruebas en el simulador y esos casos estén contrastados con el SRI.

## 9. Comprobación de revocación (OCSP y CRL)

**Estado.** `xades.Verificar` comprueba integridad, coherencia y vigencia, y opcionalmente la cadena de confianza. No consulta si el certificado fue revocado.

**Por qué.** Al emitir, el SRI valida el certificado al autorizar. Importa para quien **recibe** comprobantes y quiere verificarlos por su cuenta.

**Qué falta.** Leer del certificado las direcciones OCSP y CRL, consultarlas con límites de tiempo y caché, y exponerlo como opción, sin hacerlo obligatorio.

**Terminado cuando:** la verificación pueda rechazar un certificado revocado, con pruebas contra un respondedor simulado.

## 10. Firma con dispositivos criptográficos (PKCS#11)

**Estado.** Solo se cargan certificados de archivo `.p12`. `certificado.DesdeClave` acepta una clave RSA en memoria.

**Qué debería hacer.** Firmar con un token USB o un HSM, donde la clave no puede exportarse.

**Qué se sabe.** La firma solo necesita la operación «firmar un resumen»; el paquete `xades` ya usa la interfaz `crypto.Signer`, de modo que el cambio principal está en `certificado` (aceptar un `crypto.Signer` externo). PKCS#11 en Go exige cgo, que este proyecto evita en su núcleo: debería ir en un módulo aparte.

**Terminado cuando:** se pueda construir un `Certificado` a partir de un `crypto.Signer` y haya un ejemplo documentado con un dispositivo real.

## 11. Catálogos históricos y de ISD

**Estado.** El catálogo de retenciones de renta es solo el vigente; los comprobantes con fecha anterior no se contrastan. No se incluyen los porcentajes de ISD.

**Qué falta.** Para el histórico, conservar versiones anteriores del Catálogo ATS con su vigencia (el script `scripts/catalogo-ats/` ya las genera una a una). Para el ISD, localizar la tabla oficial.

**Terminado cuando:** la validación elija el catálogo por la fecha del comprobante.

## 12. Requisitos por actividad que el SDK no puede reconocer

Fundas plásticas (Anexo 18), materiales de construcción (Anexo 23) y gran contribuyente (Anexo 24) exigen códigos o leyendas concretos, pero un comprobante no dice por sí mismo si el emisor está en esos casos.

**Qué falta.** Una forma de que la aplicación declare el perfil del emisor (por ejemplo, opciones de validación) para activar esas reglas, más los catálogos de las Tablas 31 y afines.

**Terminado cuando:** exista esa opción con sus reglas y pruebas.

## 13. RIDE: fuentes incrustadas y PDF/A

**Estado.** El RIDE usa las fuentes base del PDF sin incrustarlas y la codificación Windows-1252: los caracteres fuera de ella se sustituyen por `?`. No es PDF/A. No muestra subsidios ni los datos de sustitución de guía de remisión.

**Qué falta.** Incrustar un subconjunto de una fuente de licencia libre (TrueType), con lo que se resolverían los caracteres y se acercaría a PDF/A; añadir los bloques que faltan.

**Terminado cuando:** un RIDE con caracteres fuera de Windows-1252 se vea correctamente y pase un validador de PDF/A.

## 14. Verificación en producción

Desde este proyecto no se ha enviado nada a producción, y el ambiente de pruebas puede validar menos. El SDK está preparado (direcciones de producción, comprobación de ambiente), pero no hay evidencia propia.

**Cómo contribuir.** Quien lo use en producción puede informar, con la plantilla de interoperabilidad y sin datos del emisor, de qué comprobantes y versiones le autoriza el SRI y de cualquier rechazo que no ocurra en pruebas.

## 15. Reglas de la auditoría de v0.1.0 frente al SRI

**Estado.** La versión 0.2.0 añadió reglas de validación y de interpretación de respuestas a raíz de una auditoría independiente ([`auditoria-v0.1.0.md`](auditoria-v0.1.0.md)). Se basan en la ficha técnica y en la ley, y tienen pruebas locales y contra `xmlsec1` y libxml2, pero no se han contrastado con el ambiente de pruebas del SRI.

**Qué falta comprobar en el ambiente de pruebas.**

- Que el SRI devuelve (código 52) una base de IVA que no es el total de la línea más su ICE, y qué hace con un detalle sin IVA (hoy, advertencia `iva.ausente`) y con dos IVA en un detalle.
- Que acepta un total de IVA igual a la suma de las líneas cuando difiere de la base acumulada en más de un centavo (el SDK lo admite, decisión D5).
- Qué responde la recepción al reenviar con la misma clave un comprobante **corregido** cuando la versión anterior quedó recibida, autorizada o no autorizada, y qué comprobante devuelve después la autorización. El SDK lo detecta como `EstadoDiscrepancia` comparando con lo que devuelve la autorización; falta observarlo en el SRI real.
- Si el SRI contrasta una nota de crédito con su factura o una retención con su sustento.

**Qué no puede resolver el SDK.** Las notas de crédito anteriores sobre una misma factura (hay que pasarlas a `AsociarFactura`), los importes del sustento en la retención 1.0.0 (que no los declara) y las fechas de las tarifas que no son la general (`comprobante/vigencia.go`).

**Terminado cuando:** cada punto tenga un resultado registrado en [`pruebas-sri.md`](pruebas-sri.md) y, si el SRI se comporta distinto, la regla se haya ajustado.
