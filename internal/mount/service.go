package mount

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"openlist-tvbox/internal/catvod"
	"openlist-tvbox/internal/config"
	"openlist-tvbox/internal/openlist"
	"openlist-tvbox/internal/utils"
)

const (
	folderPic        = "/assets/icons/folder.png"
	videoPic         = "/assets/icons/video.png"
	audioPic         = "/assets/icons/audio.png"
	filePic          = "/assets/icons/file.png"
	listPic          = "/assets/icons/playlist.png"
	refreshPic       = "/assets/icons/refresh.png"
	maxNoteNameRunes = 12
)

type OpenListClient interface {
	List(ctx context.Context, backend config.Backend, path, password string) ([]openlist.Item, error)
	RefreshList(ctx context.Context, backend config.Backend, path, password string) ([]openlist.Item, error)
	Get(ctx context.Context, backend config.Backend, path, password string) (openlist.Item, error)
	Search(ctx context.Context, backend config.Backend, path, keyword, password string) ([]openlist.Item, error)
}

type Service struct {
	cfg      *config.Config
	client   OpenListClient
	logger   *slog.Logger
	backends map[string]config.Backend
	scopes   map[string]*scope
}

type scope struct {
	id     string
	tvbox  config.TVBox
	mounts []config.Mount
	byID   map[string]config.Mount
}

type playToken struct {
	ID   string         `json:"id"`
	Subs []playSubToken `json:"subs,omitempty"`
}

type playSubToken struct {
	Name string `json:"name"`
	Ext  string `json:"ext"`
	ID   string `json:"id"`
}

func NewService(cfg *config.Config, client OpenListClient, logger *slog.Logger) *Service {
	s := &Service{cfg: cfg, client: client, logger: logger, backends: map[string]config.Backend{}, scopes: map[string]*scope{}}
	for _, b := range cfg.Backends {
		s.backends[b.ID] = b
	}
	for _, sub := range cfg.Subs {
		sc := &scope{id: sub.ID, tvbox: sub.TVBox, mounts: sub.Mounts, byID: map[string]config.Mount{}}
		for _, m := range sub.Mounts {
			sc.byID[m.ID] = m
		}
		s.scopes[sub.ID] = sc
	}
	return s
}

func (s *Service) Config() *config.Config {
	return s.cfg
}

func (s *Service) HomeForSub(subID string) catvod.Result {
	sc, err := s.scope(subID)
	if err != nil {
		return catvod.Result{Error: err.Error()}
	}
	classes := []catvod.Class{}
	filters := map[string][]catvod.Filter{}
	for _, m := range sc.mounts {
		if m.Hidden {
			continue
		}
		classes = append(classes, catvod.Class{TypeID: m.ID, TypeName: m.Name, TypeFlag: "1"})
		filters[m.ID] = standardFilters()
	}
	return catvod.Result{Class: classes, Filters: filters}
}

func (s *Service) CategoryForSub(ctx context.Context, subID, tid, sortType, order string) (catvod.Result, error) {
	ref, err := s.resolveScopedID(subID, tid)
	if err != nil {
		return catvod.Result{}, err
	}
	items, err := s.client.List(ctx, ref.backend, ref.backendPath, ref.password)
	if err != nil {
		return catvod.Result{}, err
	}
	folders, files := splitItems(items)
	sortItems(sortType, order, folders)
	sortItems(sortType, order, files)
	vods := make([]catvod.Vod, 0, len(folders)+len(files))
	if ref.mount.Refresh {
		vods = append(vods, refreshDirectoryVod(ref.mount, ref.relPath))
	}
	if hasMedia(files) {
		// Synthetic action items are shown before real OpenList entries so
		// remote-control users can play the current directory immediately.
		vods = append(vods, playDirectoryVod(ref.mount, ref.relPath, len(orderedMediaItems(files, ""))))
	}
	for _, item := range append(folders, files...) {
		if utils.Ignore(item.Name, item.Type) {
			continue
		}
		vods = append(vods, s.vodForItem(ref.mount, ref.relPath, item, ""))
	}
	return paged(vods), nil
}

