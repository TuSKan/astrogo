package docsguard_test

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/tools/go/packages"
)

// floatVerbs are the verbs fmt applies to a float operand without calling its
// String method. %v, %s, %q, %x and %X go through String, which names the unit.
const floatVerbs = "beEfFgG"

// TestNoFloatVerbPrintsARawUnitValue keeps every format string's unit label
// honest about the number beside it.
//
// # Why
//
// unit.Length holds meters, unit.Velocity m/s, unit.Duration seconds and
// angle.Angle radians. %f, %g and %e accept any named float type and print the
// raw value, so a label reading "km/s" or "AU" or "°" is only as good as the
// author's memory of which unit the type holds. go vet's printf check does not
// object, since the operand is a float. #885 found two examples printing meters
// as AU and m/s as km/s; #886 then found six test messages wrong the same way,
// and coord.ObserversLocation.String printing meters with no unit at all.
//
// # The rule
//
// A float verb never takes a unit or angle value that offers a float view of
// itself: it takes the view, which names the unit at the call site, so
// l.Km() under "%.1f km" is checkable by reading and l under it is not. The
// same holds for float64(l), which strips the type and keeps the meters.
//
// Which types that covers is read from the code, not listed: a type in unit
// or angle with a method returning float64 and taking nothing is a type whose
// raw value has a unit the reader cannot see. A new such type is covered the
// day it is added. Types without one, such as WavelengthNM or OpticalDepth,
// carry their unit in the name or have none, and are left alone.
//
// # Scope
//
// Every printf-shaped call, in every file of the module under all three tiers
// and in the examples module: a function or method whose name ends in f, whose
// last parameter is ...any, and whose format is a constant. That catches
// fmt.Sprintf and t.Errorf alike. It needs the type checker, which is why this
// test loads packages rather than parsing files as the others here do.
func TestNoFloatVerbPrintsARawUnitValue(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	scan := unitVerbScan{seen: map[token.Position]bool{}, guarded: map[string]bool{}}
	scan.load(t, root, "-tags="+strings.Join(tiers, ","))
	scan.load(t, filepath.Join(root, "examples"))

	// The scan must have seen the code and the types it guards, or it passes
	// by finding nothing to look at.
	if scan.calls < 2000 {
		t.Fatalf("only %d printf-shaped calls checked; the load is not reaching the module", scan.calls)
	}

	for _, want := range []string{"unit.Length", "unit.Velocity", "unit.Duration", "angle.Angle"} {
		if !scan.guarded[want] {
			t.Errorf("%s is not among the guarded types; the rule that selects them has stopped seeing it", want)
		}
	}

	t.Logf("%d printf-shaped calls checked, %d types guarded", scan.calls, len(scan.guarded))
}

// unitVerbScan accumulates over the loads of both modules.
type unitVerbScan struct {
	// seen holds the operands already reported. A package's own files are
	// loaded twice, once as the package and once inside its test variant.
	seen map[token.Position]bool

	// guarded holds every guarded type unit and angle declare, as pkg.Name.
	guarded map[string]bool

	calls int
}

// recordGuarded notes the guarded types p declares at package level.
func (s *unitVerbScan) recordGuarded(p *packages.Package) {
	scope := p.Types.Scope()

	for _, name := range scope.Names() {
		if tn, ok := scope.Lookup(name).(*types.TypeName); ok {
			if named := guardedType(tn.Type()); named != nil {
				s.guarded[typeName(named)] = true
			}
		}
	}
}

// load type-checks every package under dir, tests included, and checks its
// printf-shaped calls.
func (s *unitVerbScan) load(t *testing.T, dir string, buildFlags ...string) {
	t.Helper()

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir:        dir,
		Tests:      true,
		BuildFlags: buildFlags,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		t.Fatalf("loading %s: %v", dir, err)
	}

	// A package that does not type-check is a package this scan cannot see
	// into, so it fails rather than reporting nothing.
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		for _, e := range p.Errors {
			t.Errorf("%s: %v", p.PkgPath, e)
		}
	})

	for _, p := range pkgs {
		s.recordGuarded(p)

		for _, f := range p.Syntax {
			ast.Inspect(f, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					s.check(t, p, call)
				}

				return true
			})
		}
	}
}

