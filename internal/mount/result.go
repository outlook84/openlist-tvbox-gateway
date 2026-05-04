package mount

import (
	"strings"

	"openlist-tvbox/internal/catvod"
)

func standardFilters() []catvod.Filter {
	return []catvod.Filter{
		{Key: "type", Name: "排序类型", Value: []catvod.FilterValue{{N: "默认", V: ""}, {N: "名称", V: "name"}, {N: "大小", V: "size"}, {N: "修改时间", V: "date"}}},
		{Key: "order", Name: "排序方式", Value: []catvod.FilterValue{{N: "默认", V: ""}, {N: "升序", V: "asc"}, {N: "降序", V: "desc"}}},
	}
}

func paged(vods []catvod.Vod) catvod.Result {
	return catvod.Result{List: vods, Page: 1, PageCount: 1, Limit: len(vods), Total: len(vods)}
}

func serviceErrorKind(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "authorization"):
		return "authorization"
	case strings.Contains(msg, "permission denied"):
		return "permission"
	case strings.Contains(msg, "openlist request failed"):
		return "upstream_request"
	case strings.Contains(msg, "openlist"):
		return "upstream"
	default:
		return "request"
	}
}
