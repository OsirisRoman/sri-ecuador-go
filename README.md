# sri-ecuador-go

SDK de código abierto, nativo en Go, para la **facturación electrónica del SRI de Ecuador** (esquema offline): modelos tipados, validación XSD real, firma XAdES-BES y clientes SOAP de recepción y autorización.

> **Estado: en desarrollo activo (pre-1.0).** El flujo completo de **factura**, **nota de crédito**, **nota de débito** y **comprobante de retención** funciona: el ambiente de certificación del SRI ha **autorizado** comprobantes generados y firmados con este SDK. Faltan los demás comprobantes y más casos reales; consulte [docs/pruebas-sri.md](docs/pruebas-sri.md) para saber qué está comprobado y qué no. La API puede cambiar.

- Go puro, **sin cgo**, sin Java, sin Node.js, sin base de datos.
- Tres dependencias de ejecución (XSD, PKCS#12 y `x/crypto`).
- Basado en la *Ficha técnica de comprobantes electrónicos esquema offline* **v2.34** (julio 2026) y en los XSD oficiales, que van incrustados.

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

## Paquetes

| Paquete | Para qué |
|---|---|
| `sri` (raíz) | Flujo completo: `Preparar`, `Emitir`, `Reanudar`, `Autorizar` |
| `factura` | Modelo tipado de la factura (v1.0.0, 1.1.0, 2.0.0, 2.1.0), totales y validación |
| `notacredito` | Nota de crédito (v1.0.0, 1.1.0); `DesdeFactura` revierte una factura |
| `notadebito` | Nota de débito (v1.0.0) |
| `retencion` | Comprobante de retención (v2.0.0, ATS) |
| `comprobante` | Tipos, catálogos y cálculo de impuestos comunes |
| `clave` | Clave de acceso: generar, analizar, validar |
| `decimal` | Decimales exactos para importes |
| `esquema` | Validación contra los XSD oficiales incrustados |
| `certificado` | Carga de certificados PKCS#12 (.p12) |
| `xades` | Firma y verificación XAdES-BES |
| `ws` | Clientes SOAP: recepción, autorización, consulta de validez |
| `sritest` | Simulador del SRI para pruebas sin conexión |
| `correo` | Opcional: entrega del comprobante autorizado por SMTP |
| `cmd/sri` | Herramienta de línea de comandos |

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
