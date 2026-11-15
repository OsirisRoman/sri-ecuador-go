# Cálculo y validación

## Aritmética

- **No use coma flotante binaria** (`float`, `double`) para importes: `0.1 + 0.2` no es `0.3`, y el SRI rechaza diferencias de un centavo (código 52). Use un tipo decimal exacto.
- **Redondeo:** a 2 decimales, **mitad hacia arriba en magnitud** (`2.345 → 2.35`, `−2.345 → −2.35`). Es `RoundingMode.HALF_UP` en Java y `ROUND_HALF_UP` en Python. El redondeo «bancario» (mitad al par), que es el valor por defecto en varios lenguajes, da resultados distintos.
- Las operaciones intermedias se hacen exactas y solo se redondea el resultado que se escribe en el XML.

## Fórmulas de la factura

Son las mismas en la liquidación de compra y, con otros nombres de campo, en la nota de crédito.

| Campo | Fórmula |
|---|---|
| `detalle.precioTotalSinImpuesto` | `redondear(cantidad × precioUnitario − descuento, 2)` |
| Base imponible del IVA de una línea | `precioTotalSinImpuesto` más el ICE de esa misma línea, que integra la base del IVA |
| Valor del IVA de una línea | `redondear(base × tarifa / 100, 2)` |
| `totalSinImpuestos` | suma de los `precioTotalSinImpuesto` |
| `totalDescuento` | suma de los descuentos de las líneas |
| `totalConImpuestos` | un `totalImpuesto` por cada combinación de código de impuesto, código de porcentaje y tarifa presente en las líneas, con la suma de sus bases |
| `importeTotal` | `totalSinImpuestos + impuestos + propina` más, si existen, flete, seguro, gastos aduaneros y de transporte (exportación) y rubros de terceros, menos compensaciones |

**Valor del IVA en los totales.** Hay dos criterios en uso que difieren en centavos: aplicar la tarifa a la base acumulada del grupo, o sumar el IVA ya redondeado de cada línea. **Decisión de este SDK:** calcular sobre la base acumulada y, al validar un comprobante hecho por otro sistema, aceptar cualquiera de los dos. **Comprobado:** el SRI autorizó facturas calculadas sobre la base acumulada sin el código 52.

**Tarifa.** Para los códigos de IVA con tarifa fija (Tabla 17), la tarifa del XML debe ser la de la tabla. El código 8 (IVA diferenciado) no fija la tarifa.

**Reembolsos (Anexo 5).** Con reembolsos, `codDocReembolso` es `41` y los tres totales son: suma de las bases, suma de los impuestos y la suma de ambas. **Comprobado** en factura.

**Exportación (Anexo 4).** `comercioExterior` vale `EXPORTADOR` y exige los incoterms y los puertos. **Comprobado:** el importe total incluye flete y seguro.

**Devolución de IVA (Anexo 20).** El importe total debe restar `valorDevolucionIva`, cuyo monto autorizan los servicios DIG. **No implementado en este SDK**, que rechaza cualquier valor mayor que cero; ver [`../devolucion-iva-dig.md`](../devolucion-iva-dig.md).

## Otros comprobantes

- **Nota de débito:** `totalSinImpuestos` es la suma de los motivos; los impuestos se declaran sobre ese total; `valorTotal = totalSinImpuestos + impuestos`.
- **Comprobante de retención:** `valorRetenido = redondear(baseImponible × porcentajeRetener / 100, 2)`. En la versión 2.0.0, `importeTotal` del documento de sustento es `totalSinImpuestos` más sus impuestos, y el período fiscal (`mm/aaaa`) suele ser el mes de emisión.
- **Guía de remisión:** no tiene importes.

## Vectores de cálculo

[`vectores/calculo-factura.json`](../../vectores/calculo-factura.json) contiene casos con las entradas (cantidad, precio, descuento, código de IVA) y todos los resultados esperados, incluidos un redondeo en el límite y el caso de tres líneas de 0,33 en el que los dos criterios de IVA difieren.

## Reglas de validación

