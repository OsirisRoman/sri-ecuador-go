# Notas de versión

Los cambios notables de cada versión. El formato sigue [Keep a Changelog](https://keepachangelog.com/es-ES/1.1.0/) y las versiones, [SemVer](https://semver.org/lang/es/), según la [política de versiones](docs/versionado.md). Mientras la versión sea 0.x, la API puede cambiar entre versiones menores.

## Sin publicar

- Documentación: política de versiones (`docs/versionado.md`).

## v0.2.0 — sin publicar

Corrige los hallazgos de dos auditorías adversariales independientes: la de la v0.1.0 y la de la candidata a esta versión. El detalle de cada hallazgo, la prueba que lo protege y lo que sigue abierto están en [`docs/auditoria-v0.1.0.md`](docs/auditoria-v0.1.0.md) y [`docs/auditoria-v0.2.0.md`](docs/auditoria-v0.2.0.md). Sigue basada en la ficha técnica v2.34.

### Cambios incompatibles

Comprobantes que la v0.1.0 firmaba ahora se rechazan o cambian de estado. Las reglas forman parte de la API ([`docs/versionado.md`](docs/versionado.md)).

- **Nuevos errores de validación** (impiden `Preparar`): `iva.base` (la base del IVA de una línea debe ser su total más su ICE), `iva.unico` (un solo IVA por detalle), `totales.tarifa` (tarifa del `totalImpuesto` distinta de la de los detalles), `caracter-invalido` (caracteres que XML 1.0 no admite) y, en las notas de crédito asociadas a su factura, `origen.*`. Cómo adaptarse: usar `Calcular`, que ya producía importes correctos; para documentos de otros sistemas, corregir los importes.
- **Redondeo más estricto:** `iva.valor`, `retencion.valor` y el IVA del sustento ya no admiten un centavo de diferencia, solo el redondeo al centavo más cercano (±0,005).
- **Estados de autorización:** un estado vacío o desconocido ya no es `EstadoNoAutorizado` sino `EstadoRecibido` con `ErrEstadoIndeterminado`; una respuesta de otra clave u otro ambiente devuelve `ErrRespuestaIncoherente`; una recepción sin estado ya no es `EstadoDevuelto` sino un error `ErrEstadoIndeterminado`. Cómo adaptarse: tratar esos errores como estado desconocido y reintentar con el mismo XML.
- **Nuevo estado `EstadoDiscrepancia`** (al final de la enumeración: los valores anteriores no cambian), con `*ErrorDiscrepancia`: el SRI tiene otro comprobante registrado con la misma clave. Un `switch` sobre `Estado` debe contemplarlo.
- **`Parse` de todos los comprobantes** rechaza ahora DOCTYPE, atributos repetidos y documentos que superen los límites del analizador de firmas, y normaliza los valores de atributo según XML 1.0.
- **Compensaciones acotadas:** nuevos errores `compensacion.codigo`, `compensacion.valor` y `compensacion.total`, y advertencia `compensacion.historica`. Una compensación ya no puede superar su tarifa aplicada a la base gravada con IVA, ni todas juntas el IVA del comprobante. Nuevo error `liquidacion.impuesto` (ICE o IRBPNR en una liquidación, que el XSD ya rechazaba).
- **`Calcular` recalcula lo que dedujo:** en la v0.1.0, el ICE y el IRBPNR deducidos y el total de un pago único se conservaban al volver a llamar a `Calcular`; ahora siguen a los datos del comprobante. Lo que indica la aplicación se respeta como antes. En la nota de débito, la base de un único IVA se deduce siempre (total sin impuestos más ICE), aunque la aplicación hubiera escrito otra.
- **`AUTORIZADO` sin comprobante ya no es `EstadoAutorizado`** en `Reanudar`, `Emitir` y `ConsultarDocumento`: queda `EstadoRecibido` con el nuevo `ErrSinComprobante` (que también es `ErrEstadoIndeterminado`), porque no se puede comprobar qué documento autorizó el SRI. En esos métodos `EstadoAutorizado` implica ahora `Verificado`. `Resultado.XMLAutorizado` devuelve nil si la autorización no trae el comprobante (antes envolvía el XML local). El SRI real siempre lo ha devuelto.
- **Varias autorizaciones para una clave:** el resultado ya no es el de la última de la lista sino el de la que contiene el documento enviado; `Autorizar` (solo clave) informa, entre varios rechazos, el de fecha más reciente. `ws.RespuestaAutorizacion.EnProceso` deja de depender de la posición.
- `xades.Verificar` rechaza una firma con más de 16 referencias o 16 certificados (`xades.MaxReferencias`, `xades.MaxCertificados`); una firma del SRI lleva 3 referencias.
- `sri.Nuevo` rechaza un `Certificado` sin clave privada; `ws.Lote.XML` exige un RUC válido; `ride.Generar` marca como vista previa lo que no viene con la autorización ni con `Opciones.FechaAutorizacion`, y rechaza logotipos de más de 25 megapíxeles.

### Añadido

- `sri.Cliente.ConsultarDocumento`: consulta la autorización de un XML firmado y comprueba que el autorizado sea ese documento. `Resultado.Verificado`.
- `Resultado.Autorizaciones` (todas las autorizaciones recibidas para la clave), `ErrSinComprobante` y `ws.Autorizacion.EnProceso`.
- `xades.Huella`: SHA-256 de la forma canónica de un comprobante sin su firma.
- `notacredito.NotaCredito.AsociarFactura` y `ValidarContra`; `DesdeFactura` asocia la factura.
- `retencion.OpcionesValidacion.SustentoEstricto` y las reglas `retencion.base-iva`, `retencion.base-renta` y `sustento.base-iva` (advertencias).
- Advertencias `iva.ausente`, `iva.vigencia` (tarifas generales de IVA fuera de su vigencia legal) y `propina.limite`.
- `comprobante.ImporteRedondeado`, `Validador.ImpuestosDetalle`, `Validador.VigenciaIVA`, `IVAVigente`, `CaracterInvalido`, `Deserializar`, `TarifaIVADe`, `FormaPagoDe`, `PaisDe`; `factura.VigenciaDetalles`; `certificado.Certificado.Utilizable`; `ws.Autorizacion.NoAutorizado`.
- **Precio final con IVA incluido** (decisión D18): `factura.LineaPrecioFinal`, `Detalle.PrecioFinal` y `DeclararPreciosFinales` en factura, liquidación de compra y nota de crédito. La aplicación declara el precio que paga el comprador y `Calcular` deduce la base, el IVA y el precio unitario, de modo que el comprobante suma exactamente ese precio (12,00 al 15 %: 10,43 + 1,57). `Validar` contrasta cada detalle con su precio declarado (`precio-final.importe`, `precio-final.base`, `precio-final.impuestos`, `precio-final.total`); los detalles que no lo declaran siguen como antes. Nuevas `comprobante.DescomponerPrecioFinal`, `CalcularPrecioFinal`, `AdmitePrecioFinal`, `PrecioFinalDeclarado`, `Validador.ImpuestosDetallePrecioFinal`, `Validador.TotalesPrecioFinal`, `GrupoImpuesto.ValorCon`, `factura.CalcularDetalle`, `factura.ImpuestosFinales`, `factura.DecimalesDePrecio` y `decimal.Decimal.Cociente`. Advertencia `origen.precio-final`. Vectores en `vectores/precio-final.json`.
- `comprobante.RecalcularImpuestos`, `comprobante.AsignarPagoUnico` y `Validador.Compensaciones`.
- CI: `govulncheck` y `go mod verify` (también semanal), contraste con `xmlsec1` y `xmllint` (`internal/interop`), fuzzing breve del analizador XML y acciones fijadas por SHA.

### Corregido

- El validador aceptaba una línea de 100,00 con IVA declarado de 0,15, dos IVA por línea, un centavo de más por línea acumulado en el total y una tarifa del total incoherente (AUD-001, AUD-003, AUD-010).
- Reenviar con la misma clave un comprobante distinto terminaba en AUTORIZADO con el XML de otro documento (AUD-002). Una corrección reenviada con la misma clave según la ficha (5.10) queda pendiente mientras el SRI solo muestra el NO AUTORIZADO de la versión anterior.
- Volver a llamar a `Calcular` después de corregir una cantidad, un precio o una tarifa dejaba el ICE, el IRBPNR y el pago único con los importes anteriores, también en una nota de crédito parcial hecha con `DesdeFactura`: 1 de 10 unidades con ICE del 75 % salía por 977,50 en lugar de 201,25 (segunda auditoría, N-01).
- `notadebito.Calcular` calculaba el IVA sin incluir el ICE en la base: 125,00 en lugar de 126,50 para 100,00 con ICE del 10 % e IVA del 15 % (N-02).
- Las compensaciones se restaban del importe total sin ninguna comprobación (N-05).
- Con varias autorizaciones para una clave se tomaba como vigente la última de la lista: si el SRI las devolviera con la más reciente primero, el rechazo de una corrección quedaba como pendiente o como discrepancia (segunda auditoría, N-03). Un `AUTORIZADO` sin comprobante se daba por autorizado sin poder verificarlo (N-08).
- Canonicalizar costaba un tiempo cuadrático con muchos espacios de nombres en ámbito, y analizar, con texto partido en muchas secciones CDATA o comentarios: un XML hostil de 467 kB ocupaba 19,5 s de CPU en `xades.Verificar` o `xades.Huella`. Ahora el coste es proporcional al tamaño. Una firma con miles de referencias al documento entero multiplicaba el coste de verificarla (segunda auditoría, N-04).
- Un comprobante con la marca de orden de bytes (BOM) de UTF-8 al principio se rechazaba en `Verificar`, `Parse` y el RIDE (N-06).
- `retencion.Calcular` calculaba el IVA del sustento sobre la base sin redondear.
- Retenciones con bases mayores que el sustento y notas de crédito mayores que la factura pasaban sin hallazgos (AUD-004).
- `XMLAutorizado` no escapaba los campos de la autorización (AUD-006).
- Firmar con un certificado sin clave provocaba un pánico (AUD-007).
- Un atributo con un salto de línea literal daba una firma válida para el SDK e inválida para `xmlsec1` (AUD-008).
- Los caracteres de control se sustituían en silencio por U+FFFD (AUD-009).
- El RIDE de vista previa era indistinguible de uno válido (AUD-013).
- Los catálogos exportados podían modificarse y alterar la validación de todo el proceso; `sri.Espera` sin intervalo consultaba en ráfaga.
- La guía de integración decía que un comprobante rechazado se vuelve a emitir con otra clave; la ficha (5.10) exige la misma clave y el mismo secuencial.
- La guía sugería `ConsultarDocumento` para un estado desconocido tras un fallo de envío; el SRI responde igual a una clave que no recibió que a una en proceso, así que hay que reenviar con `Reanudar`.

### Limitaciones

Las de la v0.1.0 siguen vigentes, salvo la nota de crédito 1.0.0, ya autorizada por el SRI. Una ronda en el ambiente de pruebas con este código confirmó la regla de base del IVA, el reenvío corregido con la misma clave y los dos criterios de redondeo del total ([`docs/pruebas-sri.md`](docs/pruebas-sri.md)). El SRI autoriza un centavo de diferencia en el IVA de una línea que `iva.valor` rechaza: el SDK es más estricto **por decisión** (D17), para evitar diferencias acumuladas. Quien pacta precios con IVA incluido declara el precio final de cada detalle (D18), y el SRI autorizó dos facturas calculadas así; el modo solo admite detalles cuyo único impuesto es el IVA, no viaja en el XML (tras `Parse` hay que volver a declararlo) y no existe en la nota de débito. Sin observar en el SRI (pendiente 15): `iva.unico`, `totales.tarifa`, los estados intermedios, una nota de débito con ICE y varias autorizaciones para una clave. En un comprobante leído de XML, cambiar la tarifa de un ICE no actualiza su valor al recalcular: hay que ponerlo a cero. Una tarifa de ICE del 100 % se escribe `100`, no `100.00`.

## v0.1.0 — primera versión pública (pre-release) — 2026-11-15

Primera versión utilizable del SDK. Es una versión previa a la 1.0: funciona y está probada contra el ambiente de pruebas del SRI, pero la API puede cambiar y hay casos sin verificar, que se declaran abajo.

### Requisitos

- Go 1.26 o superior. Sin cgo ni programas externos.
- Para emitir: un certificado de firma electrónica en archivo `.p12` de una entidad autorizada en Ecuador y un RUC habilitado en el ambiente correspondiente del SRI.
- Especificación de referencia: *Ficha técnica de comprobantes electrónicos esquema offline* v2.34 (julio 2026), vigente al 15 de noviembre de 2026.

### Funcionalidades

**Comprobantes** (modelo tipado, lectura y escritura de XML, cálculo de totales, clave de acceso y validación):

- Factura 1.0.0, 1.1.0, 2.0.0 y 2.1.0, con exportación, reembolsos y requisitos de la factura comercial negociable.
- Nota de crédito 1.0.0 y 1.1.0; `notacredito.DesdeFactura` revierte una factura.
- Nota de débito 1.0.0.
- Comprobante de retención 2.0.0 (ATS) y 1.0.0, con el catálogo de retenciones de renta.
- Guía de remisión 1.0.0 y 1.1.0.
- Liquidación de compra 1.0.0 y 1.1.0.

**Base**

- Decimales exactos (`decimal`) y clave de acceso con módulo 11 (`clave`).
- Validación contra los XSD oficiales del SRI, incrustados (`esquema`).
- Reglas de negocio con identificadores estables, que distinguen errores de advertencias.
- Catálogos de la ficha: tarifas de IVA, formas de pago, países, retenciones.

**Firma y servicios**

- Carga de certificados PKCS#12 (`certificado`).
- Firma y verificación XAdES-BES con RSA-SHA256 (por defecto) y RSA-SHA1 (`xades`).
- Clientes SOAP de recepción, autorización, lotes, consulta de validez y consulta de factura comercial negociable (`ws`).
- Flujo de emisión con reintentos seguros y reanudación (`sri`).
- Simulador del SRI y certificado desechable para pruebas (`sritest`).

**Entrega**

- RIDE en PDF de los seis comprobantes, con código de barras y logotipo opcional (`ride`).
- Correo de entrega con XML y RIDE, opcional (`correo`).
- Herramienta de línea de comandos (`cmd/sri`).

**Documentación**

- Guía de integración, ejemplo ejecutable e instrucciones para asistentes de IA.
- Referencia técnica independiente del lenguaje y vectores de prueba.
- Capacidades con su estado, pendientes documentados y guía de contribución.

### Compatibilidad demostrada

Autorizado por el **ambiente de pruebas del SRI** (un emisor, certificado del Banco Central del Ecuador):

- Factura en sus cuatro versiones, con RSA-SHA256 y RSA-SHA1; factura de exportación y de reembolso de gastos.
- Nota de crédito 1.1.0 (total y parcial), nota de débito 1.0.0.
- Comprobante de retención 2.0.0 y 1.0.0.
- Guía de remisión 1.0.0 y 1.1.0; liquidación de compra 1.0.0 y 1.1.0.
- Envío por lotes, consulta de autorización y de validez, y manejo de los códigos 35, 39 y 43.

El detalle está en [`docs/pruebas-sri.md`](docs/pruebas-sri.md).

### Limitaciones

- **No se ha probado en producción** ni con certificados de otras entidades de certificación.
- **Devolución automática de IVA (servicios DIG): no implementada.** El SDK rechaza un `valorDevolucionIva` mayor que cero, porque no descuenta ese valor del importe total (Anexo 20).
- Solo con pruebas locales: ICE e IRBPNR, factura comercial negociable confirmada, sustitutiva de guía de remisión, casos de retención distintos del pago local (ISD, exterior, dividendos), nota de crédito 1.0.0, entre otros.
- La verificación de firmas no consulta la revocación de certificados. No se admiten tokens ni HSM.
- El RIDE no incrusta fuentes ni es PDF/A; los caracteres fuera de Windows-1252 se sustituyen.
- Los catálogos incluidos caducan con los cambios normativos.
- El SDK no asigna secuenciales, no almacena comprobantes y no anula comprobantes.

Cada limitación está explicada, con lo que falta para resolverla, en [`docs/pendientes.md`](docs/pendientes.md).
