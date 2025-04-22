package event

type Middleware func(next Handler) Handler
