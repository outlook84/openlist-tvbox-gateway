package mount

import (
	"context"
	"strings"
	"testing"

	"openlist-tvbox/internal/catvod"
	"openlist-tvbox/internal/config"
	"openlist-tvbox/internal/openlist"
)

type fakeClient struct {
	items  []openlist.Item
	getURL string
	urls   map[string]string
}

func (f fakeClient) List(context.Context, config.Backend, string, string) ([]openlist.Item, error) {
	return f.items, nil
}

func (f fakeClient) RefreshList(context.Context, config.Backend, string, string) ([]openlist.Item, error) {
	return f.items, nil
}

func (f fakeClient) Get(_ context.Context, _ config.Backend, p string, _ string) (openlist.Item, error) {
	if f.urls != nil {
		if raw, ok := f.urls[p]; ok {
			return openlist.Item{Name: pathBase(p), Type: 2, URL: raw}, nil
		}
	}
	if f.getURL == "" {
		f.getURL = "https://cdn.example.com/a.mkv"
	}
	return openlist.Item{Name: "a.mkv", Type: 2, URL: f.getURL}, nil
}

func (f fakeClient) Search(context.Context, config.Backend, string, string, string) ([]openlist.Item, error) {
	return f.items, nil
}

func testService(items []openlist.Item) *Service {
	cfg := &config.Config{
		Backends: []config.Backend{{ID: "b1", Server: "https://example.com"}},
		Subs:     []config.Subscription{{Mounts: []config.Mount{{ID: "m1", Name: "M1", Backend: "b1", Path: "/root"}}}},
	}
	if err := cfg.Validate(); err != nil {
		panic(err)
	}
	return NewService(cfg, fakeClient{items: items}, nil)
}

func TestCategoryPrioritizesPlayDirectoryBeforeSortedEntries(t *testing.T) {
	svc := testService([]openlist.Item{
		{Name: "b.mkv", Type: 2, Size: 2},
		{Name: "z", Type: 1},
		{Name: "a.mkv", Type: 2, Size: 1},
		{Name: "a", Type: 1},
	})
	got, err := svc.CategoryForSub(context.Background(), "default", "m1", "name", "asc")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, vod := range got.List {
		names = append(names, vod.VodName)
	}
	want := []string{"播放此目录", "a", "z", "a.mkv", "b.mkv"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names = %#v, want %#v", names, want)
		}
	}
}

func TestCategoryNameSortsNaturally(t *testing.T) {
	svc := testService([]openlist.Item{
		{Name: "第11集.mkv", Type: 2},
		{Name: "第8集.mkv", Type: 2},
		{Name: "第10集.mkv", Type: 2},
		{Name: "第09集.mkv", Type: 2},
	})
	got, err := svc.CategoryForSub(context.Background(), "default", "m1", "name", "asc")
	if err != nil {
		t.Fatal(err)
	}
	names := vodNames(got.List[1:])
	want := []string{"第8集.mkv", "第09集.mkv", "第10集.mkv", "第11集.mkv"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names = %#v, want %#v", names, want)
		}
	}
}

func TestCategoryDefaultSortsNameAscending(t *testing.T) {
	svc := testService([]openlist.Item{
		{Name: "5.mkv", Type: 2},
		{Name: "4.mkv", Type: 2},
		{Name: "3.mkv", Type: 2},
		{Name: "2.mkv", Type: 2},
		{Name: "1.mkv", Type: 2},
	})
	got, err := svc.CategoryForSub(context.Background(), "default", "m1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	names := vodNames(got.List[1:])
	want := []string{"1.mkv", "2.mkv", "3.mkv", "4.mkv", "5.mkv"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names = %#v, want %#v", names, want)
		}
	}
}

