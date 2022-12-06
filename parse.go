package predicate

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strconv"
	"strings"

	"github.com/gravitational/trace"
)

func NewParser(d Def) (Parser, error) {
	return &predicateParser{d: d}, nil
}

type predicateParser struct {
	d Def
}

func (p *predicateParser) Parse(in string) (interface{}, error) {
	expr, err := parser.ParseExpr(in)
	if err != nil {
		return nil, err
	}

	return p.parse(expr)
}

func (p *predicateParser) parse(expr ast.Expr) (interface{}, error) {
	switch n := expr.(type) {
	case *ast.BinaryExpr:
		val, err := p.parseBinaryExpr(n)
		return val, trace.Wrap(err)

	case *ast.ParenExpr:
		val, err := p.parse(n.X)
		return val, trace.Wrap(err)

	case *ast.UnaryExpr:
		val, err := p.parseUnaryExpr(n)
		return val, trace.Wrap(err)

	case *ast.BasicLit:
		val, err := literalToValue(n)
		return val, trace.Wrap(err)

	case *ast.IndexExpr:
		val, err := p.parseIndexExpr(n)
		return val, trace.Wrap(err)

	case *ast.SelectorExpr:
		val, err := p.parseSelectorExpr(n)
		return val, trace.Wrap(err)

	case *ast.Ident:
		if p.d.GetIdentifier == nil {
			return nil, trace.NotFound("%v is not defined", n.Name)
		}
		val, err := p.d.GetIdentifier([]string{n.Name})
		return val, trace.Wrap(err)

	case *ast.CallExpr:
		val, err := p.parseCallExpr(n)
		return val, trace.Wrap(err)

	default:
		return nil, trace.BadParameter("%T is not supported", expr)
	}
}

func (p *predicateParser) parseBinaryExpr(expr *ast.BinaryExpr) (interface{}, error) {
	x, err := p.parse(expr.X)
	if err != nil {
		return nil, err
	}

	y, err := p.parse(expr.Y)
	if err != nil {
		return nil, err
	}

	val, err := p.joinPredicates(expr.Op, x, y)
	return val, trace.Wrap(err)
}

func (p *predicateParser) parseUnaryExpr(expr *ast.UnaryExpr) (interface{}, error) {
	joinFn, err := p.getJoinFunction(expr.Op)
	if err != nil {
		return nil, err
	}

	node, err := p.parse(expr.X)
	if err != nil {
		return nil, err
	}

	val, err := callFunction(joinFn, []interface{}{node})
	return val, trace.Wrap(err)
}

func (p *predicateParser) parseIndexExpr(expr *ast.IndexExpr) (interface{}, error) {
	if p.d.GetProperty == nil {
		return nil, trace.NotFound("properties are not supported")
	}

	mapVal, err := p.parse(expr.X)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	keyVal, err := p.parse(expr.Index)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	val, err := p.d.GetProperty(mapVal, keyVal)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return val, nil
}

func (p *predicateParser) evaluateArguments(nodes []ast.Expr) ([]interface{}, error) {
	out := make([]interface{}, len(nodes))
	for i, n := range nodes {
		val, err := p.parse(n)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		out[i] = val
	}
	return out, nil
}

func (p *predicateParser) parseSelectorExpr(expr *ast.SelectorExpr) (interface{}, error) {
	fields, err := evaluateSelector(expr, []string{})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if p.d.GetIdentifier == nil {
		return nil, trace.NotFound("%v is not defined", strings.Join(fields, "."))
	}

	val, err := p.d.GetIdentifier(fields)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return val, nil
}

// evaluateSelector recursively evaluates the selector field and returns a list
// of properties at the end.
func evaluateSelector(sel *ast.SelectorExpr, fields []string) ([]string, error) {
	fields = append([]string{sel.Sel.Name}, fields...)
	switch l := sel.X.(type) {
	case *ast.SelectorExpr:
		return evaluateSelector(l, fields)

	case *ast.Ident:
		fields = append([]string{l.Name}, fields...)
		return fields, nil

	default:
		return nil, trace.BadParameter("unsupported selector type: %T", l)
	}
}

func (p *predicateParser) parseCallExpr(expr *ast.CallExpr) (interface{}, error) {
	fn, args, err := p.getFunctionAndArgs(expr)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	arguments, err := p.evaluateArguments(args)
	if err != nil {
		return nil, err
	}

	val, err := callFunction(fn, arguments)
	return val, trace.Wrap(err)
}

