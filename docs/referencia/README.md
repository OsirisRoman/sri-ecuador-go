# Referencia técnica independiente del lenguaje

Estas páginas describen **cómo funciona la facturación electrónica del SRI (esquema offline) en la práctica**, de forma que sirvan para implementarla en cualquier lenguaje: Java, Python, Rust, C#, TypeScript u otro. No hace falta saber Go para leerlas.

> **La autoridad normativa es la documentación oficial del SRI**, en particular la *Ficha técnica de comprobantes electrónicos esquema offline* (aquí se cita la versión 2.34) y los esquemas XSD que publica en <https://www.sri.gob.ec/facturacion-electronica>. Esta referencia la resume, la precisa donde es ambigua y anota lo que se comprobó contra el SRI; no la sustituye. Si hay diferencia, manda el SRI.

Cada página separa tres cosas:

- **Regla oficial:** lo que exige la ficha o el XSD, con su sección.
- **Comprobado:** lo que además se verificó contra el ambiente de pruebas del SRI (detalle en [`../pruebas-sri.md`](../pruebas-sri.md)).
- **Decisión de este SDK:** lo que es una elección de implementación y otro SDK puede resolver de otra manera.

| Página | Contenido |
|---|---|
| [Clave de acceso](clave-acceso.md) | Composición de los 49 dígitos y dígito verificador módulo 11 |
| [Comprobantes XML](comprobantes-xml.md) | Estructura común, versiones, serialización, catálogos y causas frecuentes de rechazo |
| [Cálculo y validación](calculo-y-validacion.md) | Aritmética decimal, redondeo, totales e identificadores de las reglas de validación |
| [Firma XAdES-BES](firma-xades.md) | Estructura exacta de la firma, canonicalización, resúmenes y nombre del emisor |
| [Servicios web](servicios-web.md) | SOAP de recepción, autorización, lotes y consultas; estados y reintentos |
| [Vectores de prueba](../../vectores/README.md) | Datos reproducibles para comprobar una implementación paso a paso |

## Qué se puede trasladar a otro lenguaje

| Componente | Paquete en este SDK | Dificultad al portarlo | Qué reutilizar |
|---|---|---|---|
| Clave de acceso | `clave` | Baja | El algoritmo y `vectores/clave-acceso.json` |
| Decimales exactos | `decimal` | Baja: casi todos los lenguajes tienen un tipo decimal (`BigDecimal`, `decimal.Decimal`, `rust_decimal`) | La regla de redondeo y `vectores/calculo-factura.json` |
| Modelos de los comprobantes | `factura`, `notacredito`, … | Media: se pueden generar desde los XSD | Los XSD oficiales (`esquema/xsd/`), el orden de los elementos y las notas de [comprobantes XML](comprobantes-xml.md) |
| Validación XSD | `esquema` | Baja con libxml2, Xerces o `System.Xml`; media sin ellas | Los XSD y el ajuste documentado de RIMPE |
| Reglas de negocio | métodos `Validar` | Media | La tabla de reglas de [cálculo y validación](calculo-y-validacion.md) |
| Firma XAdES-BES | `xades`, `internal/xmldom` | **Alta**: es donde fallan la mayoría de las implementaciones | [Firma XAdES-BES](firma-xades.md) y `vectores/firma/`, que permiten comprobar cada resumen por separado |
| Clientes SOAP | `ws` | Baja: son cuatro operaciones con sobres fijos | [Servicios web](servicios-web.md) |
| Flujo de emisión | `sri` | Baja | La máquina de estados y las reglas de reintento |
| Simulador del SRI | `sritest` | Media | Puede usarse tal cual desde otro lenguaje: es un servidor HTTP (ver abajo) |
| RIDE | `ride`, `internal/pdf` | Media; con una biblioteca de PDF es directa | Los modelos del Anexo 2 de la ficha |

### Usar el simulador desde otro lenguaje

`sritest` es un servidor HTTP que habla el mismo SOAP que el SRI, valida el XSD y verifica la firma. Un programa Go mínimo lo deja escuchando y cualquier cliente, en cualquier lenguaje, puede apuntar a su dirección para probar su firma sin gastar secuenciales del ambiente de pruebas:

```go
srv := sritest.Nuevo()
fmt.Println(srv.URL()) // base de los servicios; las rutas son las del SRI
select {}
```

Un comprobante que el simulador autoriza no está garantizado ante el SRI (ver los límites en [`../pruebas-sri.md`](../pruebas-sri.md)), pero uno que rechaza por XSD o por firma casi con seguridad será rechazado también allí.

## Licencia y reutilización

El código de este repositorio se distribuye bajo **Apache License 2.0**: puede copiarse, modificarse y portarse a otros lenguajes, también con fines comerciales, conservando los avisos de copyright y de licencia y el archivo `NOTICE`. Los materiales de terceros incluidos (esquemas XSD del SRI, esquema XMLDSig del W3C, métricas de fuentes de Adobe) mantienen sus propias condiciones, descritas en [`../../NOTICE`](../../NOTICE) y [`../fuentes.md`](../fuentes.md).

Las reglas del SRI, los algoritmos y los formatos descritos aquí no son propiedad de este proyecto: cualquiera puede implementarlos de forma independiente.
