package main

import (
	"strconv"
)

func primarySuccessStatus(route routeDef) int {
	if route.SuccessStatus != 0 {
		return route.SuccessStatus
	}
	if len(route.SuccessCodes) > 0 {
		return choosePrimarySuccessCode(route.SuccessCodes)
	}
	return 200
}

func ensurePrimarySuccessResponse(op *operation, route routeDef) {
	if op == nil {
		return
	}
	if op.Responses == nil {
		op.Responses = map[string]response{}
	}

	primary := primarySuccessStatus(route)
	if primary == 0 {
		primary = 200
	}
	primaryKey := strconv.Itoa(primary)

	if _, ok := op.Responses[primaryKey]; ok {
		if primary != 200 {
			delete(op.Responses, "200")
		}
		return
	}

	if primary != 200 {
		if existing, ok := op.Responses["200"]; ok {
			op.Responses[primaryKey] = existing
			delete(op.Responses, "200")
			return
		}
	}

	op.Responses[primaryKey] = response{Description: "OK"}
	if primary != 200 {
		delete(op.Responses, "200")
	}
}
