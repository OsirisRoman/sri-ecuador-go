# sri-ecuador-go

[![CI](https://github.com/OsirisRoman/sri-ecuador-go/actions/workflows/ci.yml/badge.svg)](https://github.com/OsirisRoman/sri-ecuador-go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/OsirisRoman/sri-ecuador-go.svg)](https://pkg.go.dev/github.com/OsirisRoman/sri-ecuador-go)
[![Licencia: Apache 2.0](https://img.shields.io/badge/licencia-Apache%202.0-blue.svg)](LICENSE)
![Estado: pre-1.0](https://img.shields.io/badge/estado-pre--1.0-orange.svg)

SDK de código abierto, nativo en Go, para la **facturación electrónica del SRI de Ecuador** (esquema offline): comprobantes tipados, validación XSD, firma XAdES-BES, servicios web, RIDE en PDF y una referencia técnica para implementarlo en cualquier lenguaje.

> 🌎 **In English:** an open-source, pure-Go SDK for Ecuador's SRI electronic invoicing (*offline scheme*): typed documents for all six voucher types, exact decimal totals, official XSD validation, XAdES-BES signing, SOAP reception/authorization clients, PDF rendering (RIDE) and an in-memory SRI simulator. All six document types have been **authorized by the SRI test environment**. It also ships language-independent protocol notes and test vectors for anyone building an equivalent library in another language. Documentation is in Spanish; start at [`docs/`](docs/README.md).

## 🇪🇨 Construyendo un Ecuador con más oportunidades a través del software y la inteligencia artificial

Creemos que el conocimiento y la tecnología deben ser herramientas para construir oportunidades, no barreras que limiten quién puede innovar.

La inteligencia artificial está transformando la manera en que desarrollamos software. Hoy nos permite explorar tecnologías complejas, comprender problemas, acelerar el aprendizaje y convertir ideas en proyectos que antes podían parecer fuera de nuestro alcance.

**Este proyecto nace de esa convicción: utilizar la inteligencia artificial como aliada del talento humano para crear tecnología abierta que beneficie a todos.**

La IA no sustituye el criterio, la creatividad ni la responsabilidad de los desarrolladores. Potencia nuestra capacidad de investigar, construir, experimentar y encontrar mejores soluciones. El verdadero valor aparece cuando combinamos esas capacidades con el conocimiento, las pruebas rigurosas y la colaboración de una comunidad.

Queremos demostrar que desde Ecuador también podemos impulsar iniciativas tecnológicas que aporten al ecosistema global del software libre.

Que una herramienta desarrollada para resolver una necesidad local pueda servir a cientos de desarrolladores, emprendedores y empresas. Que otros puedan estudiarla, mejorarla, adaptarla a nuevos lenguajes y construir sobre ella sin tener que comenzar desde cero.

**No queremos que este proyecto sea solamente un SDK. Queremos que sea un punto de partida para nuevas ideas, contribuciones y oportunidades.**

Porque el avance tecnológico de nuestro país no depende únicamente de consumir soluciones creadas en otros lugares, sino también de nuestra capacidad para construirlas, compartirlas y mejorarlas juntos.

Si este proyecto ayuda a un estudiante a aprender, a un desarrollador a superar una barrera técnica, a un emprendedor a crear su producto o a otra comunidad a desarrollar una solución mejor, entonces habrá cumplido una parte importante de su propósito.

**El conocimiento crece cuando se comparte. La innovación avanza cuando colaboramos. Y la inteligencia artificial puede ayudarnos a llevar ambas cosas mucho más lejos.**

*Hecho desde Ecuador 🇪🇨, con visión abierta al mundo.*

---

## 📌 Estado del proyecto

**Funcional y probado contra el SRI; versión previa a la 1.0** (última publicada: [v0.1.0](https://github.com/OsirisRoman/sri-ecuador-go/releases/tag/v0.1.0), pre-release). Los seis tipos de comprobante del esquema offline han sido **autorizados por el ambiente de pruebas del SRI** con documentos generados y firmados por este SDK. La API todavía puede cambiar antes de la versión 1.0.

- 🧩 Go puro, **sin cgo**, sin Java, sin Node.js, sin base de datos. Tres dependencias (XSD, PKCS#12 y `x/crypto`).
- 📄 Basado en la *Ficha técnica de comprobantes electrónicos esquema offline* **v2.34** (julio 2026), la versión vigente publicada por el SRI al 15 de noviembre de 2026, y en los XSD oficiales, que van incrustados.
- 🔎 Cada capacidad declara su estado: **verificada en el SRI**, **probada localmente**, **parcial** o **no implementada**. Ver [capacidades](docs/capacidades.md).

## ✅ Probado contra el SRI

No es un SDK teórico: se comprobó enviando comprobantes al **ambiente de pruebas (certificación) del SRI** con un certificado de firma electrónica real. Resultados al **15 de noviembre de 2026**:

| Qué | Resultado en el ambiente de pruebas del SRI |
|---|---|
| Factura v1.0.0, 1.1.0, 2.0.0 y 2.1.0 | **AUTORIZADO** (firma RSA-SHA256 y RSA-SHA1) |
| Factura de exportación y de reembolso de gastos | **AUTORIZADO** |
| Nota de crédito v1.1.0 (total y parcial) y v1.0.0 | **AUTORIZADO** |
| Nota de débito | **AUTORIZADO** |
| Comprobante de retención v2.0.0 y v1.0.0 | **AUTORIZADO** |
| Guía de remisión v1.0.0 y 1.1.0 | **AUTORIZADO** |
| Liquidación de compra v1.0.0 y 1.1.0 | **AUTORIZADO** |
| Envío por lotes | Recibido; resultado por comprobante |
| Manejo de respuestas (reenvío del mismo comprobante ya autorizado, firma alterada, XML fuera de esquema) | El SDK interpreta los códigos del SRI (43, 39, 35); al reenviar, comprueba que el autorizado sea el documento enviado |
| Corrección de un comprobante no autorizado con la misma clave y secuencial (ficha 5.10) | **AUTORIZADO**, y el SDK verifica que es el corregido |
| Base de IVA incoherente con su línea (enviada a propósito, sin la validación local) | **NO AUTORIZADO** (52), como lo rechaza el SDK |

Además:

- Los comprobantes que devuelve el SRI son idénticos, byte a byte, a los enviados.
- Las firmas se verifican con una herramienta independiente (`xmlsec1`), y la canonicalización y la validación XSD se contrastan con `xmllint` (libxml2), en cada cambio, en la integración continua.
- Una auditoría independiente de la versión 0.1.0 encontró validaciones de importes que dejaban pasar comprobantes incoherentes y respuestas del SRI mal interpretadas en casos límite; la versión 0.2.0 corrige cada hallazgo con una prueba de regresión. Ver [docs/auditoria-v0.1.0.md](docs/auditoria-v0.1.0.md).
- Los RIDE en PDF se abrieron con dos visores independientes y su código de barras se leyó con un lector (`zbar`).
- El correo de entrega, con XML y RIDE adjuntos, llegó a la bandeja de entrada por un servidor SMTP real.

El detalle, con las condiciones de las pruebas, está en [docs/pruebas-sri.md](docs/pruebas-sri.md). El repositorio no contiene certificados, contraseñas ni datos del emisor con el que se hicieron las pruebas.

## ⚠️ Límites que conviene conocer

- **Solo el ambiente de pruebas, un emisor y un certificado** (Banco Central del Ecuador). No se ha enviado nada a producción ni se han probado certificados de otras entidades.
- **La devolución automática de IVA (servicios DIG) no está implementada.** El SDK rechaza un `valorDevolucionIva` mayor que cero para no emitir importes incorrectos. La investigación está en [docs/devolucion-iva-dig.md](docs/devolucion-iva-dig.md).
- **Algunos casos solo tienen pruebas locales:** ICE e IRBPNR, factura comercial negociable, retenciones al exterior, entre otros. De las reglas de coherencia de importes de la versión 0.2.0, el SRI confirmó la de la base del IVA; en el IVA de cada línea el SDK es más estricto que el SRI, que tolera un centavo, por decisión de integridad tributaria ([D17](docs/decisiones.md)). Quien pacta precios con IVA incluido (con el 15 %, el 13 % de los precios finales, como 12,00 o 35,00, no es la suma de ninguna base y su IVA) declara el precio final de cada detalle con `LineaPrecioFinal`, y el comprobante suma exactamente ese precio ([D18](docs/decisiones.md)).
- **La validación revisa la coherencia del comprobante, no la ley aplicable:** no sabe qué tarifa de IVA o qué retención corresponde a cada bien o pago, y solo contrasta una nota de crédito o una retención con su documento de origen en la medida en que lo conoce.
- **Los catálogos caducan.** Antes de adoptarlo, compruebe en <https://www.sri.gob.ec/facturacion-electronica> si hay una ficha posterior a la 2.34 y revise [docs/actualizar-catalogos.md](docs/actualizar-catalogos.md).
- El SDK no asigna secuenciales, no guarda comprobantes y no sustituye el criterio tributario.

La lista completa, con lo que falta para cerrar cada punto, está en [docs/pendientes.md](docs/pendientes.md).

## 🚀 Instalación

```sh
go get github.com/OsirisRoman/sri-ecuador-go@v0.1.0
```

Requiere Go 1.26 o superior. Las versiones siguen la [política de versiones](docs/versionado.md); mientras sean 0.x, fije la versión y lea las [notas](CHANGELOG.md) antes de actualizar. Para verlo funcionar sin configurar nada (usa un simulador del SRI y un certificado desechable):

```sh
git clone https://github.com/OsirisRoman/sri-ecuador-go && cd sri-ecuador-go
go run ./ejemplos/emision
```

## 🧾 Uso

```go
cert, err := certificado.CargarArchivo("secrets/firma.p12", os.Getenv("SRI_P12_PASSWORD"))

cliente, err := sri.Nuevo(sri.Config{
    Ambiente:    comprobante.Pruebas, // obligatorio: producción nunca es el valor por omisión
    Certificado: cert,
})

f := &factura.Factura{
    InfoTributaria: comprobante.InfoTributaria{
        Ambiente: comprobante.Pruebas, RazonSocial: "MI EMPRESA S.A.", RUC: "1790000000001",
        Estab: "001", PtoEmi: "001", Secuencial: "000000001", DirMatriz: "Av. Amazonas N34-10",
    },
    InfoFactura: factura.InfoFactura{
        FechaEmision:                comprobante.Hoy(),
        TipoIdentificacionComprador: comprobante.IDCedula,
        RazonSocialComprador:        "CLIENTE DE EJEMPLO",
        IdentificacionComprador:     "1700000001",
        Pagos: []comprobante.Pago{{FormaPago: comprobante.PagoTarjetaCredito}},
    },
    Detalles: []factura.Detalle{
        factura.LineaIVA("SRV-01", "Servicio profesional", decimal.Entero(1), decimal.MustParse("100.00"), comprobante.IVA15),
    },
}
f.Calcular()                    // totales e impuestos con aritmética decimal exacta
clave, _ := f.GenerarClaveAcceso("") // 49 dígitos, módulo 11; guárdela

res, err := cliente.Emitir(ctx, f) // valida → firma → envía → consulta la autorización
switch {
case err != nil:            // no se pudo completar (validación local, red…)
case res.Autorizado():      guardar(res.XMLAutorizado())
case res.Pendiente():       // recibido; consulte luego con cliente.Autorizar(ctx, clave)
default:                    // DEVUELTO o NO AUTORIZADO: revise res.Mensajes
}
```

El SDK distingue cuatro hechos que no deben confundirse:

| Hecho | Cómo se comprueba | Qué significa |
|---|---|---|
| Válido localmente | `Validar()` + `esquema.Validar()` | Cumple las reglas y el XSD |
| Firmado | `xades.Verificar()` | La firma es criptográficamente correcta |
| **Recibido** | `sri.EstadoRecibido` | El SRI lo aceptó para procesarlo. **No es aún un comprobante válido** |
| **Autorizado** | `sri.EstadoAutorizado` | El SRI lo autorizó |

La [**guía de integración**](docs/guia-de-integracion.md) explica el flujo recomendado para producción, qué debe guardar su aplicación, cómo tratar cada estado y los errores frecuentes del SRI. El programa [`ejemplos/emision`](ejemplos/emision/main.go) es su ejemplo de referencia.

🤖 Si desarrolla con un asistente de IA, indíquele que lea [`AGENTS.md`](AGENTS.md) (o [`llms.txt`](llms.txt)): resume las reglas que no debe saltarse y dónde está cada cosa.

### RIDE (PDF) y entrega

```go
if res.Autorizado() {
    pdf, err := ride.Generar(res.XMLAutorizado(), &ride.Opciones{Logo: logoPNGoJPEG})
}
```

`ride.Generar` admite los seis comprobantes, incluye el código de barras de la clave de acceso y no necesita bibliotecas externas. Sin logotipo, su espacio queda reservado con la leyenda «NO TIENE LOGO», como en los RIDE del SRI. El paquete opcional `correo` envía el XML y el RIDE por SMTP.

### Pruebas sin conexión al SRI

```go
srv := sritest.Nuevo()                       // SRI simulado: valida XSD y firma
defer srv.Cerrar()
cert, _ := sritest.CertificadoDePrueba()     // certificado desechable
cliente, _ := sri.Nuevo(sri.Config{
    Ambiente: comprobante.Pruebas, Certificado: cert,
    Servicios: ws.Nuevo(comprobante.Pruebas, ws.ConEndpoints(srv.Endpoints())),
})
```

### Línea de comandos

```sh
cp .env.example .env        # complete sus datos; .env y *.p12 están excluidos de Git
go run ./cmd/sri certificado            # comprueba el .p12 sin enviar nada
go run ./cmd/sri validar factura.xml    # XSD oficial
go run ./cmd/sri firmar factura.xml
go run ./cmd/sri verificar factura.firmado.xml
go run ./cmd/sri ride -logo logo.png factura.autorizado.xml   # RIDE en PDF
```

## 📦 Paquetes

| Paquete | Para qué |
|---|---|
| `sri` (raíz) | Flujo completo: `Preparar`, `Emitir`, `Reanudar`, `Autorizar` |
| `factura` | Modelo tipado de la factura (v1.0.0, 1.1.0, 2.0.0, 2.1.0), totales y validación |
| `notacredito` | Nota de crédito (v1.0.0, 1.1.0); `DesdeFactura` revierte una factura |
| `notadebito` | Nota de débito (v1.0.0) |
| `retencion` | Comprobante de retención (v2.0.0 ATS y v1.0.0) |
| `guiaremision` | Guía de remisión (v1.0.0 y v1.1.0) |
| `liquidacion` | Liquidación de compra (v1.0.0 y v1.1.0) |
| `comprobante` | Tipos, catálogos y cálculo de impuestos comunes |
| `clave` | Clave de acceso: generar, analizar, validar |
| `decimal` | Decimales exactos para importes |
| `esquema` | Validación contra los XSD oficiales incrustados |
| `certificado` | Carga de certificados PKCS#12 (.p12) |
| `xades` | Firma y verificación XAdES-BES |
| `ws` | Clientes SOAP: recepción, autorización, lotes, consulta de validez y de factura negociable |
| `sritest` | Simulador del SRI para pruebas sin conexión |
| `ride` | Representación impresa (RIDE) en PDF de cualquier comprobante |
| `correo` | Opcional: entrega del comprobante autorizado (XML y RIDE) por SMTP |
| `cmd/sri` | Herramienta de línea de comandos |

## 📚 Documentación

Todo está indexado en [`docs/`](docs/README.md):

| Quiero… | Lea |
|---|---|
| Integrar el SDK en mi aplicación | [Guía de integración](docs/guia-de-integracion.md) |
| Saber qué funciona y con qué garantías | [Capacidades y estado](docs/capacidades.md) · [Pruebas contra el SRI](docs/pruebas-sri.md) |
| Saber qué falta | [Pendientes](docs/pendientes.md) |
| Entender el protocolo o implementarlo en otro lenguaje | [Referencia técnica](docs/referencia/README.md) · [Vectores de prueba](vectores/README.md) |
| Contribuir | [CONTRIBUTING.md](CONTRIBUTING.md) |
| Mantenerlo al día con el SRI | [Datos que caducan](docs/actualizar-catalogos.md) |

## 🌐 Una referencia para otros lenguajes

Este repositorio también quiere servir a quien construya una solución equivalente en Java, Python, Rust, C#, TypeScript o cualquier otro lenguaje. La [referencia técnica](docs/referencia/README.md) describe, sin depender de Go, la clave de acceso, la estructura de los XML, el cálculo de totales, la firma XAdES-BES paso a paso y los servicios web, separando lo que exige el SRI, lo que se comprobó contra él y lo que es una decisión de este SDK. Los [vectores de prueba](vectores/README.md) permiten comprobar cada paso con datos ficticios, y el simulador `sritest` puede usarse desde cualquier lenguaje.

La documentación oficial del SRI conserva la autoridad normativa.

## 🤝 Contribuir

Las contribuciones son bienvenidas, y no hace falta empezar por algo grande:

- 🧪 **Cuente cómo le fue contra el SRI**, sobre todo con un certificado de otra entidad o en producción. Es la información que más falta hace y puede aportarse sin exponer datos.
- 🧭 **Tome un pendiente.** [docs/pendientes.md](docs/pendientes.md) explica qué falta en cada caso, qué se sabe y cuándo se considera terminado.
- 🐞 **Reporte un error** o mejore una explicación.
- 🔁 **Avise de cambios del SRI** que dejen desactualizado un catálogo o un esquema.

[CONTRIBUTING.md](CONTRIBUTING.md) explica cómo preparar el entorno, las normas del código y cómo aportar pruebas de interoperabilidad sin exponer certificados ni datos privados. Es un proyecto mantenido por voluntarios: no se garantizan plazos de respuesta.

## 🔐 Seguridad

- Los certificados (`*.p12`), las contraseñas y `.env` están excluidos de Git; nunca los suba.
- La contraseña del `.p12` se lee del entorno o de un archivo; no pasa por argumentos de línea de comandos ni se registra.
- `sri.Config.Ambiente` es obligatorio y el cliente se niega a enviar un comprobante de otro ambiente.

Para reportar una vulnerabilidad, vea [SECURITY.md](SECURITY.md).

## ⚖️ Licencia y avisos

[Apache License 2.0](LICENSE). Los avisos de materiales de terceros están en [NOTICE](NOTICE) y el origen y la licencia de cada fuente en [docs/fuentes.md](docs/fuentes.md).

Este proyecto no está afiliado al Servicio de Rentas Internas ni cuenta con su respaldo. Se distribuye «tal cual», sin garantías: quien lo usa es responsable de verificar que sus comprobantes cumplen la normativa vigente.
