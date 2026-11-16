# Notas de versión

Los cambios notables de cada versión. El formato sigue [Keep a Changelog](https://keepachangelog.com/es-ES/1.1.0/) y las versiones, [SemVer](https://semver.org/lang/es/), según la [política de versiones](docs/versionado.md). Mientras la versión sea 0.x, la API puede cambiar entre versiones menores.

## Sin publicar

- Documentación: política de versiones (`docs/versionado.md`).

## v0.2.0 — sin publicar

Corrige los hallazgos de una auditoría adversarial independiente de la v0.1.0. El detalle de cada hallazgo, la prueba que lo protege y lo que sigue abierto están en [`docs/auditoria-v0.1.0.md`](docs/auditoria-v0.1.0.md). Sigue basada en la ficha técnica v2.34.

### Cambios incompatibles

Comprobantes que la v0.1.0 firmaba ahora se rechazan o cambian de estado. Las reglas forman parte de la API ([`docs/versionado.md`](docs/versionado.md)).

- **Nuevos errores de validación** (impiden `Preparar`): `iva.base` (la base del IVA de una línea debe ser su total más su ICE), `iva.unico` (un solo IVA por detalle), `totales.tarifa` (tarifa del `totalImpuesto` distinta de la de los detalles), `caracter-invalido` (caracteres que XML 1.0 no admite) y, en las notas de crédito asociadas a su factura, `origen.*`. Cómo adaptarse: usar `Calcular`, que ya producía importes correctos; para documentos de otros sistemas, corregir los importes.
- **Redondeo más estricto:** `iva.valor`, `retencion.valor` y el IVA del sustento ya no admiten un centavo de diferencia, solo el redondeo al centavo más cercano (±0,005).
- **Estados de autorización:** un estado vacío o desconocido ya no es `EstadoNoAutorizado` sino `EstadoRecibido` con `ErrEstadoIndeterminado`; una respuesta de otra clave u otro ambiente devuelve `ErrRespuestaIncoherente`; una recepción sin estado ya no es `EstadoDevuelto` sino un error `ErrEstadoIndeterminado`. Cómo adaptarse: tratar esos errores como estado desconocido y reintentar con el mismo XML.
- **Nuevo estado `EstadoDiscrepancia`** (al final de la enumeración: los valores anteriores no cambian), con `*ErrorDiscrepancia`: el SRI tiene otro comprobante registrado con la misma clave. Un `switch` sobre `Estado` debe contemplarlo.
- **`Parse` de todos los comprobantes** rechaza ahora DOCTYPE, atributos repetidos y documentos que superen los límites del analizador de firmas, y normaliza los valores de atributo según XML 1.0.
- **`AUTORIZADO` sin comprobante ya no es `EstadoAutorizado`** en `Reanudar`, `Emitir` y `ConsultarDocumento`: queda `EstadoRecibido` con el nuevo `ErrSinComprobante` (que también es `ErrEstadoIndeterminado`), porque no se puede comprobar qué documento autorizó el SRI. En esos métodos `EstadoAutorizado` implica ahora `Verificado`. `Resultado.XMLAutorizado` devuelve nil si la autorización no trae el comprobante (antes envolvía el XML local). El SRI real siempre lo ha devuelto.
- **Varias autorizaciones para una clave:** el resultado ya no es el de la última de la lista sino el de la que contiene el documento enviado; `Autorizar` (solo clave) informa, entre varios rechazos, el de fecha más reciente. `ws.RespuestaAutorizacion.EnProceso` deja de depender de la posición.
- `sri.Nuevo` rechaza un `Certificado` sin clave privada; `ws.Lote.XML` exige un RUC válido; `ride.Generar` marca como vista previa lo que no viene con la autorización ni con `Opciones.FechaAutorizacion`, y rechaza logotipos de más de 25 megapíxeles.

### Añadido

- `sri.Cliente.ConsultarDocumento`: consulta la autorización de un XML firmado y comprueba que el autorizado sea ese documento. `Resultado.Verificado`.
- `Resultado.Autorizaciones` (todas las autorizaciones recibidas para la clave), `ErrSinComprobante` y `ws.Autorizacion.EnProceso`.
- `xades.Huella`: SHA-256 de la forma canónica de un comprobante sin su firma.
- `notacredito.NotaCredito.AsociarFactura` y `ValidarContra`; `DesdeFactura` asocia la factura.
- `retencion.OpcionesValidacion.SustentoEstricto` y las reglas `retencion.base-iva`, `retencion.base-renta` y `sustento.base-iva` (advertencias).
- Advertencias `iva.ausente`, `iva.vigencia` (tarifas generales de IVA fuera de su vigencia legal) y `propina.limite`.
- `comprobante.ImporteRedondeado`, `Validador.ImpuestosDetalle`, `Validador.VigenciaIVA`, `IVAVigente`, `CaracterInvalido`, `Deserializar`, `TarifaIVADe`, `FormaPagoDe`, `PaisDe`; `factura.VigenciaDetalles`; `certificado.Certificado.Utilizable`; `ws.Autorizacion.NoAutorizado`.
- CI: `govulncheck` y `go mod verify` (también semanal), contraste con `xmlsec1` y `xmllint` (`internal/interop`), fuzzing breve del analizador XML y acciones fijadas por SHA.

### Corregido

- El validador aceptaba una línea de 100,00 con IVA declarado de 0,15, dos IVA por línea, un centavo de más por línea acumulado en el total y una tarifa del total incoherente (AUD-001, AUD-003, AUD-010).
- Reenviar con la misma clave un comprobante distinto terminaba en AUTORIZADO con el XML de otro documento (AUD-002). Una corrección reenviada con la misma clave según la ficha (5.10) queda pendiente mientras el SRI solo muestra el NO AUTORIZADO de la versión anterior.
- Con varias autorizaciones para una clave se tomaba como vigente la última de la lista: si el SRI las devolviera con la más reciente primero, el rechazo de una corrección quedaba como pendiente o como discrepancia (segunda auditoría, N-03). Un `AUTORIZADO` sin comprobante se daba por autorizado sin poder verificarlo (N-08).
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

Las de la v0.1.0 siguen vigentes, salvo la nota de crédito 1.0.0, ya autorizada por el SRI. Una ronda en el ambiente de pruebas con este código confirmó la regla de base del IVA, el reenvío corregido con la misma clave y los dos criterios de redondeo del total ([`docs/pruebas-sri.md`](docs/pruebas-sri.md)). El SRI autoriza un centavo de diferencia en el IVA de una línea que `iva.valor` rechaza: el SDK es más estricto **por decisión** (D17), para evitar diferencias acumuladas. Sin observar en el SRI (pendiente 15): `iva.unico`, `totales.tarifa` y los estados intermedios.

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
