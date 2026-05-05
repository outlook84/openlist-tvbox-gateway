package i18n

const (
	DefaultLanguage = "zh-CN"
	English         = "en"
)

type Key string

const (
	FilterSortType         Key = "filter.sortType"
	FilterSortOrder        Key = "filter.sortOrder"
	FilterDefault          Key = "filter.default"
	FilterName             Key = "filter.name"
	FilterSize             Key = "filter.size"
	FilterDate             Key = "filter.date"
	FilterAscending        Key = "filter.ascending"
	FilterDescending       Key = "filter.descending"
	ActionPlayDirectory    Key = "action.playDirectory"
	ActionRefreshDirectory Key = "action.refreshDirectory"
	ActionRefreshDone      Key = "action.refreshDone"
	RemarkPlayDirectory    Key = "remark.playDirectory"
	RemarkCurrentDir       Key = "remark.currentDir"
	RemarkRefreshDone      Key = "remark.refreshDone"
)

var messages = map[string]map[Key]string{
	DefaultLanguage: {
		FilterSortType:         "排序类型",
		FilterSortOrder:        "排序方式",
		FilterDefault:          "默认",
		FilterName:             "名称",
		FilterSize:             "大小",
		FilterDate:             "修改时间",
		FilterAscending:        "升序",
		FilterDescending:       "降序",
		ActionPlayDirectory:    "播放此目录",
		ActionRefreshDirectory: "刷新此目录",
		ActionRefreshDone:      "刷新完成",
		RemarkPlayDirectory:    "播放当前目录",
		RemarkCurrentDir:       "当前目录",
		RemarkRefreshDone:      "返回目录查看最新内容",
	},
	English: {
		FilterSortType:         "Sort by",
		FilterSortOrder:        "Order",
		FilterDefault:          "Default",
		FilterName:             "Name",
		FilterSize:             "Size",
		FilterDate:             "Modified time",
		FilterAscending:        "Ascending",
		FilterDescending:       "Descending",
		ActionPlayDirectory:    "Play this folder",
		ActionRefreshDirectory: "Refresh this folder",
		ActionRefreshDone:      "Refresh complete",
		RemarkPlayDirectory:    "Play current folder",
		RemarkCurrentDir:       "Current folder",
		RemarkRefreshDone:      "Return to the folder to view the latest content",
	},
}

func NormalizeLanguage(language string) string {
	switch language {
	case "en", "en-US", "en-GB":
		return English
	case "", "zh", "zh-CN", "zh-Hans":
		return DefaultLanguage
	default:
		return DefaultLanguage
	}
}

func T(language string, key Key) string {
	lang := NormalizeLanguage(language)
	if value := messages[lang][key]; value != "" {
		return value
	}
	return messages[DefaultLanguage][key]
}
