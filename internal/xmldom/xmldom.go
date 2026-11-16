// Package xmldom es un árbol XML mínimo que conserva lo que la firma
// electrónica necesita y encoding/xml descarta: los prefijos tal como se
// escribieron y las declaraciones de espacios de nombres de cada elemento.
//
// Incluye la canonicalización C14N 1.0 inclusiva (REC-xml-c14n-20010315),
// con y sin el elemento excluido que requiere la transformación
// enveloped-signature de XMLDSig.
//
// Restricciones deliberadas de seguridad: se rechazan los documentos con
// DOCTYPE (sin DTD no hay entidades externas ni expansión exponencial) y se
// limitan la profundidad y el tamaño.
package xmldom

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Límites de análisis para documentos de procedencia no confiable.
const (
	MaxProfundidad = 128
	MaxBytes       = 8 << 20 // el SRI admite 320 kB por comprobante y ~512 kB por lote
)

// Errores de análisis.
var (
	ErrDoctype     = errors.New("xml: no se admiten documentos con DOCTYPE")
	ErrDemasiado   = errors.New("xml: el documento supera los límites de tamaño o profundidad")
	ErrMalFormado  = errors.New("xml: documento mal formado")
	ErrPrefijoNoNS = errors.New("xml: prefijo sin espacio de nombres declarado")
)

// Atributo es un atributo que no es declaración de espacio de nombres.
type Atributo struct {
	Prefijo, Local, Valor string
}

// Nodo es un hijo de un elemento: *Elemento, Texto o InstruccionProc.
type Nodo interface{ nodo() }

// Texto es contenido de caracteres ya decodificado (sin entidades).
type Texto string

// InstruccionProc es una instrucción de procesamiento <?destino datos?>.
type InstruccionProc struct{ Destino, Datos string }

// Elemento es un elemento XML.
type Elemento struct {
	Prefijo, Local string
	// NS son las declaraciones xmlns hechas en este elemento: prefijo → URI.
	// El prefijo "" es el espacio de nombres por defecto.
	NS    map[string]string
	Attrs []Atributo
	Hijos []Nodo
	Padre *Elemento
}

func (*Elemento) nodo()       {}
func (Texto) nodo()           {}
func (InstruccionProc) nodo() {}

// Documento es un documento XML analizado.
type Documento struct {
	Raiz *Elemento
}

// bomUTF8 es la marca de orden de bytes de UTF-8.
var bomUTF8 = []byte{0xEF, 0xBB, 0xBF}

