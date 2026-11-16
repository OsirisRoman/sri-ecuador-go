# Servicios web

**Regla oficial:** ficha técnica v2.34, secciones 7 (recepción y autorización) y 8 (consultas). Los servicios son SOAP 1.1 sobre HTTPS, sin autenticación: la identidad del emisor la da la firma del comprobante.

## Direcciones

| Ambiente | Base |
|---|---|
| Pruebas (certificación) | `https://celcer.sri.gob.ec/comprobantes-electronicos-ws/` |
| Producción | `https://cel.sri.gob.ec/comprobantes-electronicos-ws/` |

| Servicio | Ruta (añadir `?wsdl` para la descripción) | Espacio de nombres |
|---|---|---|
| Recepción | `RecepcionComprobantesOffline` | `http://ec.gob.sri.ws.recepcion` |
| Autorización | `AutorizacionComprobantesOffline` | `http://ec.gob.sri.ws.autorizacion` |
| Consulta de validez | `ConsultaComprobante` | `http://ec.gob.sri.ws.consultas` |
| Consulta de factura comercial negociable | `ConsultaFactura` | `http://ec.gob.sri.ws.consultas` |

Las peticiones son `POST` con `Content-Type: text/xml; charset=utf-8`.

## Recepción: `validarComprobante`

El comprobante firmado se envía en Base64 dentro del parámetro `xml`:

```xml
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:ec="http://ec.gob.sri.ws.recepcion">
  <soapenv:Header/>
  <soapenv:Body>
    <ec:validarComprobante><xml>BASE64_DEL_XML_FIRMADO</xml></ec:validarComprobante>
  </soapenv:Body>
</soapenv:Envelope>
```

Respuesta: `RespuestaRecepcionComprobante` con `estado` (`RECIBIDA` o `DEVUELTA`) y, si se devuelve, una lista de `comprobante` con su `claveAcceso` y sus `mensajes` (`identificador`, `mensaje`, `informacionAdicional`, `tipo`).

- Tamaño máximo de un comprobante: **320 kB** (código 26 si se supera).
- **`RECIBIDA` no es una autorización**: solo indica que el SRI aceptó el documento para procesarlo.

## Autorización: `autorizacionComprobante`

```xml
<ec:autorizacionComprobante xmlns:ec="http://ec.gob.sri.ws.autorizacion">
  <claveAccesoComprobante>49_DIGITOS</claveAccesoComprobante>
</ec:autorizacionComprobante>
```

Respuesta: `RespuestaAutorizacionComprobante` con `claveAccesoConsultada`, `numeroComprobantes` y una lista de `autorizacion`, cada una con `estado`, `numeroAutorizacion`, `fechaAutorizacion`, `ambiente`, `comprobante` (el XML firmado, en una sección CDATA) y `mensajes`.

**Comprobado:**

- El estado es `AUTORIZADO` o el texto `NO AUTORIZADO`.
- En pruebas, la autorización tarda unos pocos segundos; la ficha admite hasta 24 horas.
- El número de autorización es la clave de acceso.
- El comprobante devuelto es idéntico, byte a byte, al enviado.
- La fecha llega en formato ISO 8601 con desplazamiento (`2026-11-14T23:56:59-05:00`).

A tener en cuenta, según la definición del servicio (no observado en las pruebas):

- Mientras el SRI procesa, la respuesta puede llegar sin autorizaciones: hay que volver a consultar.
- La respuesta admite varias autorizaciones para una misma clave (por ejemplo, un intento no autorizado y otro autorizado): busque la que esté en `AUTORIZADO`. Según la sección 5.11, si el comprobante fue no autorizado varias veces, el servicio devuelve solo el último estado.
- La Tabla 6 nombra los estados con siglas (PPR en procesamiento, AUT autorizado, NAT no autorizado) y el ejemplo de la sección 7.2.3 usa `RECHAZADO` para un no autorizado. Un estado vacío o distinto de estos no permite saber qué pasó: trátelo como desconocido, nunca como rechazo, o una aplicación podría emitir otro comprobante mientras el original sigue en curso.

**Contraste con la consulta.** Compruebe que `claveAccesoConsultada`, la clave del comprobante devuelto y el `ambiente` correspondan a lo que se consultó, y que el comprobante devuelto sea, en contenido, el que se envió. Lo segundo importa porque el emisor debe corregir y reenviar un comprobante rechazado **con la misma clave** (sección 5.10): si un envío anterior de resultado incierto sí llegó, el SRI puede tener otra versión con esa clave y responder por ella. **Decisión de este SDK:** comparar el SHA-256 de la forma canónica de los dos comprobantes sin su firma (lo que cubre la referencia `#comprobante`), que no cambia con la serialización y sí con cualquier cambio de contenido; si difieren, el resultado es una discrepancia y no una autorización.

El documento que el emisor debe conservar y entregar al receptor es el comprobante firmado junto con su autorización. **Decisión de este SDK:** componerlo como un elemento `<autorizacion>` con `estado`, `numeroAutorizacion`, `fechaAutorizacion`, `ambiente` y el comprobante en CDATA, que es el formato habitual de intercambio.

## Flujo y estados

```
construir → validar (reglas + XSD) → firmar → [guardar] → validarComprobante
                                                              │
                                   DEVUELTA ◄─────────────────┤
                                   (corregir y reemitir)      │ RECIBIDA
                                                              ▼
                                                  autorizacionComprobante ──► sin resultado: esperar y repetir
                                                              │
                                    NO AUTORIZADO ◄───────────┴──────────► AUTORIZADO
                                    (corregir y reemitir)                  (guardar y entregar)
```

