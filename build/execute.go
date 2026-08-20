package build

import "context"

type execution struct {
	instances map[string]any
	result    *Result
}

func executePlan(ctx context.Context, p *plan) (*execution, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	constructed := make(map[string]any, len(p.steps))
	result := &Result{Instances: make([]Instance, 0, len(p.steps))}
	for _, n := range p.steps {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		val, err := construct(n, constructed)
		if err != nil {
			return nil, err
		}
		if n.want != nil {
			if err := checkRuntimeType(n.want, val, n.use); err != nil {
				return nil, assembleErr(n.field, n.use.Use, n.use.ID, StageTypeCheck, err)
			}
		}
		constructed[n.use.ID] = val
		result.Instances = append(result.Instances, Instance{ID: n.use.ID, Use: n.use.Use, Value: val})
	}
	return &execution{instances: constructed, result: result}, nil
}
