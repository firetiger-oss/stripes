package table

import (
	"encoding"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

type align int

const (
	alignLeft align = iota
	alignRight
)

type rowShape int

const (
	rowStruct rowShape = iota // T is a struct; cells from exported fields
	rowSlice                  // T is a slice/array; cells from elements
)

type column struct {
	header     string
	align      align
	format     func(reflect.Value) string
	colorize   func(string) string // applied after width-fitting; defaults to identityColorize
	fieldIndex int                 // struct field index, or slice cell index
}

// identityColorize is the default colorize hook for columns that don't
// need post-fit styling. Using a no-op instead of nil lets the render
// loop skip a per-cell nil check.
func identityColorize(s string) string { return s }

type schema struct {
	columns  []column
	rowType  reflect.Type
	rowIsPtr bool
	shape    rowShape
}

var (
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
	formatterType     = reflect.TypeFor[fmt.Formatter]()
	stringerType      = reflect.TypeFor[fmt.Stringer]()
	timeType          = reflect.TypeFor[time.Time]()
	durationType      = reflect.TypeFor[time.Duration]()
)

func buildSchema[T any](opts *Options) (*schema, error) {
	t := reflect.TypeFor[T]()
	rowIsPtr := false
	if t.Kind() == reflect.Pointer {
		rowIsPtr = true
		t = t.Elem()
	}

	switch {
	case len(opts.Columns) > 0 && t.Kind() == reflect.Struct:
		return nil, fmt.Errorf("WithColumns / WithHeaders is for slice/array rows; use struct tags to customise struct columns")
	case len(opts.Columns) > 0 && (t.Kind() == reflect.Slice || t.Kind() == reflect.Array):
		return buildSliceSchema(t, rowIsPtr, opts)
	case len(opts.Columns) > 0:
		return nil, fmt.Errorf("row type must be a struct or slice/array, got %s", t)
	case t.Kind() == reflect.Struct:
		return buildStructSchema(t, rowIsPtr, opts)
	default:
		return nil, fmt.Errorf("row type must be a struct or slice/array, got %s (consider WithColumns / WithHeaders)", t)
	}
}

func buildStructSchema(t reflect.Type, rowIsPtr bool, opts *Options) (*schema, error) {
	s := &schema{rowType: t, rowIsPtr: rowIsPtr, shape: rowStruct}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("table")
		if tag == "-" {
			continue
		}
		name, mods := parseTag(tag)
		if name == "" {
			name = headerFromName(f.Name)
		}
		col, err := buildColumn(f, name, mods, opts)
		if err != nil {
			return nil, err
		}
		col.fieldIndex = i
		s.columns = append(s.columns, col)
	}
	if len(s.columns) == 0 {
		return nil, fmt.Errorf("struct %s has no exported fields to render", t)
	}
	return s, nil
}

func buildSliceSchema(t reflect.Type, rowIsPtr bool, opts *Options) (*schema, error) {
	elem := t.Elem()
	s := &schema{rowType: t, rowIsPtr: rowIsPtr, shape: rowSlice}
	for i, c := range opts.Columns {
		fn, a, colorize, err := formatterForCell(elem, c.Modifier, opts)
		if err != nil {
			return nil, fmt.Errorf("column %d (%q): %w", i, c.Header, err)
		}
		s.columns = append(s.columns, column{
			header:     c.Header,
			align:      a,
			format:     fn,
			colorize:   colorize,
			fieldIndex: i,
		})
	}
	return s, nil
}

func parseTag(tag string) (name string, modifiers []string) {
	if tag == "" {
		return "", nil
	}
	parts := strings.Split(tag, ",")
	return parts[0], parts[1:]
}

// headerFromName splits a Go identifier on CamelCase / digit boundaries and
// uppercases each word, joining with spaces. "FirstName" -> "FIRST NAME";
// "HTTPRequest" -> "HTTP REQUEST"; "UserID" -> "USER ID".
func headerFromName(name string) string {
	runes := []rune(name)
	var b strings.Builder
	b.Grow(len(name) + 4)
	for i, r := range runes {
		if i > 0 {
			prev := runes[i-1]
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			switch {
			case unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev)):
				b.WriteByte(' ')
			case unicode.IsUpper(r) && unicode.IsUpper(prev) && nextLower:
				// Acronym → Pascal boundary, e.g. "HTTPR" + "equest".
				b.WriteByte(' ')
			case unicode.IsDigit(r) && unicode.IsLetter(prev):
				b.WriteByte(' ')
			case unicode.IsLetter(r) && unicode.IsDigit(prev):
				b.WriteByte(' ')
			}
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	return b.String()
}