// check reports each float verb in call's format whose operand is a guarded
// value, or float64 of one.
func (s *unitVerbScan) check(t *testing.T, p *packages.Package, call *ast.CallExpr) {
	t.Helper()

	fi := formatParam(p.TypesInfo, call)
	if fi < 0 || fi >= len(call.Args) || call.Ellipsis.IsValid() {
		return
	}

	format := p.TypesInfo.Types[call.Args[fi]].Value
	if format == nil || format.Kind() != constant.String {
		return
	}

	s.calls++

	operands := call.Args[fi+1:]

	for _, op := range formatOperands(constant.StringVal(format)) {
		if op.arg >= len(operands) || !strings.ContainsRune(floatVerbs, op.verb) {
			continue
		}

		expr := operands[op.arg]

		named, stripped := guardedOperand(p.TypesInfo, expr)
		if named == nil {
			continue
		}

		pos := p.Fset.Position(expr.Pos())
		if s.seen[pos] {
			continue
		}

		s.seen[pos] = true

		rel, err := filepath.Rel(repoRoot(t), pos.Filename)
		if err != nil {
			rel = pos.Filename
		}

		what := "this " + typeName(named)
		if stripped {
			what += ", converted to a plain float,"
		}

		t.Errorf("%s:%d: %%%c prints the raw value of %s whatever the label beside it says.\n"+
			"  Format one of its float methods instead, which names what it returns: %s. See #886.",
			filepath.ToSlash(rel), pos.Line, op.verb, what, strings.Join(floatViews(named), ", "))
	}
}

// guardedOperand returns the guarded type behind expr, and whether expr is a
// conversion of such a value to a plain float; or nil when expr is neither.
func guardedOperand(info *types.Info, expr ast.Expr) (*types.Named, bool) {
	if named := guardedType(info.TypeOf(expr)); named != nil {
		return named, false
	}

	conv, ok := ast.Unparen(expr).(*ast.CallExpr)
	if !ok || len(conv.Args) != 1 {
		return nil, false
	}

	if tv, ok := info.Types[conv.Fun]; !ok || !tv.IsType() {
		return nil, false
	}

	if named := guardedType(info.TypeOf(conv.Args[0])); named != nil {
		return named, true
	}

	return nil, false
}

// guardedType returns t as a named type from unit or angle offering a float
// view of itself, or nil.
func guardedType(t types.Type) *types.Named {
	named, ok := t.(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return nil
	}

	switch named.Obj().Pkg().Path() {
	case "github.com/TuSKan/astrogo/unit", "github.com/TuSKan/astrogo/angle":
	default:
		return nil
	}

	if len(floatViews(named)) == 0 {
		return nil
	}

	return named
}

// floatViews lists named's methods that take nothing and return a float64:
// the views of its value in some unit.
func floatViews(named *types.Named) []string {
	var views []string

	for m := range named.Methods() {
		sig, ok := m.Type().(*types.Signature)
		if !ok || sig.Params().Len() != 0 || sig.Results().Len() != 1 {
			continue
		}

		if types.Identical(sig.Results().At(0).Type(), types.Typ[types.Float64]) {
			views = append(views, "."+m.Name()+"()")
		}
	}

	slices.Sort(views)

	return views
}

func typeName(named *types.Named) string {
	return named.Obj().Pkg().Name() + "." + named.Obj().Name()
}