func TestCategoryNameSortsSeasonEpisode(t *testing.T) {
	svc := testService([]openlist.Item{
		{Name: "Show S01E10.mkv", Type: 2},
		{Name: "Show S02E01.mkv", Type: 2},
		{Name: "Show S01E09.mkv", Type: 2},
		{Name: "Show S1E2.mkv", Type: 2},
	})
	got, err := svc.CategoryForSub(context.Background(), "default", "m1", "name", "asc")
	if err != nil {
		t.Fatal(err)
	}
	names := vodNames(got.List[1:])
	want := []string{"Show S1E2.mkv", "Show S01E09.mkv", "Show S01E10.mkv", "Show S02E01.mkv"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names = %#v, want %#v", names, want)
		}
	}
}

func TestCategoryNameSortsSeasonEpisodeWithinSeries(t *testing.T) {
	svc := testService([]openlist.Item{
		{Name: "ShowB S01E01.mkv", Type: 2},
		{Name: "ShowA S01E02.mkv", Type: 2},
		{Name: "ShowB S01E02.mkv", Type: 2},
		{Name: "ShowA S01E01.mkv", Type: 2},
	})
	got, err := svc.CategoryForSub(context.Background(), "default", "m1", "name", "asc")
	if err != nil {
		t.Fatal(err)
	}
	names := vodNames(got.List[1:])
	want := []string{"ShowA S01E01.mkv", "ShowA S01E02.mkv", "ShowB S01E01.mkv", "ShowB S01E02.mkv"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names = %#v, want %#v", names, want)
		}
	}
}

func TestCategoryAddsPlayDirectoryVodForCurrentDirectoryMedia(t *testing.T) {
	svc := testService([]openlist.Item{
		{Name: "dir", Type: 1},
		{Name: "movie.mkv", Type: 2},
		{Name: "clip.ts", Type: 0},
	})
	got, err := svc.CategoryForSub(context.Background(), "default", "m1/season", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.List) != 4 {
		t.Fatalf("list = %#v", got.List)
	}
	vod := got.List[0]
	if vod.VodID != "m1/season" || vod.VodName != "播放此目录" || vod.VodPic != listPic || vod.VodTag != "file" || vod.TypeFlag == "folder" {
		t.Fatalf("play directory vod = %#v", vod)
	}
	if vod.VodRemarks != "2 个视频" {
		t.Fatalf("remarks = %q", vod.VodRemarks)
	}
}

