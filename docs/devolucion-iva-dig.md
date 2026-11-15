# Devolución automática del IVA (servicios DIG)

**Estado: no implementado.** El SDK no incluye un cliente de los servicios DIG. Queda pendiente de implementación y validación porque depende de elementos externos que no se han podido resolver: una habilitación del SRI cuyo procedimiento no está documentado y detalles del protocolo que la ficha técnica no define. Este documento conserva lo investigado para facilitar una implementación futura.

No bloquea el resto del SDK: los comprobantes se emiten sin estos servicios. Solo los necesita un emisor que aplique en el punto de venta la devolución automática del IVA a personas adultas mayores o con discapacidad.

## Qué es

El emisor consulta al SRI, antes de emitir la factura o la nota de débito, cuánto IVA puede devolver al beneficiario; el SRI responde con el monto y el emisor lo declara en el comprobante. Según la información pública del SRI, el beneficio se aplica en establecimientos comerciales autorizados.

Lo que el SDK **sí** admite hoy es el campo del comprobante: `valorDevolucionIva` en los totales de la factura y en los impuestos de la nota de débito (Anexo 20). El SDK no puede obtener ese valor del SRI; quien lo use debe conseguirlo por su cuenta. `Calcular` y `Validar` todavía no restan la devolución del importe total, como exige el Anexo 20, y el campo no se ha probado contra el SRI: hoy no es un flujo utilizable de extremo a extremo.

## Referencias oficiales

Ficha técnica de comprobantes electrónicos esquema offline, v2.34:

- **Sección 12:** códigos de error para la aplicación de la devolución automática del IVA.
- **Sección 13:** servicios web para la devolución automática del IVA a personas adultas mayores (DIG).
- **Anexo 20:** campo `valorDevolucionIva` en facturas, notas de crédito y notas de débito.
- **Sección 11, código 92:** el SRI rechaza el comprobante si el valor de devolución no coincide con el autorizado por el servicio DIG.

## Requisitos técnicos identificados

### Ambientes

| Ambiente | Dirección base |
|---|---|
| Certificación (pruebas) | `https://celcer.sri.gob.ec/devolucion-iva/rest` |
| Producción | `https://srienlinea.sri.gob.ec/devolucion-iva/rest` |

### Autenticación

Los servicios no usan el certificado de firma. Usan un token OAuth2 que se obtiene con un **usuario adicional** del portal SRI en Línea: RUC del contribuyente, cédula del usuario adicional y su clave cifrada con SHA-512. La ficha documenta esta dirección (petición GET):

```
<host>/sri-seguridad-sso-api-servicio-internet/rest/seguridad-sso-rest/access-token/RUC_CONTRIBUYENTE[AD]CEDULA_ADICIONAL/CLAVE_ENCRIPTADA_ADICIONAL_SHA-512
```

- El token tiene una vigencia de 35 minutos.
- Se envía en cada llamada en la cabecera `Authorization`.
- Los importes van con dos decimales.

### Servicios (todos POST, con el token en la cabecera)

| Servicio | Ruta | Entrada | Salida |
|---|---|---|---|
| Lista de beneficiarios | `/devolucionesBeneficiarios` | `ruc` | Archivo zip `cedulas_canal` con las identificaciones habilitadas; se actualiza a diario entre las 0:30 y las 2:00 |
| Recepción por lotes | `/devolucionesLotesRecepciones` | Lista de hasta 10 000 elementos: `rucEmisor`, `claveAccesoComprobante`, `idBeneficiario`, `baseImponible`, `porcentajeIva`, `montoIva` | `codigoLote` y mensaje |
| Respuesta de un lote | `/devolucionesLotesRespuestas` | `codigoLote` | `listaDescuento` (por comprobante: `claveAcceso`, `valor`, `descripcion`), `codigo`, `mensaje` |
| Recepción individual | `/devolucionesIndividualesRecepciones` | Lo anterior más `codigoBeneficio` (el código que el beneficiario entrega en caja) | `montoIvaDevolver`, `codigo`, `mensaje` |
| Anulación | `/devolucionesIndividualesAnulaciones` | Lo anterior más `montoIvaDevolver` | `mensaje` |

En certificación, el código de confirmación de los beneficiarios que figuran en la lista es `1234`.

### Reglas del comprobante (Anexo 20)

- El comprador se identifica con cédula (tipo 05).
- `valorDevolucionIva` debe ser mayor o igual a cero y menor o igual al valor del impuesto.
- Si es mayor que cero, la clave de acceso debe estar registrada en el control de saldos del beneficiario y el monto debe ser igual al autorizado por el servicio.
- El importe total de la factura, el valor de modificación de la nota de crédito y el valor total de la nota de débito restan el valor de devolución.

### Códigos de respuesta (sección 12)