| Situación | Significado | Qué hacer |
|---|---|---|
| `DEVUELTA` | No pasó la recepción (estructura, clave, emisor) | Corregir y reenviar con la misma clave y secuencial (5.10) |
| `RECIBIDA` sin autorización todavía | Pendiente | Consultar más tarde; no reemitir |
| `NO AUTORIZADO` | No pasó la autorización (firma, diferencias de cálculo) | Corregir y reenviar con la misma clave y secuencial (5.10) |
| `AUTORIZADO` del mismo documento | Comprobante válido | Guardar y entregar |
| `AUTORIZADO` u otro estado, pero de un documento distinto | Hay otra versión registrada con esa clave | No entregar el documento enviado; resolver con lo que tiene el SRI |
| Estado vacío o desconocido; respuesta de otra clave u otro ambiente | **Estado desconocido** | Volver a consultar; no reemitir |
| Sin respuesta (red, tiempo agotado) | **Estado desconocido** | Reintentar con el mismo documento firmado |

## Reintentos e idempotencia

Reenviar un comprobante que el SRI ya recibió no lo duplica. El SRI responde `DEVUELTA` con uno de estos códigos, que **no son un rechazo del comprobante**, pero tampoco prueban que lo que tiene registrado sea el documento que se acaba de enviar (ver «Contraste con la consulta»):

| Código | Mensaje | Qué hacer |
|---|---|---|
| 43 | Clave de acceso registrada | Pasar a consultar la autorización |
| 70 | Clave de acceso en procesamiento | Esperar y consultar la autorización |

**Comprobado:** el 43, reenviando un comprobante ya autorizado. El 70 no se ha observado.

**Decisión de este SDK:** reintentar automáticamente los fallos de red y los errores HTTP 5xx con espera creciente; tratar 43 y 70 como «ya recibido», comparando después el comprobante autorizado con el enviado; y consultar la autorización hasta seis veces en aproximadamente un minuto antes de devolver el comprobante como pendiente.

## Códigos frecuentes

| Código | Etapa | Significado |
|---|---|---|
| 26 | Recepción | Tamaño máximo superado |
| 35 | Recepción | El XML no cumple el esquema |
| 39 | Autorización | Firma inválida |
| 43, 70 | Recepción | Ya recibido o en procesamiento (ver arriba) |
| 45 | Recepción | Secuencial registrado |
| 52 | Autorización | Error en diferencias: cálculos, tarifas o códigos que no coinciden con los del SRI |
| 56, 57 | Autorización | Establecimiento cerrado; autorización para emitir suspendida |
| 58 | Autorización | La clave de acceso no coincide con el comprobante |
| 60 | — | Advertencia: proceso realizado en el ambiente de pruebas |
| 65 | Recepción | Fecha de emisión extemporánea |
| 82 | Recepción | Inicio de transporte anterior a la emisión de la guía |
| 92 | Recepción | El valor de devolución de IVA no es el autorizado por el servicio DIG |

La lista completa está en la sección 11 de la ficha.

## Envío por lotes

**Regla oficial:** Tabla 8. Hasta 50 comprobantes y 500 kB.

```xml
<?xml version="1.0" encoding="UTF-8"?>
<lote version="1.0.0">
  <claveAcceso>CLAVE_DEL_LOTE</claveAcceso>
  <ruc>RUC_DEL_EMISOR</ruc>
  <comprobantes>
    <comprobante><![CDATA[XML_FIRMADO_1]]></comprobante>
    <comprobante><![CDATA[XML_FIRMADO_2]]></comprobante>
  </comprobantes>
</lote>
```

Se envía por la misma operación `validarComprobante` y se consulta con `autorizacionComprobanteLote` (parámetro `claveAccesoLote`), que devuelve `RespuestaAutorizacionLote` con `claveAccesoLoteConsultada`, `numeroComprobantesLote` y una `autorizacion` por comprobante.

**Comprobado** (la ficha no lo aclara):

- El documento `<lote>` **no lleva firma propia**; cada comprobante va firmado.
- La clave del lote se compone como la de un comprobante, con tipo `01` y una serie reservada para lotes.
- El lote se recibe entero y **cada comprobante se resuelve por separado**: uno defectuoso no afecta a los demás.

## Consultas

**Validez** (`consultarEstadoAutorizacionComprobante`, parámetro `claveAcceso`): devuelve `EstadoAutorizacionComprobante` con `estadoAutorizacion` (`AUTORIZADO`, `NO AUTORIZADO`, `PENDIENTE DE ANULAR`, `ANULADO`), `tipoComprobante`, `rucEmisor` y `fechaAutorizacion`. Si la consulta no puede resolverse, `estadoConsulta` vale `RECHAZADA` y los motivos van en `mensajes`. **Comprobado:** en una consulta satisfactoria, `estadoConsulta` llega vacío.

**Factura comercial negociable** (`consultarEstadoConfirmacionFacturaComercialNegociable`, parámetro `claveAcceso`): devuelve `estadoConfirmacion` con el valor `SI` si la factura fue aceptada como negociable. **Comprobado:** para una factura que no es negociable responde `RECHAZADA` con el código 99.

## Devolución automática de IVA (DIG)

Son servicios REST distintos, con autenticación propia. No están implementados; lo investigado está en [`../devolucion-iva-dig.md`](../devolucion-iva-dig.md).

Implementación: [`ws`](../../ws) (clientes), [`sri.go`](../../sri.go) (flujo) y [`sritest`](../../sritest) (simulador).
