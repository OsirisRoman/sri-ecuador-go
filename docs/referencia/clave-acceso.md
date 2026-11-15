# Clave de acceso

Identificador único de 49 dígitos de cada comprobante. Es también su número de autorización.

**Regla oficial:** ficha técnica v2.34, sección 5.2, Tabla 1 (comprobante) y Tabla 9 (lote).

## Composición

Se concatenan, sin separadores, estos campos:

| Posición | Campo | Formato | Largo |
|---|---|---|---|
| 1–8 | Fecha de emisión | `ddmmaaaa` | 8 |
| 9–10 | Tipo de comprobante (Tabla 3) | `01` factura, `03` liquidación de compra, `04` nota de crédito, `05` nota de débito, `06` guía de remisión, `07` comprobante de retención | 2 |
| 11–23 | RUC del emisor | 13 dígitos | 13 |
| 24 | Ambiente (Tabla 4) | `1` pruebas, `2` producción | 1 |
| 25–30 | Serie | establecimiento (3) + punto de emisión (3) | 6 |
| 31–39 | Secuencial | con ceros a la izquierda | 9 |
| 40–47 | Código numérico | 8 dígitos a elección del emisor | 8 |
| 48 | Tipo de emisión (Tabla 2) | `1` (emisión normal; es el único valor del esquema offline) | 1 |
| 49 | Dígito verificador | módulo 11 | 1 |

## Dígito verificador (módulo 11)

Sobre los primeros 48 dígitos:

1. Recorra los dígitos **de derecha a izquierda** y multiplique cada uno por un factor que empieza en 2 y sube hasta 7, y después vuelve a 2: `2, 3, 4, 5, 6, 7, 2, 3, …`.
2. Sume los productos.
3. Calcule `11 − (suma mod 11)`.
4. Si el resultado es **11**, el dígito es **0**; si es **10**, el dígito es **1**; en otro caso, es el resultado.

Ejemplo de la ficha (pág. 12): la cadena `41261533` da el dígito `6`.

```
función digitoVerificador(base48):
    suma ← 0; factor ← 2
    para cada dígito d de base48, del último al primero:
        suma ← suma + d × factor
        factor ← (factor = 7) ? 2 : factor + 1
    r ← 11 − (suma mod 11)
    si r = 11 → 0;  si r = 10 → 1;  si no → r
```

## Coherencia con el comprobante

El SRI rechaza con el **código 58** una clave cuyos componentes no coinciden con los del XML (`fechaEmision`, `codDoc`, `ruc`, `ambiente`, `estab`, `ptoEmi`, `secuencial`, `tipoEmision`). Genere la clave después del último cambio en esos datos.

Casos particulares:

- **Guía de remisión:** no tiene `fechaEmision`. La fecha de la clave hace de fecha de emisión y no puede ser posterior a `fechaIniTransporte` (código 82). El ejemplo de la ficha usa la fecha de inicio del transporte.
- **Lote:** la clave del lote se compone igual, pero con una serie y un secuencial que el emisor reserva para sus lotes. **Comprobado:** el SRI la acepta con tipo de comprobante `01`.

## Reglas de uso

- La clave identifica al comprobante durante toda su vida. Un reintento por un fallo de red debe usar la **misma** clave.
- **Comprobado:** un comprobante devuelto o no autorizado puede reenviarse corregido con la misma clave; su secuencial también puede reutilizarse con una clave nueva.
- El código numérico es libre. **Decisión de este SDK:** generarlo al azar con un generador criptográfico si no se indica.

## Vectores de prueba

[`vectores/clave-acceso.json`](../../vectores/clave-acceso.json) contiene casos con sus componentes, los 48 dígitos base, el dígito verificador y la clave completa, incluidos los dos casos límite (dígitos 0 y 1) y la clave del Anexo 14 de la ficha.

Implementación en este SDK: paquete [`clave`](../../clave/clave.go).
