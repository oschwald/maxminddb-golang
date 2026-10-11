package main

import (
	"fmt"
	"go/token"
	"go/types"
	"strings"
)

// inferredDecoder uses type parameters instead of private imported names.
// Its plan uses the same emitters and validation as a concrete decoder.
type inferredDecoder struct {
	plan      *decodePlan
	name      string
	params    []*types.TypeParam
	nextParam int
}

func (g *generator) prepareInferredDecoder(plan *decodePlan) {
	helper := &inferredDecoder{}
	helper.plan = g.inferPlanTypes(plan, helper)
	if len(helper.params) == 0 {
		return
	}
	helper.name = fmt.Sprintf("maxminddbGen_%s_Inferred%d", g.helperNamespace, len(g.inferred))
	plan.inferred = helper
	g.inferred = append(g.inferred, helper)
}

func (g *generator) inferPlanTypes(plan *decodePlan, helper *inferredDecoder) *decodePlan {
	copyPlan := *plan
	constraint := plan.typ.Underlying()
	changed := !g.canNameType(plan.typ)
	switch plan.kind {
	case decodeBytes:
		constraint = types.NewSlice(types.Universe.Lookup("byte").Type())
	case decodePointer, decodeSlice:
		copyPlan.elem = g.inferPlanTypes(plan.elem, helper)
		changed = changed || copyPlan.elem != plan.elem
		if plan.kind == decodePointer {
			constraint = types.NewPointer(copyPlan.elem.typ)
		} else {
			constraint = types.NewSlice(copyPlan.elem.typ)
		}
	case decodeMap:
		keyPlan := plan.keyPlan
		if keyPlan == nil {
			keyPlan = &decodePlan{
				typ:   plan.key,
				kind:  decodeBasic,
				basic: plan.key.Underlying().(*types.Basic),
			}
		}
		inferredKey := g.inferPlanTypes(keyPlan, helper)
		copyPlan.key = inferredKey.typ
		if plan.keyPlan != nil {
			copyPlan.keyPlan = inferredKey
		}
		copyPlan.elem = g.inferPlanTypes(plan.elem, helper)
		changed = changed || inferredKey != keyPlan || copyPlan.elem != plan.elem
		constraint = types.NewMap(copyPlan.key, copyPlan.elem.typ)
	case decodeUnmarshaler, decodeCursorUnmarshaler:
		constraint = nil
		if basic, ok := plan.typ.Underlying().(*types.Basic); ok {
			constraint = basic
		}
	default:
	}
	if !changed {
		return plan
	}
	var terms []types.Type
	if constraint != nil {
		terms = []types.Type{types.NewUnion([]*types.Term{types.NewTerm(true, constraint)})}
	}
	copyPlan.typ = g.inferredTypeParam(helper, types.NewInterfaceType(nil, terms).Complete())
	if plan.kind == decodeUnmarshaler || plan.kind == decodeCursorUnmarshaler {
		iface := g.unmarshaler
		if plan.kind == decodeCursorUnmarshaler {
			iface = g.cursorDecoder
		}
		// Infer the pointer type too, so its custom methods remain callable.
		copyPlan.unmarshalType = g.inferredTypeParam(
			helper,
			types.NewInterfaceType(nil, []types.Type{
				types.NewPointer(copyPlan.typ), iface,
			}).Complete(),
		)
	}
	return &copyPlan
}

func (g *generator) inferredTypeParam(
	helper *inferredDecoder,
	constraint types.Type,
) *types.TypeParam {
	var name string
	for {
		name = fmt.Sprintf("_maxminddbgenType%d", helper.nextParam)
		helper.nextParam++
		if g.pkg.Types.Scope().Lookup(name) == nil {
			break
		}
	}
	parameter := types.NewTypeParam(types.NewTypeName(token.NoPos, nil, name, nil), constraint)
	helper.params = append(helper.params, parameter)
	return parameter
}

func (g *generator) canNameType(t types.Type) bool {
	var object *types.TypeName
	var arguments *types.TypeList
	switch t := t.(type) {
	case *types.Alias:
		object, arguments = t.Obj(), t.TypeArgs()
	case *types.Named:
		object, arguments = t.Obj(), t.TypeArgs()
	case *types.Pointer:
		return g.canNameType(t.Elem())
	case *types.Slice:
		return g.canNameType(t.Elem())
	case *types.Array:
		return g.canNameType(t.Elem())
	case *types.Chan:
		return g.canNameType(t.Elem())
	case *types.Map:
		return g.canNameType(t.Key()) && g.canNameType(t.Elem())
	case *types.Struct:
		for field := range t.Fields() {
			if !g.canNameObject(field) || !g.canNameType(field.Type()) {
				return false
			}
		}
		return true
	case *types.Signature:
		return g.canNameType(t.Params()) && g.canNameType(t.Results())
	case *types.Tuple:
		for variable := range t.Variables() {
			if !g.canNameType(variable.Type()) {
				return false
			}
		}
		return true
	case *types.Interface:
		for method := range t.ExplicitMethods() {
			if !g.canNameObject(method) || !g.canNameType(method.Type()) {
				return false
			}
		}
		for embedded := range t.EmbeddedTypes() {
			if !g.canNameType(embedded) {
				return false
			}
		}
		return true
	default:
		return true
	}
	if !g.canNameObject(object) {
		return false
	}
	for argument := range arguments.Types() {
		if !g.canNameType(argument) {
			return false
		}
	}
	return true
}

func (g *generator) canNameObject(object types.Object) bool {
	return object.Pkg() == nil || object.Pkg() == g.pkg.Types || object.Exported()
}

func (g *generator) emitInferredDecoder(out *strings.Builder, helper *inferredDecoder) {
	mmdbdata := g.importNames[mmdbdataPath]
	fmt.Fprintf(out, "func %s[", helper.name)
	for _, parameter := range helper.params {
		fmt.Fprintf(out, "%s %s,", parameter.Obj().Name(), g.typeString(parameter.Constraint()))
	}
	fmt.Fprintf(out, "](cursor %s.Cursor, out *%s) (%s.Cursor, error) {\n",
		mmdbdata, g.typeString(helper.plan.typ), mmdbdata)
	fmt.Fprintf(out, "\tvar next %s.Cursor\n\tvar err error\n", mmdbdata)
	g.emitDecode(out, helper.plan, "cursor", "(*out)", "next", "err", 1)
	fmt.Fprintf(out, "\treturn next, %s\n}\n\n", g.errorExpression(helper.plan, "err"))
}
