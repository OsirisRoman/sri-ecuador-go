# Decisiones técnicas

Cada entrada indica qué se decidió, por qué y qué alternativa se descartó. Las marcadas **(revisable)** dependen de resultados de pruebas reales.

## D1. Árbol XML y canonicalización propios
`encoding/xml` descarta los prefijos y no canonicaliza. En lugar de sumar `etree` + `goxmldsig` (como las referencias en Go), `internal/xmldom` implementa en ~450 líneas un árbol que conserva prefijos y la canonicalización C14N 1.0 inclusiva, que es la única que usa el perfil del SRI.
*Verificación:* casos de la recomendación C14N, y firmas validadas por una implementación independiente (`xmlsec1`/OpenSSL).

## D2. Firma: estructura del Anexo 14, RSA-SHA256 por defecto
La ficha (6.8) dice literalmente «Algoritmo de firmado: RSA-SHA1» y el Anexo 14 muestra SHA-1 en firma y digest. SHA-1 está en desuso criptográfico. Se planteó como experimento: el 2026-11-14 el ambiente de certificación del SRI **autorizó** una factura firmada por este SDK con RSA-SHA256 (firma y los tres digest en SHA-256). Con esa evidencia, y por decisión del propietario del proyecto, **SHA-256 es el valor por defecto**.
RSA-SHA1 sigue disponible (`xades.SHA1`) por ser el algoritmo que nombra la norma; el SRI también lo autorizó en la misma fecha.
La firma lleva las tres referencias del Anexo 14 (SignedProperties, KeyInfo y comprobante) y nunca se entrega sin pasar la verificación del propio SDK.

## D3. `X509IssuerName` con formato RFC 2253 al estilo Java
El SRI valida con bibliotecas Java. `pkix.Name.String()` de Go usa palabras clave (`SERIALNUMBER`, `POSTALCODE`) que Java no emite. `xades.NombreRFC2253` trabaja sobre el DER del emisor y reproduce la salida de `X500Principal.getName(RFC2253)`: solo las palabras clave de RFC 2253 y el resto como OID con valor hexadecimal.

## D4. Decimales propios
Un entero de precisión arbitraria con escala. Sin dependencia externa porque solo hacen falta suma, resta, producto, porcentaje y redondeo. Redondeo «mitad hacia arriba».

## D5. Total de IVA sobre la base acumulada
`Calcular` obtiene el IVA de cada grupo como `suma de bases × tarifa`, no como suma de los IVA de cada línea. `Validar` acepta ambos criterios, porque los dos son de uso corriente y difieren en centavos. El SRI autoriza ambos (comprobado en la ronda de certificación de 0.2.0).

## D6. No se exige el dígito verificador del RUC
El SRI asigna RUC que no cumplen el módulo 11 clásico; exigirlo rechazaría contribuyentes reales. Se valida forma y provincia. La cédula con dígito incorrecto es *advertencia*, igual que en el SRI (código 62).

## D7. Ajustes a los XSD publicados, explícitos y en memoria
El XSD publicado solo admite `CONTRIBUYENTE RÉGIMEN RIMPE`, pero el Anexo 22 exige además `CONTRIBUYENTE NEGOCIO POPULAR - RÉGIMEN RIMPE`. Los archivos se incrustan intactos; la corrección se aplica al cargar y está declarada en `esquema.Ajustes`. `esquema.ValidarEstricto` usa los archivos literales. **(revisable)**: falta confirmar con el SRI real, lo que requiere un emisor que sea negocio popular.

## D8. Reintentar el envío es seguro, y el resultado se contrasta con lo enviado
Reenviar un comprobante ya recibido no lo duplica: el SRI responde 43 o 70 (el 43 está confirmado con el SRI real). El cliente lo interpreta como «ya recibido» y pasa a consultar la autorización, lo que hace idempotente a `Reanudar` **para el mismo documento**.
Un 43 no prueba que el SRI tenga ese documento: la ficha (5.10) obliga a reenviar un comprobante corregido con la misma clave, así que puede haber otra versión registrada. Por eso `Reanudar` y `ConsultarDocumento` comparan el comprobante que devuelve la autorización con el enviado mediante `xades.Huella` (SHA-256 de la forma canónica sin la firma: lo que firma la referencia `#comprobante`). Se descartaron la comparación byte a byte (el SRI podría reserializar) y la de `SignatureValue` (dos firmas del mismo contenido serían una «discrepancia» falsa). Si difieren, el resultado es `EstadoDiscrepancia`. Un estado de autorización vacío o desconocido, o una respuesta de otra clave o de otro ambiente, se trata como desconocido (`EstadoRecibido` con error) y nunca como rechazo.

## D9. El ambiente es obligatorio y se comprueba
`sri.Config.Ambiente` no tiene valor por defecto y el cliente rechaza comprobantes de otro ambiente. Un ambiente desconocido nunca resuelve a las direcciones de producción.