func buildColumn(f reflect.StructField, header string, mods []string, opts *Options) (column, error) {
	col := column{header: header}

	// Precedence 0: tag modifiers. They pin alignment and formatter.
	if len(mods) > 0 {
		// Struct tag syntax allows multiple comma-separated modifiers; in
		// practice only the first one is meaningful today. Surface any
		// extras as an error so silent typos don't hide.
		mod := mods[0]
		if len(mods) > 1 {
			return col, fmt.Errorf("field %s: only one modifier supported, got %v", f.Name, mods)
		}
		fn, a, colorize, err := formatterForCell(f.Type, mod, opts)
		if err != nil {
			return col, fmt.Errorf("field %s: %w", f.Name, err)
		}
		col.format, col.align, col.colorize = fn, a, colorize
		return col, nil
	}

	fn, a, colorize, err := formatterForCell(f.Type, "", opts)
	if err != nil {
		return col, fmt.Errorf("field %s: %w", f.Name, err)
	}
	col.format, col.align, col.colorize = fn, a, colorize
	return col, nil
}

// formatterForCell chooses a formatter, alignment, and colorize hook for
// a column whose cells have static type t. When modifier is non-empty it
// overrides the type-derived dispatch. The returned colorize is always
// non-nil — columns that don't need post-fit styling get
// identityColorize.
func formatterForCell(t reflect.Type, modifier string, opts *Options) (
	func(reflect.Value) string, align, func(string) string, error,
) {
	switch modifier {
	case "":
		// Fall through to type-based dispatch below.
	case "bytes":
		if !isIntOrUintKind(t.Kind()) {
			return nil, alignLeft, nil, fmt.Errorf("'bytes' modifier requires int or uint, got %s", t)
		}
		return bytesFormatter(t.Kind()), alignRight, identityColorize, nil
	case "count":
		if !isIntOrUintKind(t.Kind()) {
			return nil, alignLeft, nil, fmt.Errorf("'count' modifier requires int or uint, got %s", t)
		}
		return countFormatter(t.Kind()), alignRight, identityColorize, nil
	case "percent":
		if !isFloatKind(t.Kind()) {
			return nil, alignLeft, nil, fmt.Errorf("'percent' modifier requires float, got %s", t)
		}
		return percentFormatter, alignRight, identityColorize, nil
	default:
		return nil, alignLeft, nil, fmt.Errorf("unknown modifier %q", modifier)
	}

	// Empty interface (`any`) gets dynamic-per-cell dispatch.
	if t.Kind() == reflect.Interface && t.NumMethod() == 0 {
		return anyCellFormatter(opts), alignLeft, identityColorize, nil
	}

	a := alignmentForType(t)
	fn := chooseFormatter(t, opts)
	colorize := identityColorize
	if isJSONFallbackType(t) {
		styles := opts.Styles
		colorize = func(s string) string { return colorizeJSON(s, styles) }
	}
	return fn, a, colorize, nil
}

// anyCellFormatter returns a formatter for cells whose static type is
// `any` (interface{}). It unwraps the interface at render time, looks up
// (and caches) a per-type formatter via chooseFormatter, and applies it.
// Nil interfaces render as empty strings.
//
// JSON-fallback dynamic types (slice/array/map/struct) are wrapped with
// colorizeJSON here rather than via the per-column colorize hook,
// because the column's *static* colorize hook is identityColorize — we
// don't learn the cell is JSON-shaped until we unwrap the interface.
func anyCellFormatter(opts *Options) func(reflect.Value) string {
	var cache sync.Map // map[reflect.Type]func(reflect.Value) string
	return func(v reflect.Value) string {
		if !v.IsValid() {
			return ""
		}
		if v.Kind() == reflect.Interface {
			if v.IsNil() {
				return ""
			}
			v = v.Elem()
		}
		t := v.Type()
		if fn, ok := cache.Load(t); ok {
			return fn.(func(reflect.Value) string)(v)
		}
		fn := chooseFormatter(t, opts)
		if isJSONFallbackType(t) {
			styles := opts.Styles
			inner := fn
			fn = func(v reflect.Value) string { return colorizeJSON(inner(v), styles) }
		}
		cache.Store(t, fn)
		return fn(v)
	}
}

// isJSONFallbackType reports whether values of t would land in the JSON
// fallback formatter (slices, maps, structs without dedicated handling,
// etc.). Kept in sync with chooseFormatter so that the colorize hook fires
// exactly for cells that contain compact JSON.
func isJSONFallbackType(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t {
	case timeType, durationType:
		return false
	}
	if interfaceFormatter(t) != nil {
		return false
	}
	if primitiveFormatter(t) != nil {
		return false
	}
	switch t.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map, reflect.Struct, reflect.Interface:
		return true
	}
	return false
}

