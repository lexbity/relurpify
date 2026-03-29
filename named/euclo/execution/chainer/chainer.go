package chainer

import (
	"context"

	chainerpkg "github.com/lexcodex/relurpify/agents/chainer"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
)

type Runner = chainerpkg.ChainerAgent
type Chain = chainerpkg.Chain
type Link = chainerpkg.Link
type Option = chainerpkg.Option

func New(env agentenv.AgentEnvironment, opts ...Option) *Runner {
	return chainerpkg.New(env, opts...)
}

func ExecuteChain(ctx context.Context, env agentenv.AgentEnvironment, task *core.Task, state *core.Context, chain *Chain, opts ...Option) (*core.Result, error) {
	options := append([]Option{chainerpkg.WithChain(chain)}, opts...)
	return New(env, options...).Execute(ctx, task, state)
}
