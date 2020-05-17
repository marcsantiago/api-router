# api-router
![Go](https://github.com/marcsantiago/api-router/workflows/Go/badge.svg)

The pupose of this package is to pick the best API endpoint for reduced latency.

e.g API endpoint has 3 regional endpoints:

us-east-1
us-east-2
us-west-1

You have an app deployed on us-east-2
You'd want to use us-east-2, but you have to know that ahead of time.
Or you select us-east-2, but that service is down at that region...so you'd
want to gracefully fallback to us-east-1.. This package is designed to mediate which URI your app should call.