// formatParam returns the index of call's format parameter when call is
// printf-shaped: a function or method named ...f whose last parameter is
// ...any and whose one before it is a string. It returns -1 otherwise.
func formatParam(info *types.Info, call *ast.CallExpr) int {
	var id *ast.Ident

	switch fn := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		id = fn
	case *ast.SelectorExpr:
		id = fn.Sel
	default:
		return -1
	}

	f, ok := info.Uses[id].(*types.Func)
	if !ok || !strings.HasSuffix(f.Name(), "f") {
		return -1
	}

	sig, ok := f.Type().(*types.Signature)
	if !ok || !sig.Variadic() || sig.Params().Len() < 2 {
		return -1
	}

	params := sig.Params()

	last, ok := params.At(params.Len() - 1).Type().(*types.Slice)
	if !ok || !types.IsInterface(last.Elem()) {
		return -1
	}

	i := params.Len() - 2
	if b, ok := params.At(i).Type().(*types.Basic); !ok || b.Kind() != types.String {
		return -1
	}

	return i
}

// formatOperand is one operand a format consumes: the verb, or '*' for a
// width or precision, and the index of its argument after the format.
type formatOperand struct {
	verb rune
	arg  int
}

// formatOperands walks format as fmt does, following * and explicit [n]
// argument indexes, and returns every operand it consumes.
func formatOperands(format string) []formatOperand {
	var ops []formatOperand

	arg := 0

	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}

		i++

		for i < len(format) && strings.IndexByte("+-# 0", format[i]) >= 0 {
			i++
		}

		i, arg = argIndex(format, i, arg)
		i, arg = widthOrPrecision(format, i, arg, &ops)

		if i < len(format) && format[i] == '.' {
			i, arg = argIndex(format, i+1, arg)
			i, arg = widthOrPrecision(format, i, arg, &ops)
		}

		i, arg = argIndex(format, i, arg)
		if i >= len(format) {
			break
		}

		verb, size := utf8.DecodeRuneInString(format[i:])
		i += size - 1

		if verb == '%' {
			continue
		}

		ops = append(ops, formatOperand{verb: verb, arg: arg})
		arg++
	}

	return ops
}

// argIndex reads an explicit [n] at i, if there is one, and moves arg to it.
func argIndex(format string, i, arg int) (int, int) {
	if i >= len(format) || format[i] != '[' {
		return i, arg
	}

	end := strings.IndexByte(format[i:], ']')
	if end < 0 {
		return i, arg
	}

	n := 0

	for _, c := range format[i+1 : i+end] {
		if c < '0' || c > '9' {
			return i, arg
		}

		n = 10*n + int(c-'0')
	}

	if n < 1 {
		return i, arg
	}

	return i + end + 1, n - 1
}

// widthOrPrecision reads a width or precision at i: digits consume nothing,
// and * consumes an operand.
func widthOrPrecision(format string, i, arg int, ops *[]formatOperand) (int, int) {
	if i < len(format) && format[i] == '*' {
		*ops = append(*ops, formatOperand{verb: '*', arg: arg})

		return i + 1, arg + 1
	}

	for i < len(format) && format[i] >= '0' && format[i] <= '9' {
		i++
	}

	return i, arg
}

// TestFormatOperandsFollowsFmt pins the format walk to fmt's own reading,
// since a verb matched to the wrong operand reports the wrong line or none.
func TestFormatOperandsFollowsFmt(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		format string
		want   []formatOperand
	}{
		{"no verbs", nil},
		{"%d%% then %5.2f", []formatOperand{{'d', 0}, {'f', 1}}},
		{"%-*.*e km", []formatOperand{{'*', 0}, {'*', 1}, {'e', 2}}},
		{"%[2]f %[1]s %v", []formatOperand{{'f', 1}, {'s', 0}, {'v', 1}}},
		{"%[3]*.[2]*[1]f", []formatOperand{{'*', 2}, {'*', 1}, {'f', 0}}},
		{"%+.3g°", []formatOperand{{'g', 0}}},
		{"ends in %", nil},
	} {
		if got := formatOperands(tc.format); !slices.Equal(got, tc.want) {
			t.Errorf("formatOperands(%q) = %v, want %v", tc.format, got, tc.want)
		}
	}
}
