package mount

import (
	"path"
	"sort"
	"strings"

	"openlist-tvbox/internal/config"
	"openlist-tvbox/internal/openlist"
	"openlist-tvbox/internal/utils"
)

func (s *Service) findSubs(m config.Mount, parentRel string, items []openlist.Item, mediaName string) []playSubToken {
	subs := []playSubToken{}
	mediaBase := subtitleMatchBase(mediaName)
	for _, item := range items {
		if !utils.IsSubtitle(item.Name) {
			continue
		}
		if !subtitleMatchesMedia(mediaBase, item.Name) {
			continue
		}
		subID := m.ID + "/" + path.Join(parentRel, item.Name)
		subs = append(subs, playSubToken{Name: item.Name, Ext: utils.Ext(item.Name), ID: subID})
	}
	sort.SliceStable(subs, func(i, j int) bool {
		return subtitleLess(mediaBase, subs[i], subs[j])
	})
	return subs
}

func subtitleLess(mediaBase string, a, b playSubToken) bool {
	aRank, bRank := subtitleRank(mediaBase, a.Name), subtitleRank(mediaBase, b.Name)
	if aRank != bRank {
		return aRank < bRank
	}
	aExt, bExt := subtitleExtRank(a.Ext), subtitleExtRank(b.Ext)
	if aExt != bExt {
		return aExt < bExt
	}
	aLen, bLen := len([]rune(a.Name)), len([]rune(b.Name))
	if aLen != bLen {
		return aLen < bLen
	}
	return mediaNameLess(a.Name, b.Name)
}

func subtitleRank(mediaBase, name string) int {
	tags := subtitleTags(mediaBase, name)
	switch {
	case hasAnyTag(tags, "zh-cn", "chs", "sc", "simplified", "简", "简体", "简中", "中文", "中字"):
		return 0
	case hasAnyTag(tags, "zh", "chi", "zho", "cn"):
		return 1
	case hasAnyTag(tags, "bilingual", "dual", "双语", "中英"):
		return 2
	case strings.EqualFold(subtitleMatchBase(name), mediaBase):
		return 3
	case hasAnyTag(tags, "zh-tw", "cht", "tc", "traditional", "繁", "繁体"):
		return 4
	case hasAnyTag(tags, "en", "eng", "english"):
		return 5
	default:
		return 6
	}
}

func subtitleTags(mediaBase, name string) []string {
	base := subtitleMatchBase(name)
	if len(base) <= len(mediaBase) || !strings.EqualFold(base[:len(mediaBase)], mediaBase) {
		return nil
	}
	tail := strings.TrimSpace(base[len(mediaBase):])
	tail = strings.TrimLeft(tail, ".-_ []")
	if tail == "" {
		return nil
	}
	return strings.FieldsFunc(strings.ToLower(tail), func(r rune) bool {
		switch r {
		case '.', '-', '_', ' ', '[', ']', '(', ')', '+':
			return true
		default:
			return false
		}
	})
}

func hasAnyTag(tags []string, values ...string) bool {
	for _, tag := range tags {
		for _, value := range values {
			if tag == strings.ToLower(value) {
				return true
			}
		}
	}
	return false
}

func subtitleExtRank(ext string) int {
	switch strings.ToLower(ext) {
	case "srt":
		return 0
	case "ass", "ssa":
		return 1
	case "vtt":
		return 2
	default:
		return 3
	}
}

func subtitleMatchesMedia(mediaBase, subtitleName string) bool {
	subBase := subtitleMatchBase(subtitleName)
	if mediaBase == "" || subBase == "" {
		return false
	}
	if strings.EqualFold(subBase, mediaBase) {
		return true
	}
	if len(subBase) <= len(mediaBase) || !strings.EqualFold(subBase[:len(mediaBase)], mediaBase) {
		return false
	}
	switch subBase[len(mediaBase)] {
	case '.', '-', '_', ' ', '[':
		return true
	default:
		return false
	}
}

func subtitleMatchBase(name string) string {
	ext := path.Ext(name)
	if ext == "" {
		return name
	}
	return strings.TrimSuffix(name, ext)
}

func subtitleFormat(ext string) string {
	switch strings.ToLower(ext) {
	case "vtt":
		return "text/vtt"
	case "ass", "ssa":
		return "text/x-ssa"
	default:
		return "application/x-subrip"
	}
}
