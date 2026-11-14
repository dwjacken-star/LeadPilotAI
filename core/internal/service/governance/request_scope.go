package governance

import (
	"context"
	"errors"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

type principalContextKey struct{}

func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, p)
}

func RequestPrincipal(ctx context.Context) (*Principal, error) {
	if p, ok := ctx.Value(principalContextKey{}).(*Principal); ok {
		return p, nil
	}
	request := g.RequestFromCtx(ctx)
	if request == nil {
		return nil, nil
	}
	return Load(ctx, request.GetCtxVar("accountId").Int64())
}

func ScopeRequestModel(ctx context.Context, model *gdb.Model, permission, alias string) (*gdb.Model, error) {
	p, err := RequestPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return model.Ctx(ctx), nil
	}
	if !p.Has(permission) {
		return nil, errors.New("permission required: " + permission)
	}
	predicate, err := p.Predicate(ctx, permission, alias)
	if err != nil {
		return nil, err
	}
	return model.Ctx(ctx).Where(predicate), nil
}