// Analizar construye el árbol de un documento XML.
//
// Se admite una marca de orden de bytes (BOM) de UTF-8 al principio: es la
// firma de codificación que XML 1.0 permite (sección 4.3.3), no forma parte
// del documento y la escriben por defecto varias plataformas.
func Analizar(datos []byte) (*Documento, error) {
	if len(datos) > MaxBytes {
		return nil, ErrDemasiado
	}
	datos = bytes.TrimPrefix(datos, bomUTF8)
	dec := xml.NewDecoder(bytes.NewReader(datos))
	dec.Strict = true
	// Solo UTF-8 (y su subconjunto ASCII): es la codificación que exige la
	// ficha técnica; cualquier otra se rechaza en vez de adivinarse.
	dec.CharsetReader = func(cs string, in io.Reader) (io.Reader, error) {
		switch strings.ToLower(cs) {
		case "utf-8", "utf8", "us-ascii", "ascii":
			return in, nil
		}
		return nil, fmt.Errorf("%w: codificación %q no admitida, use UTF-8", ErrMalFormado, cs)
	}
	var doc Documento
	var actual *Elemento
	prof := 0
	// Los fragmentos de texto contiguos (texto, CDATA, texto separado por
	// comentarios) forman un solo nodo. Se acumulan aquí y se añaden al
	// elemento de una vez: concatenarlos uno a uno costaría un tiempo
	// cuadrático con muchos fragmentos.
	var texto []byte
	hayTexto := false
	volcar := func() {
		if hayTexto {
			actual.Hijos = append(actual.Hijos, Texto(texto))
			texto, hayTexto = texto[:0], false
		}
	}
	for {
		ini := dec.InputOffset()
		tok, err := dec.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrMalFormado, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if prof++; prof > MaxProfundidad {
				return nil, ErrDemasiado
			}
			volcar()
			if actual == nil && doc.Raiz != nil {
				return nil, fmt.Errorf("%w: más de un elemento raíz", ErrMalFormado)
			}
			e := &Elemento{Prefijo: t.Name.Space, Local: t.Name.Local, Padre: actual}
			if err := normalizarAtributos(t.Attr, datos[ini:dec.InputOffset()]); err != nil {
				return nil, err
			}
			vistos := make(map[xml.Name]bool, len(t.Attr))
			for _, a := range t.Attr {
				// Un atributo repetido hace que el documento no esté bien
				// formado (XML 1.0, restricción «Unique Att Spec»).
				if vistos[a.Name] {
					return nil, fmt.Errorf("%w: atributo %s repetido en <%s>", ErrMalFormado, nombreAttr(a.Name), t.Name.Local)
				}
				vistos[a.Name] = true
				switch {
				case a.Name.Space == "" && a.Name.Local == "xmlns":
					e.declarar("", a.Value)
				case a.Name.Space == "xmlns":
					if a.Value == "" {
						return nil, fmt.Errorf("%w: xmlns:%s vacío", ErrMalFormado, a.Name.Local)
					}
					e.declarar(a.Name.Local, a.Value)
				default:
					e.Attrs = append(e.Attrs, Atributo{Prefijo: a.Name.Space, Local: a.Name.Local, Valor: a.Value})
				}
			}
			if actual == nil {
				doc.Raiz = e
			} else {
				actual.Hijos = append(actual.Hijos, e)
			}
			actual = e
		case xml.EndElement:
			if actual == nil || actual.Prefijo != t.Name.Space || actual.Local != t.Name.Local {
				return nil, fmt.Errorf("%w: cierre </%s> inesperado", ErrMalFormado, t.Name.Local)
			}
			volcar()
			actual = actual.Padre
			prof--
		case xml.CharData:
			if actual != nil {
				texto, hayTexto = append(texto, t...), true
			} else if len(bytes.TrimSpace(t)) != 0 {
				return nil, fmt.Errorf("%w: texto fuera del elemento raíz", ErrMalFormado)
			}
		case xml.ProcInst:
			if actual != nil && t.Target != "xml" {
				volcar()
				actual.Hijos = append(actual.Hijos, InstruccionProc{Destino: t.Target, Datos: string(t.Inst)})
			}
		case xml.Directive:
			return nil, ErrDoctype
		case xml.Comment:
			// Los comentarios no forman parte de la forma canónica sin
			// comentarios, que es la única que usa el SRI.
		}
	}
	if doc.Raiz == nil || actual != nil {
		return nil, fmt.Errorf("%w: documento incompleto", ErrMalFormado)
	}
	if err := doc.Raiz.comprobarPrefijos(); err != nil {
		return nil, err
	}
	return &doc, nil
}

// normalizarAtributos reemplaza el valor de cada atributo por el que
// resulta de normalizar su texto original (ver atributos.go). Solo hace
// falta cuando la etiqueta contiene tabuladores o saltos de línea.
func normalizarAtributos(attrs []xml.Attr, etiqueta []byte) error {
	if len(attrs) == 0 || !bytes.ContainsAny(etiqueta, "\t\n\r") {
		return nil
	}
	crudos, err := valoresCrudos(etiqueta)
	if err != nil || len(crudos) != len(attrs) {
		return fmt.Errorf("%w: no se pudieron leer los atributos de %.40q", ErrMalFormado, etiqueta)
	}
	for i := range attrs {
		v, err := normalizarValor(crudos[i])
		if err != nil {
			return fmt.Errorf("%w: valor de atributo %.40q", ErrMalFormado, crudos[i])
		}
		attrs[i].Value = v
	}
	return nil
}

func nombreAttr(n xml.Name) string {
	if n.Space == "" {
		return n.Local
	}
	return n.Space + ":" + n.Local
}

func (e *Elemento) declarar(prefijo, uri string) {
	if e.NS == nil {
		e.NS = map[string]string{}
	}
	e.NS[prefijo] = uri
}

func (e *Elemento) comprobarPrefijos() error {
	if e.Prefijo != "" {
		if _, ok := e.ResolverPrefijo(e.Prefijo); !ok {
			return fmt.Errorf("%w: %s:%s", ErrPrefijoNoNS, e.Prefijo, e.Local)
		}
	}
	expandidos := map[[2]string]bool{}
	for _, a := range e.Attrs {
		uri := ""
		if a.Prefijo != "" {
			var ok bool
			if uri, ok = e.ResolverPrefijo(a.Prefijo); !ok {
				return fmt.Errorf("%w: atributo %s:%s", ErrPrefijoNoNS, a.Prefijo, a.Local)
			}
		}
		// Dos prefijos ligados al mismo espacio de nombres no pueden
		// calificar el mismo atributo (Namespaces in XML, sección 6.3).
		k := [2]string{uri, a.Local}
		if expandidos[k] {
			return fmt.Errorf("%w: atributo {%s}%s repetido en <%s>", ErrMalFormado, uri, a.Local, e.Local)
		}
		expandidos[k] = true
	}
	for _, h := range e.Hijos {
		if he, ok := h.(*Elemento); ok {
			if err := he.comprobarPrefijos(); err != nil {
				return err
			}
		}
	}
	return nil
}