func alignmentForType(t reflect.Type) align {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return alignRight
	}
	return alignLeft
}

// chooseFormatter picks a cell formatter for a column whose static type is t,
// applying the precedence documented in the package overview.
func chooseFormatter(t reflect.Type, opts *Options) func(reflect.Value) string {
	if t.Kind() == reflect.Pointer {
		inner := chooseFormatter(t.Elem(), opts)
		return func(v reflect.Value) string {
			if v.IsNil() {
				return ""
			}
			return inner(v.Elem())
		}
	}

	// Special concrete types must come before the interface checks so
	// that our friendlier formats win over MarshalText / String.
	switch t {
	case timeType:
		return timeFormatter(opts)
	case durationType:
		return durationFormatter
	}

	if fn := interfaceFormatter(t); fn != nil {
		return fn
	}
	if fn := primitiveFormatter(t); fn != nil {
		return fn
	}
	// Fallback for nested types (slice, array, map, anonymous struct, ...):
	// compact JSON. If JSON marshalling fails (channels, functions, etc.)
	// we drop down to fmt.Sprint.
	return jsonFormat
}

func jsonFormat(v reflect.Value) string {
	if !v.IsValid() {
		return ""
	}
	b, err := json.Marshal(v.Interface())
	if err != nil {
		return fmt.Sprint(v.Interface())
	}
	return string(b)
}

func interfaceFormatter(t reflect.Type) func(reflect.Value) string {
	// Value-receiver methods first.
	switch {
	case t.Implements(textMarshalerType):
		return marshalTextFormat
	case t.Implements(formatterType):
		return sprintFormat
	case t.Implements(stringerType):
		return stringerFormat
	}
	// Pointer-receiver methods: take the address of an addressable value.
	if t.Kind() == reflect.Pointer {
		return nil
	}
	pt := reflect.PointerTo(t)
	switch {
	case pt.Implements(textMarshalerType):
		return addrFormat(marshalTextFormat)
	case pt.Implements(formatterType):
		return addrFormat(sprintFormat)
	case pt.Implements(stringerType):
		return addrFormat(stringerFormat)
	}
	return nil
}

func addrFormat(inner func(reflect.Value) string) func(reflect.Value) string {
	return func(v reflect.Value) string {
		return inner(v.Addr())
	}
}

func marshalTextFormat(v reflect.Value) string {
	b, err := v.Interface().(encoding.TextMarshaler).MarshalText()
	if err != nil {
		return "!err " + err.Error()
	}
	return string(b)
}

func sprintFormat(v reflect.Value) string {
	return fmt.Sprintf("%v", v.Interface())
}

func stringerFormat(v reflect.Value) string {
	return v.Interface().(fmt.Stringer).String()
}

func primitiveFormatter(t reflect.Type) func(reflect.Value) string {
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(v reflect.Value) string { return strconv.FormatInt(v.Int(), 10) }
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return func(v reflect.Value) string { return strconv.FormatUint(v.Uint(), 10) }
	case reflect.Float32:
		return func(v reflect.Value) string { return strconv.FormatFloat(v.Float(), 'g', -1, 32) }
	case reflect.Float64:
		return func(v reflect.Value) string { return strconv.FormatFloat(v.Float(), 'g', -1, 64) }
	case reflect.Bool:
		return func(v reflect.Value) string { return strconv.FormatBool(v.Bool()) }
	case reflect.String:
		return func(v reflect.Value) string { return v.String() }
	}
	return nil
}

func isIntOrUintKind(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return true
	}
	return false
}

func isFloatKind(k reflect.Kind) bool {
	return k == reflect.Float32 || k == reflect.Float64
}

// timeFormatter picks the absolute or relative form once, based on Options.Now.
func timeFormatter(opts *Options) func(reflect.Value) string {
	if opts.Now != nil {
		now := opts.Now
		return func(v reflect.Value) string {
			t := v.Interface().(time.Time)
			if t.IsZero() {
				return ""
			}
			return humanRelative(now().Sub(t))
		}
	}
	return func(v reflect.Value) string {
		t := v.Interface().(time.Time)
		if t.IsZero() {
			return ""
		}
		return t.Format(time.DateTime)
	}
}

func durationFormatter(v reflect.Value) string {
	return humanizeDuration(time.Duration(v.Int()))
}

// humanRelative renders a delta as a single-unit "X ago" / "in X" form.
func humanRelative(d time.Duration) string {
	if d < 0 {
		return "in " + humanRelativeMag(-d)
	}
	return humanRelativeMag(d) + " ago"
}

func humanRelativeMag(d time.Duration) string {
	switch {
	case d < time.Second:
		return "<1s"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d/time.Second))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	default:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
}

