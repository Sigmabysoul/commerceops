package printing

import "context"

type agentKey struct{}

func withAgent(ctx context.Context, p AgentPrincipal) context.Context {
	return context.WithValue(ctx, agentKey{}, p)
}
func agentPrincipalFromContext(ctx context.Context) (AgentPrincipal, bool) {
	p, ok := ctx.Value(agentKey{}).(AgentPrincipal)
	return p, ok
}
func agentPrincipalRequest(ctx context.Context) AgentPrincipal {
	p, _ := agentPrincipalFromContext(ctx)
	return p
}
