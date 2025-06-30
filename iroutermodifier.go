package router

type IRouterModifier interface {
	GetEndpoint() (endpoint string)
}
