# Comprobantes XML

## Tipos y versiones

**Regla oficial:** los esquemas XSD del SRI y los anexos de la ficha. El atributo `version` del elemento raíz selecciona el esquema.

| Comprobante | `codDoc` | Elemento raíz | Versiones | Diferencia entre versiones |
|---|---|---|---|---|
| Factura | 01 | `factura` | 1.0.0, 1.1.0, 2.0.0, 2.1.0 | 1.0.0 y 2.0.0: cantidad y precio con 2 decimales; 1.1.0 y 2.1.0: hasta 6. Las 2.x añaden rubros de terceros y sustitución de guía de remisión |
| Liquidación de compra | 03 | `liquidacionCompra` | 1.0.0, 1.1.0 | 2 o 6 decimales |
| Nota de crédito | 04 | `notaCredito` | 1.0.0, 1.1.0 | 2 o 6 decimales |
| Nota de débito | 05 | `notaDebito` | 1.0.0 | — |
| Guía de remisión | 06 | `guiaRemision` | 1.0.0, 1.1.0 | cantidad con 2 o 6 decimales |
| Comprobante de retención | 07 | `comprobanteRetencion` | 1.0.0, 2.0.0 | La 2.0.0 (formato ATS) agrupa las retenciones por documento de sustento y añade los datos del pago |

Todos los comprobantes de la tabla, en todas sus versiones salvo la nota de crédito 1.0.0, fueron autorizados por el SRI en pruebas con este SDK.

## Estructura común

```xml
<?xml version="1.0" encoding="UTF-8"?>
<factura id="comprobante" version="1.1.0">
  <infoTributaria>…</infoTributaria>   <!-- igual en todos los comprobantes -->
  <infoFactura>…</infoFactura>         <!-- cabecera propia de cada tipo -->
  <detalles>…</detalles>               <!-- cuerpo propio de cada tipo -->
  <infoAdicional>…</infoAdicional>     <!-- opcional, hasta 15 campos -->
  <ds:Signature>…</ds:Signature>       <!-- la añade la firma -->
</factura>
```

- El atributo `id` de la raíz vale siempre `comprobante`; es el destino de la firma.
- `infoTributaria` lleva, en este orden: `ambiente`, `tipoEmision`, `razonSocial`, `nombreComercial` (opcional), `ruc`, `claveAcceso`, `codDoc`, `estab`, `ptoEmi`, `secuencial`, `dirMatriz`, `agenteRetencion` (opcional), `contribuyenteRimpe` (opcional).
- Los elementos van en el **orden exacto del XSD** (son secuencias). Un elemento fuera de orden es el código 35.

## Reglas de serialización

Estas reglas evitan la mayoría de los rechazos por estructura (código 35):

1. **UTF-8**, con declaración XML.
2. **Sin sangrado ni saltos de línea** entre elementos. La ficha (Anexo 1) advierte que los espacios añadidos a los campos son causa de rechazo. **Decisión de este SDK:** escribir todo el documento en una línea tras la declaración.
3. **Los textos no llevan saltos de línea** (los XSD lo prohíben con el patrón `[^\n]*`) ni espacios al principio o al final.
4. **Los contenedores opcionales vacíos se omiten.** `<pagos></pagos>` o `<detallesAdicionales/>` sin hijos no cumplen el XSD.
5. **Fechas** en formato `dd/mm/aaaa`. La fecha es la de Ecuador continental (UTC−5); un servidor en otra zona horaria puede producir una fecha «de mañana», que el SRI rechaza.
6. **Decimales** con punto, sin separador de miles ni notación exponencial. Los importes monetarios llevan 2 decimales; cantidades y precios unitarios, hasta 2 o 6 según la versión.
7. **Caracteres especiales** escapados como exige XML (`&amp;`, `&lt;`, `&gt;`); se comprobó con textos que contienen `ñ & " < > '`.

## Catálogos

Los valores admitidos están en tablas de la ficha, que cambian con la normativa (ver [`../actualizar-catalogos.md`](../actualizar-catalogos.md)):

| Dato | Tabla de la ficha | En este SDK |
|---|---|---|
| Tipo de identificación del receptor | 6 | `comprobante.ID*` |
| Código de impuesto (IVA, ICE, IRBPNR) | 16 | `comprobante.Impuesto*` |
| Tarifas de IVA | 17 | `comprobante.TarifasIVA` |
| Retención: impuesto y porcentaje de IVA | 19 y 20 | `retencion.Impuesto*`, `retencion.PorcentajesIVA` |
| Retención de renta | Catálogo ATS (no está en la ficha) | `retencion.ConceptosRenta()` |
| Formas de pago | 24 | `comprobante.FormasPago` |
| Países | 25 | `comprobante.Paises` |
| Combustibles; códigos de transporte | 30 y 32 | `factura.Combustibles`, `factura.CodAuxTransporte*` |

**Comprobado:**

- Los **códigos de país** son los de la Tabla 25 (los del Catálogo ATS), **no** los de ISO 3166: China es `331`, no `156`. El SRI rechaza con el código 52 un código que no esté en su tabla.
- El **porcentaje de una retención** debe ser el que el SRI tiene parametrizado para su código; si no, código 52.

## Receptores

- Consumidor final: tipo `07`, identificación `9999999999999`, solo en facturas y hasta USD 50 (sección 9.10).
- Notas de crédito y débito, comprobantes de retención y liquidaciones de compra exigen identificar al receptor (RUC, cédula, pasaporte o identificación del exterior).
- **Decisión de este SDK:** no exigir el dígito verificador del RUC, porque el SRI asigna RUC que no cumplen el módulo 11 clásico; se validan el largo, la provincia y el establecimiento. Una cédula con dígito incorrecto es una advertencia (el SRI la trata como advertencia 62).

## Diferencias entre los XSD publicados y la ficha

- **RIMPE:** el XSD publicado solo admite `CONTRIBUYENTE RÉGIMEN RIMPE` en `contribuyenteRimpe`, pero el Anexo 22 exige además `CONTRIBUYENTE NEGOCIO POPULAR - RÉGIMEN RIMPE`. **Decisión de este SDK:** admitir ambas al validar. No está comprobado con el SRI (requiere un emisor de ese régimen).
- **Nota de crédito y devolución de IVA:** el SRI publica dos familias de XSD de nota de crédito y débito incompatibles entre sí: una admite `agenteRetencion` y `contribuyenteRimpe` pero no `valorDevolucionIva`, y la otra al revés. Con la primera se autorizaron las notas de las pruebas. Ver [`../pendientes.md`](../pendientes.md).
- **Firma:** algunos XSD declaran la firma con el prefijo `ns1:` y otros con `ds:`. Lo que cuenta es el espacio de nombres; **comprobado** que el SRI acepta `ds:` en todos.

## Requisitos por actividad

Algunos anexos añaden requisitos según lo que se vende: exportación (Anexo 4), reembolso de gastos (5), factura comercial negociable (11), combustibles (12 y 16), fundas plásticas (18), agente de retención (21), RIMPE (22), materiales de construcción (23), gran contribuyente (24), transporte comercial (25) y RUC del proveedor del sistema de facturación en `infoAdicional` (26). El estado de cada uno en este SDK está en [`../capacidades.md`](../capacidades.md).

Implementación: paquetes [`comprobante`](../../comprobante), [`factura`](../../factura), [`notacredito`](../../notacredito), [`notadebito`](../../notadebito), [`retencion`](../../retencion), [`guiaremision`](../../guiaremision), [`liquidacion`](../../liquidacion) y [`esquema`](../../esquema) (XSD oficiales en `esquema/xsd/`, con su procedencia y SHA-256).
