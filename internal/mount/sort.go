package mount

import (
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"openlist-tvbox/internal/openlist"
)

func sortItems(sortType, order string, items []openlist.Item) {
	if sortType == "" {
		sortType = "name"
	}
	if order == "" {
		order = "asc"
	}
	asc := order == "asc"
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		switch sortType {
		case "name":
			if asc {
				return mediaNameLess(a.Name, b.Name)
			}
			return mediaNameLess(b.Name, a.Name)
		case "size":
			if asc {
				return a.Size < b.Size
			}
			return a.Size > b.Size
		case "date":
			at, aOK := a.ModTimeValue()
			bt, bOK := b.ModTimeValue()
			if aOK && bOK {
				if asc {
					return at.Before(bt)
				}
				return at.After(bt)
			}
			if asc {
				return a.ModTime() < b.ModTime()
			}
			return a.ModTime() > b.ModTime()
		default:
			return false
		}
	})
}

var seasonEpisodePattern = regexp.MustCompile(`(?i)(^|[^a-z0-9])s(\d{1,3})\s*e(\d{1,4})([^a-z0-9]|$)`)

func mediaNameLess(a, b string) bool {
	cmp := compareMediaName(a, b)
	if cmp != 0 {
		return cmp < 0
	}
	return a < b
}

func compareMediaName(a, b string) int {
	aBase, bBase := sortableName(a), sortableName(b)
	aTitle, aSeason, aEpisode, aOK := seasonEpisodeKey(aBase)
	bTitle, bSeason, bEpisode, bOK := seasonEpisodeKey(bBase)
	if aOK && bOK {
		if cmp := naturalCompare(aTitle, bTitle); cmp != 0 {
			return cmp
		}
		if aSeason != bSeason {
			return compareInt(aSeason, bSeason)
		}
		if aEpisode != bEpisode {
			return compareInt(aEpisode, bEpisode)
		}
	}
	return naturalCompare(aBase, bBase)
}

func sortableName(name string) string {
	ext := path.Ext(name)
	if ext == "" {
		return name
	}
	return strings.TrimSuffix(name, ext)
}

func seasonEpisodeKey(name string) (string, int, int, bool) {
	loc := seasonEpisodePattern.FindStringSubmatchIndex(name)
	if loc == nil {
		return "", 0, 0, false
	}
	match := seasonEpisodePattern.FindStringSubmatch(name)
	if len(match) != 5 {
		return "", 0, 0, false
	}
	season, err := strconv.Atoi(match[2])
	if err != nil {
		return "", 0, 0, false
	}
	episode, err := strconv.Atoi(match[3])
	if err != nil {
		return "", 0, 0, false
	}
	title := strings.TrimSpace(name[:loc[0]] + name[loc[1]:])
	return title, season, episode, true
}

func naturalCompare(a, b string) int {
	aRunes, bRunes := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	ai, bi := 0, 0
	for ai < len(aRunes) && bi < len(bRunes) {
		aDigit, bDigit := isASCIIDigit(aRunes[ai]), isASCIIDigit(bRunes[bi])
		if aDigit && bDigit {
			aStart, bStart := ai, bi
			for ai < len(aRunes) && isASCIIDigit(aRunes[ai]) {
				ai++
			}
			for bi < len(bRunes) && isASCIIDigit(bRunes[bi]) {
				bi++
			}
			if cmp := compareNumberRunes(aRunes[aStart:ai], bRunes[bStart:bi]); cmp != 0 {
				return cmp
			}
			continue
		}
		if aRunes[ai] != bRunes[bi] {
			if aRunes[ai] < bRunes[bi] {
				return -1
			}
			return 1
		}
		ai++
		bi++
	}
	return compareInt(len(aRunes)-ai, len(bRunes)-bi)
}

func compareNumberRunes(a, b []rune) int {
	aTrimmed, bTrimmed := trimLeadingZeroes(a), trimLeadingZeroes(b)
	if len(aTrimmed) != len(bTrimmed) {
		return compareInt(len(aTrimmed), len(bTrimmed))
	}
	for i := range aTrimmed {
		if aTrimmed[i] != bTrimmed[i] {
			if aTrimmed[i] < bTrimmed[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func trimLeadingZeroes(value []rune) []rune {
	i := 0
	for i < len(value)-1 && value[i] == '0' {
		i++
	}
	return value[i:]
}

func isASCIIDigit(value rune) bool {
	return value >= '0' && value <= '9'
}

func compareInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
