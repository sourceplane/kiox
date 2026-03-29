package gha

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

const (
	DriverName        = "gha"
	RuntimeComposite  = "gha-composite"
	RuntimeNode       = "gha-node"
	DefaultCapability = "run"
)

type Reference struct {
	Raw     string
	Owner   string
	Repo    string
	Subpath string
	Version string
}

func IsReference(ref string) bool {
	return strings.HasPrefix(ref, "gha://")
}

func ParseReference(ref string) (Reference, error) {
	if !IsReference(ref) {
		return Reference{}, fmt.Errorf("unsupported GitHub Action ref %q", ref)
	}
	raw := strings.TrimPrefix(ref, "gha://")
	at := strings.LastIndex(raw, "@")
	if at <= 0 || at == len(raw)-1 {
		return Reference{}, fmt.Errorf("GitHub Action ref must be gha://<owner>/<repo>[/path]@<ref>: %q", ref)
	}
	repositoryPath := raw[:at]
	version := strings.TrimSpace(raw[at+1:])
	if version == "" {
		return Reference{}, fmt.Errorf("GitHub Action ref is missing a version: %q", ref)
	}
	segments := strings.Split(repositoryPath, "/")
	if len(segments) < 2 || segments[0] == "" || segments[1] == "" {
		return Reference{}, fmt.Errorf("GitHub Action ref must include owner and repo: %q", ref)
	}
	subpath := ""
	if len(segments) > 2 {
		cleaned := path.Clean(strings.Join(segments[2:], "/"))
		switch {
		case cleaned == ".":
			subpath = ""
		case cleaned == "..", strings.HasPrefix(cleaned, "../"), strings.HasPrefix(cleaned, "/"):
			return Reference{}, fmt.Errorf("GitHub Action subpath must stay within the repository: %q", ref)
		default:
			subpath = cleaned
		}
	}
	return Reference{
		Raw:     ref,
		Owner:   segments[0],
		Repo:    segments[1],
		Subpath: subpath,
		Version: version,
	}, nil
}

func (r Reference) Repository() string {
	return r.Owner + "/" + r.Repo
}

func (r Reference) ProviderName() string {
	name := r.Repository()
	if r.Subpath != "" {
		name += "/" + r.Subpath
	}
	return name
}

func (r Reference) ProviderRef() string {
	return DriverName + "/" + r.ProviderName()
}

func (r Reference) ActionDir(sourceRoot string) string {
	if r.Subpath == "" {
		return sourceRoot
	}
	return filepath.Join(sourceRoot, filepath.FromSlash(r.Subpath))
}
