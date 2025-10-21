package middleware

import "github.com/pay-theory/lift/pkg/lift"

type eventAwareMiddleware func(lift.Handler) lift.Handler

func (m eventAwareMiddleware) AppliesToEvents() bool { return true }