func TestCategoryShowsRefreshVodOnlyWhenMountEnablesRefresh(t *testing.T) {
	cfg := &config.Config{
		Backends: []config.Backend{{ID: "b1", Server: "https://example.com"}},
		Subs: []config.Subscription{{
			Mounts: []config.Mount{{
				ID:      "m1",
				Name:    "M1",
				Backend: "b1",
				Path:    "/root",
				Refresh: true,
			}},
		}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	svc := NewService(cfg, fakeClient{items: []openlist.Item{{Name: "dir", Type: 1}}}, nil)
	got, err := svc.CategoryForSub(context.Background(), "default", "m1/season", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.List) == 0 || got.List[0].VodID != "__refresh__/m1/season" || got.List[0].VodName != "刷新此目录" || got.List[0].VodPic != refreshPic || got.List[0].VodRemarks != "season" {
		t.Fatalf("list = %#v, want refresh item first", got.List)
	}

	cfg.Subs[0].Mounts[0].Refresh = false
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	svc = NewService(cfg, fakeClient{items: []openlist.Item{{Name: "dir", Type: 1}}}, nil)
	got, err = svc.CategoryForSub(context.Background(), "default", "m1/season", "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, vod := range got.List {
		if strings.HasPrefix(vod.VodID, "__refresh__/") {
			t.Fatalf("list = %#v, want no refresh item", got.List)
		}
	}
}

func TestRefreshDirectoryRemarkShortensLongPath(t *testing.T) {
	cfg := &config.Config{
		Backends: []config.Backend{{ID: "b1", Server: "https://example.com"}},
		Subs: []config.Subscription{{
			Mounts: []config.Mount{{
				ID:      "m1",
				Name:    "M1",
				Backend: "b1",
				Path:    "/root",
				Refresh: true,
			}},
		}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	svc := NewService(cfg, fakeClient{items: []openlist.Item{{Name: "movie.mkv", Type: 2}}}, nil)
	got, err := svc.CategoryForSub(context.Background(), "default", "m1/电影/欧美/科幻/沙丘/导演剪辑版/Season 01", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.List) == 0 || got.List[0].VodName != "刷新此目录" {
		t.Fatalf("list = %#v", got.List)
	}
	want := "Season 01"
	if got.List[0].VodRemarks != want {
		t.Fatalf("remarks = %q, want %q", got.List[0].VodRemarks, want)
	}
}

func TestRefreshDirectoryRemarkTruncatesLongDirectoryName(t *testing.T) {
	cfg := &config.Config{
		Backends: []config.Backend{{ID: "b1", Server: "https://example.com"}},
		Subs: []config.Subscription{{
			Mounts: []config.Mount{{
				ID:      "m1",
				Name:    "M1",
				Backend: "b1",
				Path:    "/root",
				Refresh: true,
			}},
		}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	svc := NewService(cfg, fakeClient{items: []openlist.Item{{Name: "movie.mkv", Type: 2}}}, nil)
	got, err := svc.CategoryForSub(context.Background(), "default", "m1/电影/很长很长很长很长很长的目录名", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.List[0].VodRemarks != "很长很长...长的目录名" {
		t.Fatalf("remarks = %q", got.List[0].VodRemarks)
	}
}

func TestRefreshRequiresMountRefreshEnabled(t *testing.T) {
	svc := testService([]openlist.Item{{Name: "dir", Type: 1}})
	if _, err := svc.RefreshForSub(context.Background(), "default", "m1"); err == nil {
		t.Fatal("expected refresh disabled error")
	}
}

func TestCategorySkipsPlayDirectoryVodWithoutCurrentDirectoryMedia(t *testing.T) {
	svc := testService([]openlist.Item{
		{Name: "dir", Type: 1},
		{Name: "readme.txt", Type: 5},
	})
	got, err := svc.CategoryForSub(context.Background(), "default", "m1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.List) != 2 || got.List[0].VodName == "播放此目录" {
		t.Fatalf("list = %#v, want no play directory vod", got.List)
	}
}

func TestCategoryDoesNotShowFolderSizeRemark(t *testing.T) {
	svc := testService([]openlist.Item{
		{Name: "dir", Type: 1, Size: 0},
		{Name: "sized-dir", Type: 1, Size: 2048},
		{Name: "movie.mkv", Type: 2, Size: 1024},
	})
	got, err := svc.CategoryForSub(context.Background(), "default", "m1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.List) != 4 {
		t.Fatalf("list = %#v", got.List)
	}
	if got.List[1].VodName != "dir" || got.List[1].VodRemarks != "" {
		t.Fatalf("folder vod = %#v, want empty remarks", got.List[1])
	}
	if got.List[2].VodName != "sized-dir" || got.List[2].VodRemarks == "" {
		t.Fatalf("sized folder vod = %#v, want size remarks", got.List[2])
	}
	if got.List[3].VodRemarks == "" {
		t.Fatalf("file vod = %#v, want size remarks", got.List[3])
	}
}

func TestCategoryUsesBuiltInIconsWhenNoThumbnail(t *testing.T) {
	svc := testService([]openlist.Item{
		{Name: "dir", Type: 1},
		{Name: "song.flac", Type: 2},
		{Name: "movie.mkv", Type: 2},
		{Name: "readme.md", Type: 5},
		{Name: "cover.jpg", Type: 5, Thumb: "https://img.example.com/cover.jpg"},
	})
	got, err := svc.CategoryForSub(context.Background(), "default", "m1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"dir":       folderPic,
		"song.flac": audioPic,
		"movie.mkv": videoPic,
		"readme.md": filePic,
		"cover.jpg": filePic,
	}
	for _, vod := range got.List {
		if vod.VodName == "播放此目录" {
			continue
		}
		if vod.VodPic != want[vod.VodName] {
			t.Fatalf("%s pic = %q, want %q", vod.VodName, vod.VodPic, want[vod.VodName])
		}
	}
}

func TestCategorySortsDateByParsedTime(t *testing.T) {
	svc := testService([]openlist.Item{
		{Name: "late.mkv", Type: 2, Modified: "2026-04-29T12:00:00+08:00"},
		{Name: "early.mkv", Type: 2, Modified: "2026-04-29T03:00:00Z"},
	})
	got, err := svc.CategoryForSub(context.Background(), "default", "m1", "date", "asc")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.List) != 3 {
		t.Fatalf("list = %#v", got.List)
	}
	if got.List[1].VodName != "early.mkv" || got.List[2].VodName != "late.mkv" {
		t.Fatalf("names = %#v", []string{got.List[1].VodName, got.List[2].VodName})
	}
}

func TestDetailIncludesSameDirectorySubtitles(t *testing.T) {
	svc := testService([]openlist.Item{
		{Name: "a.mkv", Type: 2},
		{Name: "a.srt", Type: 0},
	})
	got, err := svc.DetailForSub(context.Background(), "default", "m1/a.mkv")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.List) != 1 || got.List[0].VodPlayURL == "" {
		t.Fatalf("missing play url: %#v", got)
	}
	id := playIDFromURL(t, got.List[0].VodPlayURL)
	play, err := svc.PlayForSub(context.Background(), "default", id)
	if err != nil {
		t.Fatal(err)
	}
	if len(play.Subs) != 1 || play.Subs[0].Name != "a.srt" || play.Subs[0].Ext != "srt" || play.Subs[0].Format != "application/x-subrip" {
		t.Fatalf("subs = %#v", play.Subs)
	}
}

func TestDetailOnlyIncludesMatchingSubtitlesForSelectedMedia(t *testing.T) {
	svc := testService([]openlist.Item{
		{Name: "Show S01E01.mkv", Type: 2},
		{Name: "Show S01E01.zh.ass", Type: 0},
		{Name: "Show S01E02.mkv", Type: 2},
		{Name: "Show S01E02.srt", Type: 0},
		{Name: "extras.srt", Type: 0},
	})
	got, err := svc.DetailForSub(context.Background(), "default", "m1/Show S01E01.mkv")
	if err != nil {
		t.Fatal(err)
	}
	play, err := svc.PlayForSub(context.Background(), "default", playIDFromURL(t, got.List[0].VodPlayURL))
	if err != nil {
		t.Fatal(err)
	}
	if len(play.Subs) != 1 || play.Subs[0].Name != "Show S01E01.zh.ass" || play.Subs[0].Format != "text/x-ssa" {
		t.Fatalf("subs = %#v, want only matching subtitle", play.Subs)
	}
}

func TestDetailSortsSubtitlesForBoxFirstSubtitleFallback(t *testing.T) {
	items := []openlist.Item{
		{Name: "Movie.mkv", Type: 2},
		{Name: "Movie.en.srt", Type: 0},
		{Name: "Movie.zh.ass", Type: 0},
		{Name: "Movie.srt", Type: 0},
		{Name: "Movie.zh.srt", Type: 0},
		{Name: "Movie.cht.srt", Type: 0},
	}
	cfg := &config.Config{
		Backends: []config.Backend{{ID: "b1", Server: "https://example.com"}},
		Subs:     []config.Subscription{{Mounts: []config.Mount{{ID: "m1", Name: "M1", Backend: "b1", Path: "/root"}}}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	svc := NewService(cfg, fakeClient{items: items, urls: map[string]string{
		"/root/Movie.mkv":     "https://cdn.example.com/Movie.mkv",
		"/root/Movie.en.srt":  "https://cdn.example.com/Movie.en",
		"/root/Movie.zh.ass":  "https://cdn.example.com/Movie.zh.ass",
		"/root/Movie.srt":     "https://cdn.example.com/Movie",
		"/root/Movie.zh.srt":  "https://cdn.example.com/Movie.zh",
		"/root/Movie.cht.srt": "https://cdn.example.com/Movie.cht",
	}}, nil)
	got, err := svc.DetailForSub(context.Background(), "default", "m1/Movie.mkv")
	if err != nil {
		t.Fatal(err)
	}
	play, err := svc.PlayForSub(context.Background(), "default", playIDFromURL(t, got.List[0].VodPlayURL))
	if err != nil {
		t.Fatal(err)
	}
	if names := subNames(play.Subs); strings.Join(names, "|") != "Movie.zh.srt|Movie.zh.ass|Movie.srt|Movie.cht.srt|Movie.en.srt" {
		t.Fatalf("subs = %#v", names)
	}
	if play.Subt != "https://cdn.example.com/Movie.zh" || play.Subt != play.Subs[0].URL {
		t.Fatalf("subt = %q, subs = %#v", play.Subt, play.Subs)
	}
}

func TestDetailPutsSelectedMediaFirst(t *testing.T) {
	svc := testService([]openlist.Item{
		{Name: "a.mkv", Type: 2},
		{Name: "b.mkv", Type: 2},
		{Name: "c.mkv", Type: 2},
	})
	got, err := svc.DetailForSub(context.Background(), "default", "m1/b.mkv")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.List) != 1 {
		t.Fatalf("list = %#v", got.List)
	}
	if !strings.HasPrefix(got.List[0].VodPlayURL, "b.mkv$") {
		t.Fatalf("play url = %q, want selected media first", got.List[0].VodPlayURL)
	}
}

func TestDetailEscapesHashInPlayNameAndEncodesPlayID(t *testing.T) {
	svc := testService([]openlist.Item{
		{Name: "a#b.mkv", Type: 2},
		{Name: "a#b.srt", Type: 0},
	})
	got, err := svc.DetailForSub(context.Background(), "default", "m1/a#b.mkv")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.List) != 1 {
		t.Fatalf("list = %#v", got.List)
	}
	playURL := got.List[0].VodPlayURL
	if strings.Contains(playURL, "a#b.mkv$") {
		t.Fatalf("play url = %q, contains raw # in play name", playURL)
	}
	if !strings.HasPrefix(playURL, "a＃b.mkv$") {
		t.Fatalf("play url = %q, want safe play name", playURL)
	}
	play, err := svc.PlayForSub(context.Background(), "default", playIDFromURL(t, playURL))
	if err != nil {
		t.Fatal(err)
	}
	if play.URL == "" {
		t.Fatalf("play = %#v", play)
	}
}

func TestDetailHandlesPlaySeparatorsInNamesAndSubtitles(t *testing.T) {
	svc := testService([]openlist.Item{
		{Name: "a#b$c~~~d@@@e.mkv", Type: 2},
		{Name: "a#b$c~~~d@@@e.ass", Type: 0},
	})
	got, err := svc.DetailForSub(context.Background(), "default", "m1/a#b$c~~~d@@@e.mkv")
	if err != nil {
		t.Fatal(err)
	}
	playURL := got.List[0].VodPlayURL
	if strings.Contains(playURL, "#") {
		t.Fatalf("play url = %q, contains raw episode separator", playURL)
	}
	if !strings.HasPrefix(playURL, "a＃b＄c~~~d@@@e.mkv$") {
		t.Fatalf("play url = %q, want safe play name", playURL)
	}
	play, err := svc.PlayForSub(context.Background(), "default", playIDFromURL(t, playURL))
	if err != nil {
		t.Fatal(err)
	}
	if play.URL == "" {
		t.Fatalf("play = %#v", play)
	}
	if len(play.Subs) != 1 || play.Subs[0].Name != "a#b$c~~~d@@@e.ass" || play.Subs[0].Ext != "ass" || play.Subs[0].Format != "text/x-ssa" {
		t.Fatalf("subs = %#v", play.Subs)
	}
}

func TestDetailSortsMediaNaturally(t *testing.T) {
	svc := testService([]openlist.Item{
		{Name: "Show S01E10.mkv", Type: 2},
		{Name: "Show S02E01.mkv", Type: 2},
		{Name: "Show S01E09.mkv", Type: 2},
		{Name: "Show S1E2.mkv", Type: 2},
	})
	got, err := svc.DetailForSub(context.Background(), "default", "m1")
	if err != nil {
		t.Fatal(err)
	}
	playURL := got.List[0].VodPlayURL
	wantOrder := []string{"Show S1E2.mkv$", "Show S01E09.mkv$", "Show S01E10.mkv$", "Show S02E01.mkv$"}
	last := -1
	for _, want := range wantOrder {
		idx := strings.Index(playURL, want)
		if idx < 0 {
			t.Fatalf("play url = %q, missing %q", playURL, want)
		}
		if idx < last {
			t.Fatalf("play url = %q, %q is out of order", playURL, want)
		}
		last = idx
	}
}

func TestDetailSortsSeasonEpisodeWithinSeries(t *testing.T) {
	svc := testService([]openlist.Item{
		{Name: "ShowB S01E01.mkv", Type: 2},
		{Name: "ShowA S01E02.mkv", Type: 2},
		{Name: "ShowB S01E02.mkv", Type: 2},
		{Name: "ShowA S01E01.mkv", Type: 2},
	})
	got, err := svc.DetailForSub(context.Background(), "default", "m1")
	if err != nil {
		t.Fatal(err)
	}
	playURL := got.List[0].VodPlayURL
	wantOrder := []string{"ShowA S01E01.mkv$", "ShowA S01E02.mkv$", "ShowB S01E01.mkv$", "ShowB S01E02.mkv$"}
	last := -1
	for _, want := range wantOrder {
		idx := strings.Index(playURL, want)
		if idx < 0 {
			t.Fatalf("play url = %q, missing %q", playURL, want)
		}
		if idx < last {
			t.Fatalf("play url = %q, %q is out of order", playURL, want)
		}
		last = idx
	}
}

func TestDetailFolderListsFolderInsteadOfParent(t *testing.T) {
	cfg := &config.Config{
		Backends: []config.Backend{{ID: "b1", Server: "https://example.com"}},
		Subs:     []config.Subscription{{Mounts: []config.Mount{{ID: "m1", Name: "M1", Backend: "b1", Path: "/root"}}}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	svc := NewService(cfg, pathListClient{items: map[string][]openlist.Item{
		"/root":      {{Name: "root.mkv", Type: 2}, {Name: "show", Type: 1}},
		"/root/show": {{Name: "episode.mkv", Type: 2}, {Name: "nested", Type: 1}},
	}}, nil)
	got, err := svc.DetailForSub(context.Background(), "default", "m1/show")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.List) != 1 {
		t.Fatalf("list = %#v", got.List)
	}
	if !strings.HasPrefix(got.List[0].VodPlayURL, "episode.mkv$") {
		t.Fatalf("play url = %q, want folder media", got.List[0].VodPlayURL)
	}
	if _, err := svc.PlayForSub(context.Background(), "default", playIDFromURL(t, got.List[0].VodPlayURL)); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.List[0].VodPlayURL, "nested") {
		t.Fatalf("play url = %q, should not include subdirectory entries", got.List[0].VodPlayURL)
	}
}

func TestCategoryForSubUsesScopedMountPath(t *testing.T) {
	client := &recordingClient{}
	cfg := &config.Config{
		Backends: []config.Backend{
			{ID: "b1", Server: "https://one.example.com"},
			{ID: "b2", Server: "https://two.example.com"},
		},
		Subs: []config.Subscription{
			{ID: "a", Mounts: []config.Mount{{ID: "m", Backend: "b1", Path: "/A"}}},
			{ID: "b", Mounts: []config.Mount{{ID: "m", Backend: "b2", Path: "/B"}}},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	svc := NewService(cfg, client, nil)
	if _, err := svc.CategoryForSub(context.Background(), "b", "m/child", "", ""); err != nil {
		t.Fatal(err)
	}
	if client.backendID != "b2" || client.path != "/B/child" {
		t.Fatalf("backend/path = %q %q", client.backendID, client.path)
	}
}

func TestPlayUsesMountPlayHeadersOverBuiltInHeaders(t *testing.T) {
	cfg := &config.Config{
		Backends: []config.Backend{{ID: "b1", Server: "https://example.com"}},
		Subs: []config.Subscription{{
			Mounts: []config.Mount{{
				ID:          "m1",
				Backend:     "b1",
				Path:        "/root",
				PlayHeaders: map[string]string{"User-Agent": "Custom-UA", "Referer": "https://referer.example.com"},
			}},
		}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	svc := NewService(cfg, fakeClient{getURL: "https://download.baidupcs.com/file/a.mkv"}, nil)
	got, err := svc.PlayForSub(context.Background(), "default", encodePlayToken(playToken{ID: "m1/a.mkv"}))
	if err != nil {
		t.Fatal(err)
	}
	if got.Parse == nil || *got.Parse != 0 {
		t.Fatalf("parse = %#v, want 0", got.Parse)
	}
	if got.Header["User-Agent"] != "Custom-UA" {
		t.Fatalf("User-Agent = %q, want Custom-UA", got.Header["User-Agent"])
	}
	if got.Header["Referer"] != "https://referer.example.com" {
		t.Fatalf("Referer = %q", got.Header["Referer"])
	}
}

type recordingClient struct {
	backendID string
	path      string
}

func (r *recordingClient) List(_ context.Context, backend config.Backend, path, _ string) ([]openlist.Item, error) {
	r.backendID = backend.ID
	r.path = path
	return nil, nil
}

func (r *recordingClient) RefreshList(_ context.Context, backend config.Backend, path, _ string) ([]openlist.Item, error) {
	r.backendID = backend.ID
	r.path = path
	return nil, nil
}

func (r *recordingClient) Get(context.Context, config.Backend, string, string) (openlist.Item, error) {
	return openlist.Item{}, nil
}

func (r *recordingClient) Search(context.Context, config.Backend, string, string, string) ([]openlist.Item, error) {
	return nil, nil
}

type pathListClient struct {
	items map[string][]openlist.Item
}

func (p pathListClient) List(_ context.Context, _ config.Backend, path, _ string) ([]openlist.Item, error) {
	return p.items[path], nil
}

func (p pathListClient) RefreshList(_ context.Context, _ config.Backend, path, _ string) ([]openlist.Item, error) {
	return p.items[path], nil
}

func (p pathListClient) Get(context.Context, config.Backend, string, string) (openlist.Item, error) {
	return openlist.Item{}, nil
}

func (p pathListClient) Search(context.Context, config.Backend, string, string, string) ([]openlist.Item, error) {
	return nil, nil
}

func playIDFromURL(t *testing.T, playURL string) string {
	t.Helper()
	parts := strings.SplitN(playURL, "$", 2)
	if len(parts) != 2 || parts[1] == "" {
		t.Fatalf("play url = %q", playURL)
	}
	id, _, _ := strings.Cut(parts[1], "#")
	return id
}

func vodNames(vods []catvod.Vod) []string {
	names := make([]string, 0, len(vods))
	for _, vod := range vods {
		names = append(names, vod.VodName)
	}
	return names
}

func subNames(subs []catvod.Sub) []string {
	names := make([]string, 0, len(subs))
	for _, sub := range subs {
		names = append(names, sub.Name)
	}
	return names
}

func pathBase(p string) string {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	return parts[len(parts)-1]
}
