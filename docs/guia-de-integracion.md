# Guía de integración

Esta guía explica cómo incorporar `sri-ecuador-go` a una aplicación que emite comprobantes electrónicos del SRI (Ecuador, esquema offline). Está escrita para que la siga tanto una persona como un asistente de IA: cada sección dice qué hacer, con qué funciones y qué errores evitar.

El ejemplo de referencia es [`ejemplos/emision/main.go`](../ejemplos/emision/main.go): se compila con el proyecto, se ejecuta sin configuración contra el simulador (`go run ./ejemplos/emision`) y contiene, comentado, el flujo completo descrito aquí. Ante la duda entre esta guía y el código, manda el código y su documentación (`go doc`).

## 1. Qué hace el SDK y qué no

Hace: construir y validar los seis comprobantes, calcular totales, generar la clave de acceso, firmar (XAdES-BES), enviar al SRI y consultar la autorización, generar el RIDE en PDF y, opcionalmente, enviar el correo de entrega.

No hace (es responsabilidad de su aplicación):

- **Asignar secuenciales.** Deben ser consecutivos por establecimiento, punto de emisión y tipo de comprobante.
- **Guardar** los comprobantes, sus claves de acceso y su estado. El SDK no tiene base de datos.
- **Decidir la tributación**: qué tarifa de IVA aplica a un producto, qué retención corresponde a una compra, etc. El SDK valida la coherencia, no el criterio tributario.
- **Custodiar** el certificado `.p12` y su contraseña.
- Anular comprobantes (se hace en el portal del SRI) ni emitir en contingencia.
- La devolución automática del IVA a adultos mayores (servicios DIG): no está implementada; ver [`devolucion-iva-dig.md`](devolucion-iva-dig.md).

## 2. Instalación y requisitos

```sh
go get github.com/OsirisRoman/sri-ecuador-go
```

Go 1.26 o superior. Sin cgo ni programas externos (ni Java, ni OpenSSL, ni bases de datos). Necesita un certificado de firma electrónica en archivo `.p12` emitido por una entidad autorizada en Ecuador, y que el RUC tenga habilitado el ambiente de pruebas (y luego producción) en el portal del SRI.

## 3. Mapa de paquetes

| Necesito… | Paquete | Punto de entrada |
|---|---|---|
| Emitir de principio a fin | `sri` (raíz) | `sri.Nuevo`, `Cliente.Preparar`, `Cliente.Reanudar`, `Cliente.Emitir`, `Cliente.ConsultarDocumento`, `Cliente.Autorizar` |
| Factura | `factura` | `factura.Factura`, `factura.LineaIVA` |
| Nota de crédito | `notacredito` | `notacredito.DesdeFactura` |
| Nota de débito | `notadebito` | `notadebito.NotaDebito`, `notadebito.ImpuestoIVA` |
| Comprobante de retención | `retencion` | `retencion.ComprobanteRetencion` |
| Guía de remisión | `guiaremision` | `guiaremision.GuiaRemision` |
| Liquidación de compra | `liquidacion` | `liquidacion.LiquidacionCompra` |
| Importes | `decimal` | `decimal.MustParse`, `decimal.Parse`, `decimal.Entero` |
| Catálogos y tipos comunes | `comprobante` | constantes `ID*`, `IVA*`, `Pago*`, `Cod*`; `comprobante.Hoy()` |
| Cargar el `.p12` | `certificado` | `certificado.CargarArchivo`, `certificado.Cargar` |
| RIDE (PDF) | `ride` | `ride.Generar` |
| Correo de entrega | `correo` | `correo.Nuevo`, `correo.DeComprobante` |
| Envío por lotes | `ws` | `ws.Lote`, `ValidarLote`, `AutorizacionLote` |
| Pruebas sin tocar el SRI | `sritest` | `sritest.Nuevo` |

Todos los comprobantes comparten la misma forma de uso: se llena la estructura, `Calcular()` (los que tienen importes), `GenerarClaveAcceso("")`, y se entrega al `Cliente`.

## 4. El flujo, paso a paso