func (s *Service) RefreshForSub(ctx context.Context, subID, id string) (catvod.Result, error) {
	ref, err := s.resolveScopedID(subID, id)
	if err != nil {
		return catvod.Result{}, err
	}
	if !ref.mount.Refresh {
		return catvod.Result{}, errors.New("refresh is not enabled for this mount")
	}
	if _, err := s.client.RefreshList(ctx, ref.backend, ref.backendPath, ref.password); err != nil {
		return catvod.Result{}, err
	}
	return catvod.Result{List: []catvod.Vod{{VodID: id, VodName: "刷新完成", VodPic: listPic, VodRemarks: "返回目录查看最新内容", VodTag: "folder", TypeFlag: "folder"}}}, nil
}

func (s *Service) DetailForSub(ctx context.Context, subID, id string) (catvod.Result, error) {
	ref, err := s.resolveScopedID(subID, id)
	if err != nil {
		return catvod.Result{}, err
	}
	items, mediaParentRel, selectedName, err := s.detailItems(ctx, ref)
	if err != nil {
		return catvod.Result{}, err
	}
	playURLs := []string{}
	mediaItems := orderedMediaItems(items, selectedName)
	for _, item := range mediaItems {
		subs := s.findSubs(ref.mount, mediaParentRel, items, item.Name)
		mediaID := encodePlayToken(playToken{ID: ref.mount.ID + "/" + path.Join(mediaParentRel, item.Name), Subs: subs})
		playURLs = append(playURLs, playItemName(item.Name)+"$"+mediaID)
	}
	vod := catvod.Vod{
		VodID:       id,
		VodName:     fallbackName(path.Base(ref.relPath), ref.mount.Name),
		VodPic:      detailPic(ref.mount, selectedName, items),
		VodPlayFrom: ref.mount.ID,
		VodPlayURL:  strings.Join(playURLs, "#"),
	}
	return catvod.Result{List: []catvod.Vod{vod}}, nil
}

func (s *Service) detailItems(ctx context.Context, ref resolved) ([]openlist.Item, string, string, error) {
	if ref.relPath == "" {
		items, err := s.client.List(ctx, ref.backend, ref.backendPath, ref.password)
		return items, "", "", err
	}
	parentRel, name := splitRel(ref.relPath)
	parentBackend, err := utils.Join(ref.mount.Path, parentRel)
	if err != nil {
		return nil, "", "", err
	}
	parentItems, err := s.client.List(ctx, ref.backend, parentBackend, s.password(ref.mount, parentBackend))
	if err != nil {
		return nil, "", "", err
	}
	for _, item := range parentItems {
		if item.Name != name {
			continue
		}
		if !utils.IsFolder(item.Type) {
			return parentItems, parentRel, name, nil
		}
		items, err := s.client.List(ctx, ref.backend, ref.backendPath, ref.password)
		return items, ref.relPath, "", err
	}
	return parentItems, parentRel, name, nil
}

