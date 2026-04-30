package catvod

type Result struct {
	Class     []Class             `json:"class,omitempty"`
	Filters   map[string][]Filter `json:"filters,omitempty"`
	List      []Vod               `json:"list,omitempty"`
	Page      int                 `json:"page,omitempty"`
	PageCount int                 `json:"pagecount,omitempty"`
	Limit     int                 `json:"limit,omitempty"`
	Total     int                 `json:"total,omitempty"`
	From      string              `json:"from,omitempty"`
	Parse     *int                `json:"parse,omitempty"`
	URL       string              `json:"url,omitempty"`
	Subt      string              `json:"subt,omitempty"`
	Header    map[string]string   `json:"header,omitempty"`
	Subs      []Sub               `json:"subs,omitempty"`
	Error     string              `json:"error,omitempty"`
}

type Class struct {
	TypeID   string `json:"type_id"`
	TypeName string `json:"type_name"`
	TypeFlag string `json:"type_flag,omitempty"`
}

type Filter struct {
	Key   string        `json:"key"`
	Name  string        `json:"name"`
	Value []FilterValue `json:"value"`
}

type FilterValue struct {
	N string `json:"n"`
	V string `json:"v"`
}

type Vod struct {
	VodID       string `json:"vod_id"`
	VodName     string `json:"vod_name"`
	VodPic      string `json:"vod_pic,omitempty"`
	VodRemarks  string `json:"vod_remarks,omitempty"`
	TypeFlag    string `json:"type_flag,omitempty"`
	VodTag      string `json:"vod_tag,omitempty"`
	VodPlayFrom string `json:"vod_play_from,omitempty"`
	VodPlayURL  string `json:"vod_play_url,omitempty"`
}

type Sub struct {
	Name   string `json:"name"`
	Ext    string `json:"ext,omitempty"`
	Format string `json:"format,omitempty"`
	URL    string `json:"url"`
}