| Código | Nombre | Descripción |
|---|---|---|
| 2000 | EXITO | Éxito |
| 2001 | EXITO_VALIDACION | Validación exitosa |
| 3000 | ERROR_FORMATO | Los campos no cumplen el formato establecido |
| 3001 | ERROR_TRANSACCION | No se logró hacer la transacción |
| 3003 | ERROR_CLAVE_YA_PROCESADA | Comprobante ya procesado |
| 3004 | ERROR_CODIGO_OPERACION_INVALIDO | Código de operación inválido |
| 3005 | ERROR_INTERNO_SERVIDOR | Se ha producido un error |
| 3006 | ERROR_TIME_OUT | No se ha podido responder a tiempo |
| 3007 | ERROR_CODIGO_BENEFICIO | El código de verificación no se encuentra vigente |
| 3008 | ERROR_WS_NO_DISPONIBLE | Servicio web no disponible |
| 3009 | ERROR_WS_NO_AUTORIZADO | No está autorizado |
| 3011 | ERROR_NO_MONTO_MINIMO | El valor no cumple el monto mínimo a devolver |
| 3013 | ERROR_TARIFA_IMPUESTO | La tarifa del impuesto calculado no coincide con la enviada |
| 3014 | ERROR_TOKEN_BENEFICIARIO | El token no pertenece al emisor |
| 3015 | ERROR_CANAL_AUTOMATICO_NO_HABILITADO_SALDO | El beneficiario no tiene el canal automático habilitado o no existe un monto a devolver |
| 3016 | ERROR_MULTIPLE_TRANSACCIONES_LOTE | El beneficiario registra más de una transacción en el lote |
| 4000 | LOTE_RECIBIDO | Lote recibido |
| 4001 | LOTE_RECHAZADO | Lote rechazado |
| 4002 | LOTE_EN_PROCESO | Lote en proceso |
| 4003 | LOTE_PROCESADO | Lote procesado |
| 5001 | ERROR_CONFIGURACION_PILOTO | Los parámetros (RUC, fecha de inicio o de fin) para el piloto no están configurados |
| 6000 | ANULACION_ERROR | Error al realizar la anulación |
| 6001 | ANULACION_COMPROBANTE_NO_ENCONTRADO | Comprobante electrónico no encontrado |
| 6002 | ANULACION_IVA_DEVOLVER_DIFERENTES | El IVA a devolver no es igual al registrado en el débito |
| 6004 | ANULACION_BENEFICIO_INCORRECTO | El código del beneficio no corresponde al registrado en el SRI |
| 6005 | ANULACION_FECHA_DIFERENTE_HOY | La fecha de emisión no corresponde a la de hoy |
| 6006 | ANULACION_TIPO_COMPROBANTE_INCORRECTO | El comprobante debe ser una factura o una nota de débito |
| 6007 | ANULACION_ENVIADA_EXITO | Anulación enviada con éxito |
| 6008 | ANULACION_IVA_DEVOLVER_NO_ENCONTRADO | Falta el monto del IVA a devolver |
| 6009 | ANULACION_IDENTIFICACIONES_DIFERENTES | La identificación no corresponde a la clave de acceso |
| 6010 | ANULACION_IVA_CALCULADO_DIFERENTES | El cálculo del IVA a devolver no es igual al registrado en el débito |
| 6011 | ANULACION_BASE_IMPONIBLE_DIFERENTES | La base imponible no es igual a la registrada en el débito |
| 6012 | ANULACION_NO_SALDO_DISPONIBLE | No tiene saldo disponible |
| 7000 | BENEFICIARIOS_ARCHIVO_NO_ENCONTRADO | No existen registros en el canal automático |

## Lo que falta resolver antes de implementarlo

**Habilitación.** La ficha no indica qué permiso del portal necesita el usuario adicional ni si el RUC del emisor requiere un registro o autorización específicos para DIG, distintos de la habilitación para emitir comprobantes electrónicos. Los códigos 3009, 3014 y 5001 indican que el SRI aplica controles de autorización, pero no cómo se obtiene esta. Hay que confirmarlo con el SRI.

**Protocolo.** La ficha deja sin definir:

- Qué representa `[AD]` en la dirección del token: texto literal, un separador o una parte opcional. La tabla de parámetros indica 13 caracteres para el usuario, que es la longitud del RUC solo.
- La codificación del SHA-512 de la clave (hexadecimal o Base64; mayúsculas o minúsculas).
- El formato de la respuesta del token y si la cabecera `Authorization` lleva algún prefijo.
- Varias tramas JSON solo se describen por sus campos, sin un ejemplo completo.

Resolverlo exige documentación adicional del SRI o pruebas con un emisor habilitado. No conviene deducirlo por tanteo: cada intento fallido es un inicio de sesión fallido sobre un usuario real.

## Cómo retomarlo

1. Confirmar con el SRI el requisito de habilitación y obtener un usuario adicional para el ambiente de certificación.
2. Con esas credenciales (fuera del repositorio), fijar el formato de autenticación con el menor número de intentos posible.
3. Implementar un paquete independiente, opcional como `correo`, que no añada dependencias al flujo de emisión; cubrirlo con un simulador y registrar en `pruebas-sri.md` lo que se compruebe contra el SRI.
4. Conectar el monto obtenido con `valorDevolucionIva` y con el cálculo de totales (el importe total debe restar la devolución), y probar el comprobante resultante, incluido el código 92.
