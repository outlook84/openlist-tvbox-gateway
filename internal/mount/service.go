package mount

import (
	"context"
	"errors"
	"log/slog"
	"path"
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