```go
// Una vez, al arrancar la aplicación:
cert, err := certificado.CargarArchivo(rutaP12, contrasena) // contraseña desde un secreto, nunca en el código
cliente, err := sri.Nuevo(sri.Config{
    Ambiente:    comprobante.Pruebas, // o comprobante.Produccion; no hay valor por defecto
    Certificado: cert,
    RUC:         rucEmisor,           // el cliente se niega a firmar para otro RUC
})

// Por cada comprobante:
f := &factura.Factura{ /* infoTributaria, infoFactura, detalles */ }
f.Calcular()                              // totales e impuestos
clave, err := f.GenerarClaveAcceso("")    // UNA vez por comprobante

prep, err := cliente.Preparar(f)          // valida y firma; no usa la red
// → guardar clave y prep.XMLFirmado ANTES de enviar

res, err := cliente.Reanudar(ctx, prep.XMLFirmado, prep.Advertencias) // envía y espera
// → actuar según res.Estado (sección 5)
```

`cliente.Emitir(ctx, f)` hace `Preparar` y `Reanudar` en una sola llamada. Es cómodo para pruebas; en producción prefiera los dos pasos, porque permiten guardar el XML firmado antes de enviarlo.

El `Cliente` es seguro para uso concurrente y debe reutilizarse; cargue el certificado una sola vez.

## 5. Estados: qué significa cada uno y qué hacer

Un comprobante **solo existe tributariamente cuando está AUTORIZADO**. `RECIBIDA` no es autorización.

| `res.Estado` | Qué pasó | Qué hacer |
|---|---|---|
| `sri.EstadoAutorizado` | El SRI lo autorizó, y el documento que devolvió es el enviado: con `Reanudar`, `Emitir` y `ConsultarDocumento`, `res.Verificado` es siempre `true` en este estado | Guardar `res.XMLAutorizado()` (es el documento válido), generar el RIDE, entregar al receptor |
| `sri.EstadoRecibido` | Aceptado para procesar, sin resultado todavía (`res.Pendiente()`) | Guardar como pendiente y consultar después con `cliente.ConsultarDocumento(ctx, xmlFirmado)` (la recepción ya está confirmada). No reemitir |
| `sri.EstadoDevuelto` | La recepción lo rechazó (estructura, clave, emisor…) | Leer `res.Mensajes`, corregir y reenviar con la **misma** clave y el mismo secuencial (ficha 5.10) |
| `sri.EstadoNoAutorizado` | Se recibió pero no se autorizó (firma, diferencias de cálculo…) | Leer `res.Mensajes`, corregir y reenviar con la **misma** clave y el mismo secuencial (ficha 5.10 y 5.12) |
| `sri.EstadoDiscrepancia` | El SRI tiene registrado con esta clave **otro** comprobante (por ejemplo, la versión anterior a una corrección que sí había llegado). `err` es un `*sri.ErrorDiscrepancia` | No entregar `res.XMLFirmado`. Ver qué tiene el SRI en `res.Autorizacion`: si está autorizado, ese es el comprobante válido (`res.Autorizacion.Comprobante`) y una corrección se hace con nota de crédito o de débito; si no lo está, enviar este documento con `Reanudar` (es seguro: si el SRI ya lo tiene en proceso, queda pendiente) |
| `err != nil` | No hubo respuesta (red, tiempo agotado), o el SRI respondió algo que no permite saber el estado (`sri.ErrEstadoIndeterminado`, `sri.ErrRespuestaIncoherente`). **Estado desconocido** | Reintentar `Reanudar` con el **mismo** XML firmado. No generar otra clave ni otro secuencial. No basta con `ConsultarDocumento`: el SRI responde igual a una clave que nunca recibió que a una en proceso, de modo que un comprobante que no llegó quedaría como pendiente para siempre |

Un estado de autorización vacío o que el SDK no conoce no se convierte en `NO AUTORIZADO`: queda `EstadoRecibido` con `sri.ErrEstadoIndeterminado`, para que la aplicación no emita otro comprobante mientras el original puede seguir en curso.

Lo mismo ocurre si el SRI informa `AUTORIZADO` pero no devuelve el comprobante: no se puede comprobar que el documento autorizado sea el suyo, y el resultado es `EstadoRecibido` con `sri.ErrSinComprobante` (que también es un `ErrEstadoIndeterminado`). `res.Autorizacion` lleva lo que respondió el SRI (número y fecha de autorización) para conciliarlo; no entregue el documento como autorizado y vuelva a consultar.

