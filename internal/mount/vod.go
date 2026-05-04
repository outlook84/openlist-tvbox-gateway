package mount

import (
	"path"
	"strings"

	"openlist-tvbox/internal/catvod"
	"openlist-tvbox/internal/config"
	"openlist-tvbox/internal/openlist"
	"openlist-tvbox/internal/utils"
)

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
