package mount

import (
	"errors"
	"path"
	"strings"

	"openlist-tvbox/internal/config"
	"openlist-tvbox/internal/utils"
)

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
