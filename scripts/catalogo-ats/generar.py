"""Genera retencion/catalogo_renta.go a partir del volcado del Catálogo ATS.

Uso (ver docs/actualizar-catalogos.md):

    python3 generar.py catalogo.tsv <sha256 del .xls> <fecha de descarga aaaa-mm-dd>

Lee la hoja "TABLAS RETENCIONES", cuyo primer bloque de columnas es la tabla
3.10 vigente ("Conceptos de retención en la fuente de impuesto a la renta").
Solo usa la biblioteca estándar.
"""
import re
import sys

MESES = {"ENERO": 1, "FEBRERO": 2, "MARZO": 3, "ABRIL": 4, "MAYO": 5, "JUNIO": 6, "JULIO": 7,
         "AGOSTO": 8, "SEPTIEMBRE": 9, "OCTUBRE": 10, "NOVIEMBRE": 11, "DICIEMBRE": 12}


def interpretar(texto):
    """Devuelve (lista de porcentajes, minimo, maximo). Lista vacía y sin
    rango significa que el porcentaje no es comprobable."""
    t = texto.strip()
    if re.fullmatch(r"\d+(\.\d+)?", t):
        return [t], None, None
    m = re.fullmatch(r"(\d+(?:\.\d+)?)\s+al?\s+(\d+(?:\.\d+)?)", t)
    if m:
        return [], m.group(1), m.group(2)
    # Se toma lo anterior a la primera aclaración entre paréntesis, punto y
    # coma o la palabra "según".
    cabeza = re.split(r"\(|;|según|Según", t, maxsplit=1)[0].strip()
    if cabeza and re.fullmatch(r"[\d.,/o\s]+", cabeza):
        valores = re.findall(r"\d+(?:\.\d+)?", cabeza)
        if valores:
            return valores, None, None
    return [], None, None


def go_str(s):
    return '"' + s.replace("\\", "\\\\").replace('"', '\\"') + '"'


def main():
    ruta, sha, descarga = sys.argv[1], sys.argv[2], sys.argv[3]
    filas, dentro = [], False
    for linea in open(ruta, encoding="utf-8"):
        cols = linea.rstrip("\n").split("\t")
        if cols[0] == "##HOJA":
            dentro = cols[1].strip() == "TABLAS RETENCIONES"
            continue
        if dentro:
            filas.append(cols + [""] * 8)
    if not filas:
        sys.exit("no se encontró la hoja TABLAS RETENCIONES")
    titulo = " ".join(filas[0][:6]).strip()
    m = re.search(r"DESDE\s+(\d{1,2})/([A-ZÁÉÍÓÚ]+)/(\d{4})", " ".join(filas[1][:6]).upper())
    if "3.10" not in titulo or not m or m.group(2) not in MESES:
        sys.exit("la hoja cambió de formato: revise el volcado antes de generar")
    vigencia = f"{int(m.group(3)):04d}-{MESES[m.group(2)]:02d}-{int(m.group(1)):02d}"

    conceptos, vistos = [], set()
    for cols in filas[3:]:
        codigo = re.sub(r"\s+", "", cols[2])
        if not codigo:
            continue
        if not re.fullmatch(r"[0-9A-Z]{1,5}", codigo):
            sys.exit(f"código inesperado: {cols[2]!r}")
        if codigo in vistos:
            sys.exit(f"código repetido: {codigo}")
        vistos.add(codigo)
        concepto = re.sub(r"\s+", " ", cols[3]).strip()
        texto = re.sub(r"\s+", " ", cols[4]).strip()
        conceptos.append((codigo, concepto, texto) + interpretar(texto))
    if len(conceptos) < 100:
        sys.exit(f"solo se leyeron {len(conceptos)} conceptos: revise el volcado")

    out = []
    w = out.append
    w("// Código generado por scripts/catalogo-ats/generar.py. NO EDITAR A MANO.")
    w("// Para actualizarlo siga docs/actualizar-catalogos.md.")
    w("")
    w("package retencion")
    w("")
    w("// Procedencia del catálogo de retenciones de renta incluido.")
    w("const (")
    w("\t// CatalogoRentaVigencia es la fecha (aaaa-mm-dd) desde la que rige la")
    w("\t// tabla incluida, según el propio catálogo del SRI.")
    w(f"\tCatalogoRentaVigencia = {go_str(vigencia)}")
    w("\t// CatalogoRentaDescarga es la fecha en que se descargó el Catálogo ATS.")
    w(f"\tCatalogoRentaDescarga = {go_str(descarga)}")
    w("\t// CatalogoRentaSHA256 identifica el archivo Catalogo_ATS.xls usado.")
    w(f"\tCatalogoRentaSHA256 = {go_str(sha)}")
    w(")")
    w("")
    w("var conceptosRenta = []ConceptoRenta{")
    for codigo, concepto, texto, valores, mn, mx in conceptos:
        campos = [f"Codigo: {go_str(codigo)}", f"Concepto: {go_str(concepto)}", f"Texto: {go_str(texto)}"]
        if valores:
            campos.append("Porcentajes: []string{" + ", ".join(go_str(v) for v in valores) + "}")
        if mn is not None:
            campos.append(f"Minimo: {go_str(mn)}, Maximo: {go_str(mx)}")
        w("\t{" + ", ".join(campos) + "},")
    w("}")
    sys.stdout.write("\n".join(out) + "\n")
    sys.stderr.write(f"{len(conceptos)} conceptos; vigencia {vigencia}; "
                     f"{sum(1 for c in conceptos if not c[3] and c[4] is None)} sin porcentaje comprobable\n")


if __name__ == "__main__":
    main()