**Varias autorizaciones para una clave.** Si el SRI devuelve más de una (el original rechazado y su corrección, por ejemplo), el resultado es el de la que contiene su documento, sea cual sea el orden en que lleguen. Todas quedan en `res.Autorizaciones`. `cliente.Autorizar(ctx, clave)` solo conoce la clave: informa el estado de la clave con `Verificado` en `false`, y no sirve para saber si el autorizado es su documento; para eso use `ConsultarDocumento`.

**Corregir y reenviar.** Para corregir un comprobante devuelto o no autorizado, cambie los datos, llame a `Calcular()` **sin** volver a generar la clave (conserve `InfoTributaria.ClaveAcceso`) y vuelva a `Preparar` y `Reanudar`. Si la corrección cambia la fecha de emisión, la clave deja de corresponder (regla `clave.coherencia`). El SDK compara lo que el SRI devuelve con el XML enviado: mientras el SRI procesa la corrección, el `NO AUTORIZADO` de la versión anterior no se toma como resultado (queda `EstadoRecibido`), y si el SRI conserva otra versión **autorizada** con esa clave, el resultado es `EstadoDiscrepancia` y no un falso `AUTORIZADO`.

Antes de enviar, `Preparar` puede devolver `*sri.ErrorValidacion`: el comprobante incumple una regla local y no se firmó. Sus `Hallazgos` dicen el campo, la regla y el motivo. Las `Advertencias` no impiden emitir, pero conviene registrarlas.

## 6. Reglas que no se deben romper

1. **Nunca use `float64` para importes.** Use `decimal.Decimal` (`decimal.MustParse("12.50")`). El SRI rechaza diferencias de un centavo (error 52).
2. **Llame a `Calcular()` después del último cambio** y antes de `GenerarClaveAcceso`. Si cambia una línea, vuelva a calcular: `Calcular` deja el comprobante como si se hubiera construido desde cero con los datos nuevos (IVA, ICE deducido, totales y pago único). Lo que usted indicó expresamente (la base o el valor de un ICE específico, un IRBPNR por unidad, un pago que no es el importe total) se respeta: actualícelo usted si depende de lo que cambió, o póngalo a cero para que `Calcular` lo deduzca.
3. **Una clave de acceso por comprobante, generada una vez y guardada.** Un reintento por fallo de red reutiliza la misma clave y el mismo XML firmado. El SDK reconoce las respuestas «clave ya registrada» (43) y «en procesamiento» (70), continúa con la consulta y comprueba que el comprobante autorizado sea el que envió.
4. **Guarde el XML firmado antes de enviarlo** y el XML autorizado después. Sin el firmado no puede reanudar con seguridad.
5. **El secuencial es suyo.** Resérvelo de forma atómica (transacción o secuencia de base de datos). Un comprobante `DEVUELTO` o `NO AUTORIZADO` se corrige y se reenvía con su misma clave y su mismo secuencial (ficha 5.10); uno `AUTORIZADO` no se vuelve a emitir.
6. **Use `comprobante.Hoy()` para la fecha de emisión.** Es la fecha de Ecuador (UTC−5); la del servidor puede ser «mañana» y el SRI rechaza fechas futuras.
7. **No modifique el XML después de firmarlo**, ni lo reformatee, ni le añada espacios: invalida la firma (error 39).
8. **Los textos no llevan saltos de línea ni espacios al inicio o al final.** El SDK lo valida; limpie los datos de entrada.
9. **El ambiente es explícito.** `comprobante.Pruebas` y `comprobante.Produccion` van en `Config.Ambiente` y en `InfoTributaria.Ambiente`, y deben coincidir; si no, el cliente devuelve `sri.ErrAmbiente`. Use configuración distinta por entorno y no comparta secuenciales entre ambientes.
10. **El RIDE se genera solo de comprobantes autorizados**: pase a `ride.Generar` el resultado de `res.XMLAutorizado()`. Un RIDE generado sin la envoltura de autorización y sin `Opciones.FechaAutorizacion` se marca como «VISTA PREVIA: SIN VALIDEZ TRIBUTARIA».
11. **No use `valorDevolucionIva`.** La devolución automática de IVA no está implementada: el SDK rechaza un valor mayor que cero porque no lo descuenta del importe total.
12. **Secretos fuera del código.** La contraseña del `.p12` y las credenciales SMTP vienen del entorno o de un gestor de secretos; el `.p12` no se sube al repositorio. El SDK no escribe la clave privada en registros.

