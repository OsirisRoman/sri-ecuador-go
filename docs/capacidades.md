# Capacidades y estado

Qué hace el SDK, cómo se usa, qué lo respalda y qué límites tiene. Lo que falta está en [`pendientes.md`](pendientes.md); el detalle de las pruebas contra el SRI, en [`pruebas-sri.md`](pruebas-sri.md). Estado al 15 de noviembre de 2026, con la ficha técnica v2.34.

## Cómo leer el estado

| Estado | Significa |
|---|---|
| ✅ **Verificada en el SRI** | Implementada, con pruebas automáticas, y además comprobada contra el ambiente de pruebas (certificación) del SRI |
| 🧪 **Probada localmente** | Implementada y con pruebas automáticas (XSD oficial, simulador, vectores), pero **sin** confirmación del SRI |
| 🚧 **Parcial** | Funciona para algunos casos; el resto se indica |
| ⛔ **No implementada** | No existe en el SDK |

«Verificada en el SRI» se refiere siempre al ambiente de pruebas, con un solo emisor y un solo certificado. Nada se ha probado en producción.

## Comprobantes

Todos comparten la misma forma de uso: llenar la estructura, `Calcular()`, `GenerarClaveAcceso("")` y entregarla al cliente (`sri.Cliente`). Ver la [guía de integración](guia-de-integracion.md).

| Capacidad | Paquete | Estado | Requisito del SRI | Respaldo | Límites |
|---|---|---|---|---|---|
| Factura 1.0.0, 1.1.0, 2.0.0, 2.1.0 | `factura` | ✅ | Anexos 1, 3, 8 y 9; XSD de factura | Las cuatro versiones autorizadas; ejemplos oficiales del SRI en ida y vuelta | ICE e IRBPNR solo con pruebas locales |
| Factura de exportación | `factura` | ✅ | Anexo 4; Tabla 25 | Autorizada | Un caso |
| Factura de reembolso de gastos | `factura` | ✅ | Anexo 5 | Autorizada | Un comprobante reembolsado |
| Factura comercial negociable | `factura` | 🧪 | Anexo 11 | Reglas con pruebas locales | Sin una factura confirmada como negociable |
| Placa en combustibles y transporte | `factura` | 🧪 | Anexos 12, 16 y 25 | Pruebas locales | Son advertencias: el SDK no conoce la actividad del emisor |
| Sustitutiva de guía, subsidios, retenciones informativas, máquina fiscal | `factura` | 🚧 | Anexos 6, 9 y 13; sección 10 | El modelo cumple el XSD | Sin reglas propias ni prueba con el SRI |
| Nota de crédito 1.1.0 | `notacredito` | ✅ | Anexos 1 y 3 | Total y parcial autorizadas | — |
| Nota de crédito 1.0.0 | `notacredito` | ✅ | Anexo 1 | Autorizada (devolución parcial) | Un solo caso |
| Nota de débito 1.0.0 | `notadebito` | ✅ | Anexo 1 | Autorizada | — |
| Comprobante de retención 2.0.0 (ATS) | `retencion` | ✅ | Anexo 10; Tablas 19 y 20 | Autorizado: renta e IVA, pago local | ISD, pagos al exterior, dividendos, banano y reembolsos: 🧪 |
| Comprobante de retención 1.0.0 | `retencion` (`ComprobanteV1`) | ✅ | Anexo 1 | Autorizado | — |
| Guía de remisión 1.0.0 y 1.1.0 | `guiaremision` | ✅ | Anexos 1 y 3 | Ambas autorizadas, con y sin comprobante de sustento | Varios destinatarios y documento aduanero: 🧪 |
| Liquidación de compra 1.0.0 y 1.1.0 | `liquidacion` | ✅ | Anexo 17 | Ambas autorizadas | Reembolsos y proveedor del exterior: 🧪 |
| Devolución de IVA en el comprobante (`valorDevolucionIva`) | `factura`, `notadebito` | ⛔ | Anexo 20 | — | El SDK **rechaza** valores mayores que cero; ver pendientes 1 y 2 |

## Base común

| Capacidad | Paquete | Estado | Requisito del SRI | Respaldo | Límites |
|---|---|---|---|---|---|
| Clave de acceso: generar, analizar, validar | `clave` | ✅ | Sección 5.2, Tabla 1 | Claves impresas en la ficha; vectores; todas las claves de las pruebas aceptadas por el SRI | — |
| Decimales exactos y redondeo | `decimal` | ✅ | Evita el código 52 | Vectores de cálculo; facturas autorizadas con 6 decimales | — |
| Cálculo de totales | cada comprobante | ✅ | Anexos | Vectores; sin código 52 en las pruebas | ICE específico o con base propia e IRBPNR: los indica y los actualiza el usuario; ICE solo con pruebas locales |
| Reglas de validación (forma, aritmética, catálogos, clave) | cada comprobante | ✅ | Ficha y XSD | Una prueba por regla; tabla en [`referencia/calculo-y-validacion.md`](referencia/calculo-y-validacion.md) | Las de catálogos que caducan son advertencias |
| Base del IVA por línea (`iva.base`) | cada comprobante | ✅ | Tabla 18, art. 58 LRTI | El SRI rechaza con el código 52 la misma incoherencia; pruebas de regresión | — |
| Redondeo del IVA de cada línea (`iva.valor`) y criterios del total | cada comprobante | ✅ | Sección 11 (código 52) | El SRI autoriza la suma por líneas y el desempate mitad al par, como el SDK | **Más estricto que el SRI a propósito** (D17): rechaza un centavo de diferencia por línea que el SRI autoriza |
| Un IVA por detalle, tarifa del total, detalle sin IVA (advertencia) | cada comprobante | 🧪 | Tabla 17 | El SRI autoriza un detalle sin IVA, por eso es advertencia; lo demás, pruebas locales | No validan el valor del ICE ni del IRBPNR |
| Coherencia con el documento de origen | `retencion`, `notacredito` | 🚧 | Coherencia | Pruebas locales | Retención 2.0.0: advertencias (errores con `SustentoEstricto`); la 1.0.0 no declara el sustento. Nota de crédito: solo si se conoce la factura (`DesdeFactura`, `AsociarFactura`) y las notas anteriores que pase la aplicación |
| Advertencias de vigencia de tarifa y de propina | `factura`, `liquidacion` | 🧪 | Ley; sección 9.16 | Pruebas locales | Solo las tarifas generales de IVA; fechas que caducan |
| Validación XSD | `esquema` | ✅ | XSD oficiales, incrustados sin modificar | Mismo veredicto que `xmllint` en los 13 ejemplos del SRI; el SRI devuelve con el código 35 lo que el SDK rechaza | Un ajuste documentado (RIMPE negocio popular) sin confirmar |
| Catálogos (IVA, pagos, países, retenciones) | `comprobante`, `retencion` | ✅ / 🧪 | Tablas de la ficha; Catálogo ATS | Generados desde la fuente oficial | **Caducan**: ver [`actualizar-catalogos.md`](actualizar-catalogos.md) |