// XMLNamespace es el espacio de nombres reservado del prefijo xml.
const XMLNamespace = "http://www.w3.org/XML/1998/namespace"

// ResolverPrefijo devuelve el URI ligado a un prefijo en el ámbito de e.
func (e *Elemento) ResolverPrefijo(prefijo string) (string, bool) {
	if prefijo == "xml" {
		return XMLNamespace, true
	}
	for x := e; x != nil; x = x.Padre {
		if uri, ok := x.NS[prefijo]; ok {
			return uri, true
		}
	}
	return "", prefijo == ""
}

// Espacio devuelve el URI del espacio de nombres del elemento.
func (e *Elemento) Espacio() string {
	uri, _ := e.ResolverPrefijo(e.Prefijo)
	return uri
}

// Es informa si el elemento tiene ese espacio de nombres y nombre local.
func (e *Elemento) Es(espacio, local string) bool {
	return e.Local == local && e.Espacio() == espacio
}

// Attr devuelve el valor de un atributo sin prefijo.
func (e *Elemento) Attr(local string) (string, bool) {
	for _, a := range e.Attrs {
		if a.Prefijo == "" && a.Local == local {
			return a.Valor, true
		}
	}
	return "", false
}

// FijarAttr crea o reemplaza un atributo sin prefijo.
func (e *Elemento) FijarAttr(local, valor string) {
	for i, a := range e.Attrs {
		if a.Prefijo == "" && a.Local == local {
			e.Attrs[i].Valor = valor
			return
		}
	}
	e.Attrs = append(e.Attrs, Atributo{Local: local, Valor: valor})
}

// Elementos devuelve los hijos que son elementos.
func (e *Elemento) Elementos() []*Elemento {
	var out []*Elemento
	for _, h := range e.Hijos {
		if he, ok := h.(*Elemento); ok {
			out = append(out, he)
		}
	}
	return out
}

// Hijo devuelve el primer hijo directo con ese espacio y nombre, o nil.
func (e *Elemento) Hijo(espacio, local string) *Elemento {
	for _, h := range e.Hijos {
		if he, ok := h.(*Elemento); ok && he.Es(espacio, local) {
			return he
		}
	}
	return nil
}

// Ruta desciende por una cadena de hijos del mismo espacio de nombres.
func (e *Elemento) Ruta(espacio string, locales ...string) *Elemento {
	x := e
	for _, l := range locales {
		if x = x.Hijo(espacio, l); x == nil {
			return nil
		}
	}
	return x
}

// TextoPlano concatena el texto directo del elemento.
func (e *Elemento) TextoPlano() string {
	var sb strings.Builder
	for _, h := range e.Hijos {
		if t, ok := h.(Texto); ok {
			sb.WriteString(string(t))
		}
	}
	return sb.String()
}

// Agregar añade un hijo al final y fija su padre.
func (e *Elemento) Agregar(h *Elemento) *Elemento {
	h.Padre = e
	e.Hijos = append(e.Hijos, h)
	return h
}

// Quitar elimina un hijo directo; informa si lo encontró.
func (e *Elemento) Quitar(h *Elemento) bool {
	for i, x := range e.Hijos {
		if x == Nodo(h) {
			e.Hijos = append(e.Hijos[:i:i], e.Hijos[i+1:]...)
			h.Padre = nil
			return true
		}
	}
	return false
}

// Recorrer visita e y todos sus descendientes en orden de documento; si f
// devuelve false no se desciende por ese elemento.
func (e *Elemento) Recorrer(f func(*Elemento) bool) {
	if !f(e) {
		return
	}
	for _, h := range e.Hijos {
		if he, ok := h.(*Elemento); ok {
			he.Recorrer(f)
		}
	}
}

// BuscarPorID devuelve los elementos cuyo atributo Id, ID o id (sin
// prefijo) vale id. XMLDSig referencia fragmentos con URI="#id"; sin DTD no
// hay atributos de tipo ID declarados, así que se aplica esta convención,
// que es la del SRI (id="comprobante") y la de XAdES (Id="...").
//
// Se devuelven todas las coincidencias para que quien verifica pueda
// rechazar identificadores duplicados (defensa frente a ataques de
// envoltura de firma).
func (e *Elemento) BuscarPorID(id string) []*Elemento {
	var out []*Elemento
	e.Recorrer(func(x *Elemento) bool {
		for _, a := range x.Attrs {
			if a.Prefijo == "" && (a.Local == "Id" || a.Local == "ID" || a.Local == "id") && a.Valor == id {
				out = append(out, x)
				break
			}
		}
		return true
	})
	return out
}