## D10. Nombres en español para el dominio
Tipos y campos siguen las etiquetas XML del SRI (`InfoTributaria`, `RazonSocial`), de modo que el código se lee junto a la ficha técnica.

## D11. `comprobante.Lista[T]`
`encoding/xml` escribe `<pagos></pagos>` para un slice vacío con `pagos>pago,omitempty`, y el XSD lo rechaza. `Lista[T]` es un slice normal cuya serialización omite el contenedor vacío.

## D12. Catálogo de retenciones de renta: incluido, generado y no bloqueante
El SRI rechaza con el error 52 una retención cuyo porcentaje no coincide con el que tiene parametrizado para su código. Para detectarlo antes de enviar se incluye la tabla 3.10 del Catálogo ATS, generada desde el archivo oficial con `scripts/catalogo-ats/` (no transcrita a mano) y con su procedencia en constantes. Como la tabla cambia varias veces al año, las discrepancias son **advertencias** salvo que se pida el modo estricto, y no se contrastan comprobantes anteriores a su vigencia. El mantenimiento está descrito en `docs/actualizar-catalogos.md`.

## D13. RIDE con un generador de PDF propio
El RIDE solo necesita texto, líneas, una imagen y un código de barras. En vez de añadir una biblioteca de PDF se escribió un generador mínimo (`internal/pdf`, unas 350 líneas): PDF 1.4, A4, fuentes base Helvetica y Helvetica-Bold sin incrustar (todo visor las trae), codificación WinAnsi, Code 128 y logotipo PNG/JPEG. Las métricas de las fuentes proceden de los archivos AFM que publica Adobe. Límites asumidos: los caracteres fuera de Windows-1252 se sustituyen por `?`, no hay PDF/A ni fuentes personalizadas, y el diseño sigue los modelos del Anexo 2 sin reproducirlos al píxel. `ride.Generar` se niega a producir el RIDE de una envoltura cuyo estado no sea `AUTORIZADO`.

## D14. `valorDevolucionIva` positivo se rechaza
El campo existe en los XSD y en los modelos, pero su valor lo autoriza un servicio que el SDK no implementa (DIG) y debe restarse del importe total (Anexo 20), cosa que `Calcular` no hace. Aceptarlo produciría comprobantes con el total mal calculado. En vez de quitar el campo (los modelos dejarían de leer documentos de terceros que lo traen), la validación rechaza los valores mayores que cero, y como `Preparar` valida antes de firmar, el flujo público no puede emitirlos. Se retirará cuando DIG esté implementado.

## D15. Referencia abierta además de biblioteca
La documentación separa las reglas del SRI de las decisiones de este SDK (`docs/referencia/`) y se acompaña de vectores de prueba (`vectores/`) que las pruebas del proyecto mantienen coherentes con el código, para que sirva a implementaciones en otros lenguajes. Lo que falta se documenta en `docs/pendientes.md` con el mismo cuidado que lo que funciona.

## D16. Validación XSD con `github.com/cB-Sigma/xsd`
Validar contra los XSD oficiales exige un validador de XML Schema 1.0, que la biblioteca estándar no tiene. Se adoptó `cB-Sigma/xsd` (Apache-2.0, solo biblioteca estándar, sin cgo ni acceso a red: lee los esquemas de un `fs.FS`) frente a `lestrrat-go/helium` y `knroy/go-xml`, reimplementaciones completas de libxml2 o de la pila XML de cientos de miles de líneas, para una sola función (evaluación en `fuentes.md`).
*Riesgo:* es un proyecto joven, de un solo autor, con poca adopción. Se mitiga así: la versión está fijada por `go.sum` y `go mod verify` corre en el CI; el CI contrasta su veredicto con el de `xmllint` en los ejemplos oficiales y en variantes inválidas (`internal/interop`); y el SRI vuelve a validar el XSD al recibir (código 35), de modo que un fallo del validador produce una devolución, no un comprobante inválido autorizado. Cada actualización de la dependencia debe pasar esas pruebas.