Este SDK aplica dos niveles antes de firmar: el **XSD oficial** (forma) y **reglas de negocio** (contenido). Cada regla de negocio tiene un identificador estable, que conviene conservar en otras implementaciones para poder comparar resultados. Un hallazgo es un **error** (el SRI devolvería o no autorizaría el comprobante) o una **advertencia** (no impide emitir).

| Identificador | Qué comprueba | Origen |
|---|---|---|
| `requerido` | Campo o bloque obligatorio ausente | XSD y ficha |
| `longitud`, `formato`, `digitos`, `decimales`, `negativo` | Largo máximo, patrón, número de dígitos y de decimales, y que los importes no sean negativos | Facetas de los XSD |
| `salto-de-linea`, `espacios` | Textos con saltos de línea o con espacios al principio o al final | Anexo 1 |
| `catalogo` | Valor fuera de la tabla correspondiente | Tablas de la ficha |
| `version` | Versión inexistente, o bloque que no existe en esa versión | XSD |
| `maximo` | Más de 15 campos adicionales o de 3 detalles adicionales | Sección 9.11 y XSD |
| `clave.formato`, `clave.coherencia` | Clave de acceso mal formada o que no coincide con el comprobante | Código 58 |
| `ruc.formato`, `cedula.formato`, `cedula.verificador` (advertencia) | Identificaciones | Tabla 6; advertencia 62 |
| `consumidor-final.no-permitido`, `.identificacion`, `.monto` | Uso del consumidor final | Tabla 6 y sección 9.10 |
| `fecha.futura` | Fecha de emisión posterior a la fecha de Ecuador | Código 65 |
| `fecha.sustento` | Documento modificado o de sustento posterior al comprobante | Coherencia |
| `fecha.transporte`, `fecha.inicio-transporte` | Fin del transporte anterior al inicio; inicio anterior a la emisión | Código 82 |
| `detalle.total`, `iva.tarifa`, `iva.valor` | Aritmética de cada línea | Código 52 |
| `totales.sin-impuestos`, `totales.descuento`, `totales.impuesto`, `totales.impuesto-faltante`, `totales.impuesto-sin-detalle`, `totales.base`, `totales.importe` | Aritmética de los totales | Código 52 |
| `pagos.suma` (advertencia) | Los pagos no suman el importe total | Coherencia |
| `exportacion.comercio-exterior` | Campos de exportación sin `comercioExterior` | Anexo 4 |
| `pais.catalogo` (advertencia) | Código de país fuera de la Tabla 25 | Código 52 |
| `reembolso.codigo`, `reembolso.totales` | Reembolsos sin el código 41 o con totales que no cuadran | Anexo 5 |
| `negociable.direccion`, `negociable.plazo` | Requisitos de la factura comercial negociable | Anexo 11 |
| `combustible.placa`, `transporte.placa`, `placa.formato` (advertencias) | Placa del vehículo | Anexos 12, 16 y 25 |
| `retencion.porcentaje`, `retencion.valor` | Porcentaje de retención de IVA y valor retenido | Tabla 20 |
| `retencion.codigo-renta`, `retencion.porcentaje-renta` (advertencias, o errores en modo estricto) | Código y porcentaje de retención de renta | Catálogo ATS; código 52 |
| `periodo` (advertencia) | Período fiscal distinto del mes de emisión | Coherencia |
| `sustento.incompleto` (advertencia), `cantidad.cero` (advertencia) | Guía de remisión | Coherencia |
| `devolucion-iva.no-admitida` | `valorDevolucionIva` mayor que cero | Limitación de este SDK |

Las reglas basadas en catálogos que caducan (retención de renta, países) son advertencias a propósito: una copia desactualizada del catálogo no debe impedir emitir un comprobante correcto.

Implementación: [`decimal`](../../decimal/decimal.go), [`comprobante/calculo.go`](../../comprobante/calculo.go), [`comprobante/validacion.go`](../../comprobante/validacion.go) y los métodos `Calcular` y `Validar` de cada comprobante.