// humanizeDuration renders a duration in compound form (e.g. "1h 30m") with
// progressively coarser buckets as the magnitude grows. Sub-second values
// render with abbreviated time units (ns, µs, ms).
func humanizeDuration(d time.Duration) string {
	if d < 0 {
		return "-" + humanizeDuration(-d)
	}
	switch {
	case d < time.Microsecond:
		return fmt.Sprintf("%dns", int64(d))
	case d < time.Millisecond:
		return fmt.Sprintf("%dµs", int64(d/time.Microsecond))
	case d < time.Second:
		return fmt.Sprintf("%dms", int64(d/time.Millisecond))
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d/time.Second))
	case d < time.Hour:
		m := int(d / time.Minute)
		s := int((d % time.Minute) / time.Second)
		if s == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm %ds", m, s)
	case d < 24*time.Hour:
		h := int(d / time.Hour)
		m := int((d % time.Hour) / time.Minute)
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh %dm", h, m)
	default:
		day := 24 * time.Hour
		days := int(d / day)
		h := int((d % day) / time.Hour)
		if h == 0 {
			return fmt.Sprintf("%dd", days)
		}
		return fmt.Sprintf("%dd %dh", days, h)
	}
}

func bytesFormatter(k reflect.Kind) func(reflect.Value) string {
	if isIntOrUintKind(k) && (k == reflect.Uint || k == reflect.Uint8 || k == reflect.Uint16 || k == reflect.Uint32 || k == reflect.Uint64 || k == reflect.Uintptr) {
		return func(v reflect.Value) string {
			u := v.Uint()
			if u > math.MaxInt64 {
				return humanBytes(math.MaxInt64)
			}
			return humanBytes(int64(u))
		}
	}
	return func(v reflect.Value) string {
		return humanBytes(v.Int())
	}
}

func humanBytes(n int64) string {
	if n < 0 {
		return "-" + humanBytes(-n)
	}
	if n < 1024 {
		return strconv.FormatInt(n, 10) + " B"
	}
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}
	f := float64(n) / 1024
	for i, u := range units {
		if f < 1024 || i == len(units)-1 {
			return fmt.Sprintf("%.1f %s", f, u)
		}
		f /= 1024
	}
	return ""
}

func countFormatter(k reflect.Kind) func(reflect.Value) string {
	if k == reflect.Uint || k == reflect.Uint8 || k == reflect.Uint16 || k == reflect.Uint32 || k == reflect.Uint64 || k == reflect.Uintptr {
		return func(v reflect.Value) string {
			u := v.Uint()
			if u > math.MaxInt64 {
				return humanCount(math.MaxInt64)
			}
			return humanCount(int64(u))
		}
	}
	return func(v reflect.Value) string {
		return humanCount(v.Int())
	}
}

func humanCount(n int64) string {
	if n < 0 {
		return "-" + humanCount(-n)
	}
	if n < 1000 {
		return strconv.FormatInt(n, 10)
	}
	units := []string{"K", "M", "G", "T", "P", "E"}
	f := float64(n) / 1000
	for i, u := range units {
		if f < 1000 || i == len(units)-1 {
			return fmt.Sprintf("%.1f %s", f, u)
		}
		f /= 1000
	}
	return ""
}

func percentFormatter(v reflect.Value) string {
	return fmt.Sprintf("%.1f %%", v.Float()*100)
}

// formatRow turns a single T value into the per-column string slice for
// lipgloss.Row. It copies the value into a freshly allocated addressable
// reflect.Value so that pointer-receiver methods (e.g. *big.Int.String) are
// callable on field cells.
func (s *schema) formatRow(value any) []string {
	rv := reflect.ValueOf(value)
	if s.rowIsPtr {
		if !rv.IsValid() || rv.IsNil() {
			return make([]string, len(s.columns))
		}
		rv = rv.Elem() // pointer.Elem() is addressable
	}

	switch s.shape {
	case rowSlice:
		out := make([]string, len(s.columns))
		if !rv.IsValid() {
			return out
		}
		n := rv.Len()
		for i, c := range s.columns {
			if i < n {
				out[i] = c.format(rv.Index(i))
			}
		}
		return out
	default: // rowStruct
		// Copy into an addressable form so pointer-receiver methods on
		// field cells (e.g. *big.Int.String) are callable.
		if !s.rowIsPtr {
			addr := reflect.New(s.rowType).Elem()
			addr.Set(rv)
			rv = addr
		}
		out := make([]string, len(s.columns))
		for i, c := range s.columns {
			out[i] = c.format(rv.Field(c.fieldIndex))
		}
		return out
	}
}
