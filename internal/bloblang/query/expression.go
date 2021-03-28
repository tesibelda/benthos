package query

import (
	"fmt"
)

// MatchCase represents a single match case of a match expression, where a case
// query is checked and, if true, the underlying query is executed and returned.
type MatchCase struct {
	caseFn  Function
	queryFn Function
}

// NewMatchCase creates a single match case of a match expression, where a case
// query is checked and, if true, the underlying query is executed and returned.
func NewMatchCase(caseFn, queryFn Function) MatchCase {
	return MatchCase{
		caseFn, queryFn,
	}
}

// NewMatchFunction takes a contextual mapping and a list of MatchCases, when
// the function is executed
func NewMatchFunction(contextFn Function, cases ...MatchCase) Function {
	if contextFn == nil {
		contextFn = ClosureFunction(func(ctx FunctionContext) (interface{}, error) {
			var value interface{}
			if v := ctx.Value(); v != nil {
				value = *v
			}
			return value, nil
		}, nil)
	}
	return ClosureFunction(func(ctx FunctionContext) (interface{}, error) {
		ctxVal, err := contextFn.Exec(ctx)
		if err != nil {
			return nil, err
		}
		for i, c := range cases {
			caseCtx := ctx.WithValue("", ctxVal)
			var caseVal interface{}
			if caseVal, err = c.caseFn.Exec(caseCtx); err != nil {
				return nil, fmt.Errorf("failed to check match case %v: %w", i, err)
			}
			if matched, _ := caseVal.(bool); matched {
				return c.queryFn.Exec(caseCtx)
			}
		}
		return Nothing(nil), nil
	}, func(ctx TargetsContext) (TargetsContext, []TargetPath) {
		contextCtx, contextTargets := contextFn.QueryTargets(ctx)
		contextCtx = contextCtx.WithValues(contextTargets).WithValuesAsContext()

		var targets []TargetPath
		for _, c := range cases {
			_, caseTargets := c.caseFn.QueryTargets(contextCtx)
			targets = append(targets, caseTargets...)

			// TODO: Include new current targets in returned context
			_, queryTargets := c.queryFn.QueryTargets(contextCtx)
			targets = append(targets, queryTargets...)
		}

		targets = append(targets, contextTargets...)
		return ctx, targets
	})
}

// NewIfFunction creates a logical if expression from a query which should
// return a boolean value. If the returned boolean is true then the ifFn is
// executed and returned, otherwise elseFn is executed and returned.
func NewIfFunction(queryFn Function, ifFn Function, elseFn Function) Function {
	return ClosureFunction(func(ctx FunctionContext) (interface{}, error) {
		queryVal, err := queryFn.Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to check if condition: %w", err)
		}
		if queryRes, _ := queryVal.(bool); queryRes {
			return ifFn.Exec(ctx)
		}
		if elseFn != nil {
			return elseFn.Exec(ctx)
		}
		return Nothing(nil), nil
	}, aggregateTargetPaths(queryFn, ifFn, elseFn))
}

// NewNamedContextFunction wraps a function and ensures that when the function
// is executed with a new context the context is captured under a new name, with
// the "main" context left intact.
func NewNamedContextFunction(name string, fn Function) Function {
	return &NamedContextFunction{name, fn}
}

// NamedContextFunction wraps a query function in a mechanism that captures the
// current context under an alias.
type NamedContextFunction struct {
	name string
	fn   Function
}

// Name returns the alias under which the context will be captured.
func (n *NamedContextFunction) Name() string {
	return n.name
}

// Exec executes the wrapped query function with the context captured under an
// alias.
func (n *NamedContextFunction) Exec(ctx FunctionContext) (interface{}, error) {
	v, nextCtx := ctx.PopValue()
	if v == nil {
		return nil, fmt.Errorf("failed to capture context %v: %w", n.name, ErrNoContext)
	}
	if n.name != "_" {
		nextCtx = nextCtx.WithNamedValue(n.name, *v)
	}
	return n.fn.Exec(nextCtx)
}

// QueryTargets provides a summary of which fields the underlying query function
// targets.
func (n *NamedContextFunction) QueryTargets(ctx TargetsContext) (TargetsContext, []TargetPath) {
	if n.name == "_" {
		ctx = ctx.PopContext()
	} else {
		ctx = ctx.WithContextAsNamed(n.name)
	}
	return n.fn.QueryTargets(ctx)
}