// ---------------------------------------------------------------------------
// Canonicalización

// Canonicalizar devuelve la forma canónica C14N 1.0 inclusiva, sin
// comentarios, del subárbol con vértice en e. Como exige la recomendación
// para subconjuntos de documento, el vértice hereda los espacios de nombres
// y los atributos xml:* visibles desde sus ancestros.
//
// Si excluir no es nil, ese elemento y sus descendientes se omiten: es el
// efecto de la transformación enveloped-signature.
func Canonicalizar(e *Elemento, excluir *Elemento) []byte {
	var buf bytes.Buffer
	// Espacios de nombres en ámbito para el vértice.
	enAmbito := map[string]string{}
	var cadena []*Elemento
	for x := e; x != nil; x = x.Padre {
		cadena = append(cadena, x)
	}
	for i := len(cadena) - 1; i >= 0; i-- {
		for p, u := range cadena[i].NS {
			enAmbito[p] = u
		}
	}
	// Atributos xml:* heredados (xml:lang, xml:space, ...).
	var heredados []Atributo
	for i := 1; i < len(cadena); i++ { // del padre hacia la raíz
		for _, a := range cadena[i].Attrs {
			if a.Prefijo != "xml" {
				continue
			}
			ya := false
			for _, h := range heredados {
				if h.Local == a.Local {
					ya = true
				}
			}
			for _, propio := range e.Attrs {
				if propio.Prefijo == "xml" && propio.Local == a.Local {
					ya = true
				}
			}
			if !ya {
				heredados = append(heredados, a)
			}
		}
	}
	c := canonizador{buf: &buf, excluir: excluir, enAmbito: enAmbito, emitidos: map[string]string{}}
	// El vértice escribe todos los espacios de nombres en ámbito.
	prefijos := make([]string, 0, len(enAmbito))
	for p := range enAmbito {
		prefijos = append(prefijos, p)
	}
	c.elemento(e, prefijos, heredados)
	return buf.Bytes()
}

// canonizador escribe la forma canónica de un subárbol. El coste es
// proporcional al tamaño del subárbol: en cada elemento solo se examinan
// las declaraciones de espacios de nombres del propio elemento, porque las
// que hereda ya las escribió un ancestro (en C14N inclusiva, todo espacio
// en ámbito se escribe en el primer elemento de la salida que lo ve).
type canonizador struct {
	buf     *bytes.Buffer
	excluir *Elemento
	// enAmbito son los espacios de nombres visibles en el elemento que se
	// está escribiendo; emitidos, los ya escritos por él o por un ancestro
	// dentro de la salida. Se modifican al entrar en un elemento y se
	// restauran al salir.
	enAmbito, emitidos map[string]string
}

// previo es el valor que tenía una clave de un mapa antes de cambiarla.
type previo struct {
	mapa   map[string]string
	clave  string
	valor  string
	existe bool
}

func fijar(deshacer []previo, m map[string]string, k, v string) []previo {
	ant, ok := m[k]
	m[k] = v
	return append(deshacer, previo{m, k, ant, ok})
}