## D17. El IVA de cada línea, más estricto que el SRI
El SRI autoriza un IVA de línea con un centavo de diferencia respecto del exacto: en la ronda de certificación de 0.2.0 autorizó 0,16 sobre una base de 1,00 al 15 % (caso T5, ver `pruebas-sri.md`). El SDK lo rechaza (`iva.valor`) **intencionalmente**: solo admite el redondeo al centavo más cercano del valor exacto (±0,005, con cualquier criterio de desempate). Es una decisión de integridad tributaria del propietario del proyecto, no un comportamiento desconocido ni un defecto: aceptar un centavo por línea permite acumular diferencias en el total (100 líneas con un centavo de más suman 1,00 USD de IVA cobrado de más), que es el hallazgo AUD-003 de la auditoría de v0.1.0.
Se conservan los redondeos legítimos que el SRI también autoriza: el total de IVA como suma de las líneas o sobre la base acumulada (D5, caso T3) y el desempate «mitad al par» en cada línea (caso T4).
*Consecuencia medida:* un precio final con IVA incluido se descompone en una base y un IVA de 2 decimales, y no siempre existe una base cuyo IVA redondeado complete el precio. Con el IVA del 15 %, de los 100 000 precios finales entre 0,01 y 1000,00, `Validar` rechaza cualquier descomposición de 8696 (8,70 %) y `Calcular`, que solo redondea mitad hacia arriba, no llega exactamente a 13 044 (13,04 %, es decir, 15 de cada 115). Ejemplos: 12,00 (10,43 + 1,56 = 11,99; 10,44 + 1,57 = 12,01), 35,00, 150,00, 0,50, 14,99. Con el 5 % no ocurre. Es la aritmética de la ficha: el facturador del SRI aplica la tarifa al subtotal (sección 9.16, campo 8) y tampoco puede emitir esos totales.
*Para quien pacta precios finales* existe el modo de precio final (D18), que la aplicación declara detalle a detalle. D17 sigue siendo la regla de todo detalle que no lo declare.

## D18. Precio final con IVA incluido: un modo que se declara
Un detalle se puede definir por su precio sin impuestos (por defecto) o por su precio final, lo que paga el comprador con el IVA incluido (`Detalle.PrecioFinal`, `LineaPrecioFinal`). Son la misma operación redondeada desde extremos distintos: en el primero es exacta la relación entre base e IVA; en el segundo, el importe que se cobra.
*Cálculo:* importe = redondear(cantidad × precioFinal, 2); base = redondear(importe × 100 / (100 + tarifa), 2); IVA = importe − base; el precio unitario sin impuestos se deduce, (base + descuento) / cantidad. El IVA total es la suma del IVA de las líneas, para que el importe total sea la suma de los importes finales.
*Por qué no es una tolerancia.* Con t la tarifa y G el importe, el IVA así obtenido es el contenido en el precio, G·t/(1 + t), redondeado al centavo, y respecto de la base escrita cumple |IVA − base·t| ≤ 0,005·(1 + t) (0,00575 al 15 %), nunca un centavo. A la inversa, una base y un IVA cumplen esa cota solo si son la descomposición de un importe final. Para cada importe hay una única base y un único IVA: no queda margen que gastar ni que acumular. La demostración y el alcance están en `comprobante/preciofinal.go` y en `referencia/calculo-y-validacion.md`.
*Por qué se declara y no se deduce.* El XML no dice con qué modo se calculó una línea. Admitir la cota para todos los comprobantes haría válidos dos valores de IVA para el 15 % de las bases; exigiendo el precio declarado, `Validar` comprueba la línea contra él (`precio-final.importe`, `precio-final.base`) y contra nada más. Un comprobante leído de XML se valida con D17 mientras la aplicación no vuelva a indicar el precio (el suyo, o el que figura en el documento con `DeclararPreciosFinales`).
*Alcance:* detalles cuyo único impuesto es el IVA, en factura, liquidación de compra y nota de crédito. Con ICE o IRBPNR la fórmula es otra y el SDK lo rechaza (`precio-final.impuestos`) en vez de aplicarla. La nota de débito no tiene detalles con precio. Con cantidades de 10 000 o más puede no existir un precio unitario de 6 decimales que reproduzca la base; entonces `Validar` lo indica.
*Respaldo:* el SRI autorizó en pruebas una factura de una línea de 12,00 (10,43 + 1,57) y otra de diez líneas de 0,50 (4,30 + 0,70 = 5,00) calculadas así. AUD-003 sigue cerrado: 0,16 sobre 1,00 no es la descomposición de 1,16 (que es 1,01 + 0,15) y se rechaza también declarando ese precio.

## Discrepancias abiertas
Las que siguen están desarrolladas, con lo que falta para resolverlas, en `pendientes.md`.

- **Nota de crédito y `valorDevolucionIva`:** el SRI publica dos XSD de nota de crédito incompatibles: `NotaCredito_V*` admite `agenteRetencion`/`contribuyenteRimpe` pero no `valorDevolucionIva`; `NOTA_CREDITO-*` hace lo contrario. El SDK usa el primero, con el que el SRI autorizó las pruebas. Falta determinar cómo se emite una nota de crédito con devolución de IVA. (El XSD de nota de débito `NotaDebito_V1.0.0` sí admite ambos.)
- **Estados de autorización:** la ficha muestra `RECHAZADO` en un ejemplo y las siglas PPR, AUT y NAT en la Tabla 6; el servicio real responde `NO AUTORIZADO`. El cliente acepta todas esas formas y trata cualquier otro estado como desconocido (ver D8), nunca como rechazo.