## 7. Qué guardar por cada comprobante

| Dato | De dónde sale | Para qué |
|---|---|---|
| Tipo, establecimiento, punto de emisión y secuencial | Su aplicación | Numeración y búsqueda |
| Clave de acceso (49 dígitos) | `GenerarClaveAcceso` | Identifica al comprobante ante el SRI; es también el número de autorización |
| XML firmado | `prep.XMLFirmado` | Reintentos y auditoría |
| Estado | `res.Estado` | Saber qué falta por hacer |
| Mensajes del SRI | `res.Mensajes` | Diagnóstico de rechazos |
| XML autorizado | `res.XMLAutorizado()` | Documento con validez tributaria; se entrega al receptor y se conserva durante el plazo que exige la normativa |
| Fecha de autorización | `res.FechaAutorizacion()` | Reportes |

El RIDE puede regenerarse en cualquier momento a partir del XML autorizado; no es necesario guardarlo.

## 8. Recetas por comprobante

**Factura.** `factura.LineaIVA(codigo, descripcion, cantidad, precio, comprobante.IVA15)` crea una línea con un impuesto IVA a partir de su precio sin impuestos, y `factura.LineaPrecioFinal`, a partir de su precio final con IVA incluido (ver «Precios sin impuestos y precios finales», más abajo); para ICE o varios impuestos, llene `Detalle.Impuestos`. A consumidor final (`comprobante.IDConsumidorFinal`, identificación `9999999999999`) el límite es USD 50. La versión por defecto es la 1.1.0 (hasta 6 decimales en cantidad y precio). Exportación: `InfoFactura.ComercioExterior = factura.ComercioExteriorExportador` y los campos de incoterm; los países usan los códigos de `comprobante.Paises` (Tabla 25 del SRI, **no** ISO 3166). Reembolso: llene `Factura.Reembolsos` y `Calcular` completa los totales.

**Nota de crédito.** `notacredito.DesdeFactura(f, motivo)` crea la nota que revierte por completo una factura; para una devolución parcial, recorte `Detalles`, ajuste cantidades y vuelva a `Calcular()`, que recalcula el IVA y el ICE deducido de cada línea (un ICE específico copiado de la factura debe ajustarlo usted). Necesita su propio secuencial y clave.

**Nota de débito.** Lista de `Motivos` con su valor y los impuestos sobre el total (`notadebito.ImpuestoIVA(comprobante.IVA15)`).

**Comprobante de retención.** Use la versión 2.0.0 (`retencion.ComprobanteRetencion`); la 1.0.0 (`retencion.ComprobanteV1`) existe para sistemas que aún la emiten. Un `DocSustento` por comprobante de compra, con sus `ImpuestosDocSustento` y sus `Retenciones`. El porcentaje de cada retención debe ser el que el SRI tiene parametrizado para el código; `Validar` avisa si no coincide con el catálogo incluido, que caduca (ver `actualizar-catalogos.md`).

**Guía de remisión.** No tiene fecha de emisión propia: `GenerarClaveAcceso` usa la de inicio del transporte. Para emitir hoy una guía de un traslado futuro use `GenerarClaveAccesoEn(comprobante.Hoy(), "")`. El transporte no puede empezar antes de la fecha de emisión.

**Liquidación de compra.** Como una factura en la que el receptor es el proveedor; no admite consumidor final. `liquidacion.LineaIVA` crea las líneas.

Receptores: notas de crédito y débito, retenciones y liquidaciones exigen identificar al receptor (RUC, cédula, pasaporte o identificación del exterior).

### Precios sin impuestos y precios finales con IVA incluido

Un detalle se define de una de dos maneras, y se pueden mezclar en un mismo comprobante:

