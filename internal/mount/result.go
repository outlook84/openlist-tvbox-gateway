package mount

import (
	"strings"

	"openlist-tvbox/internal/catvod"
	"openlist-tvbox/internal/i18n"
)

func standardFilters(lang string) []catvod.Filter {
	return []catvod.Filter{
		{Key: "type", Name: i18n.T(lang, i18n.FilterSortType), Value: []catvod.FilterValue{{N: i18n.T(lang, i18n.FilterDefault), V: ""}, {N: i18n.T(lang, i18n.FilterName), V: "name"}, {N: i18n.T(lang, i18n.FilterSize), V: "size"}, {N: i18n.T(lang, i18n.FilterDate), V: "date"}}},
		{Key: "order", Name: i18n.T(lang, i18n.FilterSortOrder), Value: []catvod.FilterValue{{N: i18n.T(lang, i18n.FilterDefault), V: ""}, {N: i18n.T(lang, i18n.FilterAscending), V: "asc"}, {N: i18n.T(lang, i18n.FilterDescending), V: "desc"}}},
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