## Firma

| Capacidad | Paquete | Estado | Requisito del SRI | Respaldo | Límites |
|---|---|---|---|---|---|
| Carga de certificados `.p12` | `certificado` | ✅ | Sección 6 | Certificado real del Banco Central del Ecuador | Sin tokens ni HSM (pendiente 10) |
| Firma XAdES-BES, RSA-SHA256 y RSA-SHA1 | `xades` | ✅ | Sección 6, Anexo 14 | Ambos algoritmos autorizados; en cada cambio, verificada con `xmlsec1` (1.2 y 1.3) y canonicalización igual a la de libxml2 (CI, `internal/interop`); vectores | Un solo certificado probado (pendiente 3) |
| Verificación de firma | `xades` | 🧪 | — | Detecta alteraciones que el SRI también rechaza (código 39); coincide con `xmlsec1` en atributos con saltos de línea literales | Sin revocación (pendiente 9) |

## Comunicación con el SRI

| Capacidad | Paquete | Estado | Requisito del SRI | Respaldo | Límites |
|---|---|---|---|---|---|
| Recepción y autorización | `ws`, `sri` | ✅ | Sección 7 | Todas las emisiones de las pruebas | — |
| Reintentos e idempotencia del mismo documento | `ws`, `sri` | ✅ | Sección 5.10 | Código 43 comprobado; cortes de red simulados | Código 70 no observado |
| Contraste del autorizado con lo enviado y reenvío corregido con la misma clave | `sri` | ✅ | Secciones 5.10, 5.11 y Tabla 6 | En el SRI: corrección tras `NO AUTORIZADO` (52 y 39) autorizada y verificada; reenvío idéntico (43) verificado; huella coincidente en todas las respuestas | `EstadoDiscrepancia` y los estados intermedios solo con el simulador y servidores SOAP anómalos: el SRI no produjo ninguno |
| Consulta de validez | `ws` | ✅ | Sección 8 | Comprobante autorizado | Estados de anulación no observados |
| Consulta de factura comercial negociable | `ws` | 🚧 | Sección 8 | Respuesta negativa comprobada | Respuesta afirmativa sin comprobar |
| Envío por lotes | `ws` | ✅ | Sección 7.5, Tabla 8 | Dos lotes de facturas | Sin flujo de alto nivel (pendiente 8) |
| Devolución automática de IVA (DIG) | — | ⛔ | Sección 13 | — | Ver [`devolucion-iva-dig.md`](devolucion-iva-dig.md) |
| Simulador del SRI | `sritest` | 🧪 | — | Reproduce las respuestas observadas | Aproximación: no sustituye al ambiente de pruebas |

## Entrega

| Capacidad | Paquete | Estado | Requisito del SRI | Respaldo | Límites |
|---|---|---|---|---|---|
| RIDE en PDF de todos los comprobantes | `ride` | 🧪 | Anexo 2; secciones 9.19 y 9.20 | Dos visores independientes; código de barras leído con `zbar` | No es algo que valide el SRI. Lo que no consta como autorizado se marca como vista previa. Sin fuentes incrustadas ni PDF/A; caracteres fuera de Windows-1252 se sustituyen (pendiente 13) |
| Correo de entrega (XML y RIDE) | `correo` | 🧪 | — | Servidor SMTP real con STARTTLS; recibido en la bandeja de entrada | Opcional; la entregabilidad depende del remitente |
| Línea de comandos | `cmd/sri` | 🧪 | — | Usada en las pruebas | Opera solo contra el ambiente de pruebas |

## Riesgos conocidos

- **Evidencia limitada a un emisor, un certificado y el ambiente de pruebas.** Producción y otras entidades de certificación no están comprobadas.
- **La normativa cambia.** Los catálogos y los XSD incluidos corresponden a la ficha v2.34; una versión posterior puede exigir actualizarlos.
- **Versión previa a la 1.0.** La API puede cambiar entre versiones menores.
- **El SDK no sustituye el criterio tributario** ni garantiza la autorización: valida la coherencia interna del comprobante y, cuando la conoce, con su documento de origen; no la correcta aplicación de la ley a cada operación (qué tarifa corresponde a un bien, qué retención a un pago).
- **Reglas sin contrastar con el SRI.** Las reglas añadidas tras la auditoría de v0.1.0 ([`auditoria-v0.1.0.md`](auditoria-v0.1.0.md)) podrían ser más estrictas o más laxas que el SRI en algún caso (pendiente 15).
