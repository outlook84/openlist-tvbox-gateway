package openlist

import "time"

type Item struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Parent    string `json:"parent"`
	Type      int    `json:"type"`
	Size      int64  `json:"size"`
	Thumb     string `json:"thumb"`
	Thumbnail string `json:"thumbnail"`
	URL       string `json:"url"`
	RawURL    string `json:"raw_url"`
	Modified  string `json:"modified"`
	UpdatedAt string `json:"updated_at"`
}

func (i Item) DisplayPath() string {
	if i.Path != "" {
		return i.Path
	}
	return i.Parent
}

func (i Item) Image() string {
	if i.Thumb != "" {
		return i.Thumb
	}
	return i.Thumbnail
}

func (i Item) Link() string {
	if i.URL != "" {
		return normalizeProtocolRelative(i.URL)
	}
	return normalizeProtocolRelative(i.RawURL)
}

func (i Item) ModTime() string {
	if i.Modified != "" {
		return i.Modified
	}
	return i.UpdatedAt
}

func (i Item) ModTimeValue() (time.Time, bool) {
	raw := i.ModTime()
	if raw == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, true
	}
	return time.Time{}, false
}

func normalizeProtocolRelative(raw string) string {
	if len(raw) > 2 && raw[:2] == "//" {
		return "http:" + raw
	}
	return raw
}
