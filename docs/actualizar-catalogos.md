# Datos que caducan y cómo actualizarlos

El SDK incluye copias de datos que el SRI cambia sin previo aviso. **Deben revisarse periódicamente**: como mínimo cada vez que el SRI publique una nueva versión de la ficha técnica o del Catálogo ATS, y en todo caso **al inicio de cada trimestre**. Un dato desactualizado no rompe el SDK, pero hace que el SRI rechace comprobantes (típicamente con el error 52) o que la validación local dé avisos equivocados.

| Dato | Dónde está en el SDK | Fuente oficial | Versión incluida |
|---|---|---|---|
| **Porcentajes de retención de renta** | `retencion/catalogo_renta.go` (generado) | Catálogo ATS, hoja «TABLAS RETENCIONES», tabla 3.10 | Vigente desde `retencion.CatalogoRentaVigencia`; descargado en `retencion.CatalogoRentaDescarga` |
| Porcentajes de retención de IVA | `retencion.PorcentajesIVA` | Ficha técnica, Tabla 20 | Ficha v2.34 |
| Tarifas de IVA | `comprobante.TarifasIVA` | Ficha técnica, Tabla 17 | Ficha v2.34 |
| Formas de pago | `comprobante.FormasPago` | Ficha técnica, Tabla 24 | Ficha v2.34 |
| Límite de factura a consumidor final | `factura.LimiteConsumidorFinal` | Ficha técnica, sección 9.10 | Ficha v2.34 (USD 50) |
| Leyendas RIMPE y demás requisitos de llenado | `comprobante`, `esquema.Ajustes` | Ficha técnica, anexos | Ficha v2.34 |
| Esquemas XSD | `esquema/xsd/` | <https://www.sri.gob.ec/facturacion-electronica> | Ver `esquema/xsd/PROCEDENCIA.md` |
| Direcciones de los servicios web | `ws.EndpointsDe` | Ficha técnica, secciones 7.2 y 8.2 | Ficha v2.34 |

Los porcentajes de retención de ISD y las tarifas de ICE no están en el SDK.

## Catálogo de retenciones de renta (el que más cambia)

El SRI publicó tres versiones de esta tabla solo entre marzo y agosto de 2026. Por eso:

- `retencion.Validar` trata las discrepancias con el catálogo como **advertencias**, no como errores: si el catálogo incluido está viejo, no debe impedir una emisión correcta. Quien mantenga su catálogo al día puede exigirlo con `OpcionesValidacion{CatalogoRentaEstricto: true}`.
- Los comprobantes con fecha de emisión anterior a `CatalogoRentaVigencia` no se contrastan.
- La referencia definitiva es siempre el SRI, que valida el porcentaje al autorizar.

### Procedimiento de actualización

Requiere Docker (o Python 3 con `xlrd`) y Go.

1. Descargar el **Catálogo ATS** desde <https://www.sri.gob.ec/formularios-e-instructivos1> (enlace «Catálogo ATS», archivo `Catalogo_ATS.xls`) y anotar su SHA-256:

   ```sh
   sha256sum Catalogo_ATS.xls
   ```

2. Volcar el libro a texto:

   ```sh
   docker run --rm -v "$PWD/scripts/catalogo-ats:/tool:ro" -v "$PWD:/data:ro" python:3.12-slim \
     sh -c 'pip install -q xlrd==2.0.1 && python -I /tool/volcar.py /data/Catalogo_ATS.xls' > catalogo.tsv
   ```

3. Generar el código (el script se detiene si la hoja cambió de formato):

   ```sh
   python3 scripts/catalogo-ats/generar.py catalogo.tsv <sha256> <fecha de descarga aaaa-mm-dd> > retencion/catalogo_renta.go
   gofmt -w retencion/catalogo_renta.go
   ```

4. Revisar el cambio con `git diff retencion/catalogo_renta.go`: qué códigos aparecen, desaparecen o cambian de porcentaje, y que `CatalogoRentaVigencia` sea la fecha esperada.

5. Ejecutar `go test ./retencion`. La prueba `TestCatalogoRenta` fija el caso del código 312; si ese porcentaje cambia de verdad, hay que actualizar la prueba.

6. Si se dispone de acceso al ambiente de pruebas del SRI, emitir una retención real con alguno de los códigos modificados (`go run ./internal/qa -ret-desde N`).

7. No versionar `Catalogo_ATS.xls` ni `catalogo.tsv`: solo el archivo generado.

### Limitaciones de la copia incluida

- Solo contiene la tabla vigente, no las históricas: no sirve para validar comprobantes de períodos anteriores.
- Algunos conceptos no tienen un porcentaje comprobable («varios porcentajes», «2/mil», referencias a artículos de ley); para ellos no se emite ningún aviso.
- Cuando el catálogo enumera varios porcentajes (por ejemplo, según convenios de doble imposición), el SDK acepta cualquiera de ellos; no evalúa la condición que hace aplicable cada uno.

## Cuando el SRI publica una nueva ficha técnica

1. Leer el cuadro de cambios de las primeras páginas y localizar las tablas y anexos afectados.
2. Actualizar los datos de la tabla anterior y las citas de versión en el código y la documentación.
3. Descargar de nuevo los XSD, compararlos con `esquema/xsd/` y, si cambiaron, reemplazarlos y actualizar `PROCEDENCIA.md` (la prueba `TestProcedencia` falla hasta que coincidan).
4. Revisar si los `esquema.Ajustes` siguen siendo necesarios (la prueba `TestRimpeNegocioPopular` avisa cuando el XSD publicado ya los incorpora).
5. Repetir las rondas de `docs/pruebas-sri.md` en el ambiente de pruebas.