func (s *Service) SearchForSub(ctx context.Context, subID, keyword string) (catvod.Result, error) {
	sc, err := s.scope(subID)
	if err != nil {
		return catvod.Result{}, err
	}
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return paged(nil), nil
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := []catvod.Vod{}
	for _, m := range sc.mounts {
		if m.Hidden || !m.SearchEnabled() {
			continue
		}
		mountCfg := m
		backend := s.backends[m.Backend]
		wg.Add(1)
		go func() {
			defer wg.Done()
			items, err := s.client.Search(ctx, backend, mountCfg.Path, keyword, s.password(mountCfg, mountCfg.Path))
			if err != nil {
				if s.logger != nil {
					s.logger.Warn("search mount failed", "mount", mountCfg.ID, "error_kind", serviceErrorKind(err))
				}
				return
			}
			local := make([]catvod.Vod, 0, len(items))
			for _, item := range items {
				if utils.Ignore(item.Name, item.Type) {
					continue
				}
				parent := s.relFromBackend(mountCfg, item.DisplayPath())
				local = append(local, s.vodForItem(mountCfg, parent, item, mountCfg.Name))
			}
			mu.Lock()
			results = append(results, local...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return paged(results), nil
}

func (s *Service) PlayForSub(ctx context.Context, subID, encodedID string) (catvod.Result, error) {
	token, ok := decodePlayToken(encodedID)
	if !ok || token.ID == "" {
		return catvod.Result{}, errors.New("invalid play id")
	}
	ref, err := s.resolveScopedID(subID, token.ID)
	if err != nil {
		return catvod.Result{}, err
	}
	item, err := s.client.Get(ctx, ref.backend, ref.backendPath, ref.password)
	if err != nil {
		return catvod.Result{}, err
	}
	subs := []catvod.Sub{}
	for _, sub := range token.Subs {
		subRef, err := s.resolveScopedID(subID, sub.ID)
		if err != nil {
			continue
		}
		subItem, err := s.client.Get(ctx, subRef.backend, subRef.backendPath, subRef.password)
		if err != nil || subItem.Link() == "" {
			continue
		}
		subs = append(subs, catvod.Sub{Name: sub.Name, Ext: sub.Ext, Format: subtitleFormat(sub.Ext), URL: subItem.Link()})
	}
	parse := 0
	subt := ""
	if len(subs) > 0 {
		subt = subs[0].URL
	}
	return catvod.Result{Parse: &parse, URL: item.Link(), Subt: subt, Header: playHeader(item.Link(), ref.mount.PlayHeaders), Subs: subs}, nil
}

type resolved struct {
	mount       config.Mount
	backend     config.Backend
	relPath     string
	backendPath string
	password    string
}

func (s *Service) resolveScopedID(subID, id string) (resolved, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return resolved{}, errors.New("empty id")
	}
	sc, err := s.scope(subID)
	if err != nil {
		return resolved{}, err
	}
	mountID, rel, _ := strings.Cut(id, "/")
	m, ok := sc.byID[mountID]
	if !ok {
		return resolved{}, errors.New("unknown mount")
	}
	cleanRel, err := utils.CleanRelative(rel)
	if err != nil {
		return resolved{}, err
	}
	backendPath, err := utils.Join(m.Path, cleanRel)
	if err != nil {
		return resolved{}, err
	}
	return resolved{mount: m, backend: s.backends[m.Backend], relPath: cleanRel, backendPath: backendPath, password: s.password(m, backendPath)}, nil
}

func (s *Service) scope(subID string) (*scope, error) {
	sc, ok := s.scopes[subID]
	if !ok {
		return nil, errors.New("unknown sub")
	}
	return sc, nil
}

func (s *Service) password(m config.Mount, backendPath string) string {
	if len(m.Params) == 0 {
		return ""
	}
	bestPrefix := ""
	bestPassword := ""
	for raw, pass := range m.Params {
		p, err := utils.Join(m.Path, strings.TrimPrefix(raw, "/"))
		if err != nil {
			continue
		}
		if (backendPath == p || strings.HasPrefix(backendPath, strings.TrimRight(p, "/")+"/")) && len(p) > len(bestPrefix) {
			bestPrefix = p
			bestPassword = pass
		}
	}
	return bestPassword
}

func (s *Service) vodForItem(m config.Mount, parentRel string, item openlist.Item, remark string) catvod.Vod {
	itemRel := path.Join(parentRel, item.Name)
	pic := iconForItem(item.Name, item.Type)
	if remark == "" {
		if utils.IsFolder(item.Type) {
			if item.Size > 0 {
				remark = utils.FormatSize(item.Size)
			}
		} else {
			remark = utils.FormatSize(item.Size)
		}
	}
	vod := catvod.Vod{VodID: m.ID + "/" + itemRel, VodName: item.Name, VodPic: pic, VodRemarks: remark}
	if utils.IsFolder(item.Type) {
		vod.TypeFlag = "folder"
		vod.VodTag = "folder"
	} else {
		vod.VodTag = "file"
	}
	return vod
}

func detailPic(m config.Mount, selectedName string, items []openlist.Item) string {
	if selectedName == "" {
		return folderPic
	}
	for _, item := range items {
		if item.Name != selectedName {
			continue
		}
		return iconForItem(item.Name, item.Type)
	}
	return iconForItem(selectedName, 2)
}

func iconForItem(name string, itemType int) string {
	if utils.IsFolder(itemType) {
		return folderPic
	}
	if utils.IsAudio(name) {
		return audioPic
	}
	if utils.IsMedia(name, itemType) {
		return videoPic
	}
	return filePic
}

func playDirectoryVod(m config.Mount, relPath string, count int) catvod.Vod {
	id := m.ID
	if relPath != "" {
		id += "/" + relPath
	}
	remark := displayDirectoryRemark(relPath, count)
	if remark == "" {
		remark = "播放当前目录"
	}
	return catvod.Vod{VodID: id, VodName: "播放此目录", VodPic: listPic, VodRemarks: remark, VodTag: "file"}
}

func displayDirectoryRemark(relPath string, count int) string {
	if count > 0 {
		return formatMediaCount(count)
	}
	return ""
}

func middleEllipsis(value string, maxRunes int) string {
	runes := []rune(value)
	if maxRunes <= 3 || len(runes) <= maxRunes {
		return value
	}
	head := (maxRunes - 3) / 2
	tail := maxRunes - 3 - head
	return string(runes[:head]) + "..." + string(runes[len(runes)-tail:])
}

func refreshDirectoryVod(m config.Mount, relPath string) catvod.Vod {
	id := "__refresh__/" + m.ID
	if relPath != "" {
		id += "/" + relPath
	}
	return catvod.Vod{VodID: id, VodName: "刷新此目录", VodPic: refreshPic, VodRemarks: displayCurrentDirName(relPath), VodTag: "folder", TypeFlag: "folder"}
}

func displayCurrentDirName(relPath string) string {
	relPath = strings.Trim(relPath, "/")
	if relPath == "" {
		return "当前目录"
	}
	return middleEllipsis(path.Base(relPath), maxNoteNameRunes)
}

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

func (s *Service) relFromBackend(m config.Mount, backendParent string) string {
	backendParent = path.Clean("/" + strings.Trim(backendParent, "/"))
	root := path.Clean(m.Path)
	if backendParent == root {
		return ""
	}
	if strings.HasPrefix(backendParent, strings.TrimRight(root, "/")+"/") {
		return strings.TrimPrefix(backendParent, strings.TrimRight(root, "/")+"/")
	}
	return ""
}

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

func splitRel(rel string) (string, string) {
	rel = strings.Trim(rel, "/")
	if rel == "" {
		return "", ""
	}
	dir := path.Dir(rel)
	if dir == "." {
		dir = ""
	}
	return dir, path.Base(rel)
}

func fallbackName(value, fallback string) string {
	if value != "" && value != "." {
		return value
	}
	return fallback
}

func encodePlayToken(token playToken) string {
	raw, err := json.Marshal(token)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodePlayToken(value string) (playToken, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return playToken{}, false
	}
	var token playToken
	if json.Unmarshal(raw, &token) != nil || token.ID == "" {
		return playToken{}, false
	}
	return token, true
}

func playItemName(name string) string {
	name = strings.ReplaceAll(name, "#", "＃")
	return strings.ReplaceAll(name, "$", "＄")
}

func playHeader(raw string, mountHeaders map[string]string) map[string]string {
	headers := map[string]string{}
	u, err := url.Parse(raw)
	if err == nil && u.Host != "" {
		host := strings.ToLower(u.Host)
		if strings.Contains(host, "115") {
			headers["User-Agent"] = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/124 Safari/537.36"
		}
		if strings.Contains(host, "baidupcs.com") {
			headers["User-Agent"] = "pan.baidu.com"
		}
	}
	for name, value := range mountHeaders {
		headers[name] = value
	}
	return headers
}
