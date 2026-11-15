# sri-ecuador-go

SDK de código abierto, nativo en Go, para la **facturación electrónica del SRI de Ecuador** (esquema offline): modelos tipados, validación XSD real, firma XAdES-BES y clientes SOAP de recepción y autorización.

> **Estado: funcional y probado contra el SRI; en desarrollo activo (pre-1.0).** Los **seis tipos de comprobante** del esquema offline han sido **autorizados por el ambiente de pruebas del SRI** con documentos generados y firmados por este SDK. La API todavía puede cambiar antes de la versión 1.0.

- Go puro, **sin cgo**, sin Java, sin Node.js, sin base de datos.
- Tres dependencias de ejecución (XSD, PKCS#12 y `x/crypto`).
- Basado en la *Ficha técnica de comprobantes electrónicos esquema offline* **v2.34** (julio 2026), que es la versión vigente publicada por el SRI al 15 de noviembre de 2026, y en los XSD oficiales, que van incrustados.

## Probado contra el SRI

No es un SDK teórico: cada pieza se comprobó enviando comprobantes reales al **ambiente de pruebas (certificación) del SRI**, con un certificado de firma electrónica real. Resultados al **15 de noviembre de 2026**:

| Qué | Resultado en el ambiente de pruebas del SRI |
|---|---|
| Factura v1.0.0, 1.1.0, 2.0.0 y 2.1.0 | **AUTORIZADO** (firma RSA-SHA256 y RSA-SHA1) |
| Factura de exportación y de reembolso de gastos | **AUTORIZADO** |
| Nota de crédito (total y parcial) | **AUTORIZADO** |
| Nota de débito | **AUTORIZADO** |
| Comprobante de retención v2.0.0 | **AUTORIZADO** |
| Guía de remisión v1.0.0 y 1.1.0 | **AUTORIZADO** |
| Liquidación de compra v1.0.0 y 1.1.0 | **AUTORIZADO** |
| Envío por lotes | Recibido; resultado por comprobante |
| Rechazos provocados a propósito (firma alterada, XML fuera de esquema, reenvíos) | El SRI respondió con los códigos previstos (39, 35, 43) y el SDK los manejó |

Además:

- Los comprobantes que devuelve el SRI son idénticos, byte a byte, a los enviados.
- Las firmas se verificaron también con una herramienta independiente (`xmlsec1`) y la validación XSD se contrastó con `xmllint`.
- Los RIDE en PDF se abrieron con dos visores independientes y su código de barras se leyó con un lector (`zbar`).
- El correo de entrega, con XML y RIDE adjuntos, llegó a la bandeja de entrada por un servidor SMTP real.
- Dos rechazos reales del SRI durante las pruebas (un porcentaje de retención desactualizado y un código de país que no era de su tabla) se convirtieron en validaciones del SDK.

El detalle de cada ronda, con lo que **no** se ha probado (por ejemplo, certificados de otras entidades emisoras, ICE o el ambiente de producción), está en [docs/pruebas-sri.md](docs/pruebas-sri.md). El repositorio no contiene certificados, contraseñas ni datos del emisor con el que se hicieron las pruebas. Desde este proyecto no se ha enviado nada a producción.

La ficha técnica cambia: antes de adoptar el SDK compruebe en <https://www.sri.gob.ec/facturacion-electronica> si existe una versión posterior a la 2.34 y revise [docs/actualizar-catalogos.md](docs/actualizar-catalogos.md).

## Instalación

```sh
go get github.com/OsirisRoman/sri-ecuador-go
```

Requiere Go 1.26 o superior.

## Uso

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
        RazonSocialComprador:        "JUAN PÉREZ",
        IdentificacionComprador:     "1710034065",
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

Un ejemplo ejecutable completo está en [`example_test.go`](example_test.go).

### El SDK distingue cuatro hechos distintos

| Hecho | Cómo se comprueba | Qué significa |
|---|---|---|
| Válido localmente | `factura.Validar()` + `esquema.Validar()` | Cumple reglas tributarias y XSD |
| Firmado | `xades.Verificar()` | La firma es criptográficamente correcta |
| **Recibido** | `sri.EstadoRecibido` | El SRI lo aceptó para procesarlo. **No es aún un comprobante válido** |
| **Autorizado** | `sri.EstadoAutorizado` | El SRI lo autorizó |

## Cómo integrarlo

La [guía de integración](docs/guia-de-integracion.md) explica el flujo paso a paso, qué significa cada estado, qué debe guardar su aplicación, los errores frecuentes del SRI y una lista de comprobación. El programa [`ejemplos/emision`](ejemplos/emision/main.go) es su ejemplo de referencia y se ejecuta sin configuración: `go run ./ejemplos/emision`.

Si desarrolla con un asistente de IA, indíquele que lea [`AGENTS.md`](AGENTS.md) (o [`llms.txt`](llms.txt)): resume las reglas que no debe saltarse y dónde está cada cosa.

## Paquetes

| Paquete | Para qué |
|---|---|
| `sri` (raíz) | Flujo completo: `Preparar`, `Emitir`, `Reanudar`, `Autorizar` |
| `factura` | Modelo tipado de la factura (v1.0.0, 1.1.0, 2.0.0, 2.1.0), totales y validación |
| `notacredito` | Nota de crédito (v1.0.0, 1.1.0); `DesdeFactura` revierte una factura |
| `notadebito` | Nota de débito (v1.0.0) |
| `retencion` | Comprobante de retención (v2.0.0, ATS) |
| `guiaremision` | Guía de remisión (v1.0.0 y v1.1.0) |
| `liquidacion` | Liquidación de compra (v1.0.0 y v1.1.0) |
| `comprobante` | Tipos, catálogos y cálculo de impuestos comunes |
| `clave` | Clave de acceso: generar, analizar, validar |
| `decimal` | Decimales exactos para importes |
| `esquema` | Validación contra los XSD oficiales incrustados |
| `certificado` | Carga de certificados PKCS#12 (.p12) |
| `xades` | Firma y verificación XAdES-BES |
| `ws` | Clientes SOAP: recepción, autorización, consulta de validez |
| `sritest` | Simulador del SRI para pruebas sin conexión |
| `ride` | Representación impresa (RIDE) en PDF de cualquier comprobante |
| `correo` | Opcional: entrega del comprobante autorizado (XML y RIDE) por SMTP |
| `cmd/sri` | Herramienta de línea de comandos |

## RIDE (PDF)

```go
res, _ := cliente.Emitir(ctx, f)
if res.Autorizado() {
    pdf, err := ride.Generar(res.XMLAutorizado(), &ride.Opciones{Logo: logoPNG})
}
```

`ride.Generar` admite los seis tipos de comprobante, incluye el código de barras de la clave de acceso y no necesita bibliotecas externas. Sin logotipo, su espacio queda reservado con la leyenda «NO TIENE LOGO», igual que en los RIDE del SRI (`SinLeyendaLogo` lo deja en blanco). Si recibe una autorización cuyo estado no es `AUTORIZADO`, devuelve `ride.ErrNoAutorizado`. El documento con validez tributaria sigue siendo el XML.

## Envío por lotes

```go
lote := ws.Lote{ClaveAcceso: claveDelLote, RUC: ruc, Comprobantes: firmados} // hasta 50 y 500 kB
rec, _ := servicios.ValidarLote(ctx, lote)
if rec.Recibida() {
    res, _ := servicios.AutorizacionLote(ctx, lote.ClaveAcceso) // una autorización por comprobante
}
```

La clave del lote se genera como la de un comprobante (`clave.Generar`), con una serie y un secuencial que el emisor reserva para sus lotes. El SRI resuelve cada comprobante por separado.

## Pruebas sin conexión al SRI

`sritest` levanta un SRI simulado que valida XSD y firma, y permite inyectar cortes de red y demoras de procesamiento:

```go
srv := sritest.Nuevo()
defer srv.Cerrar()
servicios := ws.Nuevo(comprobante.Pruebas, ws.ConEndpoints(srv.Endpoints()))
cliente, _ := sri.Nuevo(sri.Config{Ambiente: comprobante.Pruebas, Certificado: cert, Servicios: servicios})
```

## Línea de comandos

```sh
cp .env.example .env        # complete sus datos; .env y *.p12 están excluidos de Git
go run ./cmd/sri certificado            # comprueba el .p12 sin enviar nada
go run ./cmd/sri validar factura.xml    # XSD oficial
go run ./cmd/sri firmar factura.xml
go run ./cmd/sri verificar factura.firmado.xml
go run ./cmd/sri ride -logo logo.png factura.autorizado.xml   # RIDE en PDF
```

## Mantenimiento: datos que caducan

El SDK incluye copias de catálogos del SRI (porcentajes de retención, tarifas de IVA, formas de pago, esquemas XSD) que cambian con la normativa. **Deben revisarse periódicamente**; el procedimiento está en [docs/actualizar-catalogos.md](docs/actualizar-catalogos.md). El catálogo de retenciones de renta declara su vigencia en `retencion.CatalogoRentaVigencia`.

## Seguridad

- La contraseña del certificado se lee de variables de entorno o de un archivo; nunca de argumentos de línea de comandos.
- La clave privada no se escribe en disco ni en logs y `fmt` no puede volcarla.
- El analizador XML rechaza DOCTYPE (sin XXE ni expansión de entidades) y limita tamaño y profundidad.
- El verificador de firma rechaza identificadores duplicados y exige que la firma cubra el comprobante completo.
- Para reportar una vulnerabilidad, use los *Security Advisories* privados del repositorio.

## Documentación

- [Capacidades y limitaciones](docs/capacidades.md)
- [Decisiones técnicas](docs/decisiones.md)
- [Pruebas contra el SRI](docs/pruebas-sri.md)
- [Datos que caducan y cómo actualizarlos](docs/actualizar-catalogos.md)
- [Fuentes y licencias](docs/fuentes.md)

## Licencia

[Apache 2.0](LICENSE). Vea también [NOTICE](NOTICE). Proyecto independiente, no afiliado al SRI.
