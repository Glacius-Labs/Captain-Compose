package event

type Middleware func(next Handler) Handler

func noOpMiddleware(next Handler) Handler {
	return next
}