| | Precio sin impuestos (por defecto) | Precio final con IVA incluido |
|---|---|---|
| Qué indica la aplicación | `PrecioUnitario`, sin impuestos | `PrecioFinal`: lo que paga el comprador por unidad |
| Constructor | `factura.LineaIVA(…, precioUnitario, codigoIVA)` | `factura.LineaPrecioFinal(…, precioFinal, codigoIVA)` |
| Qué deduce `Calcular` | base = cantidad × precio − descuento; IVA = base × tarifa | importe = cantidad × precio final; base = importe / (1 + tarifa); IVA = importe − base; y el precio unitario |
| Qué es exacto | El IVA respecto de la base | El importe que se cobra |
| IVA total del comprobante | Tarifa sobre la base acumulada | Suma del IVA de las líneas |
| 12,00 al 15 % | 10,43 + 1,56 = 11,99, o 10,44 + 1,57 = 12,01 | 10,43 + 1,57 = 12,00 |

```go
f.Detalles = []factura.Detalle{
    factura.LineaPrecioFinal("ARR-01", "Arriendo de noviembre", decimal.Entero(1), decimal.MustParse("350.00"), comprobante.IVA15),
    factura.LineaPrecioFinal("REC-01", "Recargo por mora", decimal.Entero(1), decimal.MustParse("12.00"), comprobante.IVA15),
}
f.Calcular() // 304,35 + 45,65 y 10,43 + 1,57; importe total 362,00
```

Use el precio final cuando el precio pactado con el cliente ya incluye el IVA y el comprobante debe sumar exactamente ese precio. Use el precio sin impuestos en los demás casos; es el modo de siempre y no cambia.

- **Se declara, no se adivina.** `Validar` solo trata como precio final el detalle que lo declara, y lo contrasta con ese precio: la base más el IVA deben sumar cantidad × `PrecioFinal`, y la base debe ser ese importe entre (1 + tarifa), redondeado. No es un margen de un centavo: 1,00 + 0,16 no es la descomposición de 1,16 y se rechaza. Los demás detalles siguen la regla estricta.
- **`PrecioFinal` no viaja en el XML.** Una factura de precios finales leída con `Parse` se valida por defecto como de precios sin impuestos, y puede dar `iva.valor`. Antes de validarla, recalcularla o hacer su nota de crédito, vuelva a dar el contexto. Si conserva el precio pactado (el contrato, la lista de precios, su base de datos), **ese es el que debe usar**: asigne a cada `Detalle.PrecioFinal` el precio de sus registros, y la factura se comprueba contra él. Solo si no lo tiene, llame a `f.DeclararPreciosFinales()`: no recupera el precio original, lo reconstruye a partir del propio XML ((base + IVA) / cantidad), de modo que solo puede comprobar que cada línea es coherente consigo misma, no que coincide con lo pactado. El SDK nunca lo hace por su cuenta: es una operación que la aplicación pide expresamente. Guarde con cada comprobante si lo emitió con precios finales, y sus precios.
- **Nota de crédito.** `DesdeFactura` copia los precios finales de la factura; si la factura viene de XML, llame antes a `DeclararPreciosFinales` (si no, `Validar` advierte con `origen.precio-final` y la nota puede diferir en centavos de la factura). Para una devolución parcial basta cambiar la cantidad y `Calcular`.
- **Descuento.** `PrecioFinal` es el precio que se cobra, ya rebajado. `Descuento` (sin impuestos) solo informa de la rebaja: sube el precio unitario deducido y no cambia el importe.
- **Límites.** Solo detalles cuyo único impuesto es el IVA: con ICE o IRBPNR indique el precio sin impuestos (`precio-final.impuestos`). Disponible en factura, liquidación de compra y nota de crédito. Cantidades menores que 10 000 y versiones con 6 decimales en el precio unitario (en la factura 1.0.0 y 2.0.0 y la nota de crédito 1.0.0, solo con cantidad 1): fuera de eso puede no existir un precio unitario exacto y `Validar` lo indica con `precio-final.base`.

## 9. Errores frecuentes del SRI