func (p *predicateParser) getFunction(name string) (interface{}, error) {
	v, ok := p.d.Functions[name]
	if !ok {
		return nil, trace.BadParameter("unsupported function: %s", name)
	}
	return v, nil
}

func (p *predicateParser) joinPredicates(op token.Token, a, b interface{}) (interface{}, error) {
	joinFn, err := p.getJoinFunction(op)
	if err != nil {
		return nil, err
	}

	return callFunction(joinFn, []interface{}{a, b})
}

func (p *predicateParser) getJoinFunction(op token.Token) (interface{}, error) {
	var fn interface{}
	switch op {
	case token.NOT:
		fn = p.d.Operators.NOT
	case token.LAND:
		fn = p.d.Operators.AND
	case token.LOR:
		fn = p.d.Operators.OR
	case token.GTR:
		fn = p.d.Operators.GT
	case token.GEQ:
		fn = p.d.Operators.GE
	case token.LSS:
		fn = p.d.Operators.LT
	case token.LEQ:
		fn = p.d.Operators.LE
	case token.EQL:
		fn = p.d.Operators.EQ
	case token.NEQ:
		fn = p.d.Operators.NEQ
	}
	if fn == nil {
		return nil, trace.BadParameter("%v is not supported", op)
	}
	return fn, nil
}

func (p *predicateParser) getFunctionAndArgs(callExpr *ast.CallExpr) (interface{}, []ast.Expr, error) {
	switch f := callExpr.Fun.(type) {
	case *ast.Ident:
		// Plain function with a single identifier name.
		fn, err := p.getFunction(f.Name)
		return fn, callExpr.Args, trace.Wrap(err)
	case *ast.SelectorExpr:
		// This is a selector like number.DivisibleBy(2) or set("a", "b").contains("b")

		// First, check if we have a matching registered method.
		method, haveMethod := p.d.Methods[f.Sel.Name]
		if haveMethod {
			// Pass the method receiver as the first arg, it will first be
			// evaluated with the rest of the arguments
			args := append([]ast.Expr{f.X}, callExpr.Args...)
			return method, args, nil
		}

		// If this isn't a method, it may be a module function like "number.DivisibleBy"
		id, okIdent := f.X.(*ast.Ident)
		if !okIdent {
			return nil, nil, trace.BadParameter("expected selector identifier, got: %T", f.X)
		}
		fnName := fmt.Sprintf("%s.%s", id.Name, f.Sel.Name)
		fn, err := p.getFunction(fnName)
		return fn, callExpr.Args, trace.Wrap(err)
	default:
		return nil, nil, trace.BadParameter("unknown function type %T", f)
	}
}

func literalToValue(a *ast.BasicLit) (interface{}, error) {
	switch a.Kind {
	case token.FLOAT:
		value, err := strconv.ParseFloat(a.Value, 64)
		if err != nil {
			return nil, trace.BadParameter("failed to parse argument: %s, error: %s", a.Value, err)
		}
		return value, nil

	case token.INT:
		value, err := strconv.Atoi(a.Value)
		if err != nil {
			return nil, trace.BadParameter("failed to parse argument: %s, error: %s", a.Value, err)
		}
		return value, nil

	case token.STRING:
		value, err := strconv.Unquote(a.Value)
		if err != nil {
			return nil, trace.BadParameter("failed to parse argument: %s, error: %s", a.Value, err)
		}
		return value, nil
	}

	return nil, trace.BadParameter("unsupported function argument type: '%v'", a.Kind)
}

func callFunction(f interface{}, args []interface{}) (v interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = trace.BadParameter("%s", r)
		}
	}()

	arguments := make([]reflect.Value, len(args))
	for i, a := range args {
		arguments[i] = reflect.ValueOf(a)
	}

	fn := reflect.ValueOf(f)

	ret := fn.Call(arguments)
	switch len(ret) {
	case 1:
		return ret[0].Interface(), nil

	case 2:
		v, e := ret[0].Interface(), ret[1].Interface()
		if e == nil {
			return v, nil
		}
		err, ok := e.(error)
		if !ok {
			return nil, trace.BadParameter("expected error as a second return value, got %T", e)
		}
		return v, err

	default:
		return nil, trace.BadParameter("expected at least one return argument for '%v'", fn)
	}
}
