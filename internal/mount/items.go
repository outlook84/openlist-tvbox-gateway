package mount

import (
	"sort"
	"strconv"

	"openlist-tvbox/internal/openlist"
	"openlist-tvbox/internal/utils"
)

func splitItems(items []openlist.Item) ([]openlist.Item, []openlist.Item) {
	folders := []openlist.Item{}
	files := []openlist.Item{}
	for _, item := range items {
		if utils.Ignore(item.Name, item.Type) {
			continue
		}
		if utils.IsFolder(item.Type) {
			folders = append(folders, item)
		} else {
			files = append(files, item)
		}
	}
	return folders, files
}

func orderedMediaItems(items []openlist.Item, selectedName string) []openlist.Item {
	selected := []openlist.Item{}
	others := []openlist.Item{}
	for _, item := range items {
		if !utils.IsMedia(item.Name, item.Type) {
			continue
		}
		if item.Name == selectedName {
			selected = append(selected, item)
			continue
		}
		others = append(others, item)
	}
	sort.SliceStable(others, func(i, j int) bool {
		return mediaNameLess(others[i].Name, others[j].Name)
	})
	return append(selected, others...)
}

func hasMedia(items []openlist.Item) bool {
	for _, item := range items {
		if utils.IsMedia(item.Name, item.Type) {
			return true
		}
	}
	return false
}

func formatMediaCount(count int) string {
	if count == 1 {
		return "1 个视频"
	}
	return strconv.Itoa(count) + " 个视频"
}