// elemento escribe e. candidatos son los prefijos cuya declaración puede
// hacer falta escribir aquí: todos los que están en ámbito para el vértice,
// y los que declara el propio elemento para los demás. c.enAmbito ya
// incluye las declaraciones de e.
func (c *canonizador) elemento(e *Elemento, candidatos []string, extra []Atributo) {
	buf := c.buf
	nombre := e.Local
	if e.Prefijo != "" {
		nombre = e.Prefijo + ":" + e.Local
	}
	buf.WriteByte('<')
	buf.WriteString(nombre)

	// Declaraciones de espacios de nombres: lo que está en ámbito y no fue
	// emitido con el mismo valor por el ancestro de salida más cercano.
	var deshacer []previo
	sort.Strings(candidatos) // "" (xmlns por defecto) ordena primero
	for _, p := range candidatos {
		u := c.enAmbito[p]
		if p == "xml" {
			continue
		}
		prev, ok := c.emitidos[p]
		if ok && prev == u {
			continue
		}
		if p == "" && u == "" && (!ok || prev == "") {
			// xmlns="" solo se escribe para anular un valor por defecto
			// no vacío ya emitido.
			continue
		}
		deshacer = fijar(deshacer, c.emitidos, p, u)
		if p == "" {
			buf.WriteString(` xmlns="`)
		} else {
			buf.WriteString(" xmlns:" + p + `="`)
		}
		escAttr(buf, u)
		buf.WriteByte('"')
	}

	// Atributos ordenados por (URI del espacio de nombres, nombre local);
	// los que no tienen prefijo no tienen espacio y van primero.
	type attrOrd struct {
		uri string
		a   Atributo
	}
	attrs := make([]attrOrd, 0, len(e.Attrs)+len(extra))
	for _, a := range append(append([]Atributo(nil), e.Attrs...), extra...) {
		uri := ""
		if a.Prefijo == "xml" {
			uri = XMLNamespace
		} else if a.Prefijo != "" {
			uri = c.enAmbito[a.Prefijo]
		}
		attrs = append(attrs, attrOrd{uri, a})
	}
	sort.SliceStable(attrs, func(i, j int) bool {
		if attrs[i].uri != attrs[j].uri {
			return attrs[i].uri < attrs[j].uri
		}
		return attrs[i].a.Local < attrs[j].a.Local
	})
	for _, x := range attrs {
		buf.WriteByte(' ')
		if x.a.Prefijo != "" {
			buf.WriteString(x.a.Prefijo + ":")
		}
		buf.WriteString(x.a.Local)
		buf.WriteString(`="`)
		escAttr(buf, x.a.Valor)
		buf.WriteByte('"')
	}
	buf.WriteByte('>')

	for _, h := range e.Hijos {
		switch n := h.(type) {
		case Texto:
			escTexto(buf, string(n))
		case InstruccionProc:
			buf.WriteString("<?" + n.Destino)
			if n.Datos != "" {
				buf.WriteString(" " + n.Datos)
			}
			buf.WriteString("?>")
		case *Elemento:
			if n == c.excluir {
				continue
			}
			var prefijos []string
			marca := len(deshacer)
			for p, u := range n.NS {
				prefijos = append(prefijos, p)
				deshacer = fijar(deshacer, c.enAmbito, p, u)
			}
			c.elemento(n, prefijos, nil)
			deshacer = restaurar(deshacer, marca)
		}
	}
	restaurar(deshacer, 0)
	buf.WriteString("</" + nombre + ">")
}

// restaurar deshace, del último al primero, los cambios anotados a partir
// de la posición desde.
func restaurar(deshacer []previo, desde int) []previo {
	for i := len(deshacer) - 1; i >= desde; i-- {
		if d := deshacer[i]; d.existe {
			d.mapa[d.clave] = d.valor
		} else {
			delete(d.mapa, d.clave)
		}
	}
	return deshacer[:desde]
}

func escTexto(buf *bytes.Buffer, s string) {
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '&':
			buf.WriteString("&amp;")
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		case '\r':
			buf.WriteString("&#xD;")
		default:
			buf.WriteByte(c)
		}
	}
}

func escAttr(buf *bytes.Buffer, s string) {
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '&':
			buf.WriteString("&amp;")
		case '<':
			buf.WriteString("&lt;")
		case '"':
			buf.WriteString("&quot;")
		case '\t':
			buf.WriteString("&#x9;")
		case '\n':
			buf.WriteString("&#xA;")
		case '\r':
			buf.WriteString("&#xD;")
		default:
			buf.WriteByte(c)
		}
	}
}

// Serializar escribe el documento completo con declaración XML. El cuerpo se
// emite en forma canónica: es una serialización XML válida y estable, y
// garantiza que lo que se envía es byte a byte lo que se firmó.
func (d *Documento) Serializar() []byte {
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	buf.Write(Canonicalizar(d.Raiz, nil))
	return buf.Bytes()
}

// Nuevo crea un elemento suelto.
func Nuevo(prefijo, local string) *Elemento {
	return &Elemento{Prefijo: prefijo, Local: local}
}

// ConTexto añade texto al elemento y lo devuelve.
func (e *Elemento) ConTexto(s string) *Elemento {
	e.Hijos = append(e.Hijos, Texto(s))
	return e
}

// ConAttr añade un atributo sin prefijo y devuelve el elemento.
func (e *Elemento) ConAttr(local, valor string) *Elemento {
	e.Attrs = append(e.Attrs, Atributo{Local: local, Valor: valor})
	return e
}

// ConNS declara un espacio de nombres y devuelve el elemento.
func (e *Elemento) ConNS(prefijo, uri string) *Elemento {
	e.declarar(prefijo, uri)
	return e
}