| Código | Mensaje | Causa habitual | Solución |
|---|---|---|---|
| 35 | Archivo no cumple estructura XML | Campo fuera de formato u orden | No debería ocurrir si no se usa `OmitirXSD`; revise datos con saltos de línea o caracteres extraños |
| 39 | Firma inválida | XML alterado tras firmar, certificado revocado o no vigente | Enviar exactamente `prep.XMLFirmado`; revisar el certificado |
| 43 | Clave de acceso registrada | Reenvío de algo ya recibido | El SDK lo trata como recibido, consulta la autorización y compara el documento; si el SRI tiene otro, devuelve `EstadoDiscrepancia` |
| 45 | Secuencial registrado | Secuencial repetido con otra clave | Corregir la numeración |
| 52 | Error en diferencias | Totales, tarifas o códigos que no coinciden con los del SRI | Usar `Calcular()`; revisar catálogos (retenciones, países) |
| 56 / 57 | Establecimiento cerrado / autorización suspendida | Situación del RUC | Regularizar en el SRI |
| 58 | Error en la estructura de clave de acceso | Clave que no corresponde al comprobante | Usar `GenerarClaveAcceso` después del último cambio |
| 65 | Fecha de emisión extemporánea | Fecha fuera del plazo permitido | Emitir con la fecha correcta de Ecuador |
| 70 | Clave de acceso en procesamiento | Reenvío mientras el SRI procesa | Esperar y consultar; el SDK lo hace |

`res.Mensajes` trae el código (`Identificador`), el mensaje y la información adicional, que suele decir el campo exacto.

## 10. Pruebas

- **Pruebas automáticas de su aplicación:** use `sritest.Nuevo()`, un SRI simulado en memoria que valida XSD y firma, con `sritest.CertificadoDePrueba()`, un certificado desechable. `Servidor.FallarProximas(503)` inyecta fallos de red y `ConsultasEnProceso` simula demoras de autorización. Vea `example_test.go`.
- **Ambiente de pruebas del SRI:** `comprobante.Pruebas`. Los comprobantes no tienen validez pero consumen secuenciales de pruebas. Como receptor de pruebas la ficha técnica sugiere la razón social `PRUEBAS SERVICIO DE RENTAS INTERNAS`.
- **Antes de pasar a producción:** emita en pruebas al menos un comprobante de cada tipo que vaya a usar, incluido un rechazo, y compruebe que su aplicación reanuda bien tras un corte de red.

## 11. Entrega al receptor

```go
pdf, err := ride.Generar(res.XMLAutorizado(), &ride.Opciones{Logo: logoPNGoJPEG})
resumen, _ := ride.Resumir(res.XMLAutorizado())
m := correo.DeComprobante(correo.Comprobante{
    Tipo: resumen.Tipo, Numero: resumen.Numero, ClaveAcceso: resumen.ClaveAcceso,
    Emisor: resumen.Emisor, Receptor: resumen.Receptor,
    XML: res.XMLAutorizado(), RIDE: pdf,
})
m.Para = []string{correoDelCliente}
err = clienteCorreo.Enviar(ctx, m)
```

El paquete `correo` es opcional; puede entregar el XML y el RIDE por cualquier otro medio. Sin logotipo, el RIDE reserva su espacio con la leyenda «NO TIENE LOGO», como los del SRI.

## 12. Mantenimiento

El SDK incluye copias de catálogos del SRI (porcentajes de retención, tarifas de IVA, formas de pago, países, esquemas XSD) que cambian con la normativa. Actualice el SDK periódicamente y revise [`actualizar-catalogos.md`](actualizar-catalogos.md) cuando el SRI publique una nueva ficha técnica.

## 13. Lista de comprobación de una integración

- [ ] Importes con `decimal.Decimal` de extremo a extremo (también en la base de datos: texto o `NUMERIC`, no `FLOAT`).
- [ ] Secuenciales reservados de forma atómica, por tipo, establecimiento y punto de emisión.
- [ ] Clave de acceso y XML firmado guardados antes de enviar.
- [ ] Los seis resultados de la sección 5 tratados, incluidos el fallo de red, el estado indeterminado y la discrepancia.
- [ ] Tarea periódica que consulta los comprobantes en `RECIBIDO` con `cliente.ConsultarDocumento`.
- [ ] XML autorizado guardado y entregado al receptor.
- [ ] Ambiente y certificado por configuración, sin secretos en el repositorio.
- [ ] Pruebas automáticas con `sritest` y una ronda en el ambiente de pruebas del SRI.

Más contexto: [`capacidades.md`](capacidades.md) (qué está implementado y con qué límites), [`pruebas-sri.md`](pruebas-sri.md) (qué se ha comprobado contra el SRI real), [`pendientes.md`](pendientes.md) (qué falta) y [`referencia/`](referencia/README.md) (cómo funciona el protocolo). El índice completo está en [`README.md`](README.md).
